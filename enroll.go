// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package chaperone

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudblue/chaperone/pkg/crypto"
)

// DefaultCommonName is the default CN for CSR generation.
const DefaultCommonName = "chaperone"

// DefaultOutputDir is the default directory for enrollment output files.
const DefaultOutputDir = "certs"

// ErrFileExists is returned when enrollment output files already exist
// and Force is not set. This prevents accidentally overwriting a private
// key that may already have a signed certificate issued against it.
var ErrFileExists = errors.New("file already exists")

// EnrollConfig configures CSR generation for production CA enrollment.
//
// See Design Spec Section 8.2 for the full enrollment workflow.
type EnrollConfig struct {
	// Domains is a comma-separated list of DNS names and IP addresses
	// for the server certificate's Subject Alternative Names.
	// Example: "proxy.example.com,10.0.0.1"
	Domains string

	// CommonName is the certificate's Common Name field.
	// Default: "chaperone"
	CommonName string

	// OutputDir is the directory where server.key and server.csr are written.
	// The directory is created if it does not exist.
	// Default: "certs"
	OutputDir string

	// Force allows overwriting existing key and CSR files.
	// When false (default), Enroll returns ErrFileExists if server.key
	// or server.csr already exist in OutputDir.
	Force bool
}

// EnrollResult contains the output of a successful enrollment.
type EnrollResult struct {
	KeyFile  string   // Path to the generated private key
	CSRFile  string   // Path to the generated CSR
	DNSNames []string // DNS SANs included in the CSR
	IPs      []net.IP // IP SANs included in the CSR
}

// Enroll generates an ECDSA P-256 key pair and Certificate Signing Request
// for production CA enrollment. The CSR can be submitted to a CA (Connect
// Portal, HashiCorp Vault, internal PKI, etc.) to obtain a signed server
// certificate for mTLS.
//
// This is the programmatic equivalent of "chaperone enroll --domains ...".
//
// Example (Distributor's main.go):
//
//	if len(os.Args) > 1 && os.Args[1] == "enroll" {
//	    result, err := chaperone.Enroll(context.Background(), chaperone.EnrollConfig{
//	        Domains: "proxy.example.com,10.0.0.1",
//	    })
//	    if err != nil {
//	        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
//	        os.Exit(1)
//	    }
//	    fmt.Printf("CSR written to %s\n", result.CSRFile)
//	    fmt.Printf("Key written to %s\n", result.KeyFile)
//	    return
//	}
//
// See Design Spec Section 8.2 for the full enrollment workflow.
func Enroll(_ context.Context, cfg EnrollConfig) (*EnrollResult, error) {
	// Apply defaults.
	if cfg.CommonName == "" {
		cfg.CommonName = DefaultCommonName
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = DefaultOutputDir
	}

	// Parse and validate domains.
	dnsNames, ips, err := parseCSRDomains(cfg.Domains)
	if err != nil {
		return nil, fmt.Errorf("parsing domains: %w", err)
	}

	// Create output directory.
	err = os.MkdirAll(cfg.OutputDir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Generate CSR via pkg/crypto.
	bundle, err := crypto.GenerateServerCSR(cfg.CommonName, dnsNames, ips)
	if err != nil {
		return nil, fmt.Errorf("generating CSR: %w", err)
	}

	// Write files with restrictive permissions (0600).
	keyPath := filepath.Join(cfg.OutputDir, "server.key")
	csrPath := filepath.Join(cfg.OutputDir, "server.csr")

	// Guard against accidental overwrite of existing key material.
	if !cfg.Force {
		for _, path := range []string{keyPath, csrPath} {
			if _, err := os.Stat(path); err == nil {
				return nil, fmt.Errorf("%s: %w (use Force to overwrite)", path, ErrFileExists)
			}
		}
	}

	if err := os.WriteFile(keyPath, bundle.KeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("writing key file: %w", err)
	}
	if err := os.WriteFile(csrPath, bundle.CSRPEM, 0o600); err != nil {
		return nil, fmt.Errorf("writing CSR file: %w", err)
	}

	return &EnrollResult{
		KeyFile:  keyPath,
		CSRFile:  csrPath,
		DNSNames: dnsNames,
		IPs:      ips,
	}, nil
}

// parseCSRDomains parses a comma-separated string of DNS names and IP
// addresses into separate slices. Each entry is trimmed of whitespace
// and classified as either an IP address or DNS name. Empty entries are
// ignored.
//
// Returns an error if no valid DNS names or IP addresses are found.
func parseCSRDomains(domains string) (dnsNames []string, ips []net.IP, err error) {
	for _, entry := range strings.Split(domains, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			ips = append(ips, ip)
		} else {
			if !isValidHostname(entry) {
				return nil, nil, fmt.Errorf("invalid DNS name %q", entry)
			}
			dnsNames = append(dnsNames, entry)
		}
	}

	if len(dnsNames) == 0 && len(ips) == 0 {
		return nil, nil, fmt.Errorf("no valid DNS names or IP addresses in domains %q", domains)
	}

	return dnsNames, ips, nil
}

// isValidHostname checks whether name is a valid DNS hostname per RFC 952/1123.
// Labels must be 1-63 characters of ASCII letters, digits, or hyphens, must not
// start or end with a hyphen, and the total length must not exceed 253 characters.
func isValidHostname(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}

	for _, label := range strings.Split(name, ".") {
		if !isValidLabel(label) {
			return false
		}
	}

	return true
}

// isValidLabel checks whether a single DNS label conforms to RFC 952/1123.
func isValidLabel(label string) bool {
	n := len(label)
	if n == 0 || n > 63 {
		return false
	}
	if label[0] == '-' || label[n-1] == '-' {
		return false
	}
	for _, c := range label {
		if !isHostnameChar(c) {
			return false
		}
	}
	return true
}

// isHostnameChar reports whether c is a valid character in a DNS label
// (ASCII letter, digit, or hyphen).
func isHostnameChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '-'
}
