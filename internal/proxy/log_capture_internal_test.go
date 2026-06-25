// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
)

// syncBuffer is a thread-safe bytes.Buffer for capturing log output in tests.
//
// slog.Default() is a process-global, so tests that swap it with a buffer-backed
// handler can race against production goroutines (or other tests) that continue
// to log through the same handler. Wrapping the buffer in a mutex eliminates
// the data race between concurrent Write and String calls.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the buffer under the mutex. Implements io.Writer.
func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// String returns the buffered contents under the mutex.
func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// captureLogs swaps slog.Default() with a JSON handler that writes to a
// thread-safe buffer. It registers a t.Cleanup to restore the previous
// default handler, and returns a closure that yields the captured output.
func captureLogs(t *testing.T) func() string {
	t.Helper()
	prev := slog.Default()
	sb := &syncBuffer{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(sb, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return sb.String
}
