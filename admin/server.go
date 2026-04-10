// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package admin

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cloudblue/chaperone/admin/api"
	"github.com/cloudblue/chaperone/admin/auth"
	"github.com/cloudblue/chaperone/admin/config"
	"github.com/cloudblue/chaperone/admin/metrics"
	"github.com/cloudblue/chaperone/admin/store"
)

// Server is the admin portal HTTP server.
type Server struct {
	httpServer *http.Server
	config     *config.Config
	store      *store.Store
	collector  *metrics.Collector
}

// NewServer creates a new admin portal server.
func NewServer(cfg *config.Config, st *store.Store, collector *metrics.Collector) (*Server, error) {
	mux := http.NewServeMux()

	authService := auth.NewService(st, cfg.Session.MaxAge.Unwrap(), cfg.Session.IdleTimeout.Unwrap())
	secureCookies := cfg.Server.SecureCookies

	handler := securityHeaders(auth.RequireAuth(authService, auth.CSRFProtection(mux)))

	s := &Server{
		httpServer: &http.Server{
			Addr:              cfg.Server.Addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		config:    cfg,
		store:     st,
		collector: collector,
	}

	if err := s.routes(mux, authService, secureCookies); err != nil {
		return nil, fmt.Errorf("setting up routes: %w", err)
	}
	return s, nil
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) routes(mux *http.ServeMux, authService *auth.Service, secureCookies bool) error {
	// API health check for the portal itself.
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Auth endpoints (login, logout, password change).
	authHandler := api.NewAuthHandler(authService, secureCookies, s.config.Session.MaxAge.Unwrap())
	authHandler.Register(mux)

	// Instance CRUD + test connection.
	instances := api.NewInstanceHandler(s.store, s.config.Scraper.Timeout.Unwrap())
	instances.Register(mux)

	// Metrics API.
	metricsAPI := api.NewMetricsHandler(s.store, s.collector)
	metricsAPI.Register(mux)

	// SPA serving — all non-API routes serve the Vue app.
	assets, err := loadUIAssets()
	if err != nil {
		return fmt.Errorf("loading UI assets: %w", err)
	}
	mux.Handle("/", spaHandler(assets))
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		slog.Error("writing health response", "error", err)
	}
}

// securityHeaders adds standard security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves static files from the embedded filesystem,
// falling back to index.html for client-side routing.
func spaHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// API routes that didn't match a registered handler should 404,
		// not fall through to the SPA.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Clean and strip leading slash for fs.Stat.
		name := path.Clean(r.URL.Path[1:])
		if _, err := fs.Stat(assets, name); err != nil {
			// File not found — serve index.html for client-side routing.
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
