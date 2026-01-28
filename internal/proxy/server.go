// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package proxy provides the core HTTP reverse proxy functionality for
// the Chaperone egress proxy. It handles request routing, credential
// injection, and response sanitization.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	chaperoneCtx "github.com/cloudblue/chaperone/internal/context"
	"github.com/cloudblue/chaperone/sdk"
)

// Default timeout values for resilience.
const (
	DefaultReadTimeout   = 5 * time.Second
	DefaultWriteTimeout  = 30 * time.Second
	DefaultIdleTimeout   = 120 * time.Second
	DefaultPluginTimeout = 10 * time.Second
)

// Config holds the configuration for the proxy server.
//
//nolint:govet // fieldalignment: optimizing for readability over memory layout
type Config struct {
	// Addr is the address to listen on (e.g., ":8080").
	Addr string

	// Version is the version string to return from /_ops/version.
	Version string

	// HeaderPrefix is the prefix for context headers (default: "X-Connect").
	HeaderPrefix string

	// Plugin is the credential provider plugin. If nil, requests are
	// forwarded without credential injection.
	Plugin sdk.Plugin

	// Timeouts
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	PluginTimeout time.Duration
}

// Server is the main proxy server.
type Server struct {
	config Config
}

// NewServer creates a new proxy server with the given configuration.
// Default values are applied for any unset configuration options.
func NewServer(cfg Config) *Server {
	// Apply defaults
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = DefaultReadTimeout
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = DefaultWriteTimeout
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.PluginTimeout == 0 {
		cfg.PluginTimeout = DefaultPluginTimeout
	}
	if cfg.HeaderPrefix == "" {
		cfg.HeaderPrefix = chaperoneCtx.DefaultHeaderPrefix
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	return &Server{
		config: cfg,
	}
}

// Config returns the server's current configuration.
func (s *Server) Config() Config {
	return s.config
}

// Handler returns the HTTP handler for the server.
// This can be used for testing or with a custom http.Server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("GET /_ops/health", s.handleHealth)
	mux.HandleFunc("GET /_ops/version", s.handleVersion)
	mux.HandleFunc("POST /proxy", s.handleProxy)

	// Apply middleware stack
	handler := s.withMiddleware(mux)

	return handler
}

// Start starts the HTTP server and blocks until it's shut down.
func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.Handler(),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	slog.Info("starting proxy server",
		"addr", s.config.Addr,
		"read_timeout", s.config.ReadTimeout,
		"write_timeout", s.config.WriteTimeout,
		"idle_timeout", s.config.IdleTimeout,
		"plugin_timeout", s.config.PluginTimeout,
	)

	return srv.ListenAndServe()
}

// withMiddleware wraps the handler with the middleware stack.
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply middleware in order: outermost (first to run) to innermost
	// 1. RequestLogging (outermost) - wraps response, captures status, always logs via defer
	// 2. PanicRecovery (innermost) - catches panics, writes 500 to wrapped response
	//
	// This order ensures that when a panic occurs:
	// - PanicRecovery catches it and writes 500 to the wrapped ResponseWriter
	// - RequestLogging's defer runs and logs with the correct status (500)
	handler = WithPanicRecovery(handler)
	handler = WithRequestLogging(handler)
	return handler
}

// handleHealth handles GET /_ops/health requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "alive"}`)) // Error ignored: client may have disconnected
}

// handleVersion handles GET /_ops/version requests.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"version": "%s"}`, s.config.Version)
}

// handleProxy handles POST /proxy requests.
// It coordinates parsing, credential injection, and forwarding.
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	traceID := s.extractTraceID(r)
	w.Header().Set("X-Trace-ID", traceID)

	txCtx, err := chaperoneCtx.ParseContext(r, s.config.HeaderPrefix)
	if err != nil {
		s.respondBadRequest(w, traceID, "failed to parse context", err)
		return
	}

	err = s.injectCredentials(r, txCtx)
	if err != nil {
		s.handlePluginError(w, traceID, err)
		return
	}

	targetURL, err := url.Parse(txCtx.TargetURL)
	if err != nil {
		s.respondBadRequest(w, traceID, "invalid target URL", err)
		return
	}

	s.forwardRequest(w, r, targetURL, traceID)
}

// extractTraceID returns the trace ID from the request header or generates a new one.
func (s *Server) extractTraceID(r *http.Request) string {
	if traceID := r.Header.Get(chaperoneCtx.DefaultTraceHeader); traceID != "" {
		return traceID
	}
	return generateTraceID()
}

// respondBadRequest logs and responds with a 400 Bad Request.
//
// SECURITY NOTE: The error message is exposed to the client. This is intentional
// for client input validation errors (missing headers, malformed Base64/JSON, invalid URLs).
// Do NOT use this for internal errors that could leak system details.
func (s *Server) respondBadRequest(w http.ResponseWriter, traceID, msg string, err error) {
	slog.Warn(msg,
		"trace_id", traceID,
		"error", err,
	)
	http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
}

// injectCredentials fetches credentials from the plugin and injects them into the request.
// Returns an error if credential injection fails; caller is responsible for handling the error.
func (s *Server) injectCredentials(r *http.Request, txCtx *sdk.TransactionContext) error {
	if s.config.Plugin == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.config.PluginTimeout)
	defer cancel()

	cred, err := s.config.Plugin.GetCredentials(ctx, *txCtx, r)
	if err != nil {
		return err
	}

	// Fast Path: plugin returned headers to inject
	if cred != nil {
		for k, v := range cred.Headers {
			r.Header.Set(k, v)
		}
	}
	// Slow Path: plugin mutated request directly (cred is nil)

	return nil
}

// forwardRequest forwards the request to the target URL via reverse proxy.
func (s *Server) forwardRequest(w http.ResponseWriter, r *http.Request, target *url.URL, traceID string) {
	proxy := s.createReverseProxy(target, traceID)
	proxy.ServeHTTP(w, r)
}

// handlePluginError handles errors from the plugin.
func (s *Server) handlePluginError(w http.ResponseWriter, traceID string, err error) {
	// Check for context errors (timeout/cancellation)
	if errors.Is(err, context.DeadlineExceeded) {
		slog.Error("plugin timeout",
			"trace_id", traceID,
			"error", err,
		)
		http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		return
	}

	if errors.Is(err, context.Canceled) {
		// Client disconnected - don't write response
		slog.Info("client disconnected",
			"trace_id", traceID,
		)
		return
	}

	// Generic plugin error
	slog.Error("plugin error",
		"trace_id", traceID,
		"error", err,
	)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

// createReverseProxy creates a configured reverse proxy for the target URL.
func (s *Server) createReverseProxy(target *url.URL, traceID string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the Director to set the correct host and path
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		// Preserve path from target URL (allows proxying to specific endpoints)
		if target.Path != "" && target.Path != "/" {
			req.URL.Path = target.Path
		}
	}

	// Modify response to strip sensitive headers
	proxy.ModifyResponse = func(resp *http.Response) error {
		stripSensitiveHeaders(resp.Header)

		// Log the response
		slog.Info("upstream response",
			"trace_id", traceID,
			"status", resp.StatusCode,
			"content_length", resp.ContentLength,
		)

		return nil
	}

	// Handle proxy errors
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error",
			"trace_id", traceID,
			"error", err,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return proxy
}

// sensitiveHeaders is the list of headers that must be stripped from responses.
var sensitiveHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Set-Cookie",
	"X-API-Key",
	"X-Auth-Token",
}

// stripSensitiveHeaders removes sensitive headers from the header map.
func stripSensitiveHeaders(h http.Header) {
	for _, header := range sensitiveHeaders {
		h.Del(header)
	}
}

// generateTraceID generates a unique trace ID for request tracking.
func generateTraceID() string {
	// Use a simple timestamp-based ID for PoC
	// TODO: Use UUID in production
	return fmt.Sprintf("trace-%d", time.Now().UnixNano())
}
