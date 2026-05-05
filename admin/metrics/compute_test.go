// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"math"
	"testing"
	"time"
)

func TestCounterRate_Normal(t *testing.T) {
	t.Parallel()
	got := counterRate(100, 200, 10*time.Second)
	if got != 10.0 {
		t.Errorf("counterRate(100, 200, 10s) = %v, want 10", got)
	}
}

func TestCounterRate_Reset_ReturnsZero(t *testing.T) {
	t.Parallel()
	got := counterRate(200, 50, 10*time.Second)
	if got != 0 {
		t.Errorf("counterRate(200, 50, 10s) = %v, want 0 (reset)", got)
	}
}

func TestCounterRate_ZeroDuration_ReturnsZero(t *testing.T) {
	t.Parallel()
	got := counterRate(100, 200, 0)
	if got != 0 {
		t.Errorf("counterRate(100, 200, 0) = %v, want 0", got)
	}
}

func TestErrorRate_Normal(t *testing.T) {
	t.Parallel()
	// 100 total requests, 10 errors → 10%
	got := errorRate(900, 1000, 40, 50)
	if math.Abs(got-0.1) > 0.001 {
		t.Errorf("errorRate(900,1000,40,50) = %v, want ~0.1", got)
	}
}

func TestErrorRate_ZeroTotal_ReturnsZero(t *testing.T) {
	t.Parallel()
	got := errorRate(100, 100, 5, 5)
	if got != 0 {
		t.Errorf("errorRate with zero total delta = %v, want 0", got)
	}
}

func TestErrorRate_Reset_ReturnsZero(t *testing.T) {
	t.Parallel()
	got := errorRate(100, 50, 10, 5)
	if got != 0 {
		t.Errorf("errorRate with counter reset = %v, want 0", got)
	}
}

func TestErrorRate_CappedAtOne(t *testing.T) {
	t.Parallel()
	// Pathological case: error delta > total delta
	got := errorRate(0, 10, 0, 20)
	if got != 1 {
		t.Errorf("errorRate capped = %v, want 1", got)
	}
}

func TestHistogramQuantile_P50(t *testing.T) {
	t.Parallel()
	h := Histogram{
		Count: 1000,
		Buckets: []Bucket{
			{UpperBound: 0.05, Count: 200},
			{UpperBound: 0.1, Count: 500},
			{UpperBound: 0.25, Count: 800},
			{UpperBound: 0.5, Count: 950},
			{UpperBound: 1.0, Count: 1000},
		},
	}
	// rank = 0.5 * 1000 = 500
	// Falls in bucket [0.05, 0.1] at count 500 exactly → boundary
	got := histogramQuantile(0.50, h)
	if got != 0.1 {
		t.Errorf("p50 = %v, want 0.1", got)
	}
}

func TestHistogramQuantile_P99(t *testing.T) {
	t.Parallel()
	h := Histogram{
		Count: 1000,
		Buckets: []Bucket{
			{UpperBound: 0.05, Count: 200},
			{UpperBound: 0.1, Count: 500},
			{UpperBound: 0.25, Count: 800},
			{UpperBound: 0.5, Count: 950},
			{UpperBound: 1.0, Count: 1000},
		},
	}
	// rank = 0.99 * 1000 = 990
	// Falls in bucket [0.5, 1.0]: prevCount=950, count=1000
	// fraction = (990 - 950) / (1000 - 950) = 40/50 = 0.8
	// result = 0.5 + 0.8 * (1.0 - 0.5) = 0.5 + 0.4 = 0.9
	got := histogramQuantile(0.99, h)
	if math.Abs(got-0.9) > 0.001 {
		t.Errorf("p99 = %v, want ~0.9", got)
	}
}

func TestHistogramQuantile_Empty_ReturnsZero(t *testing.T) {
	t.Parallel()
	got := histogramQuantile(0.5, Histogram{})
	if got != 0 {
		t.Errorf("quantile of empty histogram = %v, want 0", got)
	}
}

func TestHistogramQuantile_AllAboveHighestBucket(t *testing.T) {
	t.Parallel()
	h := Histogram{
		Count:   100,
		Buckets: []Bucket{{UpperBound: 0.1, Count: 0}},
	}
	got := histogramQuantile(0.5, h)
	// All 100 observations are above the only bucket → return bucket upper bound
	if got != 0.1 {
		t.Errorf("quantile above all buckets = %v, want 0.1", got)
	}
}

func TestHistogramDelta_Normal(t *testing.T) {
	t.Parallel()
	prev := Histogram{
		Count: 100, Sum: 10,
		Buckets: []Bucket{{UpperBound: 0.1, Count: 50}, {UpperBound: 0.5, Count: 100}},
	}
	curr := Histogram{
		Count: 200, Sum: 25,
		Buckets: []Bucket{{UpperBound: 0.1, Count: 80}, {UpperBound: 0.5, Count: 200}},
	}

	d := histogramDelta(prev, curr)
	if d.Count != 100 {
		t.Errorf("delta Count = %v, want 100", d.Count)
	}
	if d.Sum != 15 {
		t.Errorf("delta Sum = %v, want 15", d.Sum)
	}
	if d.Buckets[0].Count != 30 {
		t.Errorf("delta bucket[0] Count = %v, want 30", d.Buckets[0].Count)
	}
	if d.Buckets[1].Count != 100 {
		t.Errorf("delta bucket[1] Count = %v, want 100", d.Buckets[1].Count)
	}
}

func TestHistogramDelta_Reset_ReturnsCurr(t *testing.T) {
	t.Parallel()
	prev := Histogram{Count: 200, Buckets: []Bucket{{UpperBound: 0.1, Count: 200}}}
	curr := Histogram{Count: 50, Buckets: []Bucket{{UpperBound: 0.1, Count: 50}}}

	d := histogramDelta(prev, curr)
	if d.Count != 50 {
		t.Errorf("delta after reset Count = %v, want 50 (curr)", d.Count)
	}
}

func TestSecondsToMs(t *testing.T) {
	t.Parallel()
	got := secondsToMs(0.123)
	if math.Abs(got-123.0) > 0.01 {
		t.Errorf("secondsToMs(0.123) = %v, want ~123", got)
	}
}
