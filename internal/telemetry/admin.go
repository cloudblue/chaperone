// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package telemetry provides observability infrastructure including
// the admin server, profiling endpoints, and metrics.
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// AdminServer serves admin endpoints (health, metrics, pprof).
type AdminServer struct {
	addr   string
	mux    *http.ServeMux
	server *http.Server
}

// NewAdminServer creates a new admin server.
func NewAdminServer(addr string) *AdminServer {
	mux := http.NewServeMux()

	// Health check (always available)
	mux.HandleFunc("GET /_ops/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "alive"}`))
	})

	return &AdminServer{
		addr: addr,
		mux:  mux,
	}
}

// Mux returns the underlying ServeMux for registering additional handlers.
// Used by other packages to add routes (e.g., /metrics).
func (s *AdminServer) Mux() *http.ServeMux {
	return s.mux
}

// Addr returns the configured address.
func (s *AdminServer) Addr() string {
	return s.addr
}

// Start starts the admin server in a goroutine.
func (s *AdminServer) Start() error {
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second, // Longer for CPU profiling
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("admin server starting", "addr", s.addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("admin server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the admin server.
func (s *AdminServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
