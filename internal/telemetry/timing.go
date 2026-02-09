// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"sync/atomic"
	"time"
)

// Context keys for timing data.
type timingKey struct{}
type upstreamStartKey struct{}

// Timing holds timing information for a single request.
// It is safe for concurrent use; all methods use atomic operations.
//
// Thread-safety rationale:
// We use atomic operations because the Timing struct is written in
// ModifyResponse/ErrorHandler (potentially on a transport goroutine) and read
// in MetricsMiddleware after ServeHTTP returns. While the typical flow is
// sequential, atomics provide correctness guarantees with negligible overhead
// for edge cases like connection hijacking or HTTP/2 stream handling.
type Timing struct {
	upstreamDurationNs atomic.Int64
}

// SetUpstreamDuration records the upstream call duration atomically.
// Only the first call succeeds; subsequent calls (e.g., from redirects) are ignored.
// This ensures we measure the total upstream time, not just the final hop.
func (t *Timing) SetUpstreamDuration(d time.Duration) {
	t.upstreamDurationNs.CompareAndSwap(0, d.Nanoseconds())
}

// UpstreamDuration returns the recorded upstream duration.
// Returns zero if not yet set.
func (t *Timing) UpstreamDuration() time.Duration {
	return time.Duration(t.upstreamDurationNs.Load())
}

// WithTiming adds timing data to the context.
func WithTiming(ctx context.Context) (context.Context, *Timing) {
	t := &Timing{}
	return context.WithValue(ctx, timingKey{}, t), t
}

// TimingFromContext retrieves timing data from context.
// Returns nil if not present.
func TimingFromContext(ctx context.Context) *Timing {
	t, _ := ctx.Value(timingKey{}).(*Timing)
	return t
}

// WithUpstreamStart stores the upstream call start time in the context.
// This is used to avoid race conditions between Director and ModifyResponse.
func WithUpstreamStart(ctx context.Context, start time.Time) context.Context {
	return context.WithValue(ctx, upstreamStartKey{}, start)
}

// UpstreamStartFromContext retrieves the upstream start time from context.
// Returns zero time if not present.
func UpstreamStartFromContext(ctx context.Context) time.Time {
	if t, ok := ctx.Value(upstreamStartKey{}).(time.Time); ok {
		return t
	}
	return time.Time{}
}

// RecordUpstreamDuration is a helper that records the duration since start
// into the timing context if both are present.
func RecordUpstreamDuration(ctx context.Context, timing *Timing) {
	if timing == nil {
		return
	}
	if start := UpstreamStartFromContext(ctx); !start.IsZero() {
		timing.SetUpstreamDuration(time.Since(start))
	}
}
