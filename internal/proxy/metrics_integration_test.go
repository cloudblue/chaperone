// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cloudblue/chaperone/internal/telemetry"
)

// NOTE: mtlsTestConfig and mustNewTestServer are defined in mtls_test.go
// (shared across internal tests).

// NOTE: These tests must NOT use t.Parallel() because they share global
// Prometheus metrics. Test isolation is achieved via telemetry.ResetMetrics().

func TestHandler_HealthEndpoint_RecordsMetrics(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Create server with allow list for test host
	cfg := mtlsTestConfig()
	cfg.AllowList = map[string][]string{"httpbin.org": {"/**"}}
	srv := mustNewTestServer(t, cfg)

	handler := srv.Handler()

	// Make a request to health endpoint (should be metered)
	req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify metrics were recorded
	count := testutil.ToFloat64(telemetry.RequestsTotal.WithLabelValues("unknown", "2xx", "GET"))
	if count != 1 {
		t.Errorf("expected 1 request recorded, got %v", count)
	}
}

func TestHandler_VendorHeader_ExtractsVendorID(t *testing.T) {
	telemetry.ResetMetrics(t)

	cfg := mtlsTestConfig()
	cfg.AllowList = map[string][]string{"httpbin.org": {"/**"}}
	srv := mustNewTestServer(t, cfg)

	handler := srv.Handler()

	// Request with vendor ID
	req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	count := testutil.ToFloat64(telemetry.RequestsTotal.WithLabelValues("test-vendor", "2xx", "GET"))
	if count != 1 {
		t.Errorf("expected 1 request for test-vendor, got %v", count)
	}
}

func TestAdminServer_MetricsEndpoint_ExposesCustomMetrics(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Touch the metrics so they appear in output
	telemetry.RequestsTotal.WithLabelValues("test", "2xx", "GET").Inc()
	telemetry.RequestDuration.WithLabelValues("test").Observe(0.1)

	// Create admin server and verify /metrics is exposed
	adminSrv := telemetry.NewAdminServer("127.0.0.1:0", "test")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	adminSrv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected /metrics status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "chaperone_requests_total") {
		t.Error("expected /metrics to contain chaperone_requests_total")
	}
	if !strings.Contains(body, "chaperone_request_duration_seconds") {
		t.Error("expected /metrics to contain chaperone_request_duration_seconds")
	}
	if !strings.Contains(body, "chaperone_active_connections") {
		t.Error("expected /metrics to contain chaperone_active_connections")
	}
}

func TestAdminServer_MetricsEndpoint_ExposesPanicsTotal(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Increment panics counter so it appears in output
	telemetry.PanicsTotal.Inc()

	adminSrv := telemetry.NewAdminServer("127.0.0.1:0", "test")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	adminSrv.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected /metrics status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "chaperone_panics_total") {
		t.Error("expected /metrics to contain chaperone_panics_total")
	}
}

func TestPanicRecovery_IncrementsPanicsTotal(t *testing.T) {
	// Capture the current value (counter cannot be reset)
	before := testutil.ToFloat64(telemetry.PanicsTotal)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("metrics test panic")
	})

	handler := PanicRecoveryMiddleware(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := testutil.ToFloat64(telemetry.PanicsTotal)
	if after-before != 1 {
		t.Errorf("expected PanicsTotal to increase by 1, got increase of %v", after-before)
	}
}

func TestHandler_ConcurrentRequests_ThreadSafe(t *testing.T) {
	telemetry.ResetMetrics(t)

	cfg := mtlsTestConfig()
	cfg.AllowList = map[string][]string{}
	srv := mustNewTestServer(t, cfg)

	handler := srv.Handler()

	const numRequests = 50
	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/_ops/health", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	count := testutil.ToFloat64(telemetry.RequestsTotal.WithLabelValues("unknown", "2xx", "GET"))
	if count != numRequests {
		t.Errorf("expected %d requests, got %v", numRequests, count)
	}
}

func TestHandler_UpstreamDuration_RecordsOnSuccess(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Create a test upstream server with controlled latency
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Parse upstream URL to get host for AllowList
	// Note: AllowList validator uses Hostname() which strips the port,
	// so we must use Hostname() here too for matching
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream URL: %v", err)
	}

	cfg := mtlsTestConfig()
	cfg.AllowList = map[string][]string{upstreamURL.Hostname(): {"/**"}}
	srv := mustNewTestServer(t, cfg)

	handler := srv.Handler()

	// Make a proxy request
	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", upstream.URL)
	req.Header.Set("X-Connect-Vendor-ID", "upstream-test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify request succeeded
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify upstream duration was recorded
	count := testutil.CollectAndCount(telemetry.UpstreamDuration)
	if count == 0 {
		t.Error("expected UpstreamDuration to have observations")
	}
}

func TestHandler_UpstreamDuration_RecordsOnError(t *testing.T) {
	telemetry.ResetMetrics(t)

	// Target a non-existent server to trigger error path
	cfg := mtlsTestConfig()
	cfg.AllowList = map[string][]string{"127.0.0.1": {"/**"}}
	srv := mustNewTestServer(t, cfg)

	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "http://127.0.0.1:59999/test")
	req.Header.Set("X-Connect-Vendor-ID", "error-test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should be 502 Bad Gateway due to connection error
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", w.Code)
	}

	// Verify upstream duration was still recorded (via ErrorHandler)
	count := testutil.CollectAndCount(telemetry.UpstreamDuration)
	if count == 0 {
		t.Error("expected UpstreamDuration to have observations even on error")
	}
}
