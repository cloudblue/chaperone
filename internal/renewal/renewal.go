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

// Sentinel errors returned by Prepare and Install.
var (
	ErrNoPending         = errors.New("no pending renewal; call prepare first")
	ErrRenewalInProgress = errors.New("renewal already in progress; install or wait for expiry")
	ErrRenewalIDMismatch = errors.New("renewal_id does not match pending renewal")
	ErrExpired           = errors.New("pending renewal has expired")
	ErrKeyMismatch       = errors.New("certificate public key does not match pending private key")
)

// PendingRenewal holds the in-progress state between Prepare and Install.
// KeyPEM is zeroed when the pending is cleared to limit key material lifetime.
type PendingRenewal struct {
	RenewalID string
	KeyPEM    []byte // PEM-encoded private key — zeroed on discard
	CSRPEM    []byte
	ExpiresAt time.Time
}

// PendingInfo is a key-less snapshot of PendingRenewal, safe for logging.
type PendingInfo struct {
	RenewalID string
	ExpiresAt time.Time
}

// Manager serialises the prepare→install handshake. A second Prepare while a
// non-expired pending exists returns ErrRenewalInProgress; callers must install
// or wait for the TTL to expire before retrying.
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
// 64-character hex renewal_id.
//
// Returns ErrRenewalInProgress if a non-expired pending renewal already exists.
func (m *Manager) Prepare(currentCert tls.Certificate) (csrPEM []byte, renewalID string, err error) {
	x509Cert, err := parseCertDER(currentCert)
	if err != nil {
		return nil, "", err
	}

	dnsNames, ips := extractSANs(x509Cert)
	cn := extractCN(x509Cert)

	// Generate key material outside the lock — crypto ops are slow.
	bundle, err := crypto.GenerateServerCSR(cn, dnsNames, ips)
	if err != nil {
		return nil, "", err
	}

	id, err := generateRenewalID()
	if err != nil {
		return nil, "", err
	}

	m.mu.Lock()
	if m.pending != nil && !m.now().After(m.pending.ExpiresAt) {
		m.mu.Unlock()
		return nil, "", ErrRenewalInProgress
	}
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
// renewal and returns the new tls.Certificate and its private key PEM ready
// for hot-swap and atomic disk write.
//
// Errors: ErrNoPending, ErrRenewalIDMismatch, ErrExpired, ErrKeyMismatch.
func (m *Manager) Install(renewalID string, certPEM []byte) (tls.Certificate, []byte, error) {
	m.mu.Lock()
	pending := m.pending
	m.mu.Unlock()

	if pending == nil {
		return tls.Certificate{}, nil, ErrNoPending
	}
	if pending.RenewalID != renewalID {
		return tls.Certificate{}, nil, ErrRenewalIDMismatch
	}
	if m.now().After(pending.ExpiresAt) {
		return tls.Certificate{}, nil, ErrExpired
	}

	if err := verifyKeyMatch(certPEM, pending.KeyPEM); err != nil {
		return tls.Certificate{}, nil, err
	}

	newCert, err := tls.X509KeyPair(certPEM, pending.KeyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	// Parse and set Leaf so callers can read certificate fields (e.g. NotAfter)
	// without a second round-trip through x509.ParseCertificate.
	leaf, err := x509.ParseCertificate(newCert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	newCert.Leaf = leaf

	// Copy key material before zeroing pending so it can be written to disk.
	keyPEM := make([]byte, len(pending.KeyPEM))
	copy(keyPEM, pending.KeyPEM)

	// Zero and clear pending so the renewal_id cannot be replayed.
	m.mu.Lock()
	if m.pending == pending {
		zeroBytes(m.pending.KeyPEM)
		m.pending = nil
	}
	m.mu.Unlock()

	return newCert, keyPEM, nil
}

// Pending returns a key-less snapshot of the current pending renewal, or nil
// if none exists. Safe to log.
func (m *Manager) Pending() *PendingInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending == nil {
		return nil
	}
	return &PendingInfo{
		RenewalID: m.pending.RenewalID,
		ExpiresAt: m.pending.ExpiresAt,
	}
}

// --- helpers ---

// parseCertDER parses the first DER-encoded certificate in cert.
func parseCertDER(cert tls.Certificate) (*x509.Certificate, error) {
	if len(cert.Certificate) == 0 {
		return nil, errors.New("tls.Certificate has no DER bytes")
	}
	return x509.ParseCertificate(cert.Certificate[0])
}

func extractSANs(cert *x509.Certificate) (dnsNames []string, ips []net.IP) {
	return cert.DNSNames, cert.IPAddresses
}

func extractCN(cert *x509.Certificate) string {
	if cert.Subject.CommonName == "" {
		return "chaperone"
	}
	return cert.Subject.CommonName
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

// zeroBytes overwrites b with zeros to limit key material lifetime in memory.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
