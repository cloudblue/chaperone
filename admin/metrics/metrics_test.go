// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"testing"
	"time"
)

func TestCapacityFor_SpansAtLeastTheRequestedWindow(t *testing.T) {
	t.Parallel()

	// N snapshots at fixed interval I span (N-1)*I of wall-clock time, since
	// rates and series are computed from adjacent pairs. Capacity must be
	// large enough that (capacity-1)*interval >= window.
	tests := []struct {
		name     string
		window   time.Duration
		interval time.Duration
	}{
		{"1h at 10s", 1 * time.Hour, 10 * time.Second},
		{"1h at 3s", 1 * time.Hour, 3 * time.Second},
		{"30m at 10s", 30 * time.Minute, 10 * time.Second},
		{"1h at 7s (non-divisible)", 1 * time.Hour, 7 * time.Second},
		{"window equals interval", 10 * time.Second, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := CapacityFor(tt.window, tt.interval)
			span := time.Duration(got-1) * tt.interval
			if span < tt.window {
				t.Errorf("CapacityFor(%v, %v) = %d snapshots spans %v, want >= %v",
					tt.window, tt.interval, got, span, tt.window)
			}
		})
	}
}

func TestCapacityFor_AlwaysAllowsRatePair(t *testing.T) {
	t.Parallel()

	// Rate and series computation requires at least two adjacent snapshots.
	// CapacityFor must never return less than 2, including for degenerate
	// inputs that config validation would normally reject.
	tests := []struct {
		name     string
		window   time.Duration
		interval time.Duration
	}{
		{"window equals interval", 10 * time.Second, 10 * time.Second},
		{"window smaller than interval", 1 * time.Second, 10 * time.Second},
		{"zero interval", 1 * time.Hour, 0},
		{"zero window", 0, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := CapacityFor(tt.window, tt.interval)
			if got < 2 {
				t.Errorf("CapacityFor(%v, %v) = %d, want >= 2", tt.window, tt.interval, got)
			}
		})
	}
}
