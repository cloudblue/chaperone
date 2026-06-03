// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
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
// handshake reads the current certificate under a read lock. Swap replaces
// the certificate under a write lock; connections already past the handshake
// are unaffected.
type CertProvider struct {
	mu   sync.RWMutex
	cert *tls.Certificate
}

// NewCertProvider creates a CertProvider holding the given certificate.
func NewCertProvider(cert tls.Certificate) *CertProvider {
	return &CertProvider{cert: &cert}
}

// GetCertificate implements the tls.Config.GetCertificate callback.
// It is called for every new TLS handshake.
func (p *CertProvider) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cert, nil
}

// Swap atomically replaces the active certificate. In-flight TLS connections
// that have already completed the handshake continue using their negotiated
// certificate; only new handshakes will use the replacement.
func (p *CertProvider) Swap(newCert tls.Certificate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cert = &newCert
}

// Current returns a copy of the active certificate.
func (p *CertProvider) Current() tls.Certificate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return *p.cert
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

	// Parse and attach Leaf so callers can read NotAfter without a second parse.
	leaf, err := x509.ParseCertificate(serverCert.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parsing server certificate leaf: %w", err)
	}
	serverCert.Leaf = leaf

	provider := NewCertProvider(serverCert)

	return &tls.Config{
		// GetCertificate enables hot-swap: called for SNI connections or when
		// Certificates is empty. Certificates is also populated so that
		// non-SNI listeners (e.g. httptest) serve the correct cert without
		// falling through to the listener's own self-signed certificate.
		GetCertificate: provider.GetCertificate,
		Certificates:   []tls.Certificate{serverCert},

		// Client authentication: mTLS mandatory per Design Spec Section 5.3
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caCertPool,

		// TLS 1.3 minimum per security requirements
		MinVersion: tls.VersionTLS13,
	}, provider, nil
}
