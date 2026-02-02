// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Command gencerts generates test certificates for mTLS DEVELOPMENT/TESTING.
//
// This tool is for LOCAL DEVELOPMENT ONLY. It generates self-signed certificates
// (ECDSA P-256) that allow you to test mTLS locally with curl or other HTTP clients.
//
// For PRODUCTION enrollment, use: ./chaperone enroll (see design spec 8.2)
//
// Usage:
//
//	go run ./cmd/gencerts
//	go run ./cmd/gencerts -domains "myserver.local,192.168.1.100"
//	make gencerts
//	make gencerts DOMAINS="myserver.local,192.168.1.100"
//
// This will create a certs/ directory with:
//   - ca.crt      - Test CA certificate (used by server to verify clients)
//   - server.crt  - Server certificate (self-signed by test CA)
//   - server.key  - Server private key
//   - client.crt  - Client certificate for testing (e.g., curl --cert)
//   - client.key  - Client private key
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudblue/chaperone/internal/cli"
	"github.com/cloudblue/chaperone/pkg/crypto"
)

func main() {
	// Parse flags
	domainsFlag := flag.String("domains", "", "Additional domains/IPs for server certificate (comma-separated)")
	flag.Parse()

	// Parse extra domains and IPs
	extraDNSNames, extraIPs := cli.ParseDomainsFlag(*domainsFlag)

	// Generate and write test certificates
	if err := generateCertificates(extraDNSNames, extraIPs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printUsageInstructions(extraDNSNames, extraIPs)
}

// generateCertificates creates all certificates and writes them to the certs directory.
func generateCertificates(extraDNSNames []string, extraIPs []net.IP) error {
	certsDir := "certs"
	if err := os.MkdirAll(certsDir, 0o750); err != nil {
		return fmt.Errorf("creating certs directory: %w", err)
	}

	fmt.Println("Generating test certificates for mTLS development...")
	fmt.Println("⚠️  WARNING: These certificates are for DEVELOPMENT/TESTING ONLY!")
	fmt.Println("    For production, use: ./chaperone enroll")
	fmt.Println()

	// Generate CA (valid for 1 year)
	ca, err := crypto.GenerateCA(365 * 24 * time.Hour)
	if err != nil {
		return fmt.Errorf("generating CA: %w", err)
	}

	// Generate server certificate (valid for 1 year)
	server, err := crypto.GenerateServerCertWithSANs(ca, 365*24*time.Hour, extraDNSNames, extraIPs)
	if err != nil {
		return fmt.Errorf("generating server cert: %w", err)
	}

	// Generate client certificate (valid for 1 year)
	client, err := crypto.GenerateClientCert(ca, 365*24*time.Hour)
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
	fmt.Println("To start the proxy with mTLS:")
	fmt.Println("  go run ./cmd/chaperone")
	fmt.Println()
	fmt.Println("To test with curl:")
	fmt.Println("  curl --cacert certs/ca.crt \\")
	fmt.Println("       --cert certs/client.crt \\")
	fmt.Println("       --key certs/client.key \\")
	fmt.Println("       https://localhost:8443/_ops/health")
	fmt.Println()
	fmt.Println("To add custom domains/IPs:")
	fmt.Println("  make gencerts DOMAINS=\"myserver.local,192.168.1.100\"")
}
