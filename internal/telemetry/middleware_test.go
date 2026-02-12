// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// NOTE: These tests must NOT use t.Parallel() because they share global
// Prometheus metrics. Test isolation is achieved via ResetMetrics() and t.Cleanup().

func TestMetricsMiddleware_RecordsRequestTotal(t *testing.T) {
	ResetMetrics(t)

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("test-vendor", "2xx", "POST"))
	if count != 1 {
		t.Errorf("expected request count 1, got %v", count)
	}
}

func TestMetricsMiddleware_RecordsDuration(t *testing.T) {
	ResetMetrics(t)

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Vendor-ID", "duration-test")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify histogram has observations
	count := testutil.CollectAndCount(RequestDuration)
	if count == 0 {
		t.Error("expected RequestDuration to have observations")
	}
}

func TestMetricsMiddleware_TracksActiveConnections(t *testing.T) {
	ResetMetrics(t)

	// Use a barrier to ensure gauge is read only after Inc() has been called
	incDone := make(chan struct{})
	continueHandler := make(chan struct{})
	handlerDone := make(chan struct{})

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signal that we're past the Inc() point (middleware already called Inc)
		close(incDone)
		// Wait for test to check gauge
		<-continueHandler
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	w := httptest.NewRecorder()

	go func() {
		handler.ServeHTTP(w, req)
		close(handlerDone)
	}()

	// Wait for handler to confirm Inc() has been called
	<-incDone

	// Check active connections while request is in-flight
	count := testutil.ToFloat64(ActiveConnections)
	if count != 1 {
		t.Errorf("expected 1 active connection during request, got %v", count)
	}

	// Let handler complete
	close(continueHandler)

	// Wait for goroutine to fully complete (including defer)
	<-handlerDone

	// Verify gauge decremented
	count = testutil.ToFloat64(ActiveConnections)
	if count != 0 {
		t.Errorf("expected 0 active connections after request, got %v", count)
	}
}

func TestMetricsMiddleware_ConcurrentRequests(t *testing.T) {
	ResetMetrics(t)

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const numRequests = 100
	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("unknown", "2xx", "GET"))
	if count != numRequests {
		t.Errorf("expected %d requests, got %v", numRequests, count)
	}
}

func TestMetricsMiddleware_DefaultVendorID(t *testing.T) {
	ResetMetrics(t)

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without vendor ID header
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should use "unknown" as default vendor ID
	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("unknown", "2xx", "GET"))
	if count != 1 {
		t.Errorf("expected request count 1 for unknown vendor, got %v", count)
	}
}

func TestMetricsMiddleware_CapturesErrorStatus(t *testing.T) {
	ResetMetrics(t)

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Vendor-ID", "error-test")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("error-test", "5xx", "POST"))
	if count != 1 {
		t.Errorf("expected request count 1 for 5xx status, got %v", count)
	}
}

func TestMetricsMiddleware_ImplicitWriteHeader(t *testing.T) {
	ResetMetrics(t)

	handler := MetricsMiddleware("X-Connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write without calling WriteHeader - should default to 200
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Vendor-ID", "implicit-test")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should record as 2xx
	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("implicit-test", "2xx", "GET"))
	if count != 1 {
		t.Errorf("expected request count 1 for implicit 200, got %v", count)
	}
}
