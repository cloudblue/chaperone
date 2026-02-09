// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package timing

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNew_StartsTimer(t *testing.T) {
	before := time.Now()
	r := New()
	after := time.Now()

	// Total duration should be within the test window
	total := r.TotalDuration()
	if total < 0 {
		t.Errorf("TotalDuration() = %v, want >= 0", total)
	}
	if total > after.Sub(before)+time.Millisecond {
		t.Errorf("TotalDuration() = %v, unexpectedly large", total)
	}
}

func TestRecordPlugin_StoresDuration(t *testing.T) {
	r := New()
	d := 150 * time.Millisecond

	r.RecordPlugin(d)

	if got := r.PluginDuration(); got != d {
		t.Errorf("PluginDuration() = %v, want %v", got, d)
	}
}

func TestRecordUpstream_StoresDuration(t *testing.T) {
	r := New()
	d := 320 * time.Millisecond

	r.RecordUpstream(d)

	if got := r.UpstreamDuration(); got != d {
		t.Errorf("UpstreamDuration() = %v, want %v", got, d)
	}
}

func TestHeader_FormatCorrect(t *testing.T) {
	r := New()
	r.RecordPlugin(150 * time.Millisecond)
	r.RecordUpstream(320 * time.Millisecond)

	header := r.Header()

	// Verify format contains all three components
	if !strings.Contains(header, "plugin;dur=150.00") {
		t.Errorf("Header() = %q, want to contain 'plugin;dur=150.00'", header)
	}
	if !strings.Contains(header, "upstream;dur=320.00") {
		t.Errorf("Header() = %q, want to contain 'upstream;dur=320.00'", header)
	}
	// Overhead is total - plugin - upstream; always >= 0 due to clamping
	if !strings.Contains(header, "overhead;dur=") {
		t.Errorf("Header() = %q, want to contain 'overhead;dur='", header)
	}
}

func TestHeader_ZeroDurations(t *testing.T) {
	r := New()
	// Don't record any phases - all should be zero

	header := r.Header()

	if !strings.Contains(header, "plugin;dur=0.00") {
		t.Errorf("Header() = %q, want to contain 'plugin;dur=0.00'", header)
	}
	if !strings.Contains(header, "upstream;dur=0.00") {
		t.Errorf("Header() = %q, want to contain 'upstream;dur=0.00'", header)
	}
}

func TestHeader_NegativeOverheadProtection(t *testing.T) {
	r := New()
	// Record phases that exceed total (simulates clock skew)
	r.RecordPlugin(1 * time.Hour)
	r.RecordUpstream(1 * time.Hour)

	header := r.Header()

	// Overhead should be clamped to 0, not negative
	if strings.Contains(header, "overhead;dur=-") {
		t.Errorf("Header() = %q, should not contain negative overhead", header)
	}
	if !strings.Contains(header, "overhead;dur=0.00") {
		t.Errorf("Header() = %q, want overhead;dur=0.00 when clamped", header)
	}
}

func TestHeader_DecimalPrecision(t *testing.T) {
	r := New()
	// 150.256ms should round to 150.26
	r.RecordPlugin(150256 * time.Microsecond)

	header := r.Header()

	// Should have exactly 2 decimal places
	if !strings.Contains(header, "plugin;dur=150.26") {
		t.Errorf("Header() = %q, want 'plugin;dur=150.26' (2 decimal precision)", header)
	}
}

func TestContext_RoundTrip(t *testing.T) {
	r := New()
	r.RecordPlugin(100 * time.Millisecond)

	ctx := WithRecorder(context.Background(), r)
	retrieved := FromContext(ctx)

	if retrieved == nil {
		t.Fatal("FromContext() returned nil")
	}
	if retrieved.PluginDuration() != 100*time.Millisecond {
		t.Errorf("PluginDuration() = %v, want 100ms", retrieved.PluginDuration())
	}
}

func TestFromContext_NoRecorder_ReturnsNil(t *testing.T) {
	ctx := context.Background()

	r := FromContext(ctx)

	if r != nil {
		t.Errorf("FromContext() = %v, want nil when no recorder", r)
	}
}

func TestDurationToMS_Conversion(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want float64
	}{
		{"zero", 0, 0.0},
		{"one_millisecond", time.Millisecond, 1.0},
		{"100_microseconds", 100 * time.Microsecond, 0.1},
		{"sub_millisecond", 500 * time.Microsecond, 0.5},
		{"one_second", time.Second, 1000.0},
		{"fractional", 1500 * time.Microsecond, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := durationToMS(tt.dur)
			if got != tt.want {
				t.Errorf("durationToMS(%v) = %v, want %v", tt.dur, got, tt.want)
			}
		})
	}
}
