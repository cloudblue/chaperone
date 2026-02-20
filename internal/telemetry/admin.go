// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package telemetry provides observability infrastructure including
// the admin server, profiling endpoints, and metrics.
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AdminServer serves admin endpoints (health, version, metrics, pprof).
type AdminServer struct {
	addr   string
	mux    *http.ServeMux
	server *http.Server
	done   chan struct{} // closed when the Serve goroutine exits
}

// NewAdminServer creates a new admin server.
// The version parameter is surfaced via the GET /_ops/version endpoint.
func NewAdminServer(addr, version string) *AdminServer {
	mux := http.NewServeMux()

	// Health check (always available)
	mux.HandleFunc("GET /_ops/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "alive"}`))
	})

	// Version endpoint (also on traffic port via proxy server)
	mux.HandleFunc("GET /_ops/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	})

	// Prometheus metrics endpoint
	// Per Design Spec Section 5.1.C: /metrics on admin port
	mux.Handle("GET /metrics", promhttp.Handler())

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
// It returns an error if the server fails to bind to the address,
// ensuring callers are notified of startup failures.
func (s *AdminServer) Start() error {
	// Bind early to detect port conflicts before returning
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("admin server bind failed: %w", err)
	}

	s.server = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second, // Longer for CPU profiling
		IdleTimeout:  120 * time.Second,
	}

	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("admin server error", "error", err)
		}
	}()

	slog.Info("admin server started", "addr", s.addr)
	return nil
}

// Shutdown gracefully shuts down the admin server and waits for the
// Serve goroutine to fully exit, ensuring the listening port is released.
func (s *AdminServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	err := s.server.Shutdown(ctx)
	if s.done != nil {
		<-s.done
	}
	return err
}
