// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"math"
	"time"
)

// counterRate computes the per-second rate between two counter values.
// Returns 0 if the counter was reset (curr < prev) or the time delta is zero.
func counterRate(prev, curr float64, dt time.Duration) float64 {
	if dt <= 0 || curr < prev {
		return 0
	}
	return (curr - prev) / dt.Seconds()
}

// errorRate computes the fraction of errors in the interval.
// Returns 0 if the total delta is zero or a counter reset occurred.
func errorRate(prevTotal, currTotal, prevErrors, currErrors float64) float64 {
	totalDelta := currTotal - prevTotal
	if totalDelta <= 0 {
		return 0
	}
	errDelta := currErrors - prevErrors
	if errDelta < 0 {
		return 0
	}
	rate := errDelta / totalDelta
	return math.Min(rate, 1)
}

// histogramQuantile computes the q-th quantile (0 ≤ q ≤ 1) from cumulative
// histogram buckets using linear interpolation — the standard Prometheus method.
//
// Buckets must be sorted by UpperBound with the +Inf bucket excluded.
func histogramQuantile(q float64, h Histogram) float64 {
	if h.Count == 0 || len(h.Buckets) == 0 {
		return 0
	}

	rank := q * h.Count
	var prevCount, prevBound float64

	for _, b := range h.Buckets {
		if b.Count >= rank {
			if b.Count == prevCount {
				return prevBound
			}
			fraction := (rank - prevCount) / (b.Count - prevCount)
			return prevBound + fraction*(b.UpperBound-prevBound)
		}
		prevCount = b.Count
		prevBound = b.UpperBound
	}

	// All observations above the highest finite bucket.
	return h.Buckets[len(h.Buckets)-1].UpperBound
}

// histogramDelta computes the per-interval histogram by subtracting prev from
// curr. On counter reset (curr.Count < prev.Count) it returns curr as-is.
func histogramDelta(prev, curr Histogram) Histogram {
	if curr.Count < prev.Count {
		return curr
	}

	delta := Histogram{
		Count: curr.Count - prev.Count,
		Sum:   curr.Sum - prev.Sum,
	}

	for i, b := range curr.Buckets {
		bc := b.Count
		if i < len(prev.Buckets) {
			bc -= prev.Buckets[i].Count
			if bc < 0 {
				bc = 0
			}
		}
		delta.Buckets = append(delta.Buckets, Bucket{
			UpperBound: b.UpperBound,
			Count:      bc,
		})
	}

	return delta
}

// secondsToMs converts seconds to milliseconds, rounding to 2 decimal places.
func secondsToMs(s float64) float64 {
	return math.Round(s*100000) / 100
}
