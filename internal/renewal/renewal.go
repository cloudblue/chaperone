// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package renewal implements the in-proxy side of the Connect-driven
// certificate rotation protocol.
//
// The two-step flow:
//  1. Connect calls POST /_ops/renew/prepare → Manager.Prepare()
//     generates a fresh ECDSA key pair and CSR whose SANs match the current
//     certificate, stores the pending state with a 10-minute TTL, and returns
//     the CSR PEM + a random renewal_id.
//  2. Connect signs the CSR and calls POST /_ops/renew/install →
//     Manager.Install() validates the renewal_id, TTL, and public key match,
//     then returns the new tls.Certificate ready for hot-swap.
package renewal

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/cloudblue/chaperone/pkg/crypto"
)

const (
	// RenewalIDBytes is the number of random bytes used for the renewal_id.
	// The hex-encoded form is twice this length (64 characters).
	RenewalIDBytes = 32

	// PendingTTL is how long a pending renewal remains valid after Prepare.
	PendingTTL = 10 * time.Minute
)

// Sentinel errors returned by Install.
var (
	ErrNoPending         = errors.New("no pending renewal; call prepare first")
	ErrRenewalIDMismatch = errors.New("renewal_id does not match pending renewal")
	ErrExpired           = errors.New("pending renewal has expired")
	ErrKeyMismatch       = errors.New("certificate public key does not match pending private key")
)

// PendingRenewal holds the in-progress state between Prepare and Install.
type PendingRenewal struct {
	RenewalID string
	KeyPEM    []byte // PEM-encoded private key — used to form tls.Certificate on Install
	CSRPEM    []byte
	ExpiresAt time.Time
}

// Manager serialises the prepare→install handshake. A new Prepare call
// supersedes any in-flight pending renewal (the old key material is discarded).
type Manager struct {
	mu      sync.Mutex
	pending *PendingRenewal
	now     func() time.Time // injectable for tests
}

// NewManager returns a ready Manager.
func NewManager() *Manager {
	return &Manager{now: time.Now}
}

// Prepare generates a fresh ECDSA P-256 key pair and CSR preserving the SANs
// of currentCert. It stores the pending state and returns the CSR PEM and a
// 64-character hex renewal_id. A second call to Prepare supersedes any
// previous pending renewal.
func (m *Manager) Prepare(currentCert tls.Certificate) (csrPEM []byte, renewalID string, err error) {
	dnsNames, ips, err := extractSANs(currentCert)
	if err != nil {
		return nil, "", err
	}

	cn := extractCN(currentCert)

	bundle, err := crypto.GenerateServerCSR(cn, dnsNames, ips)
	if err != nil {
		return nil, "", err
	}

	id, err := generateRenewalID()
	if err != nil {
		return nil, "", err
	}

	m.mu.Lock()
	m.pending = &PendingRenewal{
		RenewalID: id,
		KeyPEM:    bundle.KeyPEM,
		CSRPEM:    bundle.CSRPEM,
		ExpiresAt: m.now().Add(PendingTTL),
	}
	m.mu.Unlock()

	return bundle.CSRPEM, id, nil
}

// Install validates the incoming signed certificate against the pending
// renewal and returns a tls.Certificate ready for hot-swap.
//
// Errors: ErrNoPending, ErrRenewalIDMismatch, ErrExpired, ErrKeyMismatch.
func (m *Manager) Install(renewalID string, certPEM []byte) (tls.Certificate, error) {
	m.mu.Lock()
	pending := m.pending
	m.mu.Unlock()

	if pending == nil {
		return tls.Certificate{}, ErrNoPending
	}
	if pending.RenewalID != renewalID {
		return tls.Certificate{}, ErrRenewalIDMismatch
	}
	if m.now().After(pending.ExpiresAt) {
		return tls.Certificate{}, ErrExpired
	}

	if err := verifyKeyMatch(certPEM, pending.KeyPEM); err != nil {
		return tls.Certificate{}, err
	}

	newCert, err := tls.X509KeyPair(certPEM, pending.KeyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Clear pending state on success so the renewal_id cannot be replayed.
	m.mu.Lock()
	if m.pending == pending {
		m.pending = nil
	}
	m.mu.Unlock()

	return newCert, nil
}

// Pending returns the current pending renewal, or nil if none exists.
// Intended for handler logging; callers must not mutate the returned value.
func (m *Manager) Pending() *PendingRenewal {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pending
}

// --- helpers ---

func extractSANs(cert tls.Certificate) (dnsNames []string, ips []net.IP, err error) {
	if len(cert.Certificate) == 0 {
		return nil, nil, errors.New("tls.Certificate has no DER bytes")
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	return x509Cert.DNSNames, x509Cert.IPAddresses, nil
}

func extractCN(cert tls.Certificate) string {
	if len(cert.Certificate) == 0 {
		return "chaperone"
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil || x509Cert.Subject.CommonName == "" {
		return "chaperone"
	}
	return x509Cert.Subject.CommonName
}

func generateRenewalID() (string, error) {
	b := make([]byte, RenewalIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// verifyKeyMatch checks that the public key in certPEM matches the private key
// in keyPEM. Called before tls.X509KeyPair so ErrKeyMismatch is returned
// rather than the opaque TLS error.
func verifyKeyMatch(certPEM, keyPEM []byte) error {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return errors.New("failed to decode certificate PEM")
	}
	x509Cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}
	certPub, ok := x509Cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("certificate public key is not ECDSA")
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return errors.New("failed to decode private key PEM")
	}
	privKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return err
	}

	if !certPub.Equal(&privKey.PublicKey) {
		return ErrKeyMismatch
	}
	return nil
}
