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
	"os"
	"time"

	"github.com/cloudblue/chaperone/internal/config"
	chaperoneCtx "github.com/cloudblue/chaperone/internal/context"
	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/internal/router"
	"github.com/cloudblue/chaperone/internal/security"
	"github.com/cloudblue/chaperone/sdk"
)

// Default timeout values for resilience.
const (
	DefaultReadTimeout   = 5 * time.Second
	DefaultWriteTimeout  = 30 * time.Second
	DefaultIdleTimeout   = 120 * time.Second
	DefaultPluginTimeout = 10 * time.Second
)

// DefaultMTLSEnabled controls whether mTLS is enabled by default.
// Set to false for development/testing (basic Mode B).
// In production (Mode A), this should always be true.
const DefaultMTLSEnabled = true

// Default certificate file paths (per Design Spec Section 5.5).
const (
	DefaultCertFile = "/certs/server.crt"
	DefaultKeyFile  = "/certs/server.key"
	DefaultCAFile   = "/certs/ca.crt"
)

// TLSConfig holds the TLS/mTLS configuration for the server.
// When Enabled is true, the server enforces mTLS (Mode A) with TLS 1.3 minimum.
// When Enabled is false, the server runs plain HTTP (basic Mode B for testing).
type TLSConfig struct {
	// Enabled controls whether mTLS is active. Defaults to DefaultMTLSEnabled.
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
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	PluginTimeout time.Duration
}

// Server is the main proxy server.
type Server struct {
	config    Config
	reflector *security.Reflector
}

// NewServer creates a new proxy server with the given configuration.
// Default values are applied for any unset configuration options.
func NewServer(cfg Config) *Server {
	// Apply defaults for timeouts
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
		cfg.HeaderPrefix = config.DefaultHeaderPrefix
	}
	if cfg.TraceHeader == "" {
		cfg.TraceHeader = config.DefaultTraceHeader
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	// Apply TLS defaults
	if cfg.TLS == nil {
		cfg.TLS = &TLSConfig{
			Enabled:  DefaultMTLSEnabled,
			CertFile: DefaultCertFile,
			KeyFile:  DefaultKeyFile,
			CAFile:   DefaultCAFile,
		}
	} else {
		// Apply defaults for any unset TLS fields
		if cfg.TLS.CertFile == "" {
			cfg.TLS.CertFile = DefaultCertFile
		}
		if cfg.TLS.KeyFile == "" {
			cfg.TLS.KeyFile = DefaultKeyFile
		}
		if cfg.TLS.CAFile == "" {
			cfg.TLS.CAFile = DefaultCAFile
		}
	}

	// Security: Merge user-provided sensitive headers with mandatory defaults.
	// Even if the config loader already merged, NewServer can be called directly
	// in tests — always ensure defaults are present.
	sensitiveHeaders := config.MergeSensitiveHeaders(cfg.SensitiveHeaders)

	return &Server{
		config:    cfg,
		reflector: security.NewReflector(sensitiveHeaders),
	}
}

// Config returns the server's current configuration.
func (s *Server) Config() Config {
	return s.config
}

// Handler returns the HTTP handler for the server.
// This can be used for testing or with a custom http.Server.
//
// Middleware execution order (outermost to innermost):
//  1. RequestLogging - wraps response, always logs via defer (even on panic)
//  2. PanicRecovery - catches panics, writes 500 to wrapped response
//  3. AllowList (on /proxy only) - validates target URL before credential injection
//  4. handleProxy - actual request handling
//
// This order ensures:
//   - All requests are logged (including rejections and panics)
//   - Panics are caught and logged properly
//   - URL validation happens before credential injection
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Register operational endpoints (no allow list validation needed)
	mux.HandleFunc("GET /_ops/health", s.handleHealth)
	mux.HandleFunc("GET /_ops/version", s.handleVersion)

	// Register proxy endpoint with allow list validation
	// Security: AllowList is REQUIRED per Design Spec Section 5.3
	proxyHandler := WithAllowListMiddleware(
		s.config.AllowList,
		s.config.HeaderPrefix,
		http.HandlerFunc(s.handleProxy),
	)
	mux.Handle("/proxy", proxyHandler)

	// Apply global middleware stack (logging, panic recovery)
	handler := s.withMiddleware(mux)

	return handler
}

// Start starts the HTTP server and blocks until it's shut down.
// If TLS is enabled (Mode A), the server starts with mTLS requiring client certificates.
// If TLS is disabled (basic Mode B), the server starts as plain HTTP.
func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.Handler(),
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	if s.config.TLS.Enabled {
		return s.startTLS(srv)
	}

	return s.startHTTP(srv)
}

// startHTTP starts the server in plain HTTP mode (basic Mode B).
func (s *Server) startHTTP(srv *http.Server) error {
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

	return srv.ListenAndServe()
}

// startTLS starts the server with mTLS enabled (Mode A).
func (s *Server) startTLS(srv *http.Server) error {
	tlsCfg := s.config.TLS

	// Load certificates from files
	caCert, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return fmt.Errorf("reading CA certificate %s: %w", tlsCfg.CAFile, err)
	}

	serverCert, err := os.ReadFile(tlsCfg.CertFile)
	if err != nil {
		return fmt.Errorf("reading server certificate %s: %w", tlsCfg.CertFile, err)
	}

	serverKey, err := os.ReadFile(tlsCfg.KeyFile)
	if err != nil {
		return fmt.Errorf("reading server key %s: %w", tlsCfg.KeyFile, err)
	}

	// Create TLS config with mTLS
	tlsConfig, err := NewTLSConfig(caCert, serverCert, serverKey)
	if err != nil {
		return fmt.Errorf("creating TLS config: %w", err)
	}

	srv.TLSConfig = tlsConfig

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
		"cert_file", tlsCfg.CertFile,
		"ca_file", tlsCfg.CAFile,
	)

	// Use empty strings for cert/key files since they're already loaded into TLSConfig
	return srv.ListenAndServeTLS("", "")
}

// withMiddleware wraps the handler with the global middleware stack.
// See Handler() for complete middleware ordering documentation.
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply middleware: outermost runs first
	// Order: RequestLogging -> PanicRecovery -> handler
	handler = WithPanicRecovery(handler)
	handler = WithRequestLogging(s.config.TraceHeader, handler)
	return handler
}

// WithAllowListMiddleware wraps a handler with allow list validation.
// Security: AllowList is REQUIRED per Design Spec Section 5.3.
// Empty AllowList will deny all requests (secure default).
func WithAllowListMiddleware(allowList map[string][]string, headerPrefix string, next http.Handler) http.Handler {
	return router.NewAllowListMiddleware(allowList, headerPrefix, next)
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
	traceID := s.extractTraceID(r)

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

// extractTraceID returns the trace ID from the request header or generates a new one.
// Uses the configured TraceHeader from proxy.Config.
func (s *Server) extractTraceID(r *http.Request) string {
	if traceID := r.Header.Get(s.config.TraceHeader); traceID != "" {
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

	cred, err := s.config.Plugin.GetCredentials(ctx, *txCtx, r)
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
	proxy := s.createReverseProxy(target, traceID, txCtx)
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
// The response modification chain runs in this order:
//  1. Plugin.ModifyResponse (if plugin exists) - returns *ResponseAction for Core instructions
//  2. Strip sensitive headers (Credential Reflection Protection) - always runs
//  3. Core error normalization - runs unless plugin returned ResponseAction{SkipErrorNormalization: true}
func (s *Server) createReverseProxy(target *url.URL, traceID string, txCtx *sdk.TransactionContext) *httputil.ReverseProxy {
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

	// Response modification chain: Plugin → Strip Headers → Error Normalization
	proxy.ModifyResponse = func(resp *http.Response) error {
		var action *sdk.ResponseAction

		// Step 1: Plugin's ModifyResponse runs FIRST (allows Distributor customization)
		if s.config.Plugin != nil {
			// Use the request's context with a timeout for plugin execution
			ctx, cancel := context.WithTimeout(resp.Request.Context(), s.config.PluginTimeout)
			defer cancel()

			var err error
			action, err = s.config.Plugin.ModifyResponse(ctx, *txCtx, resp)
			if err != nil {
				slog.Warn("plugin ModifyResponse error",
					"trace_id", traceID,
					"error", err,
				)
				// Continue with response processing even if plugin fails
				// action remains nil, so Core applies safety net
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

// generateTraceID generates a unique trace ID for request tracking.
func generateTraceID() string {
	// Use a simple timestamp-based ID for PoC
	// TODO: Use UUID in production
	return fmt.Sprintf("trace-%d", time.Now().UnixNano())
}
