// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package chaperone

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnroll_WritesKeyAndCSR(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify key file exists with correct permissions
	keyInfo, err := os.Stat(result.KeyFile)
	if err != nil {
		t.Fatalf("key file not found: %v", err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file permissions: got %o, want 0600", perm)
	}

	// Verify CSR file exists with correct permissions
	csrInfo, err := os.Stat(result.CSRFile)
	if err != nil {
		t.Fatalf("CSR file not found: %v", err)
	}
	if perm := csrInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("CSR file permissions: got %o, want 0600", perm)
	}

	// Verify key is valid PEM
	keyPEM, err := os.ReadFile(result.KeyFile)
	if err != nil {
		t.Fatalf("reading key file: %v", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("key file does not contain valid PEM data")
	}
	if keyBlock.Type != "EC PRIVATE KEY" {
		t.Errorf("key PEM type: got %q, want %q", keyBlock.Type, "EC PRIVATE KEY")
	}

	// Verify CSR is valid PEM and parseable
	csrPEM, err := os.ReadFile(result.CSRFile)
	if err != nil {
		t.Fatalf("reading CSR file: %v", err)
	}
	csrBlock, _ := pem.Decode(csrPEM)
	if csrBlock == nil {
		t.Fatal("CSR file does not contain valid PEM data")
	}
	if csrBlock.Type != "CERTIFICATE REQUEST" {
		t.Errorf("CSR PEM type: got %q, want %q", csrBlock.Type, "CERTIFICATE REQUEST")
	}

	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		t.Fatalf("parsing CSR: %v", err)
	}
	if csr.Subject.CommonName != "chaperone" {
		t.Errorf("CSR CN: got %q, want %q", csr.Subject.CommonName, "chaperone")
	}
}

func TestEnroll_ParsesDNSAndIPs(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com,10.0.0.1,api.example.com,::1",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify DNS names
	wantDNS := []string{"proxy.example.com", "api.example.com"}
	if len(result.DNSNames) != len(wantDNS) {
		t.Fatalf("DNSNames: got %v, want %v", result.DNSNames, wantDNS)
	}
	for i, got := range result.DNSNames {
		if got != wantDNS[i] {
			t.Errorf("DNSNames[%d]: got %q, want %q", i, got, wantDNS[i])
		}
	}

	// Verify IPs
	wantIPs := []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("::1")}
	if len(result.IPs) != len(wantIPs) {
		t.Fatalf("IPs: got %v, want %v", result.IPs, wantIPs)
	}
	for i, got := range result.IPs {
		if !got.Equal(wantIPs[i]) {
			t.Errorf("IPs[%d]: got %v, want %v", i, got, wantIPs[i])
		}
	}

	// Verify CSR contains the SANs
	csrPEM, err := os.ReadFile(result.CSRFile)
	if err != nil {
		t.Fatalf("reading CSR file: %v", err)
	}
	csrBlock, _ := pem.Decode(csrPEM)
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		t.Fatalf("parsing CSR: %v", err)
	}

	if len(csr.DNSNames) != 2 {
		t.Errorf("CSR DNS SANs: got %v, want 2 entries", csr.DNSNames)
	}
	if len(csr.IPAddresses) != 2 {
		t.Errorf("CSR IP SANs: got %v, want 2 entries", csr.IPAddresses)
	}
}

func TestEnroll_EmptyDomains_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "",
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for empty domains, got nil")
	}
}

func TestEnroll_WhitespaceOnlyDomains_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "  , , ",
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only domains, got nil")
	}
}

func TestEnroll_CreatesOutputDir(t *testing.T) {
	t.Parallel()

	// Use a nested path that doesn't exist yet
	outDir := filepath.Join(t.TempDir(), "nested", "certs")

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the directory was created
	info, err := os.Stat(outDir)
	if err != nil {
		t.Fatalf("output directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("output path is not a directory")
	}

	// Verify files are in the created directory
	if filepath.Dir(result.KeyFile) != outDir {
		t.Errorf("key file not in output dir: got %q", result.KeyFile)
	}
	if filepath.Dir(result.CSRFile) != outDir {
		t.Errorf("CSR file not in output dir: got %q", result.CSRFile)
	}
}

func TestEnroll_DefaultValues(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify default CommonName
	csrPEM, err := os.ReadFile(result.CSRFile)
	if err != nil {
		t.Fatalf("reading CSR file: %v", err)
	}
	csrBlock, _ := pem.Decode(csrPEM)
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		t.Fatalf("parsing CSR: %v", err)
	}
	if csr.Subject.CommonName != "chaperone" {
		t.Errorf("default CommonName: got %q, want %q", csr.Subject.CommonName, "chaperone")
	}
}

func TestEnroll_CustomCommonName(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains:    "proxy.example.com",
		CommonName: "my-proxy",
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	csrPEM, err := os.ReadFile(result.CSRFile)
	if err != nil {
		t.Fatalf("reading CSR file: %v", err)
	}
	csrBlock, _ := pem.Decode(csrPEM)
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		t.Fatalf("parsing CSR: %v", err)
	}
	if csr.Subject.CommonName != "my-proxy" {
		t.Errorf("custom CommonName: got %q, want %q", csr.Subject.CommonName, "my-proxy")
	}
}

func TestEnroll_OutputDir_DefaultsToCurrentDir(t *testing.T) {
	// Cannot use t.Parallel() — t.Chdir changes process-wide state.

	// When OutputDir is empty, Enroll should default to "certs" subdirectory.
	// We run from a temp dir to avoid polluting the workspace.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains: "proxy.example.com",
		// OutputDir intentionally omitted — should default to "certs"
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The default output dir is "certs" (relative to cwd, which is tmpDir).
	if filepath.Dir(result.KeyFile) != "certs" {
		t.Errorf("default output dir: got %q, want %q", filepath.Dir(result.KeyFile), "certs")
	}

	// Verify the files were actually created in the cwd
	expectedDir := filepath.Join(tmpDir, "certs")
	if _, err := os.Stat(filepath.Join(expectedDir, "server.key")); err != nil {
		t.Errorf("key file not created in default dir: %v", err)
	}
}

func TestEnroll_FilePathsInResult(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	result, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantKey := filepath.Join(outDir, "server.key")
	wantCSR := filepath.Join(outDir, "server.csr")

	if result.KeyFile != wantKey {
		t.Errorf("KeyFile: got %q, want %q", result.KeyFile, wantKey)
	}
	if result.CSRFile != wantCSR {
		t.Errorf("CSRFile: got %q, want %q", result.CSRFile, wantCSR)
	}
}

func TestParseCSRDomains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantDNS []string
		wantIPs []net.IP
		wantErr bool
	}{
		{
			name:    "single DNS",
			input:   "example.com",
			wantDNS: []string{"example.com"},
			wantIPs: nil,
		},
		{
			name:    "single IP",
			input:   "10.0.0.1",
			wantDNS: nil,
			wantIPs: []net.IP{net.ParseIP("10.0.0.1")},
		},
		{
			name:    "mixed DNS and IPs",
			input:   "example.com,10.0.0.1,api.example.com,::1",
			wantDNS: []string{"example.com", "api.example.com"},
			wantIPs: []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("::1")},
		},
		{
			name:    "whitespace trimming",
			input:   " example.com , 10.0.0.1 ",
			wantDNS: []string{"example.com"},
			wantIPs: []net.IP{net.ParseIP("10.0.0.1")},
		},
		{
			name:    "empty entries skipped",
			input:   "example.com,,10.0.0.1",
			wantDNS: []string{"example.com"},
			wantIPs: []net.IP{net.ParseIP("10.0.0.1")},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only commas and spaces",
			input:   " , , ",
			wantErr: true,
		},
		{
			name:    "invalid DNS name with underscore",
			input:   "my_host.example.com",
			wantErr: true,
		},
		{
			name:    "DNS label starts with hyphen",
			input:   "-bad.example.com",
			wantErr: true,
		},
		{
			name:    "DNS label ends with hyphen",
			input:   "bad-.example.com",
			wantErr: true,
		},
		{
			name:    "double dot in DNS name",
			input:   "proxy..example.com",
			wantErr: true,
		},
		{
			name:    "special characters rejected",
			input:   "proxy!.example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dnsNames, ips, err := parseCSRDomains(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(dnsNames) != len(tt.wantDNS) {
				t.Fatalf("DNS names: got %v, want %v", dnsNames, tt.wantDNS)
			}
			for i, got := range dnsNames {
				if got != tt.wantDNS[i] {
					t.Errorf("DNS[%d]: got %q, want %q", i, got, tt.wantDNS[i])
				}
			}

			if len(ips) != len(tt.wantIPs) {
				t.Fatalf("IPs: got %v, want %v", ips, tt.wantIPs)
			}
			for i, got := range ips {
				if !got.Equal(tt.wantIPs[i]) {
					t.Errorf("IPs[%d]: got %v, want %v", i, got, tt.wantIPs[i])
				}
			}
		})
	}
}

func TestEnroll_ExistingFiles_ReturnsError(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	// Create files first.
	_, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("first enroll failed: %v", err)
	}

	// Second call should fail because files exist.
	_, err = Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err == nil {
		t.Fatal("expected error when files exist, got nil")
	}
	if !errors.Is(err, ErrFileExists) {
		t.Errorf("error type: got %v, want ErrFileExists", err)
	}
}

func TestEnroll_Force_OverwritesExistingFiles(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	// Create files first.
	result1, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "proxy.example.com",
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("first enroll failed: %v", err)
	}
	origKey, err := os.ReadFile(result1.KeyFile)
	if err != nil {
		t.Fatalf("reading original key: %v", err)
	}

	// Second call with Force should succeed and produce new key material.
	result2, err := Enroll(context.Background(), EnrollConfig{
		Domains:   "api.example.com",
		OutputDir: outDir,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("forced enroll failed: %v", err)
	}

	newKey, err := os.ReadFile(result2.KeyFile)
	if err != nil {
		t.Fatalf("reading new key: %v", err)
	}
	if string(origKey) == string(newKey) {
		t.Error("expected new key material after forced overwrite")
	}
}

func TestIsValidHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple hostname", "proxy", true},
		{"FQDN", "proxy.example.com", true},
		{"with hyphen", "my-proxy.example.com", true},
		{"single letter labels", "a.b.c", true},
		{"digits in labels", "proxy1.example2.com", true},
		{"uppercase", "Proxy.Example.COM", true},
		{"empty string", "", false},
		{"starts with hyphen", "-proxy.example.com", false},
		{"ends with hyphen", "proxy-.example.com", false},
		{"double dot", "proxy..example.com", false},
		{"underscore", "my_proxy.example.com", false},
		{"space", "proxy .example.com", false},
		{"special chars", "proxy!.example.com", false},
		{"trailing dot", "proxy.example.com.", false},
		{"label too long", strings.Repeat("a", 64) + ".com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isValidHostname(tt.input); got != tt.want {
				t.Errorf("isValidHostname(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
