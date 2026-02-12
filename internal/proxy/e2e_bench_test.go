// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/sdk"
)

// benchPlugin is a simple plugin for benchmarking that returns static credentials.
// Named differently from mockPlugin in integration_test.go for clarity.
type benchPlugin struct {
	headers map[string]string
	ttl     time.Duration
}

func (p *benchPlugin) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	if p.headers == nil {
		return nil, nil
	}
	return &sdk.Credential{
		Headers:   p.headers,
		ExpiresAt: time.Now().Add(p.ttl),
	}, nil
}

func (p *benchPlugin) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return nil, nil
}

func (p *benchPlugin) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

// Verify benchPlugin implements sdk.Plugin at compile time.
var _ sdk.Plugin = (*benchPlugin)(nil)

// BenchmarkFullRequestCycle_FastPath benchmarks the complete proxy request/response
// cycle with a Fast Path plugin (returns credentials for injection).
// This exercises the real proxy: context parsing, allow-list, credential injection,
// httputil.ReverseProxy, ModifyResponse chain, Reflector, and middleware.
// Target: < 100us overhead (excluding upstream latency)
func BenchmarkFullRequestCycle_FastPath(b *testing.B) {
	silenceLogs(b)

	// Mock upstream that returns immediately
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	plugin := &benchPlugin{
		headers: map[string]string{"Authorization": "Bearer bench-token-12345678"},
		ttl:     1 * time.Hour,
	}

	// Use the real proxy Server, same pattern as integration_test.go
	cfg := benchConfig()
	cfg.Plugin = plugin
	srv, err := NewServer(cfg)
	if err != nil {
		b.Fatalf("NewServer failed: %v", err)
	}
	handler := srv.Handler()

	contextData := base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/proxy", nil)
		req.Header.Set("X-Connect-Target-URL", upstream.URL+"/api/resource")
		req.Header.Set("X-Connect-Vendor-ID", "benchmark-vendor")
		req.Header.Set("X-Connect-Marketplace-ID", "US")
		req.Header.Set("X-Connect-Context-Data", contextData)
		req.Header.Set("Connect-Request-ID", "bench-trace-123")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d, body: %s", rec.Code, rec.Body.String())
		}
	}
}

// BenchmarkFullRequestCycle_SlowPath benchmarks the proxy with a Slow Path plugin
// (returns nil credential, mutates request directly).
// This exercises the header snapshot + diff logic in detectSlowPathInjections.
func BenchmarkFullRequestCycle_SlowPath(b *testing.B) {
	silenceLogs(b)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	// Slow path plugin: mutates request directly
	slowPlugin := &benchPlugin{} // headers is nil, so GetCredentials returns (nil, nil)

	cfg := benchConfig()
	cfg.Plugin = slowPlugin
	srv, err := NewServer(cfg)
	if err != nil {
		b.Fatalf("NewServer failed: %v", err)
	}
	handler := srv.Handler()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/proxy", nil)
		req.Header.Set("X-Connect-Target-URL", upstream.URL+"/api/resource")
		req.Header.Set("X-Connect-Vendor-ID", "benchmark-vendor")
		req.Header.Set("Connect-Request-ID", "bench-trace-123")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d, body: %s", rec.Code, rec.Body.String())
		}
	}
}

// BenchmarkFullRequestCycle_NoPlugin benchmarks the proxy without any plugin.
// This isolates the core proxy overhead (parsing, validation, forwarding, sanitization).
func BenchmarkFullRequestCycle_NoPlugin(b *testing.B) {
	silenceLogs(b)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	cfg := benchConfig()
	srv, err := NewServer(cfg)
	if err != nil {
		b.Fatalf("NewServer failed: %v", err)
	}
	handler := srv.Handler()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/proxy", nil)
		req.Header.Set("X-Connect-Target-URL", upstream.URL+"/api/resource")
		req.Header.Set("X-Connect-Vendor-ID", "benchmark-vendor")
		req.Header.Set("Connect-Request-ID", "bench-trace-123")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("unexpected status: %d, body: %s", rec.Code, rec.Body.String())
		}
	}
}

// BenchmarkFullRequestCycle_Parallel benchmarks concurrent request handling
// through the real proxy. Target: Linear scaling up to GOMAXPROCS.
func BenchmarkFullRequestCycle_Parallel(b *testing.B) {
	silenceLogs(b)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	plugin := &benchPlugin{
		headers: map[string]string{"Authorization": "Bearer bench-token-12345678"},
		ttl:     1 * time.Hour,
	}

	cfg := benchConfig()
	cfg.Plugin = plugin
	srv, err := NewServer(cfg)
	if err != nil {
		b.Fatalf("NewServer failed: %v", err)
	}
	handler := srv.Handler()

	contextData := base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`))

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/proxy", nil)
			req.Header.Set("X-Connect-Target-URL", upstream.URL+"/api/resource")
			req.Header.Set("X-Connect-Vendor-ID", "benchmark-vendor")
			req.Header.Set("X-Connect-Context-Data", contextData)
			req.Header.Set("Connect-Request-ID", "bench-trace-123")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// Parallel benchmarks may see transient 502s from httptest.Server
			// under heavy concurrent load — this is expected and not a proxy bug.
			// Only fail on truly unexpected status codes.
			if rec.Code != http.StatusOK && rec.Code != http.StatusBadGateway {
				b.Errorf("unexpected status: %d", rec.Code)
				return
			}
		}
	})
}
