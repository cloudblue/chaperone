// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package crypto //nolint:revive // intentionally named crypto for clarity

import (
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Happy Path Tests
// =============================================================================

func TestGenerateCertBundle_Success(t *testing.T) {
	bundle, err := GenerateCertBundle()
	if err != nil {
		t.Fatalf("GenerateCertBundle failed: %v", err)
	}

	// Verify CA
	if len(bundle.CA.CertPEM) == 0 {
		t.Error("CA CertPEM is empty")
	}
	if len(bundle.CA.KeyPEM) == 0 {
		t.Error("CA KeyPEM is empty")
	}

	// Verify Server cert
	if len(bundle.Server.CertPEM) == 0 {
		t.Error("Server CertPEM is empty")
	}
	if len(bundle.Server.KeyPEM) == 0 {
		t.Error("Server KeyPEM is empty")
	}

	// Verify Client cert
	if len(bundle.Client.CertPEM) == 0 {
		t.Error("Client CertPEM is empty")
	}
	if len(bundle.Client.KeyPEM) == 0 {
		t.Error("Client KeyPEM is empty")
	}

	// Verify the client cert is signed by the CA
	caBlock, _ := pem.Decode(bundle.CA.CertPEM)
	if caBlock == nil {
		t.Fatal("failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}

	clientBlock, _ := pem.Decode(bundle.Client.CertPEM)
	if clientBlock == nil {
		t.Fatal("failed to decode client cert PEM")
	}
	clientCert, err := x509.ParseCertificate(clientBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse client cert: %v", err)
	}

	// Verify client cert is signed by CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	_, err = clientCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("client cert verification failed: %v", err)
	}
}

func TestGenerateCA_Success(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	if len(ca.CertPEM) == 0 {
		t.Error("CA CertPEM is empty")
	}
	if len(ca.KeyPEM) == 0 {
		t.Error("CA KeyPEM is empty")
	}

	// Verify it's ECDSA P-256
	block, _ := pem.Decode(ca.KeyPEM)
	if block == nil {
		t.Fatal("failed to decode CA key PEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA private key: %v", err)
	}
	if key.Curve != elliptic.P256() {
		t.Errorf("key curve = %v, want P-256", key.Curve.Params().Name)
	}
}

func TestGenerateServerCert_Success(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	server, err := GenerateServerCert(ca, time.Hour)
	if err != nil {
		t.Fatalf("GenerateServerCert failed: %v", err)
	}

	if len(server.CertPEM) == 0 {
		t.Error("Server CertPEM is empty")
	}
	if len(server.KeyPEM) == 0 {
		t.Error("Server KeyPEM is empty")
	}

	// Verify the cert is valid for localhost
	verifyServerCertSANs(t, server, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
}

func TestGenerateServerCertWithSANs_Success(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	extraDNS := []string{"proxy.example.com", "internal.local"}
	extraIPs := []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("192.168.1.1")}

	server, err := GenerateServerCertWithSANs(ca, time.Hour, extraDNS, extraIPs)
	if err != nil {
		t.Fatalf("GenerateServerCertWithSANs failed: %v", err)
	}

	// Should include localhost + extra SANs
	expectedDNS := append([]string{"localhost"}, extraDNS...)
	expectedIPs := append([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, extraIPs...)
	verifyServerCertSANs(t, server, expectedDNS, expectedIPs)
}

func TestGenerateClientCert_Success(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	client, err := GenerateClientCert(ca, time.Hour)
	if err != nil {
		t.Fatalf("GenerateClientCert failed: %v", err)
	}

	if len(client.CertPEM) == 0 {
		t.Error("Client CertPEM is empty")
	}
	if len(client.KeyPEM) == 0 {
		t.Error("Client KeyPEM is empty")
	}

	// Verify it's a client cert
	block, _ := pem.Decode(client.CertPEM)
	if block == nil {
		t.Fatal("failed to decode client cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse client cert: %v", err)
	}
	if cert.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Error("client cert does not have ClientAuth extended key usage")
	}
}

func TestGenerateExpiredClientCert_Success(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	expired, err := GenerateExpiredClientCert(ca)
	if err != nil {
		t.Fatalf("GenerateExpiredClientCert failed: %v", err)
	}

	// Verify it's actually expired
	block, _ := pem.Decode(expired.CertPEM)
	if block == nil {
		t.Fatal("failed to decode expired cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse expired cert: %v", err)
	}
	if time.Now().Before(cert.NotAfter) {
		t.Errorf("cert is not expired: NotAfter = %v, Now = %v", cert.NotAfter, time.Now())
	}
}

func TestGenerateServerCSR_Success(t *testing.T) {
	dnsNames := []string{"localhost", "proxy.example.com"}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")}

	csr, err := GenerateServerCSR("chaperone", dnsNames, ips)
	if err != nil {
		t.Fatalf("GenerateServerCSR failed: %v", err)
	}

	if len(csr.CSRPEM) == 0 {
		t.Error("CSR PEM is empty")
	}
	if len(csr.KeyPEM) == 0 {
		t.Error("Key PEM is empty")
	}

	// Verify CSR PEM format
	if !strings.Contains(string(csr.CSRPEM), "CERTIFICATE REQUEST") {
		t.Error("CSR PEM does not contain expected header")
	}

	// Verify Key PEM format (ECDSA P-256)
	if !strings.Contains(string(csr.KeyPEM), "EC PRIVATE KEY") {
		t.Error("Key PEM does not contain expected EC header")
	}

	// Parse and verify CSR content
	block, _ := pem.Decode(csr.CSRPEM)
	if block == nil {
		t.Fatal("failed to decode CSR PEM")
	}
	parsedCSR, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CSR: %v", err)
	}

	if parsedCSR.Subject.CommonName != "chaperone" {
		t.Errorf("CommonName = %q, want %q", parsedCSR.Subject.CommonName, "chaperone")
	}
	if len(parsedCSR.DNSNames) != len(dnsNames) {
		t.Errorf("DNSNames count = %d, want %d", len(parsedCSR.DNSNames), len(dnsNames))
	}
}

func TestGenerateServerCSR_EmptySANs_ReturnsError(t *testing.T) {
	_, err := GenerateServerCSR("chaperone", nil, nil)

	if err == nil {
		t.Fatal("expected error for empty SANs, got nil")
	}
	if !errors.Is(err, ErrEmptySANs) {
		t.Errorf("error = %v, want ErrEmptySANs", err)
	}
}

func TestGenerateServerCSR_OnlyDNS_Success(t *testing.T) {
	_, err := GenerateServerCSR("test", []string{"example.com"}, nil)
	if err != nil {
		t.Errorf("unexpected error with DNS-only SANs: %v", err)
	}
}

func TestGenerateServerCSR_OnlyIP_Success(t *testing.T) {
	_, err := GenerateServerCSR("test", nil, []net.IP{net.ParseIP("10.0.0.1")})
	if err != nil {
		t.Errorf("unexpected error with IP-only SANs: %v", err)
	}
}

func TestParseCA_Success(t *testing.T) {
	// Generate a CA first
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Parse it back
	cert, key, err := ParseCA(ca)
	if err != nil {
		t.Fatalf("ParseCA failed: %v", err)
	}

	if cert == nil {
		t.Fatal("parsed certificate is nil")
	}
	if key == nil {
		t.Fatal("parsed key is nil")
	}

	// Verify it's an ECDSA P-256 key
	if key.Curve != elliptic.P256() {
		t.Errorf("key curve = %v, want P-256", key.Curve.Params().Name)
	}
}

// =============================================================================
// Error Path Tests
// =============================================================================

func TestGenerateServerCert_InvalidCACert_ReturnsError(t *testing.T) {
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	_, err := GenerateServerCert(invalidCA, time.Hour)

	if err == nil {
		t.Fatal("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

func TestGenerateClientCert_InvalidCACert_ReturnsError(t *testing.T) {
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	_, err := GenerateClientCert(invalidCA, time.Hour)

	if err == nil {
		t.Fatal("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

func TestGenerateServerCert_InvalidCAKey_ReturnsError(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	invalidCA := &CertPair{
		CertPEM: ca.CertPEM,
		KeyPEM:  []byte("not a valid key"),
	}

	_, err = GenerateServerCert(invalidCA, time.Hour)

	if err == nil {
		t.Fatal("expected error for invalid CA key, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA key") {
		t.Errorf("error = %q, want to contain 'parsing CA key'", err.Error())
	}
}

func TestParseCA_InvalidCert_ReturnsError(t *testing.T) {
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	_, _, err := ParseCA(invalidCA)

	if err == nil {
		t.Fatal("expected error for invalid CA, got nil")
	}
}

func TestParseCA_InvalidKey_ReturnsError(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	invalidCA := &CertPair{
		CertPEM: ca.CertPEM,
		KeyPEM:  []byte("not a valid key PEM"),
	}

	_, _, err = ParseCA(invalidCA)

	if err == nil {
		t.Fatal("expected error for invalid CA key, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA key") {
		t.Errorf("error = %q, want to contain 'parsing CA key'", err.Error())
	}
}

func TestParseCA_ValidKeyPEM_InvalidKeyDER_ReturnsError(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Create a valid PEM block with invalid DER content
	invalidKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("invalid DER content"),
	})

	invalidCA := &CertPair{
		CertPEM: ca.CertPEM,
		KeyPEM:  invalidKeyPEM,
	}

	_, _, err = ParseCA(invalidCA)

	if err == nil {
		t.Fatal("expected error for invalid key DER, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA key") {
		t.Errorf("error = %q, want to contain 'parsing CA key'", err.Error())
	}
}

func TestParseCA_ValidCertPEM_InvalidCertDER_ReturnsError(t *testing.T) {
	// Create a valid PEM block with invalid DER content
	invalidCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("invalid DER content"),
	})
	invalidKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("invalid DER content"),
	})

	invalidCA := &CertPair{
		CertPEM: invalidCertPEM,
		KeyPEM:  invalidKeyPEM,
	}

	_, _, err := ParseCA(invalidCA)

	if err == nil {
		t.Fatal("expected error for invalid cert DER, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

func TestGenerateExpiredClientCert_InvalidCACert_ReturnsError(t *testing.T) {
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	_, err := GenerateExpiredClientCert(invalidCA)

	if err == nil {
		t.Fatal("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

// =============================================================================
// Key Type Verification Tests
// =============================================================================

func TestCurve_IsP256(t *testing.T) {
	if Curve != elliptic.P256() {
		t.Errorf("Curve = %v, want P-256 (per design spec 8.2)", Curve.Params().Name)
	}
}

func TestGeneratedKeys_AreECDSAP256(t *testing.T) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Generate all key types and verify they're ECDSA P-256
	certs := []struct {
		fn   func() (*CertPair, error)
		name string
	}{
		{func() (*CertPair, error) { return GenerateServerCert(ca, time.Hour) }, "ServerCert"},
		{func() (*CertPair, error) { return GenerateClientCert(ca, time.Hour) }, "ClientCert"},
		{func() (*CertPair, error) { return GenerateExpiredClientCert(ca) }, "ExpiredClientCert"},
	}

	for _, tc := range certs {
		t.Run(tc.name, func(t *testing.T) {
			pair, err := tc.fn()
			if err != nil {
				t.Fatalf("generation failed: %v", err)
			}

			block, _ := pem.Decode(pair.KeyPEM)
			if block == nil {
				t.Fatal("failed to decode key PEM")
			}
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				t.Fatalf("failed to parse private key: %v", err)
			}
			if key.Curve != elliptic.P256() {
				t.Errorf("key curve = %v, want P-256", key.Curve.Params().Name)
			}
		})
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func verifyServerCertSANs(t *testing.T, pair *CertPair, expectedDNS []string, expectedIPs []net.IP) {
	t.Helper()

	block, _ := pem.Decode(pair.CertPEM)
	if block == nil {
		t.Fatal("failed to decode server cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse server cert: %v", err)
	}

	// Check DNS names
	for _, dns := range expectedDNS {
		found := false
		for _, certDNS := range cert.DNSNames {
			if certDNS == dns {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected DNS name %q not found in cert SANs: %v", dns, cert.DNSNames)
		}
	}

	// Check IPs
	for _, ip := range expectedIPs {
		found := false
		for _, certIP := range cert.IPAddresses {
			if certIP.Equal(ip) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected IP %v not found in cert SANs: %v", ip, cert.IPAddresses)
		}
	}
}
