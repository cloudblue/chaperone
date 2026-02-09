// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package timing provides performance attribution for request processing.
// It tracks time spent in different phases (plugin, upstream, overhead)
// and generates the standard Server-Timing header.
package timing

import (
	"context"
	"fmt"
	"time"
)

// contextKey is the type for context keys to avoid collisions.
type contextKey int

const recorderKey contextKey = iota

// Recorder tracks time spent in different phases of request processing.
// It is NOT safe for concurrent use. It is designed to be used within
// a single request lifecycle where all calls happen sequentially.
type Recorder struct {
	start    time.Time
	plugin   time.Duration
	upstream time.Duration
}

// New creates a new timing recorder, starting the clock immediately.
func New() *Recorder {
	return &Recorder{
		start: time.Now(),
	}
}

// RecordPlugin records the duration of plugin execution (GetCredentials).
func (r *Recorder) RecordPlugin(d time.Duration) {
	r.plugin = d
}

// RecordUpstream records the duration of the upstream HTTP roundtrip.
func (r *Recorder) RecordUpstream(d time.Duration) {
	r.upstream = d
}

// PluginDuration returns the recorded plugin duration.
func (r *Recorder) PluginDuration() time.Duration {
	return r.plugin
}

// UpstreamDuration returns the recorded upstream duration.
func (r *Recorder) UpstreamDuration() time.Duration {
	return r.upstream
}

// TotalDuration returns the total elapsed time since recorder creation.
func (r *Recorder) TotalDuration() time.Duration {
	return time.Since(r.start)
}

// Header returns the Server-Timing header value.
// Format: plugin;dur=X.XX, upstream;dur=X.XX, overhead;dur=X.XX
// Durations are in milliseconds with 2 decimal places per W3C spec.
func (r *Recorder) Header() string {
	total := time.Since(r.start)
	overhead := total - r.plugin - r.upstream

	// Clock skew protection: ensure overhead is not negative
	if overhead < 0 {
		overhead = 0
	}

	return fmt.Sprintf(
		"plugin;dur=%.2f, upstream;dur=%.2f, overhead;dur=%.2f",
		durationToMS(r.plugin),
		durationToMS(r.upstream),
		durationToMS(overhead),
	)
}

// durationToMS converts a time.Duration to milliseconds as a float64.
// Uses full nanosecond precision from the Duration value.
func durationToMS(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// WithRecorder stores a Recorder in the context.
func WithRecorder(ctx context.Context, r *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey, r)
}

// FromContext retrieves the Recorder from the context.
// Returns nil if no recorder is present.
func FromContext(ctx context.Context) *Recorder {
	r, _ := ctx.Value(recorderKey).(*Recorder)
	return r
}
