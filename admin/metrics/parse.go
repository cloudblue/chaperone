// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// Parse parses Prometheus text exposition format data into a Snapshot.
// Only Chaperone-specific metrics are extracted; unknown metrics are ignored.
func Parse(data []byte, t time.Time) (*Snapshot, error) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parsing prometheus text: %w", err)
	}

	snap := &Snapshot{
		Time:    t,
		Vendors: make(map[string]*VendorSnapshot),
	}

	for name, family := range families {
		switch name {
		case metricRequestsTotal:
			parseRequests(family, snap)
		case metricDurationSeconds:
			parseDuration(family, snap)
		case metricActiveConns:
			parseGauge(family, &snap.ActiveConnections)
		case metricPanicsTotal:
			parseCounter(family, &snap.PanicsTotal)
		}
	}

	return snap, nil
}

// parseRequests extracts chaperone_requests_total counters, summing across
// methods but preserving vendor_id and status_class dimensions.
func parseRequests(family *dto.MetricFamily, snap *Snapshot) {
	for _, m := range family.GetMetric() {
		vendorID := labelValue(m.GetLabel(), labelVendorID)
		statusClass := labelValue(m.GetLabel(), labelStatusClass)
		value := m.GetCounter().GetValue()

		vs := snap.vendorOrCreate(vendorID)
		vs.RequestsTotal += value
		if statusClass == "4xx" || statusClass == "5xx" {
			vs.RequestsErrors += value
		}
	}
}

// parseDuration extracts chaperone_request_duration_seconds histograms per vendor.
func parseDuration(family *dto.MetricFamily, snap *Snapshot) {
	for _, m := range family.GetMetric() {
		vendorID := labelValue(m.GetLabel(), labelVendorID)
		h := m.GetHistogram()
		if h == nil {
			continue
		}

		hist := Histogram{
			Count: float64(h.GetSampleCount()),
			Sum:   h.GetSampleSum(),
		}
		for _, b := range h.GetBucket() {
			hist.Buckets = append(hist.Buckets, Bucket{
				UpperBound: b.GetUpperBound(),
				Count:      float64(b.GetCumulativeCount()),
			})
		}
		sort.Slice(hist.Buckets, func(i, j int) bool {
			return hist.Buckets[i].UpperBound < hist.Buckets[j].UpperBound
		})
		// Strip the +Inf bucket — we use Count for that.
		if n := len(hist.Buckets); n > 0 && math.IsInf(hist.Buckets[n-1].UpperBound, 1) {
			hist.Buckets = hist.Buckets[:n-1]
		}

		snap.vendorOrCreate(vendorID).Duration = hist
	}
}

// parseGauge extracts a single gauge value (first metric in the family).
func parseGauge(family *dto.MetricFamily, dst *float64) {
	if ms := family.GetMetric(); len(ms) > 0 {
		*dst = ms[0].GetGauge().GetValue()
	}
}

// parseCounter extracts a single counter value (first metric in the family).
func parseCounter(family *dto.MetricFamily, dst *float64) {
	if ms := family.GetMetric(); len(ms) > 0 {
		*dst = ms[0].GetCounter().GetValue()
	}
}

// labelValue returns the value for the named label, or "" if not found.
func labelValue(labels []*dto.LabelPair, name string) string {
	for _, l := range labels {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}
