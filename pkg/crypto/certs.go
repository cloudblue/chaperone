// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package crypto provides cryptographic utilities for certificate generation.
// This package is used by both test utilities and production tooling.
package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// Curve is the elliptic curve used for all generated keys.
// ECDSA P-256 provides strong security with better performance than RSA.
var Curve = elliptic.P256()

// CertPair holds PEM-encoded certificate and key pair.
type CertPair struct {
	CertPEM []byte
	KeyPEM  []byte
}

// CSRBundle holds a CSR and its corresponding private key.
type CSRBundle struct {
	CSRPEM []byte
	KeyPEM []byte
}

// CertBundle holds a complete certificate bundle (CA, server, client).
// Useful for setting up test mTLS scenarios.
type CertBundle struct {
	CA     CertPair
	Server CertPair
	Client CertPair
}

// GenerateCertBundle generates a complete certificate bundle for testing.
// Includes a CA, server cert, and client cert, all valid for 1 hour.
func GenerateCertBundle() (*CertBundle, error) {
	ca, err := GenerateCA(time.Hour)
	if err != nil {
		return nil, err
	}

	server, err := GenerateServerCert(ca, time.Hour)
	if err != nil {
		return nil, err
	}

	client, err := GenerateClientCert(ca, time.Hour)
	if err != nil {
		return nil, err
	}

	return &CertBundle{
		CA:     *ca,
		Server: *server,
		Client: *client,
	}, nil
}

// GenerateCA generates a self-signed CA certificate.
// The CA is valid for the specified duration.
func GenerateCA(validFor time.Duration) (*CertPair, error) {
	priv, err := ecdsa.GenerateKey(Curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating CA key: %w", err)
	}

	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("creating CA certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling CA key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CertPair{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// GenerateServerCert generates a server certificate signed by the given CA.
// The certificate is valid for localhost and 127.0.0.1 by default.
func GenerateServerCert(caCert *CertPair, validFor time.Duration) (*CertPair, error) {
	return GenerateServerCertWithSANs(caCert, validFor, nil, nil)
}

// GenerateServerCertWithSANs generates a server certificate with custom Subject Alternative Names.
// Default SANs (localhost, 127.0.0.1, ::1) are always included.
// Any provided extraDNSNames and extraIPs are added to the defaults.
func GenerateServerCertWithSANs(caCert *CertPair, validFor time.Duration, extraDNSNames []string, extraIPs []net.IP) (*CertPair, error) {
	priv, err := ecdsa.GenerateKey(Curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, err
	}

	caCertParsed, caKey, err := ParseCA(caCert)
	if err != nil {
		return nil, err
	}

	// Default DNS names with preallocated capacity
	dnsNames := make([]string, 0, 2+len(extraDNSNames))
	dnsNames = append(dnsNames, "localhost", "127.0.0.1")
	dnsNames = append(dnsNames, extraDNSNames...)

	// Default IPs with preallocated capacity
	ips := make([]net.IP, 0, 2+len(extraIPs))
	ips = append(ips, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))
	ips = append(ips, extraIPs...)

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(validFor),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ips,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCertParsed, &priv.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CertPair{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// GenerateClientCert generates a client certificate signed by the given CA.
func GenerateClientCert(caCert *CertPair, validFor time.Duration) (*CertPair, error) {
	priv, err := ecdsa.GenerateKey(Curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, err
	}

	caCertParsed, caKey, err := ParseCA(caCert)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "test-client",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(validFor),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCertParsed, &priv.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CertPair{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// GenerateExpiredClientCert generates a client certificate that is already expired.
// This is useful for testing certificate validation.
func GenerateExpiredClientCert(caCert *CertPair) (*CertPair, error) {
	priv, err := ecdsa.GenerateKey(Curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, err
	}

	caCertParsed, caKey, err := ParseCA(caCert)
	if err != nil {
		return nil, err
	}

	// Create expired certificate (NotAfter in the past)
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "expired-client",
		},
		NotBefore: time.Now().Add(-2 * time.Hour),
		NotAfter:  time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCertParsed, &priv.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CertPair{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// ErrEmptySANs is returned when a CSR is generated with no Subject Alternative Names.
var ErrEmptySANs = fmt.Errorf("at least one DNS name or IP address is required")

// GenerateServerCSR generates a Certificate Signing Request for a server certificate.
// The CSR can be submitted to an external CA (e.g., Connect) for signing.
// The dnsNames and ips are included as Subject Alternative Names in the CSR.
//
// Returns ErrEmptySANs if both dnsNames and ips are empty, as a certificate
// without SANs would not be useful for TLS.
//
// This function is intended for PRODUCTION enrollment workflows.
func GenerateServerCSR(commonName string, dnsNames []string, ips []net.IP) (*CSRBundle, error) {
	if len(dnsNames) == 0 && len(ips) == 0 {
		return nil, ErrEmptySANs
	}

	priv, err := ecdsa.GenerateKey(Curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: commonName,
		},
		DNSNames:    dnsNames,
		IPAddresses: ips,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, priv)
	if err != nil {
		return nil, fmt.Errorf("creating CSR: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling key: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CSRBundle{CSRPEM: csrPEM, KeyPEM: keyPEM}, nil
}

// ParseCA parses a CA certificate and key from PEM-encoded data.
func ParseCA(caCert *CertPair) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	caBlock, _ := pem.Decode(caCert.CertPEM)
	if caBlock == nil {
		return nil, nil, fmt.Errorf("parsing CA cert: no valid PEM block found")
	}
	caCertParsed, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CA cert: %w", err)
	}

	caKeyBlock, _ := pem.Decode(caCert.KeyPEM)
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("parsing CA key: no valid PEM block found")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CA key: %w", err)
	}

	return caCertParsed, caKey, nil
}

// generateSerialNumber generates a random serial number for certificates.
func generateSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("generating serial number: %w", err)
	}
	return serialNumber, nil
}
