// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cloudblue/chaperone/internal/config"
	chaperoneCtx "github.com/cloudblue/chaperone/internal/context"
	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/internal/telemetry"
	"github.com/cloudblue/chaperone/sdk"
)

// =============================================================================
// Task 15: End-to-end migration scenario.
//
// A single Chaperone server is configured with one plugin that ALSO implements
// sdk.RequestRouter. The router inspects tx.Data["ResellerId"] and forwards
// any reseller matching the glob pattern "migrated-*" to a fake "Company B"
// target. Other resellers (or requests missing ResellerId) fall through to
// credential injection, which adds a bearer token and forwards to the fake
// vendor target.
//
// This test exercises the full request flow across both branches in the same
// server, verifying:
//
//   - Routing decisions are driven by tx.Data, which arrives via the
//     X-Connect-Context-Data header (Base64-encoded JSON).
//   - The forward path hits Company B and bypasses GetCredentials entirely.
//   - The credentials path injects Authorization, strips X-Connect-* headers,
//     and reaches the vendor target.
//   - Per-decision metrics (chaperone_route_decisions_total) increment
//     correctly under both branches.
//   - Missing/empty Data falls through to the credentials path.
// =============================================================================

// migrationPlugin is an inline sdk.Plugin + sdk.RequestRouter used by the
// migration scenario. RouteRequest implements a simple glob match against
// tx.Data["ResellerId"]; the only supported pattern is a trailing "*" prefix
// match (sufficient for "migrated-*"). GetCredentials returns a fixed bearer
// credential and records its call count so the test can assert it never runs
// on the forward path.
type migrationPlugin struct {
	pattern             string // e.g. "migrated-*"
	forwardTo           string // forward target name
	credToken           string // bearer token returned from GetCredentials
	getCredentialsCount atomic.Int32
}

func (p *migrationPlugin) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	p.getCredentialsCount.Add(1)
	return &sdk.Credential{
		Headers: map[string]string{
			"Authorization": "Bearer " + p.credToken,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

func (p *migrationPlugin) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (p *migrationPlugin) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

func (p *migrationPlugin) RouteRequest(_ context.Context, tx sdk.TransactionContext, _ *http.Request) (*sdk.RouteAction, error) {
	rid, ok := tx.Data["ResellerId"].(string)
	if !ok || rid == "" {
		return nil, nil
	}
	if migrationGlobMatch(p.pattern, rid) {
		return &sdk.RouteAction{ForwardTo: p.forwardTo}, nil
	}
	return nil, nil
}

var _ sdk.Plugin = (*migrationPlugin)(nil)
var _ sdk.RequestRouter = (*migrationPlugin)(nil)

// migrationGlobMatch implements the trailing-"*" wildcard semantics used in
// this test. "migrated-*" matches any input starting with "migrated-".
// Patterns without a trailing "*" require exact equality. This is a
// deliberately small implementation: the full glob semantics (multi-segment,
// "?" matching, etc.) belong in contrib/glob.go and are exercised there.
func migrationGlobMatch(pattern, input string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(input, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == input
}

// migrationProxyRequest builds a /proxy request for the migration scenario.
// resellerID is embedded in X-Connect-Context-Data as Base64-encoded JSON
// (matching the production wire format parsed by internal/context). When
// resellerID is empty, the Context-Data header is omitted entirely so the
// resulting tx.Data is nil (the "missing key" scenario). When emptyData is
// true, an empty JSON object is sent — tx.Data is non-nil but lacks
// ResellerId.
func migrationProxyRequest(t *testing.T, vendorTargetURL, resellerID string, emptyData bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", vendorTargetURL)
	req.Header.Set("X-Connect-Vendor-ID", "vendor-a")
	req.Header.Set("X-Connect-Marketplace-ID", "marketplace-1")
	req.Header.Set("X-Connect-Product-ID", "product-1")
	req.Header.Set("X-Connect-Subscription-ID", "sub-1")

	switch {
	case emptyData:
		req.Header.Set("X-Connect-Context-Data", base64.StdEncoding.EncodeToString([]byte(`{}`)))
	case resellerID != "":
		payload, err := json.Marshal(map[string]any{"ResellerId": resellerID})
		if err != nil {
			t.Fatalf("encoding context data: %v", err)
		}
		req.Header.Set("X-Connect-Context-Data", base64.StdEncoding.EncodeToString(payload))
	}
	return req
}

// migrationServer wires up the proxy server, the migration plugin, both fake
// targets, and the per-target hit counters. It returns a teardown via
// t.Cleanup.
type migrationServer struct {
	srv          *proxy.Server
	plugin       *migrationPlugin
	companyBHits *atomic.Int32
	vendorHits   *atomic.Int32
	companyBBody string
	vendorBody   string
	vendorURL    string // real httptest URL of the vendor target; used as X-Connect-Target-URL
}

// migrationVendorRecord records what the vendor target observed for a single
// request. The mutex-free assignment is safe because tests issue requests
// sequentially.
type migrationVendorRecord struct {
	auth           string
	contextHeaders map[string]string
}

func newMigrationServer(t *testing.T, lastVendorRecord *migrationVendorRecord) *migrationServer {
	t.Helper()

	const companyBBody = `{"reply":"from-company-b"}`
	const vendorBody = `{"reply":"from-vendor"}`

	companyBHits := &atomic.Int32{}
	vendorHits := &atomic.Int32{}

	companyB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		companyBHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, companyBBody)
	}))
	t.Cleanup(companyB.Close)

	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vendorHits.Add(1)
		if lastVendorRecord != nil {
			lastVendorRecord.auth = r.Header.Get("Authorization")
			lastVendorRecord.contextHeaders = make(map[string]string, len(chaperoneCtx.HeaderSuffixes()))
			for _, suffix := range chaperoneCtx.HeaderSuffixes() {
				lastVendorRecord.contextHeaders["X-Connect"+suffix] = r.Header.Get("X-Connect" + suffix)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, vendorBody)
	}))
	t.Cleanup(vendor.Close)

	plugin := &migrationPlugin{
		pattern:   "migrated-*",
		forwardTo: "company-b",
		credToken: "vendor-token",
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.AllowList = map[string][]string{
		mustTargetHostPort(t, vendor.URL):   {"/**"},
		mustTargetHostPort(t, companyB.URL): {"/**"},
	}
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {
			URL: companyB.URL,
			Auth: config.ForwardTargetAuthConfig{
				Type: config.ForwardAuthNone,
			},
		},
	}

	srv := mustNewServer(t, cfg)

	return &migrationServer{
		srv:          srv,
		plugin:       plugin,
		companyBHits: companyBHits,
		vendorHits:   vendorHits,
		companyBBody: companyBBody,
		vendorBody:   vendorBody,
		vendorURL:    vendor.URL,
	}
}

// TestMigrationIntegration exercises the full migration scenario described in
// the implementation plan: one server, one plugin (router + credential
// provider), two fake upstreams (Company B and the vendor), and three
// requests that exercise both routing branches.
func TestMigrationIntegration(t *testing.T) {
	telemetry.ResetMetrics(t)

	var vendorRecord migrationVendorRecord
	env := newMigrationServer(t, &vendorRecord)

	t.Run("request A: migrated-001 is forwarded to Company B", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := migrationProxyRequest(t, env.vendorURL+"/v1/foo", "migrated-001", false)
		env.srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200. body=%s", rec.Code, rec.Body.String())
		}
		if got := env.companyBHits.Load(); got != 1 {
			t.Errorf("companyB hits after request A = %d, want 1", got)
		}
		if got := env.vendorHits.Load(); got != 0 {
			t.Errorf("vendor hits after request A = %d, want 0", got)
		}
		if got := env.plugin.getCredentialsCount.Load(); got != 0 {
			t.Errorf("GetCredentials calls after request A = %d, want 0", got)
		}
		// Response body from Company B reaches the client.
		if got := rec.Body.String(); got != env.companyBBody {
			t.Errorf("client body after request A = %q, want %q", got, env.companyBBody)
		}
	})

	t.Run("request B: legacy-99 falls through to credentials and hits the vendor", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := migrationProxyRequest(t, env.vendorURL+"/v1/foo", "legacy-99", false)
		env.srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200. body=%s", rec.Code, rec.Body.String())
		}
		if got := env.vendorHits.Load(); got != 1 {
			t.Errorf("vendor hits after request B = %d, want 1", got)
		}
		// Company B must not have been hit by request B (still 1 from request A).
		if got := env.companyBHits.Load(); got != 1 {
			t.Errorf("companyB hits after request B = %d, want 1 (unchanged from request A)", got)
		}
		if got := env.plugin.getCredentialsCount.Load(); got != 1 {
			t.Errorf("GetCredentials calls after request B = %d, want 1", got)
		}
		// Bearer token from the plugin's Credential reached the vendor target.
		if got := vendorRecord.auth; got != "Bearer vendor-token" {
			t.Errorf("vendor Authorization = %q, want %q", got, "Bearer vendor-token")
		}
		// X-Connect-* context headers MUST be stripped before reaching the vendor.
		for header, value := range vendorRecord.contextHeaders {
			if value != "" {
				t.Errorf("context header %q leaked to vendor with value %q", header, value)
			}
		}
		// Response body from the vendor reaches the client.
		if got := rec.Body.String(); got != env.vendorBody {
			t.Errorf("client body after request B = %q, want %q", got, env.vendorBody)
		}
	})

	t.Run("request C: migrated-042 (glob match) is forwarded to Company B", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := migrationProxyRequest(t, env.vendorURL+"/v1/foo", "migrated-042", false)
		env.srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200. body=%s", rec.Code, rec.Body.String())
		}
		if got := env.companyBHits.Load(); got != 2 {
			t.Errorf("companyB hits after request C = %d, want 2 (1 from request A + 1 from request C)", got)
		}
		if got := env.vendorHits.Load(); got != 1 {
			t.Errorf("vendor hits after request C = %d, want 1 (only request B)", got)
		}
		if got := env.plugin.getCredentialsCount.Load(); got != 1 {
			t.Errorf("GetCredentials calls after request C = %d, want 1 (only request B)", got)
		}
	})

	t.Run("metrics: route_decisions_total tracks both paths across the 3 requests", func(t *testing.T) {
		// 2 forwards (requests A and C) → action=forward,target=company-b
		fwd := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "company-b"))
		if fwd != 2 {
			t.Errorf("route_decisions_total{action=forward,target=company-b} = %v, want 2", fwd)
		}
		// 1 credentials decision (request B) → action=credentials,target=""
		cred := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("credentials", ""))
		if cred != 1 {
			t.Errorf("route_decisions_total{action=credentials,target=\"\"} = %v, want 1", cred)
		}
		// No cross-contamination: forward with empty target, or credentials
		// labeled with a forward target, must remain zero.
		if v := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "")); v != 0 {
			t.Errorf("forward+empty-target leaked: %v", v)
		}
		if v := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("credentials", "company-b")); v != 0 {
			t.Errorf("credentials+company-b leaked: %v", v)
		}
	})
}

// TestMigrationIntegration_MissingResellerID_FallsThrough verifies that a
// request whose Data map omits ResellerId falls through to the credentials
// path (the router cannot make a migration decision without the key).
func TestMigrationIntegration_MissingResellerID_FallsThrough(t *testing.T) {
	telemetry.ResetMetrics(t)

	env := newMigrationServer(t, nil)

	rec := httptest.NewRecorder()
	// No Context-Data header at all → tx.Data is nil.
	req := migrationProxyRequest(t, env.vendorURL+"/v1/foo", "", false)
	env.srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body=%s", rec.Code, rec.Body.String())
	}
	if got := env.companyBHits.Load(); got != 0 {
		t.Errorf("companyB hits = %d, want 0", got)
	}
	if got := env.vendorHits.Load(); got != 1 {
		t.Errorf("vendor hits = %d, want 1", got)
	}
	if got := env.plugin.getCredentialsCount.Load(); got != 1 {
		t.Errorf("GetCredentials calls = %d, want 1", got)
	}
}

// TestMigrationIntegration_EmptyDataMap_FallsThrough verifies that a request
// carrying X-Connect-Context-Data: "e30=" (base64 of `{}`) — a non-nil but
// empty Data map — also falls through to credentials.
func TestMigrationIntegration_EmptyDataMap_FallsThrough(t *testing.T) {
	telemetry.ResetMetrics(t)

	env := newMigrationServer(t, nil)

	rec := httptest.NewRecorder()
	req := migrationProxyRequest(t, env.vendorURL+"/v1/foo", "", true)
	env.srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body=%s", rec.Code, rec.Body.String())
	}
	if got := env.companyBHits.Load(); got != 0 {
		t.Errorf("companyB hits = %d, want 0", got)
	}
	if got := env.vendorHits.Load(); got != 1 {
		t.Errorf("vendor hits = %d, want 1", got)
	}
	if got := env.plugin.getCredentialsCount.Load(); got != 1 {
		t.Errorf("GetCredentials calls = %d, want 1", got)
	}
}

// TestMigrationIntegration_GlobMatch_MultipleResellerIDs verifies that the
// "migrated-*" pattern matches a range of migrated reseller IDs (not just an
// exact equality match), all of which take the forward path.
func TestMigrationIntegration_GlobMatch_MultipleResellerIDs(t *testing.T) {
	telemetry.ResetMetrics(t)

	env := newMigrationServer(t, nil)

	migrated := []string{"migrated-001", "migrated-042", "migrated-foo-bar", "migrated-"}
	for _, rid := range migrated {
		rec := httptest.NewRecorder()
		req := migrationProxyRequest(t, env.vendorURL+"/v1/foo", rid, false)
		env.srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ResellerId=%q: status = %d, want 200. body=%s", rid, rec.Code, rec.Body.String())
		}
	}

	if got := int(env.companyBHits.Load()); got != len(migrated) {
		t.Errorf("companyB hits = %d, want %d (one per migrated ID)", got, len(migrated))
	}
	if got := env.vendorHits.Load(); got != 0 {
		t.Errorf("vendor hits = %d, want 0 (all migrated IDs should be forwarded)", got)
	}
	if got := env.plugin.getCredentialsCount.Load(); got != 0 {
		t.Errorf("GetCredentials calls = %d, want 0 (forward path must bypass credentials)", got)
	}
}
