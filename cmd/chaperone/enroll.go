// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/cloudblue/chaperone"
)

// enrollCmd handles the "enroll" subcommand for production CA enrollment.
// It generates a key pair and CSR that can be submitted to a CA for signing.
//
//nolint:funlen // CLI command handlers are acceptable to be longer
func enrollCmd(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	domainsFlag := fs.String("domains", "", "Domains/IPs for the server certificate (comma-separated, required)")
	commonName := fs.String("cn", chaperone.DefaultCommonName, "Common Name for the certificate")
	outputDir := fs.String("out", chaperone.DefaultOutputDir, "Output directory for key and CSR files")
	force := fs.Bool("force", false, "Overwrite existing key and CSR files")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: chaperone enroll [options]

Generate a key pair and Certificate Signing Request (CSR) for production
CA enrollment. The CSR can be submitted to Connect or another CA for signing.

This command generates ECDSA P-256 keys for optimal security and performance.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Example:
  chaperone enroll --domains proxy.example.com,10.0.0.1
  chaperone enroll --domains proxy.example.com --cn my-proxy --out /etc/chaperone/certs

After running this command:
  1. Keep server.key secure (never share it)
  2. Submit server.csr to your CA (Connect Portal, Vault, etc.)
  3. Place the signed server.crt in the output directory
  4. Start Chaperone: ./chaperone
`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *domainsFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --domains is required")
		fmt.Fprintln(os.Stderr, "Example: chaperone enroll --domains proxy.example.com,10.0.0.1")
		os.Exit(1)
	}

	fmt.Println("Generating key pair and CSR for production CA enrollment...")
	fmt.Println()

	result, err := chaperone.Enroll(context.Background(), chaperone.EnrollConfig{
		Domains:    *domainsFlag,
		CommonName: *commonName,
		OutputDir:  *outputDir,
		Force:      *force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  ✓ %s\n", result.KeyFile)
	fmt.Printf("  ✓ %s\n", result.CSRFile)

	printEnrollmentInstructions(*outputDir, result.DNSNames, result.IPs)
}

// printEnrollmentInstructions prints the next steps after CSR generation.
func printEnrollmentInstructions(outputDir string, dnsNames []string, ips []net.IP) {
	fmt.Println()
	fmt.Println("CSR generated with Subject Alternative Names:")
	for _, dns := range dnsNames {
		fmt.Printf("  • %s (DNS)\n", dns)
	}
	for _, ip := range ips {
		fmt.Printf("  • %s (IP)\n", ip)
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Keep server.key secure (never share it)")
	fmt.Println("  2. Submit server.csr to your CA:")
	fmt.Println("     • Connect Portal: Upload via Distributor Dashboard")
	fmt.Println("     • HashiCorp Vault: vault write pki/sign/chaperone csr=@server.csr")
	fmt.Println("     • OpenSSL (self-sign): openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -out server.crt")
	fmt.Println("  3. Place the signed server.crt in:", outputDir)
	fmt.Println("  4. Start Chaperone: ./chaperone")
	fmt.Println()
	fmt.Println("To view CSR contents:")
	fmt.Printf("  openssl req -in %s/server.csr -text -noout\n", outputDir)
}
