// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/pkg/crypto"
)

func TestServer_Start_CalledTwice_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := mustNewServer(t, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	if !srv.WaitForReady(2 * time.Second) {
		t.Fatal("server did not become ready within timeout")
	}
	defer srv.Shutdown(context.Background())

	// Act - call Start() a second time
	err := srv.Start()

	// Assert - should return error, not panic
	if err == nil {
		t.Error("expected error from second Start() call, got nil")
	}
}

func TestGracefulShutdown_HandlesSignal(t *testing.T) {
	t.Parallel()

	// Arrange - create server on random port with TLS disabled
	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := mustNewServer(t, cfg)

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for server to be ready
	if !srv.WaitForReady(2 * time.Second) {
		t.Fatal("server did not become ready within timeout")
	}

	// Act - trigger shutdown
	srv.Shutdown(context.Background())

	// Assert - server should return without error (ErrServerClosed is suppressed)
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("unexpected error from Start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within timeout")
	}
}

func TestShutdown_BeforeReady_WaitsForServer(t *testing.T) {
	t.Parallel()

	// Arrange - create server but don't start it yet
	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := mustNewServer(t, cfg)

	// Start shutdown goroutine BEFORE server is ready (simulates early SIGTERM).
	// Shutdown should block until the server signals readiness, then drain.
	shutdownDone := make(chan struct{})
	go func() {
		srv.Shutdown(context.Background())
		close(shutdownDone)
	}()

	// Verify shutdown hasn't completed yet (server not started)
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before server was ready — should have waited")
	case <-time.After(100 * time.Millisecond):
		// Expected: Shutdown is still blocking
	}

	// Act - now start the server (Shutdown should unblock and drain it)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Assert - shutdown should complete (unblocked by ready signal)
	select {
	case <-shutdownDone:
		// Expected: Shutdown completed after Start signaled readiness
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete after server became ready")
	}

	// Assert - Start should return cleanly (ErrServerClosed suppressed)
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("unexpected error from Start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Shutdown")
	}
}

func TestShutdown_ContextExpired_BeforeReady_ReturnsImmediately(t *testing.T) {
	t.Parallel()

	// Arrange - create server but never start it
	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := mustNewServer(t, cfg)

	// Act - call Shutdown with an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	shutdownDone := make(chan struct{})
	go func() {
		srv.Shutdown(ctx)
		close(shutdownDone)
	}()

	// Assert - should return immediately since context is already expired
	select {
	case <-shutdownDone:
		// Expected: Shutdown returned because context expired
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown blocked despite expired context — should have returned immediately")
	}
}

func TestGracefulShutdown_DrainsInFlightRequests(t *testing.T) {
	t.Parallel()

	// Arrange - handler that takes 200ms to respond
	handlerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerDone)
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("completed"))
	})

	// Create a real server to test graceful shutdown
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	httpSrv := &http.Server{
		Handler: handler,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = httpSrv.Serve(ln)
	}()

	// Start a request in-flight
	addr := ln.Addr().String()
	responseCh := make(chan *http.Response, 1)
	go func() {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/slow", nil)
		if reqErr != nil {
			return
		}
		resp, respErr := http.DefaultClient.Do(req) //nolint:bodyclose // closed in the select below
		if respErr != nil {
			return
		}
		responseCh <- resp
	}()

	// Wait for the handler to start processing
	<-handlerDone

	// Act - trigger shutdown while request is in-flight
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if shutdownErr := httpSrv.Shutdown(ctx); shutdownErr != nil {
		t.Fatalf("shutdown returned error: %v", shutdownErr)
	}

	// Assert - the in-flight request should complete
	select {
	case resp := <-responseCh:
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		resp.Body.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request did not complete")
	}

	wg.Wait()
}

func TestGracefulShutdown_RejectsNewConnections(t *testing.T) {
	t.Parallel()

	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()

	httpSrv := &http.Server{
		Handler: handler,
	}

	go func() {
		_ = httpSrv.Serve(ln)
	}()

	// Act - shut down the server
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if shutdownErr := httpSrv.Shutdown(ctx); shutdownErr != nil {
		t.Fatalf("shutdown returned error: %v", shutdownErr)
	}

	// Assert - new connections should fail
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Error("expected error for connection after shutdown, got nil")
	}
}

func TestServer_ShutdownTimeout_Configurable(t *testing.T) {
	t.Parallel()

	// Arrange & Act
	cfg := testConfig()
	cfg.ShutdownTimeout = 45 * time.Second
	srv := mustNewServer(t, cfg)

	cfg = srv.Config()

	// Assert - custom value should be preserved
	if cfg.ShutdownTimeout != 45*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 45s", cfg.ShutdownTimeout)
	}
}

func TestServer_ConnectTimeout_AppliedToTransport(t *testing.T) {
	t.Parallel()

	// Arrange - slow backend that takes forever to accept connections
	// We use a listener that never accepts to simulate connection timeout
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	// Close listener immediately - connections will be refused
	ln.Close()

	// Parse the host (without port) for the allow list
	host, _, _ := net.SplitHostPort(addr)

	cfg := testConfig()
	cfg.ConnectTimeout = 100 * time.Millisecond
	cfg.AllowList = map[string][]string{host: {"/**"}, addr: {"/**"}}
	srv := mustNewServer(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "http://"+addr+"/api/test")
	req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
	rec := httptest.NewRecorder()

	// Act
	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	// Assert - should fail with 502 Bad Gateway (connection refused/timeout)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	// Assert - should not take much longer than the connect timeout
	if elapsed > 2*time.Second {
		t.Errorf("request took %v, expected to fail within connect timeout", elapsed)
	}
}

func TestServer_Addr_ReturnsActualAddress(t *testing.T) {
	t.Parallel()

	// Arrange - server on random port
	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	srv := mustNewServer(t, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	if !srv.WaitForReady(2 * time.Second) {
		t.Fatal("server did not become ready within timeout")
	}
	defer srv.Shutdown(context.Background())

	// Act
	addr := srv.Addr()

	// Assert - should be actual address, not ":0"
	if addr == "127.0.0.1:0" || addr == ":0" || addr == "" {
		t.Errorf("Addr() = %q, want resolved address with real port", addr)
	}

	// Assert - should be reachable
	healthReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/_ops/health", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(healthReq)
	if err != nil {
		t.Fatalf("failed to reach server at %s: %v", addr, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestServer_StartTLS_WithValidCerts_Succeeds(t *testing.T) {
	t.Parallel()

	// Arrange - generate test certificates
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.crt")
	certFile := filepath.Join(tmpDir, "server.crt")
	keyFile := filepath.Join(tmpDir, "server.key")

	writeCertFiles(t, bundle, caFile, certFile, keyFile)

	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	}
	srv := mustNewServer(t, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	if !srv.WaitForReady(2 * time.Second) {
		t.Fatal("TLS server did not become ready within timeout")
	}
	defer srv.Shutdown(context.Background())

	// Act - connect with mTLS client using matching certs
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(bundle.CA.CertPEM)

	clientCert, err := tls.X509KeyPair(bundle.Client.CertPEM, bundle.Client.KeyPEM)
	if err != nil {
		t.Fatalf("failed to load client cert: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      caCertPool,
				MinVersion:   tls.VersionTLS13,
			},
		},
	}

	healthReq, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+srv.Addr()+"/_ops/health", nil)
	if reqErr != nil {
		t.Fatalf("failed to create request: %v", reqErr)
	}
	resp, err := client.Do(healthReq)
	if err != nil {
		t.Fatalf("failed to reach TLS server: %v", err)
	}
	resp.Body.Close()

	// Assert
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestServer_StartTLS_ShutdownWorks(t *testing.T) {
	t.Parallel()

	// Arrange - generate test certificates
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.crt")
	certFile := filepath.Join(tmpDir, "server.crt")
	keyFile := filepath.Join(tmpDir, "server.key")

	writeCertFiles(t, bundle, caFile, certFile, keyFile)

	cfg := testConfig()
	cfg.Addr = "127.0.0.1:0"
	cfg.TLS = &proxy.TLSConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	}
	srv := mustNewServer(t, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	if !srv.WaitForReady(2 * time.Second) {
		t.Fatal("TLS server did not become ready within timeout")
	}

	// Act - trigger shutdown
	srv.Shutdown(context.Background())

	// Assert - Start should return nil (ErrServerClosed suppressed)
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("unexpected error from Start: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TLS server did not shut down within timeout")
	}
}

// writeCertFiles writes certificate bundle files to disk for TLS tests.
func writeCertFiles(t *testing.T, bundle *crypto.CertBundle, caFile, certFile, keyFile string) {
	t.Helper()
	if err := os.WriteFile(caFile, bundle.CA.CertPEM, 0o600); err != nil {
		t.Fatalf("failed to write CA file: %v", err)
	}
	if err := os.WriteFile(certFile, bundle.Server.CertPEM, 0o600); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, bundle.Server.KeyPEM, 0o600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
}
