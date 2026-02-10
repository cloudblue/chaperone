// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/pkg/crypto"
)

// mtlsTestConfig returns a valid Config for mTLS tests (internal package).
// This mirrors the testConfig() in the proxy_test package but is accessible
// from internal (white-box) tests.
func mtlsTestConfig() Config {
	return Config{
		Addr:             ":0",
		Version:          "test",
		HeaderPrefix:     "X-Connect",
		TraceHeader:      "Connect-Request-ID",
		TLS:              &TLSConfig{Enabled: false},
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     30 * time.Second,
		IdleTimeout:      120 * time.Second,
		KeepAliveTimeout: 30 * time.Second,
		PluginTimeout:    10 * time.Second,
		ConnectTimeout:   5 * time.Second,
		ShutdownTimeout:  30 * time.Second,
	}
}

// mustNewTestServer creates a Server from the given config for internal tests,
// failing the test immediately if the config is invalid.
func mustNewTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed with valid config: %v", err)
	}
	return srv
}

// TestMTLS_ValidClientCert_Success verifies that a client with a valid
// certificate signed by the trusted CA can successfully connect.
func TestMTLS_ValidClientCert_Success(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	// Create server with mTLS
	srv := mustNewTestServer(t, mtlsTestConfig())
	server := httptest.NewUnstartedServer(srv.Handler())
	server.TLS = createServerTLSConfig(t, bundle)
	server.StartTLS()
	defer server.Close()

	// Create client with valid cert
	client := createTLSClient(t, bundle.CA.CertPEM, bundle.Client.CertPEM, bundle.Client.KeyPEM)

	// Make request using context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/_ops/health", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMTLS_NoClientCert_Rejected verifies that a client without a
// certificate is rejected during the TLS handshake.
func TestMTLS_NoClientCert_Rejected(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	// Create server with mTLS
	srv := mustNewTestServer(t, mtlsTestConfig())
	server := httptest.NewUnstartedServer(srv.Handler())
	server.TLS = createServerTLSConfig(t, bundle)
	server.StartTLS()
	defer server.Close()

	// Create client WITHOUT cert (only trusts server CA)
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(bundle.CA.CertPEM)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caCertPool,
				MinVersion: tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}

	// Attempt request - should fail at TLS handshake
	expectTLSRejection(t, client, server.URL+"/_ops/health")
}

// TestMTLS_WrongCA_Rejected verifies that a client certificate signed
// by a different CA is rejected.
func TestMTLS_WrongCA_Rejected(t *testing.T) {
	// Generate two separate CAs
	serverBundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate server cert bundle: %v", err)
	}

	// Generate a separate CA for the client (not trusted by server)
	untrustedCA, err := crypto.GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("failed to generate untrusted CA: %v", err)
	}
	untrustedClient, err := crypto.GenerateClientCert(untrustedCA, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate untrusted client cert: %v", err)
	}

	// Create server with mTLS (trusts only serverBundle.CA)
	srv := mustNewTestServer(t, mtlsTestConfig())
	server := httptest.NewUnstartedServer(srv.Handler())
	server.TLS = createServerTLSConfig(t, serverBundle)
	server.StartTLS()
	defer server.Close()

	// Create client with cert from different CA
	client := createTLSClient(t, serverBundle.CA.CertPEM, untrustedClient.CertPEM, untrustedClient.KeyPEM)

	// Attempt request - should fail during TLS handshake
	expectTLSRejection(t, client, server.URL+"/_ops/health")
}

// TestMTLS_ExpiredCert_Rejected verifies that an expired client
// certificate is rejected.
func TestMTLS_ExpiredCert_Rejected(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	// Generate expired client cert
	expiredClient, err := crypto.GenerateExpiredClientCert(&bundle.CA)
	if err != nil {
		t.Fatalf("failed to generate expired cert: %v", err)
	}
	// Create server with mTLS
	srv := mustNewTestServer(t, mtlsTestConfig())
	server := httptest.NewUnstartedServer(srv.Handler())
	server.TLS = createServerTLSConfig(t, bundle)
	server.StartTLS()
	defer server.Close()

	// Create client with expired cert
	client := createTLSClient(t, bundle.CA.CertPEM, expiredClient.CertPEM, expiredClient.KeyPEM)

	// Attempt request - should fail during TLS handshake
	expectTLSRejection(t, client, server.URL+"/_ops/health")
}

// TestMTLS_TLS12Client_Rejected verifies that clients attempting to
// connect with TLS 1.2 are rejected (TLS 1.3 minimum required).
func TestMTLS_TLS12Client_Rejected(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	// Create server with mTLS (requires TLS 1.3)
	srv := mustNewTestServer(t, mtlsTestConfig())
	server := httptest.NewUnstartedServer(srv.Handler())
	server.TLS = createServerTLSConfig(t, bundle)
	server.StartTLS()
	defer server.Close()

	// Create client forcing TLS 1.2
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(bundle.CA.CertPEM)

	clientCert, err := tls.X509KeyPair(bundle.Client.CertPEM, bundle.Client.KeyPEM)
	if err != nil {
		t.Fatalf("failed to load client cert: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{clientCert},
				MinVersion:   tls.VersionTLS12,
				MaxVersion:   tls.VersionTLS12, // Force TLS 1.2
			},
		},
		Timeout: 5 * time.Second,
	}

	// Attempt request - should fail due to TLS version mismatch
	expectTLSRejection(t, client, server.URL+"/_ops/health")
}

// TestMTLS_SelfSignedCert_Rejected verifies that a self-signed client
// certificate (not signed by any trusted CA) is rejected.
func TestMTLS_SelfSignedCert_Rejected(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	// Generate a self-signed cert (CA acting as client cert)
	selfSignedCA, err := crypto.GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("failed to generate self-signed cert: %v", err)
	}

	// Create server with mTLS
	srv := mustNewTestServer(t, mtlsTestConfig())
	server := httptest.NewUnstartedServer(srv.Handler())
	server.TLS = createServerTLSConfig(t, bundle)
	server.StartTLS()
	defer server.Close()

	// Create client with self-signed cert (not trusted by server CA)
	client := createTLSClient(t, bundle.CA.CertPEM, selfSignedCA.CertPEM, selfSignedCA.KeyPEM)

	// Attempt request - should fail during TLS handshake
	expectTLSRejection(t, client, server.URL+"/_ops/health")
}

// TestTLSConfig_MinVersionTLS13 verifies that the TLS config enforces TLS 1.3 minimum.
func TestTLSConfig_MinVersionTLS13(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	tlsConfig, err := NewTLSConfig(bundle.CA.CertPEM, bundle.Server.CertPEM, bundle.Server.KeyPEM)
	if err != nil {
		t.Fatalf("NewTLSConfig failed: %v", err)
	}

	if tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected MinVersion TLS 1.3 (0x%x), got 0x%x", tls.VersionTLS13, tlsConfig.MinVersion)
	}
}

// TestTLSConfig_RequiresClientCert verifies that the TLS config requires client certificates.
func TestTLSConfig_RequiresClientCert(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	tlsConfig, err := NewTLSConfig(bundle.CA.CertPEM, bundle.Server.CertPEM, bundle.Server.KeyPEM)
	if err != nil {
		t.Fatalf("NewTLSConfig failed: %v", err)
	}

	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("expected ClientAuth RequireAndVerifyClientCert (%d), got %d",
			tls.RequireAndVerifyClientCert, tlsConfig.ClientAuth)
	}
}

// TestTLSConfig_InvalidCACert_ReturnsError verifies error handling for invalid CA cert.
func TestTLSConfig_InvalidCACert_ReturnsError(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	_, err = NewTLSConfig([]byte("invalid-ca"), bundle.Server.CertPEM, bundle.Server.KeyPEM)
	if err == nil {
		t.Fatal("expected error for invalid CA cert")
	}
}

// TestTLSConfig_InvalidServerCert_ReturnsError verifies error handling for invalid server cert.
func TestTLSConfig_InvalidServerCert_ReturnsError(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	_, err = NewTLSConfig(bundle.CA.CertPEM, []byte("invalid-cert"), bundle.Server.KeyPEM)
	if err == nil {
		t.Fatal("expected error for invalid server cert")
	}
}

// Helper functions

// createServerTLSConfig creates a TLS config for the test server.
func createServerTLSConfig(t *testing.T, bundle *crypto.CertBundle) *tls.Config {
	t.Helper()

	tlsConfig, err := NewTLSConfig(bundle.CA.CertPEM, bundle.Server.CertPEM, bundle.Server.KeyPEM)
	if err != nil {
		t.Fatalf("failed to create TLS config: %v", err)
	}

	return tlsConfig
}

// createTLSClient creates an HTTP client with the given TLS credentials.
func createTLSClient(t *testing.T, caCertPEM, clientCertPEM, clientKeyPEM []byte) *http.Client {
	t.Helper()

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to parse CA cert")
	}

	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatalf("failed to load client cert: %v", err)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{clientCert},
				MinVersion:   tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}
}

// expectTLSRejection attempts a request and verifies it fails due to TLS rejection.
// This is used for negative tests where the connection should be rejected at the TLS layer.
func expectTLSRejection(t *testing.T, client *http.Client, url string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected connection to be rejected, but request succeeded")
	}

	t.Logf("connection correctly rejected: %v", err)
}

// TestNewTLSConfig_ParsesCertificatesCorrectly verifies that NewTLSConfig
// correctly parses and loads all certificates.
func TestNewTLSConfig_ParsesCertificatesCorrectly(t *testing.T) {
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("failed to generate cert bundle: %v", err)
	}

	tlsConfig, err := NewTLSConfig(bundle.CA.CertPEM, bundle.Server.CertPEM, bundle.Server.KeyPEM)
	if err != nil {
		t.Fatalf("NewTLSConfig failed: %v", err)
	}

	// Verify server cert is loaded
	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("expected 1 server certificate, got %d", len(tlsConfig.Certificates))
	}

	// Verify CA pool is configured
	if tlsConfig.ClientCAs == nil {
		t.Error("ClientCAs pool is nil")
	}

	// Verify CA is in the pool by checking subject
	caBlock, _ := pem.Decode(bundle.CA.CertPEM)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}

	// The ClientCAs pool should contain our CA
	subjects := tlsConfig.ClientCAs.Subjects() //nolint:staticcheck // Using deprecated method for test verification
	found := false
	for _, subject := range subjects {
		if string(subject) == string(caCert.RawSubject) {
			found = true
			break
		}
	}
	if !found {
		t.Error("CA certificate not found in ClientCAs pool")
	}
}
