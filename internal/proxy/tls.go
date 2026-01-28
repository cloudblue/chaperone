// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
)

// TLS-related errors.
var (
	// ErrInvalidCACert indicates the CA certificate could not be parsed.
	ErrInvalidCACert = errors.New("invalid CA certificate")
)

// NewTLSConfig creates a TLS configuration for the proxy server with mTLS.
//
// The configuration enforces:
//   - TLS 1.3 minimum (per Design Spec security requirements)
//   - Client certificate verification (mTLS mandatory)
//   - Server certificate presentation
//
// Parameters:
//   - caCertPEM: PEM-encoded CA certificate(s) for validating client certificates
//   - serverCertPEM: PEM-encoded server certificate
//   - serverKeyPEM: PEM-encoded server private key
//
// Returns an error if any certificate cannot be parsed.
func NewTLSConfig(caCertPEM, serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	// Load CA certificate pool for client validation
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("%w: failed to parse CA certificate PEM", ErrInvalidCACert)
	}

	// Load server certificate
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading server certificate: %w", err)
	}

	return &tls.Config{
		// Server certificate
		Certificates: []tls.Certificate{serverCert},

		// Client authentication: mTLS mandatory per Design Spec Section 5.3
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caCertPool,

		// TLS 1.3 minimum per security requirements
		MinVersion: tls.VersionTLS13,
	}, nil
}
