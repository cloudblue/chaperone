// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net"
	"testing"
)

func TestParseDomainsFlag_MixedInput_SeparatesDNSAndIPs(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDNS     []string
		wantIPCount int
	}{
		{
			name:        "empty string returns nil slices",
			input:       "",
			wantDNS:     nil,
			wantIPCount: 0,
		},
		{
			name:        "single DNS name",
			input:       "example.com",
			wantDNS:     []string{"example.com"},
			wantIPCount: 0,
		},
		{
			name:        "single IPv4 address",
			input:       "192.168.1.1",
			wantDNS:     nil,
			wantIPCount: 1,
		},
		{
			name:        "single IPv6 address",
			input:       "::1",
			wantDNS:     nil,
			wantIPCount: 1,
		},
		{
			name:        "mixed DNS and IPv4",
			input:       "example.com,10.0.0.1,localhost",
			wantDNS:     []string{"example.com", "localhost"},
			wantIPCount: 1,
		},
		{
			name:        "handles whitespace",
			input:       " example.com , 10.0.0.1 , localhost ",
			wantDNS:     []string{"example.com", "localhost"},
			wantIPCount: 1,
		},
		{
			name:        "ignores empty entries",
			input:       "example.com,,localhost,",
			wantDNS:     []string{"example.com", "localhost"},
			wantIPCount: 0,
		},
		{
			name:        "multiple IPs",
			input:       "192.168.1.1,10.0.0.1,::1",
			wantDNS:     nil,
			wantIPCount: 3,
		},
		{
			name:        "complex mixed input",
			input:       "proxy.example.com, 10.0.0.1, internal.local, 192.168.1.100, ::1",
			wantDNS:     []string{"proxy.example.com", "internal.local"},
			wantIPCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDNS, gotIPs := ParseDomainsFlag(tt.input)

			// Check DNS names
			if tt.wantDNS == nil {
				if gotDNS != nil {
					t.Errorf("DNS names = %v, want nil", gotDNS)
				}
			} else {
				if len(gotDNS) != len(tt.wantDNS) {
					t.Errorf("DNS names count = %d, want %d", len(gotDNS), len(tt.wantDNS))
				}
				for i, want := range tt.wantDNS {
					if i < len(gotDNS) && gotDNS[i] != want {
						t.Errorf("DNS names[%d] = %q, want %q", i, gotDNS[i], want)
					}
				}
			}

			// Check IP count
			if len(gotIPs) != tt.wantIPCount {
				t.Errorf("IP count = %d, want %d", len(gotIPs), tt.wantIPCount)
			}
		})
	}
}

func TestParseDomainsFlag_IPv4Addresses_ParsedCorrectly(t *testing.T) {
	input := "192.168.1.1,10.0.0.1,172.16.0.1"
	_, ips := ParseDomainsFlag(input)

	expectedIPs := []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"}
	if len(ips) != len(expectedIPs) {
		t.Fatalf("IP count = %d, want %d", len(ips), len(expectedIPs))
	}

	for i, expected := range expectedIPs {
		if !ips[i].Equal(net.ParseIP(expected)) {
			t.Errorf("IP[%d] = %v, want %v", i, ips[i], expected)
		}
	}
}

func TestParseDomainsFlag_IPv6Addresses_ParsedCorrectly(t *testing.T) {
	input := "::1,fe80::1,2001:db8::1"
	_, ips := ParseDomainsFlag(input)

	if len(ips) != 3 {
		t.Fatalf("IP count = %d, want 3", len(ips))
	}

	// Verify each is a valid IPv6
	for i, ip := range ips {
		if ip.To4() != nil && i == 0 {
			// ::1 can be represented as IPv4-mapped, that's ok
			continue
		}
		if ip == nil {
			t.Errorf("IP[%d] is nil", i)
		}
	}
}
