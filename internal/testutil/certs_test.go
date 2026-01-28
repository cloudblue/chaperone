// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"strings"
	"testing"
	"time"
)

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
}

func TestGenerateCertBundle_Success(t *testing.T) {
	bundle, err := GenerateCertBundle()
	if err != nil {
		t.Fatalf("GenerateCertBundle failed: %v", err)
	}

	// Verify all components are present
	if len(bundle.CA.CertPEM) == 0 {
		t.Error("CA CertPEM is empty")
	}
	if len(bundle.Server.CertPEM) == 0 {
		t.Error("Server CertPEM is empty")
	}
	if len(bundle.Client.CertPEM) == 0 {
		t.Error("Client CertPEM is empty")
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

	if len(expired.CertPEM) == 0 {
		t.Error("Expired cert CertPEM is empty")
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
}

// =============================================================================
// Error Path Tests
// =============================================================================

func TestGenerateServerCert_InvalidCACert_ReturnsError(t *testing.T) {
	// Arrange - invalid CA cert PEM
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	// Act
	_, err := GenerateServerCert(invalidCA, time.Hour)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

func TestGenerateClientCert_InvalidCACert_ReturnsError(t *testing.T) {
	// Arrange - invalid CA cert PEM
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	// Act
	_, err := GenerateClientCert(invalidCA, time.Hour)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

func TestGenerateServerCert_InvalidCAKey_ReturnsError(t *testing.T) {
	// Arrange - valid CA cert but invalid key
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	invalidCA := &CertPair{
		CertPEM: ca.CertPEM,
		KeyPEM:  []byte("not a valid key"),
	}

	// Act
	_, err = GenerateServerCert(invalidCA, time.Hour)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid CA key, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA key") {
		t.Errorf("error = %q, want to contain 'parsing CA key'", err.Error())
	}
}

func TestGenerateExpiredClientCert_InvalidCACert_ReturnsError(t *testing.T) {
	// Arrange - invalid CA cert PEM
	invalidCA := &CertPair{
		CertPEM: []byte("not a valid PEM"),
		KeyPEM:  []byte("not a valid key"),
	}

	// Act
	_, err := GenerateExpiredClientCert(invalidCA)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA cert") {
		t.Errorf("error = %q, want to contain 'parsing CA cert'", err.Error())
	}
}

func TestGenerateExpiredClientCert_InvalidCAKey_ReturnsError(t *testing.T) {
	// Arrange - valid CA cert but invalid key
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	invalidCA := &CertPair{
		CertPEM: ca.CertPEM,
		KeyPEM:  []byte("not a valid key"),
	}

	// Act
	_, err = GenerateExpiredClientCert(invalidCA)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid CA key, got nil")
	}
	if !strings.Contains(err.Error(), "parsing CA key") {
		t.Errorf("error = %q, want to contain 'parsing CA key'", err.Error())
	}
}
