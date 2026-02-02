// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package cli provides shared utilities for command-line interfaces.
package cli

import (
	"net"
	"strings"
)

// ParseDomainsFlag parses a comma-separated domains flag into DNS names and IP addresses.
// Each entry is trimmed of whitespace and classified as either an IP address or DNS name.
// Empty entries are ignored.
//
// Example:
//
//	dnsNames, ips := ParseDomainsFlag("example.com, 10.0.0.1, localhost")
//	// dnsNames: ["example.com", "localhost"]
//	// ips: [10.0.0.1]
func ParseDomainsFlag(domainsFlag string) (dnsNames []string, ips []net.IP) {
	if domainsFlag == "" {
		return nil, nil
	}

	for _, entry := range strings.Split(domainsFlag, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Check if it's an IP address
		if ip := net.ParseIP(entry); ip != nil {
			ips = append(ips, ip)
		} else {
			// Treat as DNS name
			dnsNames = append(dnsNames, entry)
		}
	}

	return dnsNames, ips
}
