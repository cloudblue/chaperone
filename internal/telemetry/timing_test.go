// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTiming_SetAndGet(t *testing.T) {
	timing := &Timing{}

	timing.SetUpstreamDuration(100 * time.Millisecond)

	if got := timing.UpstreamDuration(); got != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", got)
	}
}

func TestTiming_ZeroDefault(t *testing.T) {
	timing := &Timing{}

	if got := timing.UpstreamDuration(); got != 0 {
		t.Errorf("expected 0, got %v", got)
	}
}

func TestTiming_ConcurrentAccess(t *testing.T) {
	timing := &Timing{}

	var wg sync.WaitGroup
	const iterations = 100

	// Multiple concurrent writers - only first should succeed due to CompareAndSwap
	for i := 1; i <= iterations; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			timing.SetUpstreamDuration(time.Duration(val) * time.Millisecond)
		}(i)
	}

	// Multiple concurrent readers - should not panic or return garbage
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = timing.UpstreamDuration()
		}()
	}

	wg.Wait()

	// Final value should be one of the written values (1 to iterations ms)
	// Due to CompareAndSwap, it will be whichever writer ran first
	got := timing.UpstreamDuration()
	if got < 1*time.Millisecond || got > time.Duration(iterations)*time.Millisecond {
		t.Errorf("unexpected duration: %v (expected 1ms to %dms)", got, iterations)
	}
}

func TestWithTiming(t *testing.T) {
	ctx := context.Background()
	ctx, timing := WithTiming(ctx)

	timing.SetUpstreamDuration(50 * time.Millisecond)

	// Retrieve from context
	retrieved := TimingFromContext(ctx)
	if retrieved == nil {
		t.Fatal("expected timing from context, got nil")
	}

	if got := retrieved.UpstreamDuration(); got != 50*time.Millisecond {
		t.Errorf("expected 50ms, got %v", got)
	}
}

func TestTimingFromContext_NotPresent(t *testing.T) {
	ctx := context.Background()

	timing := TimingFromContext(ctx)
	if timing != nil {
		t.Errorf("expected nil timing from empty context, got %v", timing)
	}
}

func TestWithUpstreamStart(t *testing.T) {
	ctx := context.Background()
	start := time.Now()

	ctx = WithUpstreamStart(ctx, start)

	retrieved := UpstreamStartFromContext(ctx)
	if !retrieved.Equal(start) {
		t.Errorf("expected %v, got %v", start, retrieved)
	}
}

func TestUpstreamStartFromContext_NotPresent(t *testing.T) {
	ctx := context.Background()

	retrieved := UpstreamStartFromContext(ctx)
	if !retrieved.IsZero() {
		t.Errorf("expected zero time from empty context, got %v", retrieved)
	}
}

func TestRecordUpstreamDuration(t *testing.T) {
	ctx := context.Background()
	ctx, timing := WithTiming(ctx)

	start := time.Now().Add(-50 * time.Millisecond)
	ctx = WithUpstreamStart(ctx, start)

	RecordUpstreamDuration(ctx, timing)

	// Should be approximately 50ms (with some tolerance)
	got := timing.UpstreamDuration()
	if got < 45*time.Millisecond || got > 100*time.Millisecond {
		t.Errorf("expected ~50ms, got %v", got)
	}
}

func TestRecordUpstreamDuration_NilTiming(t *testing.T) {
	ctx := context.Background()
	ctx = WithUpstreamStart(ctx, time.Now())

	// Should not panic
	RecordUpstreamDuration(ctx, nil)
}

func TestRecordUpstreamDuration_NoStart(t *testing.T) {
	ctx := context.Background()
	ctx, timing := WithTiming(ctx)

	// No start time set
	RecordUpstreamDuration(ctx, timing)

	// Should remain zero
	if got := timing.UpstreamDuration(); got != 0 {
		t.Errorf("expected 0 without start time, got %v", got)
	}
}
