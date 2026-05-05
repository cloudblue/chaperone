// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sort"
	"sync"
	"time"
)

// Collector manages per-instance metric ring buffers and computes derived
// metrics (rates, percentiles) on demand.
type Collector struct {
	mu       sync.RWMutex
	buffers  map[int64]*Ring
	capacity int
}

// NewCollector creates a Collector with the given ring buffer capacity.
func NewCollector(capacity int) *Collector {
	return &Collector{
		buffers:  make(map[int64]*Ring),
		capacity: capacity,
	}
}

// RecordScrape parses raw Prometheus text data and stores the resulting
// snapshot for the given instance.
func (c *Collector) RecordScrape(instanceID int64, data []byte, t time.Time) error {
	snap, err := Parse(data, t)
	if err != nil {
		return err
	}
	c.Record(instanceID, *snap)
	return nil
}

// Record stores a pre-parsed snapshot for the given instance.
func (c *Collector) Record(instanceID int64, snap Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()

	buf, ok := c.buffers[instanceID]
	if !ok {
		buf = NewRing(c.capacity)
		c.buffers[instanceID] = buf
	}
	buf.Push(snap)
}

// Remove deletes all stored data for the given instance.
func (c *Collector) Remove(instanceID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.buffers, instanceID)
}

// Prune removes ring buffers for instances not in the activeIDs set.
func (c *Collector) Prune(activeIDs map[int64]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.buffers {
		if !activeIDs[id] {
			delete(c.buffers, id)
		}
	}
}

// GetInstanceMetrics computes full metrics for a single instance.
// Returns nil if no data exists for the given instance.
func (c *Collector) GetInstanceMetrics(instanceID int64) *InstanceMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	buf, ok := c.buffers[instanceID]
	if !ok || buf.Len() == 0 {
		return nil
	}

	last, _ := buf.Last()
	im := &InstanceMetrics{
		InstanceID:        instanceID,
		CollectedAt:       last.Time,
		DataPoints:        buf.Len(),
		ActiveConnections: last.ActiveConnections,
		PanicsTotal:       last.PanicsTotal,
	}

	if buf.Len() >= 2 {
		prev := buf.At(buf.Len() - 2)
		c.fillCurrentKPIs(im, prev, last)
		c.fillVendorMetrics(im, prev, last)
	}

	c.fillTrends(im, buf)
	im.Series = c.buildSeries(buf)
	im.VendorSeries = c.buildVendorSeries(buf)

	return im
}

// GetInstanceSummary returns a compact summary for the fleet view.
// Returns nil if no data exists.
func (c *Collector) GetInstanceSummary(instanceID int64) *InstanceSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	buf, ok := c.buffers[instanceID]
	if !ok || buf.Len() == 0 {
		return nil
	}

	s := instanceSummary(buf, instanceID)
	return &s
}

// instanceSummary is the shared implementation for GetInstanceSummary and
// summarizeForFleet. Caller must hold at least a read lock.
func instanceSummary(buf *Ring, id int64) InstanceSummary {
	last, _ := buf.Last()
	s := InstanceSummary{
		InstanceID:        id,
		ActiveConnections: last.ActiveConnections,
		PanicsTotal:       last.PanicsTotal,
	}

	if buf.Len() >= 2 {
		prev := buf.At(buf.Len() - 2)
		dt := last.Time.Sub(prev.Time)
		s.RPS = counterRate(prev.totalRequests(), last.totalRequests(), dt)
		s.ErrorRate = errorRate(prev.totalRequests(), last.totalRequests(), prev.totalErrors(), last.totalErrors())
		s.P99 = secondsToMs(histogramQuantile(0.99, mergeHistogramDelta(prev, last)))
	}

	return s
}

// fleetAccumulator collects per-instance data for fleet-wide aggregation.
type fleetAccumulator struct {
	totalReqDelta    float64
	totalErrDelta    float64
	trendOldRPS      float64
	trendOldReqDelta float64
	trendOldErrDelta float64
	hasTrend         bool
}

// GetFleetMetrics aggregates metrics across the given instance IDs.
func (c *Collector) GetFleetMetrics(instanceIDs []int64) *FleetMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fm := &FleetMetrics{
		CollectedAt: time.Now(),
		Instances:   make([]InstanceSummary, 0, len(instanceIDs)),
	}

	var acc fleetAccumulator
	for _, id := range instanceIDs {
		buf, ok := c.buffers[id]
		if !ok || buf.Len() == 0 {
			continue
		}
		s := c.summarizeForFleet(buf, id, &acc)
		fm.TotalRPS += s.RPS
		fm.TotalActiveConnections += s.ActiveConnections
		fm.TotalPanics += s.PanicsTotal
		fm.Instances = append(fm.Instances, s)
	}

	if acc.totalReqDelta > 0 {
		fm.FleetErrorRate = acc.totalErrDelta / acc.totalReqDelta
	}
	acc.applyTrends(fm)
	return fm
}

// summarizeForFleet computes an InstanceSummary and accumulates fleet-wide deltas.
func (c *Collector) summarizeForFleet(buf *Ring, id int64, acc *fleetAccumulator) InstanceSummary {
	s := instanceSummary(buf, id)

	if buf.Len() >= 2 {
		last, _ := buf.Last()
		prev := buf.At(buf.Len() - 2)
		reqD := last.totalRequests() - prev.totalRequests()
		errD := last.totalErrors() - prev.totalErrors()
		if reqD >= 0 && errD >= 0 {
			acc.totalReqDelta += reqD
			acc.totalErrDelta += errD
		}
	}

	if td, ok := c.trendSnapshot(buf); ok {
		acc.trendOldRPS += td.rps
		acc.trendOldReqDelta += td.reqDelta
		acc.trendOldErrDelta += td.errDelta
		acc.hasTrend = true
	}
	return s
}

func (acc *fleetAccumulator) applyTrends(fm *FleetMetrics) {
	if !acc.hasTrend {
		return
	}
	rpsTrend := fm.TotalRPS - acc.trendOldRPS
	fm.RPSTrend = &rpsTrend

	var oldErrRate float64
	if acc.trendOldReqDelta > 0 {
		oldErrRate = acc.trendOldErrDelta / acc.trendOldReqDelta
	}
	errTrend := fm.FleetErrorRate - oldErrRate
	fm.ErrorRateTrend = &errTrend
}

// fillCurrentKPIs populates global RPS, error rate, and latency percentiles
// from the two most recent snapshots.
func (*Collector) fillCurrentKPIs(im *InstanceMetrics, prev, curr Snapshot) {
	dt := curr.Time.Sub(prev.Time)
	im.RPS = counterRate(prev.totalRequests(), curr.totalRequests(), dt)
	im.ErrorRate = errorRate(prev.totalRequests(), curr.totalRequests(), prev.totalErrors(), curr.totalErrors())

	dh := mergeHistogramDelta(prev, curr)
	im.P50 = secondsToMs(histogramQuantile(0.50, dh))
	im.P95 = secondsToMs(histogramQuantile(0.95, dh))
	im.P99 = secondsToMs(histogramQuantile(0.99, dh))
}

// fillVendorMetrics populates per-vendor KPIs from the two most recent snapshots.
func (*Collector) fillVendorMetrics(im *InstanceMetrics, prev, curr Snapshot) {
	dt := curr.Time.Sub(prev.Time)
	vendorIDs := collectVendorIDs(prev, curr)

	for _, vid := range vendorIDs {
		pv := vendorOr(prev, vid)
		cv := vendorOr(curr, vid)

		vm := VendorMetrics{VendorID: vid}
		vm.RPS = counterRate(pv.RequestsTotal, cv.RequestsTotal, dt)
		vm.ErrorRate = errorRate(pv.RequestsTotal, cv.RequestsTotal, pv.RequestsErrors, cv.RequestsErrors)

		dh := histogramDelta(pv.Duration, cv.Duration)
		vm.P50 = secondsToMs(histogramQuantile(0.50, dh))
		vm.P95 = secondsToMs(histogramQuantile(0.95, dh))
		vm.P99 = secondsToMs(histogramQuantile(0.99, dh))

		im.Vendors = append(im.Vendors, vm)
	}
	sort.Slice(im.Vendors, func(i, j int) bool {
		return im.Vendors[i].VendorID < im.Vendors[j].VendorID
	})
}

// historicalPair returns the two snapshots forming a rate pair from ~1 hour
// ago in the ring buffer. If the buffer doesn't span at least 50 minutes,
// ok is false.
func historicalPair(buf *Ring) (prev, curr Snapshot, ok bool) {
	if buf.Len() < 4 {
		return Snapshot{}, Snapshot{}, false
	}
	newest := buf.At(buf.Len() - 1)
	oldest := buf.At(0)
	if newest.Time.Sub(oldest.Time) < 50*time.Minute {
		return Snapshot{}, Snapshot{}, false
	}

	target := newest.Time.Add(-1 * time.Hour)
	idx := findNearest(buf, target)
	start := idx
	if start > 0 {
		start = idx - 1
	}
	if start+1 >= buf.Len() {
		return Snapshot{}, Snapshot{}, false
	}
	return buf.At(start), buf.At(start + 1), true
}

// fillTrends computes trend values by comparing the current rate to the rate
// from approximately 1 hour ago.
func (*Collector) fillTrends(im *InstanceMetrics, buf *Ring) {
	prev, curr, ok := historicalPair(buf)
	if !ok {
		return
	}
	dt := curr.Time.Sub(prev.Time)

	oldRPS := counterRate(prev.totalRequests(), curr.totalRequests(), dt)
	rpsTrend := im.RPS - oldRPS
	im.RPSTrend = &rpsTrend

	oldErr := errorRate(prev.totalRequests(), curr.totalRequests(), prev.totalErrors(), curr.totalErrors())
	errTrend := im.ErrorRate - oldErr
	im.ErrorRateTrend = &errTrend
}

type historicalTrend struct {
	rps      float64
	reqDelta float64
	errDelta float64
}

// trendSnapshot returns historical RPS and request/error deltas from ~1h ago.
func (*Collector) trendSnapshot(buf *Ring) (historicalTrend, bool) {
	prev, curr, ok := historicalPair(buf)
	if !ok {
		return historicalTrend{}, false
	}
	dt := curr.Time.Sub(prev.Time)

	reqD := curr.totalRequests() - prev.totalRequests()
	errD := curr.totalErrors() - prev.totalErrors()
	if reqD < 0 || errD < 0 {
		return historicalTrend{}, false
	}

	return historicalTrend{
		rps:      counterRate(prev.totalRequests(), curr.totalRequests(), dt),
		reqDelta: reqD,
		errDelta: errD,
	}, true
}

// buildSeries creates the global time series from the ring buffer.
func (*Collector) buildSeries(buf *Ring) []SeriesPoint {
	if buf.Len() < 2 {
		return nil
	}

	points := make([]SeriesPoint, 0, buf.Len()-1)
	for i := 1; i < buf.Len(); i++ {
		prev := buf.At(i - 1)
		curr := buf.At(i)
		dt := curr.Time.Sub(prev.Time)

		dh := mergeHistogramDelta(prev, curr)
		points = append(points, SeriesPoint{
			Time:              curr.Time.Unix(),
			RPS:               counterRate(prev.totalRequests(), curr.totalRequests(), dt),
			ErrorRate:         errorRate(prev.totalRequests(), curr.totalRequests(), prev.totalErrors(), curr.totalErrors()),
			P50:               secondsToMs(histogramQuantile(0.50, dh)),
			P95:               secondsToMs(histogramQuantile(0.95, dh)),
			P99:               secondsToMs(histogramQuantile(0.99, dh)),
			ActiveConnections: curr.ActiveConnections,
		})
	}
	return points
}

// buildVendorSeries creates per-vendor time series from the ring buffer.
func (*Collector) buildVendorSeries(buf *Ring) map[string][]VendorSeriesPoint {
	if buf.Len() < 2 {
		return nil
	}

	allVendors := make(map[string]bool)
	for i := 0; i < buf.Len(); i++ {
		for vid := range buf.At(i).Vendors {
			allVendors[vid] = true
		}
	}
	if len(allVendors) == 0 {
		return nil
	}

	result := make(map[string][]VendorSeriesPoint, len(allVendors))
	for vid := range allVendors {
		points := make([]VendorSeriesPoint, 0, buf.Len()-1)
		for i := 1; i < buf.Len(); i++ {
			prev := buf.At(i - 1)
			curr := buf.At(i)
			dt := curr.Time.Sub(prev.Time)
			pv := vendorOr(prev, vid)
			cv := vendorOr(curr, vid)

			dh := histogramDelta(pv.Duration, cv.Duration)
			points = append(points, VendorSeriesPoint{
				Time:      curr.Time.Unix(),
				RPS:       counterRate(pv.RequestsTotal, cv.RequestsTotal, dt),
				ErrorRate: errorRate(pv.RequestsTotal, cv.RequestsTotal, pv.RequestsErrors, cv.RequestsErrors),
				P50:       secondsToMs(histogramQuantile(0.50, dh)),
				P95:       secondsToMs(histogramQuantile(0.95, dh)),
				P99:       secondsToMs(histogramQuantile(0.99, dh)),
			})
		}
		result[vid] = points
	}
	return result
}

// --- helpers ---

// findNearest returns the index of the snapshot closest to target time.
func findNearest(buf *Ring, target time.Time) int {
	best := 0
	bestDelta := absDuration(buf.At(0).Time.Sub(target))
	for i := 1; i < buf.Len(); i++ {
		d := absDuration(buf.At(i).Time.Sub(target))
		if d < bestDelta {
			best = i
			bestDelta = d
		}
	}
	return best
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// mergeHistogramDelta merges all vendor histograms and computes their delta.
func mergeHistogramDelta(prev, curr Snapshot) Histogram {
	pm := mergeVendorHistograms(prev)
	cm := mergeVendorHistograms(curr)
	return histogramDelta(pm, cm)
}

// mergeVendorHistograms combines histograms across all vendors in a snapshot.
func mergeVendorHistograms(s Snapshot) Histogram {
	var merged Histogram
	for _, vs := range s.Vendors {
		merged = addHistograms(merged, vs.Duration)
	}
	return merged
}

// addHistograms sums two histograms that share the same bucket boundaries.
// All vendors on a single proxy share the same HistogramVec bucket layout,
// so boundaries always match in practice. If they ever diverge (proxy version
// drift), we fall back to the histogram with more observations rather than
// silently producing a corrupt merge.
func addHistograms(a, b Histogram) Histogram {
	if len(a.Buckets) == 0 {
		return b
	}
	if len(b.Buckets) == 0 {
		return a
	}
	if !sameBucketBoundaries(a, b) {
		if a.Count >= b.Count {
			return a
		}
		return b
	}
	result := Histogram{
		Count:   a.Count + b.Count,
		Sum:     a.Sum + b.Sum,
		Buckets: make([]Bucket, len(a.Buckets)),
	}
	for i := range result.Buckets {
		result.Buckets[i] = Bucket{
			UpperBound: a.Buckets[i].UpperBound,
			Count:      a.Buckets[i].Count + b.Buckets[i].Count,
		}
	}
	return result
}

func sameBucketBoundaries(a, b Histogram) bool {
	if len(a.Buckets) != len(b.Buckets) {
		return false
	}
	for i := range a.Buckets {
		if a.Buckets[i].UpperBound != b.Buckets[i].UpperBound {
			return false
		}
	}
	return true
}

// vendorOr returns the VendorSnapshot for a vendor or a zero value.
func vendorOr(s Snapshot, id string) VendorSnapshot {
	if vs, ok := s.Vendors[id]; ok {
		return *vs
	}
	return VendorSnapshot{}
}

// collectVendorIDs returns the sorted union of vendor IDs from two snapshots.
func collectVendorIDs(a, b Snapshot) []string {
	seen := make(map[string]bool)
	for k := range a.Vendors {
		seen[k] = true
	}
	for k := range b.Vendors {
		seen[k] = true
	}
	ids := make([]string, 0, len(seen))
	for k := range seen {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return ids
}
