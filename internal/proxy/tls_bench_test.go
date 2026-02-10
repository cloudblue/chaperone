// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkTLSHandshake benchmarks TLS 1.3 handshake time.
// Target: < 5ms for TLS (mTLS would be higher due to client cert exchange)
func BenchmarkTLSHandshake(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	server.StartTLS()
	defer server.Close()

	// Disable keep-alives to force a new TLS handshake per request.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // #nosec - benchmark only, not production code
			},
			DisableKeepAlives: true,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

// BenchmarkTLSHandshake_WithKeepAlive benchmarks TLS with connection reuse.
// This shows the benefit of keep-alives (no handshake per request).
func BenchmarkTLSHandshake_WithKeepAlive(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
	server.StartTLS()
	defer server.Close()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // #nosec - benchmark only, not production code
			},
			// Keep-alives enabled (default)
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

// BenchmarkCertificateVerification benchmarks x509 certificate verification.
// This is part of the mTLS handshake and contributes to connection setup time.
func BenchmarkCertificateVerification(b *testing.B) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()

	// Get the server's certificate for benchmarking verification
	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true, // #nosec - benchmark only, not production code
		},
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", server.Listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	tlsConn := conn.(*tls.Conn)
	certs := tlsConn.ConnectionState().PeerCertificates
	conn.Close()

	if len(certs) == 0 {
		b.Skip("No certificates available for benchmark")
	}

	cert := certs[0]
	certPool := x509.NewCertPool()
	certPool.AddCert(cert)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		opts := x509.VerifyOptions{
			Roots:       certPool,
			CurrentTime: cert.NotBefore,
		}
		// Self-signed cert verification may return error, but we're
		// benchmarking the verification operation itself.
		_, _ = cert.Verify(opts)
	}
}
