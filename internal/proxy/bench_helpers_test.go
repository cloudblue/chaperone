// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"io"
	"log/slog"
	"testing"
)

// silenceLogs redirects slog to io.Discard during benchmarks to avoid log I/O
// biasing measurements. Restores the default logger via b.Cleanup.
func silenceLogs(b *testing.B) {
	b.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	b.Cleanup(func() {
		slog.SetDefault(prev)
	})
}
