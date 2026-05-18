// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/internal/config"
)

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

func TestForwardProxy_InvalidTargetURL_ReturnsError(t *testing.T) {
	_, err := NewForwardProxy("bad", config.ForwardTargetConfig{
		URL:  "://not-a-url",
		Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone},
	})
	if err == nil {
		t.Fatal("NewForwardProxy with invalid URL: expected error, got nil")
	}
}
