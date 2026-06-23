// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync/atomic"
)

// TLS-related errors.
var (
	// ErrInvalidCACert indicates the CA certificate could not be parsed.
	ErrInvalidCACert = errors.New("invalid CA certificate")
)

// CertProvider holds the active TLS server certificate and allows atomic
// hot-swap without restarting the server or dropping in-flight connections.
//
// The GetCertificate method is wired into tls.Config so every new TLS
// handshake reads the current certificate. Swap replaces the certificate
// atomically; connections already past the handshake are unaffected.
type CertProvider struct {
	cert atomic.Pointer[tls.Certificate]
}

// NewCertProvider creates a CertProvider holding the given certificate.
func NewCertProvider(cert tls.Certificate) *CertProvider {
	p := &CertProvider{}
	p.cert.Store(&cert)
	return p
}

// GetCertificate implements the tls.Config.GetCertificate callback.
// It is called for every new TLS handshake.
func (p *CertProvider) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return p.cert.Load(), nil
}

// Swap atomically replaces the active certificate. In-flight TLS connections
// that have already completed the handshake continue using their negotiated
// certificate; only new handshakes will use the replacement.
func (p *CertProvider) Swap(newCert tls.Certificate) {
	p.cert.Store(&newCert)
}

// Current returns a copy of the active certificate.
func (p *CertProvider) Current() tls.Certificate {
	return *p.cert.Load()
}

// NewTLSConfig creates a TLS configuration for the proxy server with mTLS.
//
// The configuration enforces:
//   - TLS 1.3 minimum (per Design Spec security requirements)
//   - Client certificate verification (mTLS mandatory)
//   - Dynamic server certificate via CertProvider.GetCertificate (enables hot-swap)
//
// Parameters:
//   - caCertPEM: PEM-encoded CA certificate(s) for validating client certificates
//   - serverCertPEM: PEM-encoded server certificate
//   - serverKeyPEM: PEM-encoded server private key
//
// Returns the tls.Config and the CertProvider so the caller can hot-swap the
// certificate later without restarting the server.
func NewTLSConfig(caCertPEM, serverCertPEM, serverKeyPEM []byte) (*tls.Config, *CertProvider, error) {
	// Load CA certificate pool for client validation
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, nil, fmt.Errorf("%w: failed to parse CA certificate PEM", ErrInvalidCACert)
	}

	// Load server certificate into the provider
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("loading server certificate: %w", err)
	}

	provider := NewCertProvider(serverCert)

	return &tls.Config{
		// GetCertificate is the sole certificate source. Omitting Certificates
		// ensures this callback is invoked for every handshake — including
		// clients that connect by IP and send no SNI — so hot-swap always works.
		GetCertificate: provider.GetCertificate,

		// Client authentication: mTLS mandatory per Design Spec Section 5.3
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caCertPool,

		// TLS 1.3 minimum per security requirements
		MinVersion: tls.VersionTLS13,
	}, provider, nil
}
