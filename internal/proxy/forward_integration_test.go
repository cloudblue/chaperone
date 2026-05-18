// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/sdk"
)

// =============================================================================
// Task 14: End-to-end forwarding with bearer auth.
//
// These tests drive a full request through the proxy.Server handler stack
// (AllowList middleware → handleProxy → router branch → ForwardProxy) and
// assert the forwarding contract:
//
//   - The fake "Company B" target receives the request with X-Connect-*
//     context headers intact, the configured bearer token, and NO trace of
//     the inbound Authorization.
//   - The fake target's response body, status, and (non-sensitive) headers
//     reach the client recorder.
//   - Sensitive headers reflected by the target are stripped before reaching
//     the client (defense-in-depth).
//   - No credentials are ever injected via the plugin: the forward path is
//     mutually exclusive with the credential-injection path.
//
// The test uses an inline sdk.RequestRouter implementation (forwardRouter)
// rather than contrib.Mux because internal/proxy lives in the Core module
// and contrib lives in a separate Go module (importing it from here would
// add a stale require to the Core go.mod). The Mux's RouteRequest →
// RouteAction translation is exercised by contrib's own tests; this test
// targets the Core forwarding pipeline.
// =============================================================================

// forwardRouter is a minimal sdk.Plugin + sdk.RequestRouter that routes
// requests with the configured VendorID to a named forward target. Any
// invocation of GetCredentials is recorded so the test can assert the
// credential path was never taken.
type forwardRouter struct {
	matchVendorID string
	forwardTo     string
	credCallCount atomic.Int32
}

func (r *forwardRouter) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	r.credCallCount.Add(1)
	return nil, nil
}

func (r *forwardRouter) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (r *forwardRouter) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

func (r *forwardRouter) RouteRequest(_ context.Context, tx sdk.TransactionContext, _ *http.Request) (*sdk.RouteAction, error) {
	if tx.VendorID == r.matchVendorID {
		return &sdk.RouteAction{ForwardTo: r.forwardTo}, nil
	}
	return nil, nil
}

var _ sdk.Plugin = (*forwardRouter)(nil)
var _ sdk.RequestRouter = (*forwardRouter)(nil)

// forwardIntegrationConfig builds the proxy.Config used by these tests with
// an allow-list that permits the X-Connect-Target-URL host (api.vendor.com)
// AND the actual fake-target host. The X-Connect-Target-URL value is
// validated by the allow-list middleware even though the router short-
// circuits the request to the forward target.
func forwardIntegrationConfig(t *testing.T, plugin sdk.Plugin, forwardURL string) proxy.Config {
	t.Helper()
	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.AllowList = map[string][]string{
		"api.vendor.com":                  {"/**"},
		mustTargetHostPort(t, forwardURL): {"/**"},
	}
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {
			URL: forwardURL,
			Auth: config.ForwardTargetAuthConfig{
				Type:  config.ForwardAuthBearer,
				Token: "expected-token",
			},
		},
	}
	return cfg
}

// makeProxyRequest constructs a /proxy request configured for the forward
// scenario: VendorID=vendor-a triggers the router, X-Connect-Target-URL is
// the vendor URL (never dialed — the router forwards before that point),
// and an inbound Authorization header is set so we can verify it does NOT
// leak to the forward target.
func makeProxyRequest(body string) *http.Request {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/proxy", bodyReader)
	req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1/foo")
	req.Header.Set("X-Connect-Vendor-ID", "vendor-a")
	req.Header.Set("X-Connect-Marketplace-ID", "marketplace-1")
	req.Header.Set("Authorization", "Bearer connect-original")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestForwardIntegration_BearerAuth_FullFlow exercises the end-to-end
// forwarding path with bearer auth across a matrix of behavioral assertions.
// Each subtest spins up its own fake target with the response semantics
// relevant to that scenario.
func TestForwardIntegration_BearerAuth_FullFlow(t *testing.T) {
	t.Run("headers, body, status, and bearer all propagate", func(t *testing.T) {
		var (
			callCount       atomic.Int32
			seenHeaders     http.Header
			seenBody        string
			seenTargetURL   string
			seenVendorID    string
			seenAuth        string
			seenContentType string
			seenTraceID     string
		)

		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount.Add(1)
			seenHeaders = r.Header.Clone()
			seenTargetURL = r.Header.Get("X-Connect-Target-URL")
			seenVendorID = r.Header.Get("X-Connect-Vendor-ID")
			seenAuth = r.Header.Get("Authorization")
			seenContentType = r.Header.Get("Content-Type")
			seenTraceID = r.Header.Get("Connect-Request-ID")
			body, _ := io.ReadAll(r.Body)
			seenBody = string(body)

			w.Header().Set("X-Custom-Reply", "vendor-says-hi")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"vendor":"reply"}`)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		requestBody := `{"action":"create","name":"test"}`
		const traceID = "test-trace-1234"
		rec := httptest.NewRecorder()
		req := makeProxyRequest(requestBody)
		req.Header.Set("Connect-Request-ID", traceID)
		srv.Handler().ServeHTTP(rec, req)

		if got := callCount.Load(); got != 1 {
			t.Fatalf("fake target call count = %d, want 1", got)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200. body=%s", rec.Code, rec.Body.String())
		}

		// Inbound context headers propagated.
		if seenTargetURL != "https://api.vendor.com/v1/foo" {
			t.Errorf("forwarded X-Connect-Target-URL = %q, want %q", seenTargetURL, "https://api.vendor.com/v1/foo")
		}
		if seenVendorID != "vendor-a" {
			t.Errorf("forwarded X-Connect-Vendor-ID = %q, want %q", seenVendorID, "vendor-a")
		}
		if seenContentType != "application/json" {
			t.Errorf("forwarded Content-Type = %q, want %q", seenContentType, "application/json")
		}
		// Connect-Request-ID propagates to the forward target for trace
		// continuity (Design Spec §8.3). TraceIDMiddleware preserves valid
		// inbound IDs verbatim, so the target sees the exact ID we set.
		if seenTraceID != traceID {
			t.Errorf("forwarded Connect-Request-ID = %q, want %q", seenTraceID, traceID)
		}

		// Outbound Authorization is the configured bearer; inbound is stripped.
		if seenAuth != "Bearer expected-token" {
			t.Errorf("forwarded Authorization = %q, want %q", seenAuth, "Bearer expected-token")
		}
		if strings.Contains(seenAuth, "connect-original") {
			t.Errorf("inbound Authorization leaked to forward target: %q", seenAuth)
		}

		// Inbound Authorization MUST NOT have been propagated under any name.
		for _, v := range seenHeaders.Values("Authorization") {
			if strings.Contains(v, "connect-original") {
				t.Errorf("inbound Authorization leaked (full header list): %q", v)
			}
		}

		// Request body propagates verbatim.
		if seenBody != requestBody {
			t.Errorf("forwarded body = %q, want %q", seenBody, requestBody)
		}

		// Response from target reaches the client.
		if got := rec.Header().Get("X-Custom-Reply"); got != "vendor-says-hi" {
			t.Errorf("client X-Custom-Reply = %q, want %q", got, "vendor-says-hi")
		}
		if got := rec.Body.String(); got != `{"vendor":"reply"}` {
			t.Errorf("client body = %q, want %q", got, `{"vendor":"reply"}`)
		}

		// The plugin's credential path must NOT have been taken.
		if got := plugin.credCallCount.Load(); got != 0 {
			t.Errorf("GetCredentials was called %d time(s) on the forward path; want 0", got)
		}
	})

	t.Run("target status 418 propagates to client", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if rec.Code != http.StatusTeapot {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
		}
		if got := plugin.credCallCount.Load(); got != 0 {
			t.Errorf("GetCredentials was called on the forward path; want 0")
		}
	})

	t.Run("response body propagates verbatim", func(t *testing.T) {
		const wantBody = "hello world"
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, wantBody)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if got := rec.Body.String(); got != wantBody {
			t.Errorf("client body = %q, want %q", got, wantBody)
		}
	})

	t.Run("custom response header reaches client", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Request-Trace", "from-target-42")
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if got := rec.Header().Get("X-Request-Trace"); got != "from-target-42" {
			t.Errorf("client X-Request-Trace = %q, want %q", got, "from-target-42")
		}
	})

	t.Run("bearer token equals the configured value exactly", func(t *testing.T) {
		var seenAuth string
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if seenAuth != "Bearer expected-token" {
			t.Errorf("Authorization at target = %q, want %q", seenAuth, "Bearer expected-token")
		}
	})

	t.Run("inbound Authorization not reflected at target", func(t *testing.T) {
		var seenAuth string
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if strings.Contains(seenAuth, "connect-original") {
			t.Errorf("inbound Authorization leaked to target: %q", seenAuth)
		}
	})

	t.Run("sensitive response headers reflected by target are stripped", func(t *testing.T) {
		// Defense-in-depth: even if the target reflects Authorization or
		// Set-Cookie back, ForwardProxy.modifyResponse strips them before
		// they reach the client.
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Authorization", "Bearer leaked")
			w.Header().Set("Set-Cookie", "session=should-not-leak")
			w.Header().Set("X-Safe", "ok-to-see")
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if got := rec.Header().Get("Authorization"); got != "" {
			t.Errorf("reflected Authorization not stripped from client response: %q", got)
		}
		if got := rec.Header().Get("Set-Cookie"); got != "" {
			t.Errorf("reflected Set-Cookie not stripped from client response: %q", got)
		}
		// Non-sensitive header should still propagate so the strip is targeted.
		if got := rec.Header().Get("X-Safe"); got != "ok-to-see" {
			t.Errorf("non-sensitive header X-Safe = %q, want %q", got, "ok-to-see")
		}
	})

	t.Run("target unreachable returns 502 with sanitized JSON body", func(t *testing.T) {
		// Start and immediately close a server to obtain a guaranteed-closed
		// port. The returned URL is for a now-defunct listener; dials will
		// fail at the transport layer and hit ForwardProxy.errorHandler.
		closed := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		closedURL := closed.URL
		closed.Close()

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, closedURL)
		srv := mustNewServer(t, cfg)

		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, makeProxyRequest(""))

		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want %d. body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
		}
		const wantBody = `{"error":"forward target unavailable"}`
		if got := strings.TrimSpace(rec.Body.String()); got != wantBody {
			t.Errorf("body = %q, want %q", got, wantBody)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}
		// Still no credentials path even on transport failure.
		if got := plugin.credCallCount.Load(); got != 0 {
			t.Errorf("GetCredentials was called %d time(s) on the forward path; want 0", got)
		}
	})

	t.Run("forward path bypasses credential injection entirely", func(t *testing.T) {
		// Sanity check across two requests: the forward route consistently
		// short-circuits before reaching the credential provider.
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)

		plugin := &forwardRouter{matchVendorID: "vendor-a", forwardTo: "company-b"}
		cfg := forwardIntegrationConfig(t, plugin, target.URL)
		srv := mustNewServer(t, cfg)

		for i := 0; i < 3; i++ {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, makeProxyRequest(""))
			if rec.Code != http.StatusOK {
				t.Fatalf("iteration %d: status = %d, want 200. body=%s", i, rec.Code, rec.Body.String())
			}
		}
		if got := plugin.credCallCount.Load(); got != 0 {
			t.Errorf("GetCredentials was called %d time(s) across 3 forwarded requests; want 0", got)
		}
	})
}
