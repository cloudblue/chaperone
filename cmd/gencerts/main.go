// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Command gencerts generates test certificates for mTLS testing.
// These certificates are for DEVELOPMENT/TESTING ONLY.
//
// Usage:
//
//	go run ./cmd/gencerts
//	go run ./cmd/gencerts -domains "myserver.local,192.168.1.100"
//
// This will create a certs/ directory with:
//   - ca.crt      - CA certificate (used by server to verify clients)
//   - server.crt  - Server certificate
//   - server.key  - Server private key
//   - client.crt  - Client certificate (i.e. for curl)
//   - client.key  - Client private key
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudblue/chaperone/internal/testutil"
)

func main() {
	// Parse flags
	domainsFlag := flag.String("domains", "", "Additional domains/IPs for server certificate (comma-separated)")
	flag.Parse()

	// Parse extra domains and IPs
	extraDNSNames, extraIPs := parseDomainsFlag(*domainsFlag)

	// Generate and write certificates
	if err := generateCertificates(extraDNSNames, extraIPs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print usage instructions
	printUsageInstructions(extraDNSNames, extraIPs)
}

// parseDomainsFlag parses the comma-separated domains flag into DNS names and IPs.
func parseDomainsFlag(domainsFlag string) ([]string, []net.IP) {
	var extraDNSNames []string
	var extraIPs []net.IP

	if domainsFlag == "" {
		return extraDNSNames, extraIPs
	}

	for _, entry := range strings.Split(domainsFlag, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Check if it's an IP address
		if ip := net.ParseIP(entry); ip != nil {
			extraIPs = append(extraIPs, ip)
		} else {
			// Treat as DNS name
			extraDNSNames = append(extraDNSNames, entry)
		}
	}

	return extraDNSNames, extraIPs
}

// generateCertificates creates all certificates and writes them to the certs directory.
func generateCertificates(extraDNSNames []string, extraIPs []net.IP) error {
	certsDir := "certs"
	if err := os.MkdirAll(certsDir, 0o750); err != nil {
		return fmt.Errorf("creating certs directory: %w", err)
	}

	fmt.Println("Generating test certificates for mTLS...")
	fmt.Println("⚠️  WARNING: These certificates are for DEVELOPMENT/TESTING ONLY!")
	fmt.Println()

	// Generate CA (valid for 1 year)
	ca, err := testutil.GenerateCA(365 * 24 * time.Hour)
	if err != nil {
		return fmt.Errorf("generating CA: %w", err)
	}

	// Generate server certificate (valid for 1 year)
	server, err := testutil.GenerateServerCertWithSANs(ca, 365*24*time.Hour, extraDNSNames, extraIPs)
	if err != nil {
		return fmt.Errorf("generating server cert: %w", err)
	}

	// Generate client certificate (valid for 1 year)
	client, err := testutil.GenerateClientCert(ca, 365*24*time.Hour)
	if err != nil {
		return fmt.Errorf("generating client cert: %w", err)
	}

	// Write files
	files := map[string][]byte{
		"ca.crt":     ca.CertPEM,
		"server.crt": server.CertPEM,
		"server.key": server.KeyPEM,
		"client.crt": client.CertPEM,
		"client.key": client.KeyPEM,
	}

	for name, content := range files {
		path := filepath.Join(certsDir, name)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Printf("  ✓ %s\n", path)
	}

	return nil
}

// printUsageInstructions prints certificate info and usage examples.
func printUsageInstructions(extraDNSNames []string, extraIPs []net.IP) {
	fmt.Println()
	fmt.Println("Server certificate valid for:")
	fmt.Println("  • localhost")
	fmt.Println("  • 127.0.0.1")
	fmt.Println("  • ::1 (IPv6)")
	for _, dns := range extraDNSNames {
		fmt.Printf("  • %s\n", dns)
	}
	for _, ip := range extraIPs {
		fmt.Printf("  • %s\n", ip)
	}

	fmt.Println()
	fmt.Println("Certificates generated successfully!")
	fmt.Println()
	fmt.Println("To start the proxy with mTLS (Mode A):")
	fmt.Println("  go run ./cmd/chaperone")
	fmt.Println()
	fmt.Println("To test with curl:")
	fmt.Println("  curl --cacert certs/ca.crt \\")
	fmt.Println("       --cert certs/client.crt \\")
	fmt.Println("       --key certs/client.key \\")
	fmt.Println("       https://localhost:8443/_ops/health")
	fmt.Println()
	fmt.Println("To add custom domains/IPs to the server certificate:")
	fmt.Println("  go run ./cmd/gencerts -domains \"myserver.local,192.168.1.100\"")
	fmt.Println("  make gencerts DOMAINS=\"myserver.local,192.168.1.100\"")
}
