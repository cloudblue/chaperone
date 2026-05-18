// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/telemetry"
)

// errCtxDeadlineExceeded is captured once so test tables can reference it
// without re-importing context in every helper.
var errCtxDeadlineExceeded = context.DeadlineExceeded

func newTestTarget(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestForwardProxy_PassesXConnectHeaders(t *testing.T) {
	var seen http.Header
	target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	})
	defer target.Close()

	h, err := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})
	if err != nil {
		t.Fatalf("NewForwardProxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1/foo")
	req.Header.Set("X-Connect-Vendor-ID", "vendor-a")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got := seen.Get("X-Connect-Target-URL"); got != "https://api.vendor.com/v1/foo" {
		t.Errorf("X-Connect-Target-URL forwarded = %q", got)
	}
	if got := seen.Get("X-Connect-Vendor-ID"); got != "vendor-a" {
		t.Errorf("X-Connect-Vendor-ID forwarded = %q", got)
	}
}

func TestForwardProxy_StripsInboundAuthorization_AddsBearer(t *testing.T) {
	var seen http.Header
	target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) { seen = r.Header.Clone() })
	defer target.Close()

	h, err := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthBearer, Token: "secret-xyz"},
	})
	if err != nil {
		t.Fatalf("NewForwardProxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("Authorization", "Bearer connect-original")
	h.ServeHTTP(httptest.NewRecorder(), req)

	auth := seen.Get("Authorization")
	if auth != "Bearer secret-xyz" {
		t.Errorf("forwarded Authorization = %q, want %q", auth, "Bearer secret-xyz")
	}
	if strings.Contains(auth, "connect-original") {
		t.Errorf("inbound Authorization leaked: %q", auth)
	}
}

func TestForwardProxy_SanitizesReflectedSensitiveResponseHeaders(t *testing.T) {
	target := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Authorization", "Bearer reflected-secret")
		w.Header().Set("Set-Cookie", "session=abc")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Result().Header.Get("Authorization") != "" {
		t.Errorf("reflected Authorization not stripped")
	}
	if rec.Result().Header.Get("Set-Cookie") != "" {
		t.Errorf("reflected Set-Cookie not stripped")
	}
}

func TestForwardProxy_BearerToken_NotInLogOutput(t *testing.T) {
	target := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer target.Close()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthBearer, Token: "secret-not-in-logs"},
	})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if strings.Contains(buf.String(), "secret-not-in-logs") {
		t.Errorf("bearer token leaked into log output: %s", buf.String())
	}
}

func TestForwardProxy_HonorsTimeout(t *testing.T) {
	target := newTestTarget(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:     target.URL,
		Timeout: 50 * time.Millisecond,
		Auth:    config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Code == http.StatusOK {
		t.Errorf("expected non-200 due to timeout, got 200")
	}
}

// TestForwardProxy_PreservesAllXConnectHeaders confirms multiple X-Connect-*
// headers are forwarded verbatim — they are part of the customer's context
// (the forward target's system needs them) and must not be stripped.
func TestForwardProxy_PreservesAllXConnectHeaders(t *testing.T) {
	var seen http.Header
	target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	headers := map[string]string{
		"X-Connect-Target-URL":      "https://api.vendor.com/v1/foo",
		"X-Connect-Vendor-ID":       "vendor-a",
		"X-Connect-Marketplace-ID":  "marketplace-1",
		"X-Connect-Product-ID":      "PRD-001",
		"X-Connect-Subscription-ID": "AS-1234",
		"X-Connect-Context-Data":    "eyJrIjoidiJ9",
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)

	for k, want := range headers {
		if got := seen.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
}

// TestForwardProxy_AuthNoneStripsInboundAuthorization verifies that even
// when no bearer is configured (auth.type=none), any inbound Authorization
// header is still stripped before forwarding. This prevents Connect's auth
// posture from leaking to the forward target.
func TestForwardProxy_AuthNoneStripsInboundAuthorization(t *testing.T) {
	var seen http.Header
	target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("Authorization", "Bearer connect-original")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got := seen.Get("Authorization"); got != "" {
		t.Errorf("auth.type=none: inbound Authorization not stripped, got %q", got)
	}
}

// TestForwardProxy_AuthNoneNoAuthorizationAdded confirms no Authorization
// header is added when auth.type=none and the inbound request has none.
func TestForwardProxy_AuthNoneNoAuthorizationAdded(t *testing.T) {
	var seen http.Header
	target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got := seen.Get("Authorization"); got != "" {
		t.Errorf("auth.type=none: Authorization should not be set, got %q", got)
	}
}

// TestForwardProxy_BearerTokenWithWhitespace verifies the token is
// forwarded verbatim (we do NOT trim/normalize whitespace — operators may
// have intentional content there, and any malformedness is their concern).
func TestForwardProxy_BearerTokenWithWhitespace(t *testing.T) {
	var seen http.Header
	target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	})
	defer target.Close()

	token := "abc def ghi"
	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthBearer, Token: token},
	})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proxy", nil))

	want := "Bearer " + token
	if got := seen.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

// TestForwardProxy_PathJoining covers singleJoiningSlash correctness across
// the four corner cases of trailing/leading slashes between the target URL
// path and the inbound request path.
func TestForwardProxy_PathJoining(t *testing.T) {
	tests := []struct {
		name        string
		targetPath  string
		requestPath string
		wantPath    string
	}{
		{"both empty", "", "", "/"},
		{"target trailing, req leading", "/api/", "/v1/foo", "/api/v1/foo"},
		{"target no trailing, req no leading", "/api", "v1/foo", "/api/v1/foo"},
		{"target trailing, req no leading", "/api/", "v1/foo", "/api/v1/foo"},
		{"target no trailing, req leading", "/api", "/v1/foo", "/api/v1/foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seenPath string
			target := newTestTarget(t, func(_ http.ResponseWriter, r *http.Request) {
				seenPath = r.URL.Path
			})
			defer target.Close()

			h, err := NewForwardProxy("t", config.ForwardTargetConfig{
				URL:  target.URL + tt.targetPath,
				Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
			})
			if err != nil {
				t.Fatalf("NewForwardProxy: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			req.URL.Path = tt.requestPath
			h.ServeHTTP(httptest.NewRecorder(), req)

			if seenPath != tt.wantPath {
				t.Errorf("forwarded path = %q, want %q", seenPath, tt.wantPath)
			}
		})
	}
}

// TestForwardProxy_Passes500Status verifies that a 5xx response from the
// forward target is passed through verbatim. The forward path explicitly
// does NOT perform error normalization (that is reserved for the vendor
// proxy path where ResponseModifier may opt out).
func TestForwardProxy_Passes500Status(t *testing.T) {
	target := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body := rec.Body.String(); body != "boom" {
		t.Errorf("body = %q, want %q", body, "boom")
	}
}

// TestForwardProxy_TargetUnreachable_Returns502 verifies the ErrorHandler
// returns 502 Bad Gateway when the target refuses the connection.
func TestForwardProxy_TargetUnreachable_Returns502(t *testing.T) {
	// Start and immediately close a server to obtain a guaranteed-closed
	// port. The returned URL is for a now-defunct listener; dials will fail.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  url,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

// =============================================================================
// Task 8: forward-target metrics
// =============================================================================
//
// These tests assert on the global telemetry.Forward* metrics. They MUST NOT
// use t.Parallel() because the metrics are registered with the default
// Prometheus registry. Test isolation is via telemetry.ResetMetrics().

func TestMetrics_ForwardTarget_DurationHistogram_Records(t *testing.T) {
	telemetry.ResetMetrics(t)

	target := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer target.Close()

	h, err := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthBearer, Token: "secret"},
	})
	if err != nil {
		t.Fatalf("NewForwardProxy: %v", err)
	}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proxy", nil))

	// SampleCount: total observations under the {target=company-b} histogram.
	if got := testutil.CollectAndCount(telemetry.ForwardTargetDuration); got == 0 {
		t.Error("expected ForwardTargetDuration to have at least one observation")
	}
	// No infrastructure errors expected on a successful round-trip.
	if got := testutil.ToFloat64(telemetry.ForwardTargetErrors.WithLabelValues("company-b", "connection")); got != 0 {
		t.Errorf("connection errors counter = %v, want 0", got)
	}
}

func TestMetrics_ForwardTarget_Errors_IncrementsByKind_Connection(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Start and immediately close a server to obtain a guaranteed-closed port.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  url,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	got := testutil.ToFloat64(telemetry.ForwardTargetErrors.WithLabelValues("company-b", "connection"))
	if got != 1 {
		t.Errorf("forward_target_errors_total{target=company-b,kind=connection} = %v, want 1", got)
	}
	// Duration is still observed: the deferred Observe in ServeHTTP runs
	// regardless of whether the request succeeded or hit errorHandler. This
	// is intentional — operators want end-to-end latency including failures.
	if got := testutil.CollectAndCount(telemetry.ForwardTargetDuration); got == 0 {
		t.Error("expected ForwardTargetDuration to record even on error path")
	}
}

func TestMetrics_ForwardTarget_Errors_IncrementsByKind_Timeout(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Target that never responds within the test budget.
	target := newTestTarget(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:     target.URL,
		Timeout: 25 * time.Millisecond,
		Auth:    config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	got := testutil.ToFloat64(telemetry.ForwardTargetErrors.WithLabelValues("company-b", "timeout"))
	if got != 1 {
		t.Errorf("forward_target_errors_total{target=company-b,kind=timeout} = %v, want 1", got)
	}
}

// TestMetrics_ForwardTarget_500Response_NoErrorCounter verifies that a 5xx
// response from the target is treated as a target response (not a Chaperone
// infrastructure error) — the duration histogram observes but the errors
// counter does NOT increment.
func TestMetrics_ForwardTarget_500Response_NoErrorCounter(t *testing.T) {
	telemetry.ResetMetrics(t)

	target := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	})
	defer target.Close()

	h, _ := NewForwardProxy("company-b", config.ForwardTargetConfig{
		URL:  target.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	// Duration: observed.
	if got := testutil.CollectAndCount(telemetry.ForwardTargetDuration); got == 0 {
		t.Error("expected duration histogram to record on 5xx response")
	}
	// Errors: NOT incremented (any kind).
	for _, kind := range []string{"connection", "timeout", "tls", "other"} {
		if got := testutil.ToFloat64(telemetry.ForwardTargetErrors.WithLabelValues("company-b", kind)); got != 0 {
			t.Errorf("5xx response must not increment errors_total{kind=%s}, got %v", kind, got)
		}
	}
}

// TestMetrics_ForwardTarget_MultipleTargets verifies each target gets its own
// histogram cell (no cross-aliasing).
func TestMetrics_ForwardTarget_MultipleTargets(t *testing.T) {
	telemetry.ResetMetrics(t)

	targetA := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer targetA.Close()
	targetB := newTestTarget(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer targetB.Close()

	hA, _ := NewForwardProxy("a", config.ForwardTargetConfig{
		URL:  targetA.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})
	hB, _ := NewForwardProxy("b", config.ForwardTargetConfig{
		URL:  targetB.URL,
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})

	hA.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proxy", nil))
	hB.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proxy", nil))
	hB.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/proxy", nil))

	// Each named target should have its own histogram cell.
	// CollectAndCount counts the number of distinct label sets — two here.
	count := testutil.CollectAndCount(telemetry.ForwardTargetDuration)
	if count < 2 {
		t.Errorf("expected at least 2 distinct duration histogram cells, got %d", count)
	}
}

// TestClassifyForwardError_Matrix exercises the error classifier directly to
// pin the kind labels we surface in the metric.
func TestClassifyForwardError_Matrix(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "other"},
		{"deadline exceeded", errCtxDeadlineExceeded, "timeout"},
		{"net timeout", testNetTimeoutError{}, "timeout"},
		{"dns failure", &net.DNSError{Err: "no such host", Name: "nope.example"}, "connection"},
		{"op error refused", &net.OpError{Op: "dial", Net: "tcp", Err: stringError("connection refused")}, "connection"},
		{"tls substring", stringError("tls: handshake failure"), "tls"},
		{"x509 substring", stringError("x509: certificate signed by unknown authority"), "tls"},
		{"plain", stringError("something weird"), "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyForwardError(tt.err)
			if got != tt.want {
				t.Errorf("classifyForwardError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestForwardProxy_InvalidTargetURL_ReturnsError(t *testing.T) {
	_, err := NewForwardProxy("bad", config.ForwardTargetConfig{
		URL:  "://not-a-url",
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})
	if err == nil {
		t.Fatal("NewForwardProxy with invalid URL: expected error, got nil")
	}
}

// -----------------------------------------------------------------------------
// Test doubles for classifyForwardError matrix.
// -----------------------------------------------------------------------------

// stringError is a trivial error with a controllable message; used to
// exercise the substring-based TLS/x509 classification paths.
type stringError string

func (e stringError) Error() string { return string(e) }

// testNetTimeoutError satisfies net.Error with Timeout()=true so the classifier
// can identify it without depending on a real network call.
type testNetTimeoutError struct{}

func (testNetTimeoutError) Error() string   { return "i/o timeout" }
func (testNetTimeoutError) Timeout() bool   { return true }
func (testNetTimeoutError) Temporary() bool { return true }
