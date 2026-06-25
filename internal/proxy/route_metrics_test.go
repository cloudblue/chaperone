// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/telemetry"
	"github.com/cloudblue/chaperone/sdk"
)

// NOTE: These tests must NOT use t.Parallel() — they share the global
// Prometheus registries (RouteDecisionsTotal, ForwardTargetDuration,
// ForwardTargetErrors). Test isolation is achieved via telemetry.ResetMetrics().

func TestMetrics_RouteDecision_Forward_IncrementsCounter(t *testing.T) {
	telemetry.ResetMetrics(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	plugin := &routerPlugin{
		action:    &sdk.RouteAction{ForwardTo: "company-b"},
		actionSet: true,
	}
	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
		"company-b": {URL: target.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
	}
	srv := mustNewServerForTarget(t, cfg, target.URL)

	srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, target.URL+"/v1/foo"))

	got := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "company-b"))
	if got != 1 {
		t.Errorf("route_decisions_total{action=forward,target=company-b} = %v, want 1", got)
	}
	// No credentials decision should fire for a forwarded request.
	credCount := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("credentials", ""))
	if credCount != 0 {
		t.Errorf("credentials counter must be 0 on forward path, got %v", credCount)
	}
}

func TestMetrics_RouteDecision_Credentials_IncrementsCounter(t *testing.T) {
	telemetry.ResetMetrics(t)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Plain plugin (no RequestRouter) — straight credentials path.
	cfg := testConfig()
	cfg.Plugin = &plainPlugin{}
	srv := mustNewServerForTarget(t, cfg, backend.URL)

	srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, backend.URL))

	got := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("credentials", ""))
	if got != 1 {
		t.Errorf("route_decisions_total{action=credentials,target=\"\"} = %v, want 1", got)
	}
}

// TestMetrics_RouteDecisions_ForwardAndCredentials_NoCrossContamination drives
// both flows in the same test and verifies the counters track independently
// without label aliasing.
func TestMetrics_RouteDecisions_ForwardAndCredentials_NoCrossContamination(t *testing.T) {
	telemetry.ResetMetrics(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// First request: forward path.
	{
		plugin := &routerPlugin{
			action:    &sdk.RouteAction{ForwardTo: "company-b"},
			actionSet: true,
		}
		cfg := testConfig()
		cfg.Plugin = plugin
		cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
			"company-b": {URL: target.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
		}
		srv := mustNewServerForTarget(t, cfg, target.URL)
		srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, target.URL+"/v1/foo"))
	}

	// Second request: credentials path (nil action falls through).
	{
		plugin := &routerPlugin{
			action:    nil,
			actionSet: true,
		}
		cfg := testConfig()
		cfg.Plugin = plugin
		srv := mustNewServerForTarget(t, cfg, backend.URL)
		srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, backend.URL))
	}

	fwd := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "company-b"))
	if fwd != 1 {
		t.Errorf("forward counter = %v, want 1", fwd)
	}
	cred := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("credentials", ""))
	if cred != 1 {
		t.Errorf("credentials counter = %v, want 1", cred)
	}
	// Cross-contamination sanity checks: these label combinations must not exist.
	if v := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "")); v != 0 {
		t.Errorf("forward+empty-target leaked: %v", v)
	}
	if v := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("credentials", "company-b")); v != 0 {
		t.Errorf("credentials+company-b leaked: %v", v)
	}
}

// TestMetrics_RouteDecision_MultipleForwardTargets verifies each named target
// gets its own counter cell (no aliasing across targets).
func TestMetrics_RouteDecision_MultipleForwardTargets(t *testing.T) {
	telemetry.ResetMetrics(t)

	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer b.Close()

	// Drive a request to "a"
	{
		plugin := &routerPlugin{action: &sdk.RouteAction{ForwardTo: "a"}, actionSet: true}
		cfg := testConfig()
		cfg.Plugin = plugin
		cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
			"a": {URL: a.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
			"b": {URL: b.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
		}
		srv := mustNewServerForTarget(t, cfg, a.URL)
		srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, a.URL+"/x"))
	}
	// Drive two requests to "b"
	{
		plugin := &routerPlugin{action: &sdk.RouteAction{ForwardTo: "b"}, actionSet: true}
		cfg := testConfig()
		cfg.Plugin = plugin
		cfg.ForwardTargets = map[string]config.ForwardTargetConfig{
			"a": {URL: a.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
			"b": {URL: b.URL, Auth: config.ForwardTargetAuthConfig{Type: config.ForwardAuthNone}},
		}
		srv := mustNewServerForTarget(t, cfg, b.URL)
		srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, b.URL+"/x"))
		srv.Handler().ServeHTTP(httptest.NewRecorder(), newProxyRequest(t, b.URL+"/x"))
	}

	if v := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "a")); v != 1 {
		t.Errorf("forward/a counter = %v, want 1", v)
	}
	if v := testutil.ToFloat64(telemetry.RouteDecisionsTotal.WithLabelValues("forward", "b")); v != 2 {
		t.Errorf("forward/b counter = %v, want 2", v)
	}
}
