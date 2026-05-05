// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"math"
	"testing"
	"time"
)

func makeSnapshot(t time.Time, totalReq, errReq, active, panics float64) Snapshot {
	return Snapshot{
		Time: t,
		Vendors: map[string]*VendorSnapshot{
			"acme": {
				RequestsTotal:  totalReq * 0.6,
				RequestsErrors: errReq * 0.6,
				Duration: Histogram{
					Count: totalReq * 0.6,
					Sum:   totalReq * 0.6 * 0.15,
					Buckets: []Bucket{
						{UpperBound: 0.05, Count: totalReq * 0.6 * 0.2},
						{UpperBound: 0.1, Count: totalReq * 0.6 * 0.5},
						{UpperBound: 0.25, Count: totalReq * 0.6 * 0.8},
						{UpperBound: 0.5, Count: totalReq * 0.6 * 0.95},
						{UpperBound: 1.0, Count: totalReq * 0.6},
					},
				},
			},
			"beta": {
				RequestsTotal:  totalReq * 0.4,
				RequestsErrors: errReq * 0.4,
				Duration: Histogram{
					Count: totalReq * 0.4,
					Sum:   totalReq * 0.4 * 0.1,
					Buckets: []Bucket{
						{UpperBound: 0.05, Count: totalReq * 0.4 * 0.3},
						{UpperBound: 0.1, Count: totalReq * 0.4 * 0.6},
						{UpperBound: 0.25, Count: totalReq * 0.4 * 0.9},
						{UpperBound: 0.5, Count: totalReq * 0.4 * 0.98},
						{UpperBound: 1.0, Count: totalReq * 0.4},
					},
				},
			},
		},
		ActiveConnections: active,
		PanicsTotal:       panics,
	}
}

func TestCollector_RecordScrape_ParsesAndStores(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)

	err := c.RecordScrape(1, []byte(sampleMetrics), time.Now())
	if err != nil {
		t.Fatalf("RecordScrape() error = %v", err)
	}

	c.mu.RLock()
	buf, ok := c.buffers[1]
	c.mu.RUnlock()

	if !ok {
		t.Fatal("expected buffer for instance 1")
	}
	if buf.Len() != 1 {
		t.Errorf("buffer Len() = %d, want 1", buf.Len())
	}
}

func TestCollector_RecordScrape_MalformedData_ReturnsError(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)

	err := c.RecordScrape(1, []byte("# TYPE foo gauge\n# TYPE foo counter\nfoo 1\n"), time.Now())
	if err == nil {
		t.Error("expected error for malformed data")
	}
}

func TestCollector_Remove(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	c.Record(1, Snapshot{Time: time.Now()})
	c.Remove(1)

	c.mu.RLock()
	_, ok := c.buffers[1]
	c.mu.RUnlock()

	if ok {
		t.Error("expected buffer to be removed")
	}
}

func TestCollector_Prune(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	c.Record(1, Snapshot{Time: time.Now()})
	c.Record(2, Snapshot{Time: time.Now()})
	c.Record(3, Snapshot{Time: time.Now()})

	c.Prune(map[int64]bool{1: true, 3: true})

	c.mu.RLock()
	_, has2 := c.buffers[2]
	c.mu.RUnlock()

	if has2 {
		t.Error("expected instance 2 to be pruned")
	}
}

func TestCollector_GetInstanceMetrics_NoData_ReturnsNil(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	if got := c.GetInstanceMetrics(99); got != nil {
		t.Error("expected nil for unknown instance")
	}
}

func TestCollector_GetInstanceMetrics_SingleSnapshot_NoRates(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	c.Record(1, makeSnapshot(time.Now(), 1000, 50, 10, 2))

	im := c.GetInstanceMetrics(1)
	if im == nil {
		t.Fatal("expected non-nil InstanceMetrics")
	}
	if im.DataPoints != 1 {
		t.Errorf("DataPoints = %d, want 1", im.DataPoints)
	}
	// With only 1 snapshot, rates should be zero.
	if im.RPS != 0 {
		t.Errorf("RPS = %v, want 0 (single snapshot)", im.RPS)
	}
	if im.ActiveConnections != 10 {
		t.Errorf("ActiveConnections = %v, want 10", im.ActiveConnections)
	}
}

func TestCollector_GetInstanceMetrics_TwoSnapshots_ComputesRates(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	c.Record(1, makeSnapshot(t0, 1000, 50, 10, 1))
	c.Record(1, makeSnapshot(t1, 1100, 55, 12, 2))

	im := c.GetInstanceMetrics(1)
	if im == nil {
		t.Fatal("expected non-nil InstanceMetrics")
	}

	// RPS: (1100 - 1000) / 10 = 10
	if math.Abs(im.RPS-10.0) > 0.01 {
		t.Errorf("RPS = %v, want ~10", im.RPS)
	}

	// Error rate: (55-50) / (1100-1000) = 5/100 = 0.05
	if math.Abs(im.ErrorRate-0.05) > 0.001 {
		t.Errorf("ErrorRate = %v, want ~0.05", im.ErrorRate)
	}

	if im.ActiveConnections != 12 {
		t.Errorf("ActiveConnections = %v, want 12", im.ActiveConnections)
	}
	if im.PanicsTotal != 2 {
		t.Errorf("PanicsTotal = %v, want 2", im.PanicsTotal)
	}

	// Should have latency percentiles > 0
	if im.P50 <= 0 {
		t.Errorf("P50 = %v, want > 0", im.P50)
	}
	if im.P99 <= 0 {
		t.Errorf("P99 = %v, want > 0", im.P99)
	}

	// Should have vendor metrics
	if len(im.Vendors) != 2 {
		t.Errorf("Vendors = %d, want 2", len(im.Vendors))
	}
}

func TestCollector_GetInstanceMetrics_SeriesGenerated(t *testing.T) {
	t.Parallel()
	c := NewCollector(100)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		c.Record(1, makeSnapshot(
			t0.Add(time.Duration(i)*10*time.Second),
			float64(1000+i*100),
			float64(50+i*5),
			10,
			float64(i),
		))
	}

	im := c.GetInstanceMetrics(1)
	if im == nil {
		t.Fatal("expected non-nil InstanceMetrics")
	}

	// 5 snapshots → 4 series points
	if len(im.Series) != 4 {
		t.Errorf("Series length = %d, want 4", len(im.Series))
	}

	// Should have vendor series for both vendors
	if len(im.VendorSeries) != 2 {
		t.Errorf("VendorSeries count = %d, want 2", len(im.VendorSeries))
	}
	for vid, series := range im.VendorSeries {
		if len(series) != 4 {
			t.Errorf("VendorSeries[%s] length = %d, want 4", vid, len(series))
		}
	}
}

func TestCollector_GetInstanceMetrics_TrendWithEnoughData(t *testing.T) {
	t.Parallel()
	c := NewCollector(DefaultCapacity)
	t0 := time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)

	// Fill 1h of data at 10s intervals
	for i := 0; i <= 360; i++ {
		c.Record(1, makeSnapshot(
			t0.Add(time.Duration(i)*10*time.Second),
			float64(i*100),
			float64(i*5),
			10,
			0,
		))
	}

	im := c.GetInstanceMetrics(1)
	if im == nil {
		t.Fatal("expected non-nil InstanceMetrics")
	}
	if im.RPSTrend == nil {
		t.Error("expected RPSTrend to be set with 1h of data")
	}
	if im.ErrorRateTrend == nil {
		t.Error("expected ErrorRateTrend to be set with 1h of data")
	}
}

func TestCollector_GetInstanceMetrics_NoTrendWithInsufficientData(t *testing.T) {
	t.Parallel()
	c := NewCollector(100)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		c.Record(1, makeSnapshot(
			t0.Add(time.Duration(i)*10*time.Second),
			float64(1000+i*100),
			float64(50+i*5),
			10,
			0,
		))
	}

	im := c.GetInstanceMetrics(1)
	if im == nil {
		t.Fatal("expected non-nil InstanceMetrics")
	}
	if im.RPSTrend != nil {
		t.Error("expected nil RPSTrend with < 50min of data")
	}
}

func TestCollector_GetFleetMetrics_AggregatesAcrossInstances(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	c.Record(1, makeSnapshot(t0, 1000, 50, 10, 1))
	c.Record(1, makeSnapshot(t1, 1100, 55, 12, 2))

	c.Record(2, makeSnapshot(t0, 2000, 100, 20, 0))
	c.Record(2, makeSnapshot(t1, 2200, 110, 22, 1))

	fm := c.GetFleetMetrics([]int64{1, 2})

	// RPS: instance1=10, instance2=20 → 30
	if math.Abs(fm.TotalRPS-30.0) > 0.01 {
		t.Errorf("TotalRPS = %v, want ~30", fm.TotalRPS)
	}
	if fm.TotalActiveConnections != 34 {
		t.Errorf("TotalActiveConnections = %v, want 34", fm.TotalActiveConnections)
	}
	if fm.TotalPanics != 3 {
		t.Errorf("TotalPanics = %v, want 3", fm.TotalPanics)
	}
	if len(fm.Instances) != 2 {
		t.Errorf("Instances = %d, want 2", len(fm.Instances))
	}
}

func TestCollector_GetFleetMetrics_SkipsMissingInstances(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	c.Record(1, makeSnapshot(t0, 1000, 50, 10, 0))
	c.Record(1, makeSnapshot(t1, 1100, 55, 12, 0))

	// Instance 99 has no data
	fm := c.GetFleetMetrics([]int64{1, 99})

	if len(fm.Instances) != 1 {
		t.Errorf("Instances = %d, want 1 (skip missing)", len(fm.Instances))
	}
}

func TestAddHistograms_MismatchedBoundaries_FallsBack(t *testing.T) {
	t.Parallel()
	a := Histogram{
		Count:   100,
		Buckets: []Bucket{{UpperBound: 0.05, Count: 50}, {UpperBound: 0.1, Count: 100}},
	}
	b := Histogram{
		Count:   200,
		Buckets: []Bucket{{UpperBound: 0.01, Count: 100}, {UpperBound: 0.5, Count: 200}},
	}
	result := addHistograms(a, b)

	// Should fall back to b (higher count) instead of merging mismatched buckets.
	if result.Count != 200 {
		t.Errorf("Count = %v, want 200 (fallback to b)", result.Count)
	}
	if result.Buckets[0].UpperBound != 0.01 {
		t.Errorf("Bucket[0].UpperBound = %v, want 0.01 (b's boundaries)", result.Buckets[0].UpperBound)
	}
}

func TestGetFleetMetrics_CounterReset_DoesNotCorruptErrorRate(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	// Instance 1: normal operation
	c.Record(1, makeSnapshot(t0, 1000, 50, 10, 0))
	c.Record(1, makeSnapshot(t1, 1100, 55, 12, 0))

	// Instance 2: counter reset (counters dropped from 2000 to 100)
	c.Record(2, makeSnapshot(t0, 2000, 100, 20, 0))
	c.Record(2, makeSnapshot(t1, 100, 5, 22, 0))

	fm := c.GetFleetMetrics([]int64{1, 2})

	// Fleet error rate should only reflect instance 1 (instance 2 skipped due to reset).
	// Instance 1: 5 errors / 100 requests = 0.05
	if fm.FleetErrorRate < 0 {
		t.Errorf("FleetErrorRate = %v, want >= 0 (counter reset should not corrupt)", fm.FleetErrorRate)
	}
	if math.Abs(fm.FleetErrorRate-0.05) > 0.01 {
		t.Errorf("FleetErrorRate = %v, want ~0.05 (only instance 1)", fm.FleetErrorRate)
	}
}

func TestGetFleetMetrics_ErrorRateTrend_Populated(t *testing.T) {
	t.Parallel()
	c := NewCollector(DefaultCapacity)
	t0 := time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)

	for i := 0; i <= 360; i++ {
		c.Record(1, makeSnapshot(
			t0.Add(time.Duration(i)*10*time.Second),
			float64(i*100),
			float64(i*5),
			10,
			0,
		))
	}

	fm := c.GetFleetMetrics([]int64{1})
	if fm.ErrorRateTrend == nil {
		t.Error("expected ErrorRateTrend to be set with 1h of data")
	}
}

func TestCollector_GetInstanceSummary_NoData_ReturnsNil(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	if got := c.GetInstanceSummary(1); got != nil {
		t.Error("expected nil for unknown instance")
	}
}

func TestCollector_GetInstanceSummary_ComputesKPIs(t *testing.T) {
	t.Parallel()
	c := NewCollector(10)
	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Second)

	c.Record(1, makeSnapshot(t0, 1000, 50, 10, 1))
	c.Record(1, makeSnapshot(t1, 1100, 55, 12, 2))

	s := c.GetInstanceSummary(1)
	if s == nil {
		t.Fatal("expected non-nil InstanceSummary")
	}
	if math.Abs(s.RPS-10.0) > 0.01 {
		t.Errorf("RPS = %v, want ~10", s.RPS)
	}
	if s.ActiveConnections != 12 {
		t.Errorf("ActiveConnections = %v, want 12", s.ActiveConnections)
	}
}
