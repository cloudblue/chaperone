// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/security"
	"github.com/cloudblue/chaperone/internal/telemetry"
)

// defaultForwardTimeout is applied when ForwardTargetConfig.Timeout is zero
// or negative. It bounds the response-header wait so that a hung forward
// target cannot pin a Connect goroutine indefinitely.
const defaultForwardTimeout = 30 * time.Second

// ForwardProxy wraps a httputil.ReverseProxy for a single forward target.
// One instance is built at startup per named target in config.ForwardTargets
// and reused across requests.
//
// Compared with the vendor proxy path (server.createReverseProxy), the
// forward path is intentionally stripped down:
//
//   - Inbound Authorization is dropped so Connect's auth posture cannot
//     leak to the forward target.
//   - A static bearer token is injected when auth.type == "bearer".
//   - X-Connect-* context headers are forwarded verbatim (the forward
//     target — typically the customer's own system — needs them).
//   - The plugin's ResponseModifier is NOT invoked, and Core error
//     normalization is NOT applied; the forward target's status code and
//     body pass through unmodified.
//   - Sensitive response headers (the static default set from
//     internal/security) are stripped as a defense-in-depth measure
//     against credential reflection.
type ForwardProxy struct {
	name   string
	target *url.URL
	auth   config.ForwardTargetAuthConfig
	proxy  *httputil.ReverseProxy
}

// NewForwardProxy builds a forward proxy for the given target configuration.
// The returned handler is safe for concurrent use and is intended to be
// cached at startup and reused across requests.
func NewForwardProxy(name string, cfg config.ForwardTargetConfig) (*ForwardProxy, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("forward_target[%q]: parse url: %w", name, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("forward_target[%q]: invalid url %q", name, cfg.URL)
	}

	fp := &ForwardProxy{name: name, target: u, auth: cfg.Auth}
	fp.proxy = &httputil.ReverseProxy{
		Director:       fp.director,
		ModifyResponse: fp.modifyResponse,
		ErrorHandler:   fp.errorHandler,
		Transport:      newForwardTransport(cfg.Timeout),
	}
	return fp, nil
}

// ServeHTTP forwards the request to the configured target.
//
// Observability:
//   - chaperone_forward_target_duration_seconds{target} is observed for every
//     request that enters ServeHTTP, including those that fail at the
//     transport layer. The deferred observation captures end-to-end time
//     (entry → response written / error handled) so dashboards reflect the
//     real wall-clock cost of forwarding even when the target is unreachable.
//   - chaperone_forward_target_errors_total{target,kind} is incremented by
//     errorHandler for infrastructure failures. 5xx responses returned by the
//     target are NOT counted here — they are target responses, not Chaperone
//     errors.
func (fp *ForwardProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		telemetry.ForwardTargetDuration.WithLabelValues(fp.name).Observe(time.Since(start).Seconds())
	}()
	fp.proxy.ServeHTTP(w, r)
}

// director rewrites the outbound request: target host/scheme, path joining,
// inbound-Authorization stripping, and (optional) bearer-token injection.
//
// SECURITY: The bearer token must not be logged anywhere in this function.
// The static sensitive_headers redaction in the request logger already
// covers Authorization; do not emit log lines that include req.Header here.
func (fp *ForwardProxy) director(req *http.Request) {
	req.URL.Scheme = fp.target.Scheme
	req.URL.Host = fp.target.Host
	req.URL.Path = singleJoiningSlash(fp.target.Path, req.URL.Path)
	if fp.target.RawQuery != "" && req.URL.RawQuery != "" {
		req.URL.RawQuery = fp.target.RawQuery + "&" + req.URL.RawQuery
	} else {
		req.URL.RawQuery = fp.target.RawQuery + req.URL.RawQuery
	}
	req.Host = fp.target.Host

	// Strip inbound Authorization to avoid leaking Connect's auth posture
	// to the forward target. This happens regardless of fp.auth.Type — the
	// forward target should only ever see credentials we choose to inject.
	req.Header.Del("Authorization")

	if fp.auth.Type == config.ForwardAuthBearer {
		req.Header.Set("Authorization", "Bearer "+fp.auth.Token)
	}

	// X-Connect-* headers are intentionally preserved — the forward target
	// (typically the customer's own system) needs the routing/context.
	// Connect-Request-ID is likewise preserved by default; no action needed.
}

// modifyResponse strips the static set of sensitive headers from the forward
// target's response. This is defense-in-depth: even if the forward target
// reflects an Authorization header back, it never reaches Connect.
//
// NOTE: Unlike the vendor proxy path, we do NOT invoke the plugin's
// ResponseModifier and we do NOT apply Core error normalization. Forward
// targets are by definition outside the plugin contract; their responses
// pass through verbatim modulo the credential-reflection sanitization.
func (fp *ForwardProxy) modifyResponse(resp *http.Response) error {
	security.StripSensitiveResponseHeaders(resp.Header)
	return nil
}

// errorHandler returns 502 Bad Gateway when the forward target is
// unreachable. The error itself is not surfaced to the caller to avoid
// leaking internal infrastructure details (host names, ports, etc.).
//
// SECURITY: Do not include the error string in the response body. Internal
// observability of the cause belongs in logs, not in the wire response.
func (fp *ForwardProxy) errorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	telemetry.ForwardTargetErrors.WithLabelValues(fp.name, classifyForwardError(err)).Inc()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_, _ = w.Write([]byte(`{"error":"forward target unavailable"}`))
}

// classifyForwardError maps a transport-level error from httputil.ReverseProxy
// into a small set of well-known kinds for the forward_target_errors_total
// metric. Go's net/http error surface is intentionally fuzzy, so we classify
// what we can confidently identify and fall back to "other" for the rest.
//
// Order matters: timeouts and TLS failures can both surface as *net.OpError
// with a wrapped underlying error, so we check the most specific signals
// first.
func classifyForwardError(err error) string {
	if err == nil {
		return "other"
	}

	// Timeout: context deadline, response-header timeout, or any error that
	// implements the net.Error timeout contract.
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	// TLS: handshake / record errors. The TLS package's error types aren't all
	// exported, so we fall back to a substring check against the well-known
	// "tls:" prefix used by crypto/tls error messages.
	var recordHeaderErr tls.RecordHeaderError
	if errors.As(err, &recordHeaderErr) {
		return "tls"
	}
	if msg := err.Error(); strings.Contains(msg, "tls:") || strings.Contains(msg, "x509:") {
		return "tls"
	}

	// Connection: DNS failure, refused, reset, EOF mid-handshake, etc.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "connection"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return "connection"
	}

	return "other"
}

// newForwardTransport returns the per-target transport. Timeouts apply to
// the response-header wait (i.e., how long we are willing to block before
// the target writes status); body streaming is not bounded here, which
// matches the streaming semantics of httputil.ReverseProxy.
//
// The transport is built by cloning http.DefaultTransport so we inherit
// HTTP/2 negotiation (ForceAttemptHTTP2), connection pooling (MaxIdleConns,
// IdleConnTimeout), the dialer defaults (DialContext) and the various
// stdlib-tuned timeouts (TLSHandshakeTimeout, ExpectContinueTimeout).
// We override only ResponseHeaderTimeout and TLSClientConfig.
func newForwardTransport(timeout time.Duration) *http.Transport {
	if timeout <= 0 {
		timeout = defaultForwardTimeout
	}
	// http.DefaultTransport is documented as *http.Transport. The comma-ok
	// form keeps the linter happy without losing the invariant.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// Stdlib invariant broken — fall back to a fresh transport rather
		// than panicking. This branch is effectively unreachable.
		base = &http.Transport{}
	}
	t := base.Clone()
	t.ResponseHeaderTimeout = timeout
	t.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS13,
		// Explicit: we always verify forward-target certificates. The
		// linter flags this field because the zero value is also false,
		// but being explicit guards against future refactors silently
		// flipping the default.
		InsecureSkipVerify: false, //nolint:gosec // explicit: always verify
	}
	return t
}

// singleJoiningSlash mirrors httputil.singleJoiningSlash (unexported in
// net/http/httputil). Given a target-URL path and a request path, it joins
// them with exactly one separator slash. Used by director to rewrite
// req.URL.Path so that target paths with or without trailing slashes — and
// request paths with or without leading slashes — concatenate cleanly.
func singleJoiningSlash(a, b string) string {
	aSlash := a != "" && a[len(a)-1] == '/'
	bSlash := b != "" && b[0] == '/'
	switch {
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	}
	return a + b
}
