// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package renewal

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/pkg/crypto"
)

// mustGenerateCA creates a test CA; fatal on error.
func mustGenerateCA(t *testing.T) *crypto.CertPair {
	t.Helper()
	ca, err := crypto.GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	return ca
}

// mustGenerateServerCert returns a tls.Certificate signed by ca with the
// given DNS names and IPs as SANs.
func mustGenerateServerCert(t *testing.T, ca *crypto.CertPair, dnsNames []string, ips []net.IP) tls.Certificate {
	t.Helper()
	pair, err := crypto.GenerateServerCertWithSANs(ca, time.Hour, dnsNames, ips)
	if err != nil {
		t.Fatalf("GenerateServerCertWithSANs: %v", err)
	}
	cert, err := tls.X509KeyPair(pair.CertPEM, pair.KeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	return cert
}

// signCSRPEM parses csrPEM and signs it with ca, returning the new certificate PEM.
func signCSRPEM(t *testing.T, ca *crypto.CertPair, csrPEM []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		t.Fatal("signCSRPEM: no PEM block in CSR")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificateRequest: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature invalid: %v", err)
	}

	caCert, caKey, err := crypto.ParseCA(ca)
	if err != nil {
		t.Fatalf("ParseCA: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, csr.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

func TestManager_Prepare_ReturnsCSRAndRenewalID(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	csrPEM, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if len(csrPEM) == 0 {
		t.Error("Prepare returned empty CSR PEM")
	}
	if len(id) != RenewalIDBytes*2 {
		t.Errorf("renewal_id length = %d, want %d", len(id), RenewalIDBytes*2)
	}

	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Error("Prepare CSR PEM is not a valid CERTIFICATE REQUEST")
	}
}

func TestManager_Prepare_StoresPendingState(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	_, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	pending := m.Pending()
	if pending == nil {
		t.Fatal("Pending() returned nil after Prepare")
	}
	if pending.RenewalID != id {
		t.Errorf("pending.RenewalID = %q, want %q", pending.RenewalID, id)
	}
	if pending.ExpiresAt.IsZero() {
		t.Error("pending.ExpiresAt is zero")
	}
}

func TestManager_Install_HappyPath(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	csrPEM, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	newCertPEM := signCSRPEM(t, ca, csrPEM)

	cert, _, err := m.Install(id, newCertPEM)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("Install returned certificate with no DER bytes")
	}
}

func TestManager_Install_ClearsPendingOnSuccess(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	csrPEM, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	newCertPEM := signCSRPEM(t, ca, csrPEM)
	if _, _, err := m.Install(id, newCertPEM); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if m.Pending() != nil {
		t.Error("Pending() should be nil after successful Install")
	}
}

func TestManager_Install_ErrNoPending(t *testing.T) {
	m := NewManager()
	_, _, err := m.Install("someid", []byte("certpem"))
	if err != ErrNoPending {
		t.Errorf("Install without Prepare: got %v, want ErrNoPending", err)
	}
}

func TestManager_Install_ErrRenewalIDMismatch(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	csrPEM, _, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	newCertPEM := signCSRPEM(t, ca, csrPEM)
	_, _, err = m.Install("wrong-id", newCertPEM)
	if err != ErrRenewalIDMismatch {
		t.Errorf("Install with wrong id: got %v, want ErrRenewalIDMismatch", err)
	}
}

func TestManager_Install_ErrExpired(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	// Freeze time so TTL expires immediately.
	frozenPast := time.Now().Add(-PendingTTL - time.Second)
	m.now = func() time.Time { return frozenPast }

	csrPEM, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// Restore real time so Install sees now > ExpiresAt.
	m.now = time.Now

	newCertPEM := signCSRPEM(t, ca, csrPEM)
	_, _, err = m.Install(id, newCertPEM)
	if err != ErrExpired {
		t.Errorf("Install after TTL: got %v, want ErrExpired", err)
	}
}

func TestManager_Install_ErrKeyMismatch(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	_, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// Sign a *different* CSR so the cert's public key won't match the pending key.
	otherBundle, err := crypto.GenerateServerCSR("proxy.example.com", []string{"proxy.example.com"}, nil)
	if err != nil {
		t.Fatalf("GenerateServerCSR: %v", err)
	}
	mismatchedCertPEM := signCSRPEM(t, ca, otherBundle.CSRPEM)

	_, _, err = m.Install(id, mismatchedCertPEM)
	if err != ErrKeyMismatch {
		t.Errorf("Install with wrong key: got %v, want ErrKeyMismatch", err)
	}
}

func TestManager_Prepare_SupersedesPreviousPending(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	_, firstID, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("first Prepare: %v", err)
	}

	_, secondID, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("second Prepare: %v", err)
	}

	if firstID == secondID {
		t.Error("second Prepare returned the same renewal_id as the first")
	}

	pending := m.Pending()
	if pending == nil || pending.RenewalID != secondID {
		t.Error("pending state should reflect the second Prepare")
	}

	// Installing with the first id must now fail.
	csrPEM := pending.CSRPEM
	newCertPEM := signCSRPEM(t, ca, csrPEM)
	_, _, err = m.Install(firstID, newCertPEM)
	if err != ErrRenewalIDMismatch {
		t.Errorf("Install with first id after second Prepare: got %v, want ErrRenewalIDMismatch", err)
	}
}

func TestManager_Install_PreservesNewCertSANs(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, []net.IP{net.ParseIP("10.0.0.1")})

	m := NewManager()
	csrPEM, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// Parse CSR to verify SANs were preserved.
	block, _ := pem.Decode(csrPEM)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}

	hasDNS := false
	for _, name := range csr.DNSNames {
		if name == "proxy.example.com" {
			hasDNS = true
		}
	}
	if !hasDNS {
		t.Error("CSR missing expected DNS SAN proxy.example.com")
	}

	newCertPEM := signCSRPEM(t, ca, csrPEM)
	if _, _, err := m.Install(id, newCertPEM); err != nil {
		t.Fatalf("Install: %v", err)
	}
}

func TestManager_ConcurrentPrepare_NoDataRace(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = m.Prepare(currentCert)
		}()
	}
	wg.Wait()
}

func TestManager_RenewalID_IsHex64Chars(t *testing.T) {
	ca := mustGenerateCA(t)
	currentCert := mustGenerateServerCert(t, ca, []string{"proxy.example.com"}, nil)

	m := NewManager()
	_, id, err := m.Prepare(currentCert)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	const wantLen = RenewalIDBytes * 2
	if len(id) != wantLen {
		t.Errorf("renewal_id len = %d, want %d", len(id), wantLen)
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("renewal_id contains non-hex character: %c", c)
		}
	}
}
