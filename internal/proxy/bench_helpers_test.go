// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// benchConfig returns a valid Config with all required fields for benchmarks.
// Callers override only the fields they need (e.g., Plugin, AllowList).
func benchConfig() Config {
	return Config{
		Addr:             ":0",
		Version:          "bench",
		HeaderPrefix:     "X-Connect",
		TraceHeader:      "Connect-Request-ID",
		TLS:              &TLSConfig{Enabled: false},
		AllowList:        map[string][]string{"127.0.0.1": {"/**"}},
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     30 * time.Second,
		IdleTimeout:      120 * time.Second,
		KeepAliveTimeout: 30 * time.Second,
		PluginTimeout:    10 * time.Second,
		ConnectTimeout:   5 * time.Second,
		ShutdownTimeout:  30 * time.Second,
	}
}

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
