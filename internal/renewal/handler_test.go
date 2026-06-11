// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package renewal

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/pkg/crypto"
)

// fakeCertSwapper implements CertSwapper for tests.
type fakeCertSwapper struct {
	cert    tls.Certificate
	swapped []tls.Certificate
}

func (f *fakeCertSwapper) Current() tls.Certificate { return f.cert }
func (f *fakeCertSwapper) Swap(c tls.Certificate)   { f.swapped = append(f.swapped, c) }
func (f *fakeCertSwapper) SwapCount() int           { return len(f.swapped) }

func mustMakeSwapper(t *testing.T) (*fakeCertSwapper, *crypto.CertPair) {
	t.Helper()
	ca, err := crypto.GenerateCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	pair, err := crypto.GenerateServerCertWithSANs(ca, time.Hour, []string{"proxy.test"}, nil)
	if err != nil {
		t.Fatalf("GenerateServerCertWithSANs: %v", err)
	}
	cert, err := tls.X509KeyPair(pair.CertPEM, pair.KeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	return &fakeCertSwapper{cert: cert}, ca
}

// signCSR parses csrPEM and signs it with ca, returning the new cert PEM.
func signCSR(t *testing.T, ca *crypto.CertPair, csrPEM []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		t.Fatal("signCSR: no PEM block")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificateRequest: %v", err)
	}
	caCert, caKey, err := crypto.ParseCA(ca)
	if err != nil {
		t.Fatalf("ParseCA: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
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

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestHandler_Prepare_ReturnsCSRAndRenewalID(t *testing.T) {
	swapper, _ := mustMakeSwapper(t)
	h := NewHandler(NewManager(), swapper, "/tmp/cert.pem", "/tmp/key.pem", true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("prepare: got status %d, want 200; body: %s", rr.Code, rr.Body)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !strings.HasPrefix(resp["csr"], "-----BEGIN CERTIFICATE REQUEST-----") {
		t.Error("csr field missing or not PEM")
	}
	if len(resp["renewal_id"]) != RenewalIDBytes*2 {
		t.Errorf("renewal_id len = %d, want %d", len(resp["renewal_id"]), RenewalIDBytes*2)
	}
}

func TestHandler_Install_HappyPath_SwapsAndWrites(t *testing.T) {
	swapper, ca := mustMakeSwapper(t)

	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")

	manager := NewManager()
	h := NewHandler(manager, swapper, certFile, keyFile, true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)

	// Step 1: prepare
	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("prepare: %d %s", rr.Code, rr.Body)
	}
	var prepResp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &prepResp); err != nil {
		t.Fatalf("unmarshal prepare: %v", err)
	}

	// Step 2: sign the CSR
	newCertPEM := signCSR(t, ca, []byte(prepResp["csr"]))

	// Step 3: install
	rr = postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id":  prepResp["renewal_id"],
		"certificate": string(newCertPEM),
	})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("install: got %d, want 202; body: %s", rr.Code, rr.Body)
	}

	// Verify cert was hot-swapped.
	if swapper.SwapCount() != 1 {
		t.Errorf("Swap called %d times, want 1", swapper.SwapCount())
	}

	// Verify cert written to disk.
	diskCert, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read cert file: %v", err)
	}
	if !strings.HasPrefix(string(diskCert), "-----BEGIN CERTIFICATE-----") {
		t.Error("cert file does not contain certificate PEM")
	}

	// Verify key written to disk.
	diskKey, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if !strings.HasPrefix(string(diskKey), "-----BEGIN EC PRIVATE KEY-----") {
		t.Error("key file does not contain EC PRIVATE KEY PEM")
	}
}

func TestHandler_Prepare_AutoRotateFalse_Returns501(t *testing.T) {
	swapper, _ := mustMakeSwapper(t)
	h := NewHandler(NewManager(), swapper, "/tmp/cert.pem", "/tmp/key.pem", false)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("prepare with autoRotate=false: got %d, want 501", rr.Code)
	}
}

func TestHandler_Install_AutoRotateFalse_Returns501(t *testing.T) {
	swapper, _ := mustMakeSwapper(t)
	h := NewHandler(NewManager(), swapper, "/tmp/cert.pem", "/tmp/key.pem", false)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)
	rr := postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id":  "abc123",
		"certificate": "certpem",
	})

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("install with autoRotate=false: got %d, want 501", rr.Code)
	}
}

func TestHandler_Install_NoPending_Returns409(t *testing.T) {
	swapper, _ := mustMakeSwapper(t)
	h := NewHandler(NewManager(), swapper, "/tmp/cert.pem", "/tmp/key.pem", true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)
	rr := postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id":  "someid",
		"certificate": "certpem",
	})

	if rr.Code != http.StatusConflict {
		t.Errorf("install without prepare: got %d, want 409", rr.Code)
	}
}

func TestHandler_Install_RenewalIDMismatch_Returns409(t *testing.T) {
	swapper, ca := mustMakeSwapper(t)
	manager := NewManager()
	h := NewHandler(manager, swapper, "/tmp/cert.pem", "/tmp/key.pem", true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)

	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)
	var prepResp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &prepResp)

	newCertPEM := signCSR(t, ca, []byte(prepResp["csr"]))
	rr = postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id":  "wrong-id",
		"certificate": string(newCertPEM),
	})
	if rr.Code != http.StatusConflict {
		t.Errorf("install with wrong id: got %d, want 409", rr.Code)
	}
}

func TestHandler_Install_KeyMismatch_Returns422(t *testing.T) {
	swapper, ca := mustMakeSwapper(t)
	manager := NewManager()
	h := NewHandler(manager, swapper, "/tmp/cert.pem", "/tmp/key.pem", true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)

	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)
	var prepResp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &prepResp)

	// Sign a different CSR (wrong key) and try to install.
	otherBundle, err := crypto.GenerateServerCSR("proxy.test", []string{"proxy.test"}, nil)
	if err != nil {
		t.Fatalf("GenerateServerCSR: %v", err)
	}
	mismatchedCert := signCSR(t, ca, otherBundle.CSRPEM)

	rr = postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id":  prepResp["renewal_id"],
		"certificate": string(mismatchedCert),
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("install with wrong key: got %d, want 422", rr.Code)
	}
}

func TestHandler_Install_MissingFields_Returns400(t *testing.T) {
	swapper, _ := mustMakeSwapper(t)
	h := NewHandler(NewManager(), swapper, "/tmp/cert.pem", "/tmp/key.pem", true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)
	rr := postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id": "abc",
		// certificate missing
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("install with missing certificate: got %d, want 400", rr.Code)
	}
}

func TestHandler_NilProvider_AutoRotateForcedFalse(t *testing.T) {
	h := NewHandler(NewManager(), nil, "/tmp/cert.pem", "/tmp/key.pem", true)
	if h.autoRotate {
		t.Error("nil provider should force autoRotate=false")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("nil provider prepare: got %d, want 501", rr.Code)
	}
}

func TestHandler_GetCertificate_ReturnsNewCertAfterInstall(t *testing.T) {
	swapper, ca := mustMakeSwapper(t)
	dir := t.TempDir()
	manager := NewManager()
	h := NewHandler(manager, swapper, filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key"), true)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /_ops/renew/prepare", h.HandlePrepare)
	mux.HandleFunc("POST /_ops/renew/install", h.HandleInstall)

	rr := postJSON(t, mux, "/_ops/renew/prepare", nil)
	var prepResp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &prepResp)

	newCertPEM := signCSR(t, ca, []byte(prepResp["csr"]))
	rr = postJSON(t, mux, "/_ops/renew/install", map[string]string{
		"renewal_id":  prepResp["renewal_id"],
		"certificate": string(newCertPEM),
	})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("install: %d %s", rr.Code, rr.Body)
	}

	// The swapper's last cert should have DER bytes matching the new cert PEM.
	if swapper.SwapCount() != 1 {
		t.Fatalf("expected 1 Swap, got %d", swapper.SwapCount())
	}
	newCert := swapper.swapped[0]
	if len(newCert.Certificate) == 0 {
		t.Error("swapped certificate has no DER bytes")
	}
	if newCert.Leaf == nil {
		t.Error("swapped certificate has nil Leaf")
	}
}
