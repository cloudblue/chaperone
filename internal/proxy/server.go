// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package proxy provides the core HTTP reverse proxy functionality for
// the Chaperone egress proxy. It handles request routing, credential
// injection, and response sanitization.
package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"time"

	"github.com/cloudblue/chaperone/internal/config"
	chaperoneCtx "github.com/cloudblue/chaperone/internal/context"
	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/router"
	"github.com/cloudblue/chaperone/internal/security"
	"github.com/cloudblue/chaperone/internal/telemetry"
	"github.com/cloudblue/chaperone/internal/timing"
	"github.com/cloudblue/chaperone/sdk"
)

// TLSConfig holds the TLS/mTLS configuration for the server.
// When Enabled is true, the server enforces mTLS (Mode A) with TLS 1.3 minimum.
// When Enabled is false, the server runs plain HTTP (basic Mode B for testing).
type TLSConfig struct {
	// Enabled controls whether mTLS is active.
	Enabled bool

	// CertFile is the path to the server certificate PEM file.
	CertFile string

	// KeyFile is the path to the server private key PEM file.
	KeyFile string

	// CAFile is the path to the CA certificate PEM file for client verification.
	CAFile string
}

// Config holds the configuration for the proxy server.
type Config struct {
	// Addr is the address to listen on (e.g., ":8080").
	Addr string

	// Version is the version string to return from /_ops/version.
	Version string

	// HeaderPrefix is the prefix for context headers (default: "X-Connect").
	HeaderPrefix string

	// TraceHeader is the correlation ID header name (default: "Connect-Request-ID").
	// Per ADR-005, this is configurable to support non-Connect platforms.
	TraceHeader string

	// Plugin is the credential provider plugin. If nil, requests are
	// forwarded without credential injection.
	Plugin sdk.Plugin

	// TLS holds the mTLS configuration. If nil, defaults are applied.
	TLS *TLSConfig

	// AllowList maps hosts to allowed path patterns for URL validation.
	// Security: This enforces "Default Deny" - requests to hosts/paths not
	// in this list are rejected with 403 Forbidden.
	AllowList map[string][]string

	// SensitiveHeaders lists additional headers to redact from logs and strip
	// from responses. These are merged with built-in defaults by NewServer.
	SensitiveHeaders []string

	// Timeouts
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	KeepAliveTimeout time.Duration
	PluginTimeout    time.Duration
	ConnectTimeout   time.Duration
	ShutdownTimeout  time.Duration
}

// Server is the main proxy server.
type Server struct {
	config    Config
	reflector *security.Reflector
	httpSrv   *http.Server
	transport *http.Transport

	// started guards against calling Start() more than once, which would
	// panic on double-close of the ready channel.
	started atomic.Bool

	// ready is closed when the server is listening and ready to accept connections.
	ready chan struct{}
}

// NewServer creates a new proxy server with the given configuration.
// All required fields must be explicitly set; NewServer does not apply defaults.
// Returns an error if any required field is missing or invalid.
//
// Required fields: Addr, Version, HeaderPrefix, TraceHeader, TLS (non-nil),
// and all timeout values (must be > 0).
func NewServer(cfg Config) (*Server, error) {
	if err := validateProxyConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid proxy config: %w", err)
	}

	// Security: Merge user-provided sensitive headers with mandatory defaults.
	// Even if the config loader already merged, NewServer can be called directly
	// in tests — always ensure defaults are present.
	sensitiveHeaders := config.MergeSensitiveHeaders(cfg.SensitiveHeaders)

	// Clone http.DefaultTransport to inherit its production-ready defaults
	// (TLSHandshakeTimeout, KeepAlive, ForceAttemptHTTP2, MaxIdleConns, etc.)
	// and override only the fields we need to make configurable.
	dt, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http.DefaultTransport is not *http.Transport")
	}
	t := dt.Clone()
	t.DialContext = (&net.Dialer{
		Timeout:   cfg.ConnectTimeout,
		KeepAlive: cfg.KeepAliveTimeout,
	}).DialContext
	t.ResponseHeaderTimeout = cfg.ReadTimeout
	t.IdleConnTimeout = cfg.IdleTimeout

	return &Server{
		config:    cfg,
		reflector: security.NewReflector(sensitiveHeaders),
		transport: t,
		ready:     make(chan struct{}),
	}, nil
}

// validateProxyConfig validates that all required proxy configuration fields
// are set. This is defense-in-depth: the config loader validates too, but
// NewServer may be called directly by tests or Distributor code.
func validateProxyConfig(cfg *Config) error {
	var errs []error

	if cfg.Addr == "" {
		errs = append(errs, errors.New("addr is required"))
	}
	if cfg.Version == "" {
		errs = append(errs, errors.New("version is required"))
	}
	if cfg.HeaderPrefix == "" {
		errs = append(errs, errors.New("header prefix is required"))
	}
	if cfg.TraceHeader == "" {
		errs = append(errs, errors.New("trace header is required"))
	}
	if cfg.TLS == nil {
		errs = append(errs, errors.New("TLS config is required (use &TLSConfig{Enabled: false} to disable)"))
	}
	if cfg.ReadTimeout <= 0 {
		errs = append(errs, errors.New("read timeout must be positive"))
	}
	if cfg.WriteTimeout <= 0 {
		errs = append(errs, errors.New("write timeout must be positive"))
	}
	if cfg.IdleTimeout <= 0 {
		errs = append(errs, errors.New("idle timeout must be positive"))
	}
	if cfg.PluginTimeout <= 0 {
		errs = append(errs, errors.New("plugin timeout must be positive"))
	}
	if cfg.ConnectTimeout <= 0 {
		errs = append(errs, errors.New("connect timeout must be positive"))
	}
	if cfg.KeepAliveTimeout <= 0 {
		errs = append(errs, errors.New("keep-alive timeout must be positive"))
	}
	if cfg.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown timeout must be positive"))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Config returns the server's current configuration.
func (s *Server) Config() Config {
	return s.config
}

// Handler returns the HTTP handler for the server.
// This can be used for testing or with a custom http.Server.
//
// Middleware execution order for /proxy (outermost to innermost):
//  1. TraceIDMiddleware - extracts or generates trace ID, stores in context
//  2. RequestLoggerMiddleware - wraps response, always logs via defer (even on panic)
//  3. MetricsMiddleware - records Prometheus metrics (request count, duration, active connections)
//  4. PanicRecoveryMiddleware (global) - catches panics on /_ops endpoints
//
// Per-route middleware (applied only to /proxy):
//  5. Timing - creates recorder, wraps response to inject Server-Timing header
//  6. PanicRecoveryMiddleware (per-route) - catches panics, writes 500 to timing-wrapped response
//  7. AllowListMiddleware - validates target URL before credential injection
//  8. handleProxy - actual request handling
//
// Timing is scoped to /proxy only — operational endpoints (/_ops/*) do not
// emit Server-Timing headers, avoiding latency leakage on health checks
// that may be exposed without mTLS.
//
// Timing wraps PanicRecovery so that panic-induced 500 responses still flow
// through the timingResponseWriter, ensuring Server-Timing is present on
// all /proxy responses including panics.
//
// This order ensures:
//   - All requests are logged (including rejections and panics)
//   - Server-Timing header is present on all /proxy responses (including panics)
//   - Panics are caught and logged properly
//   - URL validation happens before credential injection
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Register operational endpoints (no allow list validation needed)
	mux.HandleFunc("GET /_ops/health", s.handleHealth)
	mux.HandleFunc("GET /_ops/version", s.handleVersion)

	// Register proxy endpoint: Timing -> PanicRecovery -> AllowList -> handler
	// Timing wraps PanicRecovery so panic 500s still get Server-Timing headers.
	// Security: AllowList is REQUIRED per Design Spec Section 5.3
	proxyHandler := timing.WithTiming(
		PanicRecoveryMiddleware(
			router.NewAllowListMiddleware(
				s.config.AllowList,
				s.config.HeaderPrefix,
				http.HandlerFunc(s.handleProxy),
			),
		),
	)
	mux.Handle("/proxy", proxyHandler)

	// Apply global middleware stack (logging, panic recovery)
	handler := s.withMiddleware(mux)

	return handler
}

// Start starts the HTTP server and blocks until it's shut down.
// If TLS is enabled (Mode A), the server starts with mTLS requiring client certificates.
// If TLS is disabled (basic Mode B), the server starts as plain HTTP.
//
// Start supports graceful shutdown via Shutdown(). It signals readiness via
// WaitForReady() once the server is listening. Returns nil if stopped cleanly
// via Shutdown (http.ErrServerClosed is suppressed).
//
// The actual listening address is available via Addr() after WaitForReady returns.
// Start must only be called once; subsequent calls return an error.
func (s *Server) Start() error {
	if !s.started.CompareAndSwap(false, true) {
		return errors.New("server already started")
	}
	s.httpSrv = &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.Handler(),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	// Load TLS configuration if mTLS is enabled (Mode A)
	if s.config.TLS.Enabled {
		tlsConfig, err := s.loadTLSConfig()
		if err != nil {
			return err
		}
		s.httpSrv.TLSConfig = tlsConfig
	}

	s.logStartup()

	// Create listener
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.httpSrv.Addr, err)
	}

	// When TLS is enabled, wrap the listener with TLS
	if s.config.TLS.Enabled {
		ln = tls.NewListener(ln, s.httpSrv.TLSConfig)
	}

	// Update Addr with the actual address (useful when :0 is used)
	s.httpSrv.Addr = ln.Addr().String()

	// Signal readiness
	close(s.ready)

	slog.Info("server listening", "addr", s.httpSrv.Addr)

	err = s.httpSrv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Addr returns the address the server is listening on.
// Only valid after WaitForReady() returns true.
func (s *Server) Addr() string {
	if s.httpSrv != nil {
		return s.httpSrv.Addr
	}
	return s.config.Addr
}

// Shutdown gracefully shuts down the server, allowing in-flight requests
// to complete within the given context's deadline.
//
// It waits for the server to become ready before draining, so it is safe
// to call concurrently with Start(). If the context expires first,
// Shutdown returns without draining.
func (s *Server) Shutdown(ctx context.Context) {
	select {
	case <-s.ready:
		// Server is listening, proceed with shutdown.
	case <-ctx.Done():
		slog.Warn("shutdown context expired before server was ready",
			"error", ctx.Err(),
		)
		return
	}

	slog.Info("shutdown initiated, draining connections...")

	if err := s.httpSrv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error, forcing close", "error", err)
		_ = s.httpSrv.Close()
	}

	slog.Info("shutdown complete")
}

// WaitForReady blocks until the server is ready to accept connections
// or the timeout is reached. Returns true if ready, false if timed out.
func (s *Server) WaitForReady(timeout time.Duration) bool {
	select {
	case <-s.ready:
		return true
	case <-time.After(timeout):
		return false
	}
}

// loadTLSConfig reads certificate files and creates a TLS configuration for mTLS.
func (s *Server) loadTLSConfig() (*tls.Config, error) {
	tlsCfg := s.config.TLS

	caCert, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate %s: %w", tlsCfg.CAFile, err)
	}

	serverCert, err := os.ReadFile(tlsCfg.CertFile)
	if err != nil {
		return nil, fmt.Errorf("reading server certificate %s: %w", tlsCfg.CertFile, err)
	}

	serverKey, err := os.ReadFile(tlsCfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("reading server key %s: %w", tlsCfg.KeyFile, err)
	}

	tlsConfig, err := NewTLSConfig(caCert, serverCert, serverKey)
	if err != nil {
		return nil, fmt.Errorf("creating TLS config: %w", err)
	}

	return tlsConfig, nil
}

// logStartup logs the server startup configuration.
func (s *Server) logStartup() {
	if s.config.TLS.Enabled {
		slog.Info("starting proxy server with mTLS (Mode A)",
			"addr", s.config.Addr,
			"mode", "A (mTLS)",
			"tls_min_version", "1.3",
			"client_auth", "RequireAndVerifyClientCert",
		)
		slog.Info("server configuration",
			"read_timeout", s.config.ReadTimeout,
			"write_timeout", s.config.WriteTimeout,
			"idle_timeout", s.config.IdleTimeout,
			"plugin_timeout", s.config.PluginTimeout,
			"cert_file", s.config.TLS.CertFile,
			"ca_file", s.config.TLS.CAFile,
		)
	} else {
		slog.Warn("starting proxy server in HTTP mode (no mTLS) - FOR DEVELOPMENT ONLY",
			"addr", s.config.Addr,
			"mode", "B (basic)",
		)
		slog.Info("server configuration",
			"read_timeout", s.config.ReadTimeout,
			"write_timeout", s.config.WriteTimeout,
			"idle_timeout", s.config.IdleTimeout,
			"plugin_timeout", s.config.PluginTimeout,
		)
	}
}

// withMiddleware wraps the handler with the global middleware stack.
// See Handler() for complete middleware ordering documentation.
// AllowListMiddleware is intentionally NOT here — it's per-route (only /proxy).
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply middleware: outermost runs first.
	// Order: TraceID -> RequestLogger -> Metrics -> PanicRecovery -> handler
	//
	// Note: timing.WithTiming and a per-route PanicRecovery are applied to /proxy
	// in Handler() — they are NOT applied here. The global PanicRecovery below
	// protects /_ops endpoints and acts as a safety net for /proxy.
	handler = PanicRecoveryMiddleware(handler)
	handler = telemetry.MetricsMiddleware(s.config.HeaderPrefix, handler)
	handler = observability.RequestLoggerMiddleware(slog.Default(), s.config.HeaderPrefix+"-Vendor-ID", handler)
	handler = observability.TraceIDMiddleware(s.config.TraceHeader, handler)
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

// handleProxy handles /proxy requests for all HTTP methods.
// The HTTP method is passed through to the target URL (method passthrough).
// It coordinates parsing, credential injection, and forwarding.
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Trace ID is already in context, set by TraceIDMiddleware.
	traceID := observability.TraceIDFromContext(r.Context())

	txCtx, err := chaperoneCtx.ParseContext(r, s.config.HeaderPrefix, s.config.TraceHeader)
	if err != nil {
		s.respondBadRequest(w, traceID, "failed to parse context", err)
		return
	}

	targetURL, err := url.Parse(txCtx.TargetURL)
	if err != nil {
		s.respondBadRequest(w, traceID, "invalid target URL", err)
		return
	}

	// SECURITY: Validate target URL scheme (HTTPS required in production)
	err = ValidateTargetScheme(targetURL)
	if err != nil {
		slog.Warn("insecure target URL rejected",
			"trace_id", traceID,
			"target_scheme", targetURL.Scheme,
			"target_host", targetURL.Host,
		)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Warn if using HTTP in development mode
	if targetURL.Scheme == "http" {
		slog.Warn("forwarding to insecure HTTP target - DEVELOPMENT ONLY",
			"trace_id", traceID,
			"target_host", targetURL.Host,
		)
	}

	r, err = s.injectCredentials(r, txCtx)
	if err != nil {
		s.handlePluginError(w, traceID, err)
		return
	}

	//nolint:contextcheck // ModifyResponse uses resp.Request.Context() internally
	s.forwardRequest(w, r, targetURL, traceID, txCtx)
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
// After injection, it stores credential values in the request context so the
// RedactingHandler can detect and redact them if they leak into log output
// (value-based scanning, Layers 3 & 4).
//
// Returns the (possibly updated) request and any error. The caller MUST use
// the returned request for all subsequent operations, because the context may
// have been enriched with secret values and injected header keys.
func (s *Server) injectCredentials(r *http.Request, txCtx *sdk.TransactionContext) (*http.Request, error) {
	if s.config.Plugin == nil {
		return r, nil
	}

	// Snapshot headers BEFORE the plugin call so we can detect Slow Path
	// mutations. This costs one Header.Clone() per request, but only for
	// requests that actually have a plugin — acceptable overhead given that
	// Slow Path plugins already do expensive operations (HMAC, Vault calls).
	headersBefore := r.Header.Clone()

	ctx, cancel := context.WithTimeout(r.Context(), s.config.PluginTimeout)
	defer cancel()

	pluginStart := time.Now()
	cred, err := s.config.Plugin.GetCredentials(ctx, *txCtx, r)
	pluginDuration := time.Since(pluginStart)

	// Store plugin duration in timing recorder (if present)
	if recorder := timing.FromContext(r.Context()); recorder != nil {
		recorder.RecordPlugin(pluginDuration)
	}

	if err != nil {
		return nil, err
	}

	// Fast Path: plugin returned headers to inject
	if cred != nil {
		reqCtx := r.Context()
		injectedKeys := make([]string, 0, len(cred.Headers))
		for k, v := range cred.Headers {
			r.Header.Set(k, v)
			injectedKeys = append(injectedKeys, k)
			// Store each credential value in context for value-based log redaction.
			// The RedactingHandler will scan all slog string attrs and messages
			// for these values. Short values (< MinSecretLength) are automatically
			// skipped by the handler to avoid false positives.
			reqCtx = observability.WithSecretValue(reqCtx, v)
		}
		// Store injected header keys so the Reflector can strip them from
		// responses (prevents credential reflection for non-standard headers).
		reqCtx = security.WithInjectedHeaders(reqCtx, injectedKeys)
		// Propagate enriched context to the returned request. The reverse proxy
		// will Clone this request, so resp.Request.Context() in ModifyResponse
		// will carry the secret values and injected header keys.
		r = r.WithContext(reqCtx)
		return r, nil
	}

	// Slow Path: plugin mutated the request directly (cred is nil).
	// Defense-in-depth: detect what the plugin injected by diffing headers
	// against our pre-call snapshot. This ensures log redaction and response
	// stripping work even if the plugin doesn't call WithSecretValue() or
	// WithInjectedHeaders() itself.
	r = s.detectSlowPathInjections(r, headersBefore)
	return r, nil
}

// detectSlowPathInjections compares the current request headers against
// a pre-plugin snapshot to discover what a Slow Path plugin injected.
// Any new or modified headers are treated as injected credentials:
//   - Their values are stored in context for value-based log redaction
//   - Their keys are stored in context for response header stripping
//
// This is a safety net — Slow Path plugins MAY still call
// observability.WithSecretValue() and security.WithInjectedHeaders()
// for finer control, but forgetting to do so is no longer a security gap.
func (s *Server) detectSlowPathInjections(r *http.Request, before http.Header) *http.Request {
	var injectedKeys []string
	reqCtx := r.Context()

	for key, newValues := range r.Header {
		oldValues, existed := before[key]
		if !existed || !headerValuesEqual(oldValues, newValues) {
			injectedKeys = append(injectedKeys, key)
			for _, v := range newValues {
				reqCtx = observability.WithSecretValue(reqCtx, v)
			}
		}
	}

	if len(injectedKeys) > 0 {
		reqCtx = security.WithInjectedHeaders(reqCtx, injectedKeys)
		r = r.WithContext(reqCtx)
	}

	return r
}

// headerValuesEqual returns true if two header value slices are identical.
func headerValuesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// forwardRequest forwards the request to the target URL via reverse proxy.
func (s *Server) forwardRequest(w http.ResponseWriter, r *http.Request, target *url.URL, traceID string, txCtx *sdk.TransactionContext) {
	// Record upstream timing for both telemetry metrics and Server-Timing header
	telTiming := telemetry.TimingFromContext(r.Context())
	upstreamStart := time.Now()

	proxy := s.createReverseProxy(target, traceID, txCtx, telTiming, upstreamStart)
	proxy.ServeHTTP(w, r) // #nosec G704 -- target validated against allow-list in handleProxy before reaching here
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
// The response modification chain runs in this order:
//  0. Record upstream duration (both telemetry metrics and Server-Timing recorder)
//  1. Plugin.ModifyResponse (if plugin exists) - returns *ResponseAction for Core instructions
//  2. Strip sensitive headers (Credential Reflection Protection) - always runs
//  3. Core error normalization - runs unless plugin returned ResponseAction{SkipErrorNormalization: true}
//
// upstreamStart is captured in forwardRequest before proxy.ServeHTTP is called.
// Exactly one of ModifyResponse or ErrorHandler fires per request — never both.
//
//nolint:contextcheck // ErrorHandler signature is defined by httputil.ReverseProxy; context is accessed via r.Context()
func (s *Server) createReverseProxy(target *url.URL, traceID string, txCtx *sdk.TransactionContext, telTiming *telemetry.Timing, upstreamStart time.Time) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target) // #nosec G704 -- target validated against allow-list in handleProxy before reaching here

	// Apply upstream transport with configurable timeouts.
	proxy.Transport = s.upstreamTransport()

	// Customize the Director to set the correct host and path
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Store upstream start time in context to avoid race condition
		// between Director and ModifyResponse (which may run on different goroutines)
		*req = *req.WithContext(telemetry.WithUpstreamStart(req.Context(), time.Now()))

		originalDirector(req)
		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		if target.Path != "" && target.Path != "/" {
			req.URL.Path = target.Path
		}
	}

	// Response modification chain: Timing → Plugin → Strip Headers → Error Normalization
	proxy.ModifyResponse = s.buildModifyResponse(traceID, txCtx, telTiming, upstreamStart) //nolint:bodyclose // resp.Body is managed by httputil.ReverseProxy

	// Handle proxy errors (upstream unreachable, connection refused, etc.)
	// ErrorHandler fires instead of ModifyResponse when the upstream is unreachable.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// Record telemetry upstream duration
		telemetry.RecordUpstreamDuration(r.Context(), telTiming)

		// Record the failed upstream attempt duration for Server-Timing header.
		if recorder := timing.FromContext(r.Context()); recorder != nil {
			recorder.RecordUpstream(time.Since(upstreamStart))
		}

		slog.Error("proxy error",
			"trace_id", traceID,
			"error", err,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return proxy
}

// upstreamTransport returns the shared HTTP transport with configurable
// connect, read, and idle timeouts. Created once in NewServer and reused
// across requests so all connections share a single pool.
func (s *Server) upstreamTransport() http.RoundTripper {
	return s.transport
}

// buildModifyResponse creates the response modification closure that runs
// the timing → plugin → strip headers → error normalization chain.
func (s *Server) buildModifyResponse(traceID string, txCtx *sdk.TransactionContext, telTiming *telemetry.Timing, upstreamStart time.Time) func(*http.Response) error {
	return func(resp *http.Response) error {
		// Step 0a: Record upstream duration for telemetry metrics (safe across goroutines)
		telemetry.RecordUpstreamDuration(resp.Request.Context(), telTiming)

		// Step 0b: Record upstream duration for Server-Timing header
		if recorder := timing.FromContext(resp.Request.Context()); recorder != nil {
			recorder.RecordUpstream(time.Since(upstreamStart))
		}

		var action *sdk.ResponseAction

		// Step 1: Plugin's ModifyResponse runs FIRST (allows Distributor customization)
		if s.config.Plugin != nil {
			ctx, cancel := context.WithTimeout(resp.Request.Context(), s.config.PluginTimeout)
			defer cancel()

			var err error
			action, err = s.config.Plugin.ModifyResponse(ctx, *txCtx, resp)
			if err != nil {
				slog.Warn("plugin ModifyResponse error", "trace_id", traceID, "error", err)
				// Continue with response processing even if plugin fails
			}
		}

		// Step 2: Strip sensitive headers (Credential Reflection Protection)
		// This ALWAYS runs, regardless of plugin action.
		// Static list: well-known sensitive headers (Authorization, Cookie, etc.)
		s.reflector.StripResponseHeaders(resp.Header)
		// Dynamic list: whatever headers the plugin actually injected per-request.
		// Prevents credential reflection for non-standard headers (e.g., X-Vendor-Token)
		// that aren't in the static sensitive list.
		security.StripInjectedHeaders(resp.Request.Context(), resp.Header)

		// Step 3: Core error normalization (safety net - unless plugin opted out)
		if action == nil || !action.SkipErrorNormalization {
			if err := security.NormalizeError(resp, traceID); err != nil {
				slog.Error("error normalization failed",
					"trace_id", traceID,
					"error", err,
				)
				// Continue even if normalization fails - response will be sent as-is
			}
		}

		slog.Info("upstream response", "trace_id", traceID, "status", resp.StatusCode, "content_length", resp.ContentLength)
		return nil
	}
}
