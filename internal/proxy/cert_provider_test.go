// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"crypto/tls"
	"sync"
	"testing"

	"github.com/cloudblue/chaperone/pkg/crypto"
)

func mustGenerateCert(t *testing.T) tls.Certificate {
	t.Helper()
	bundle, err := crypto.GenerateCertBundle()
	if err != nil {
		t.Fatalf("GenerateCertBundle: %v", err)
	}
	cert, err := tls.X509KeyPair(bundle.Server.CertPEM, bundle.Server.KeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	return cert
}

func TestCertProvider_GetCertificate_ReturnsInitialCert(t *testing.T) {
	cert := mustGenerateCert(t)
	p := NewCertProvider(cert)

	got, err := p.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate error: %v", err)
	}
	if got == nil {
		t.Fatal("GetCertificate returned nil")
	}
	if len(got.Certificate) == 0 {
		t.Error("returned certificate has no DER bytes")
	}
}

func TestCertProvider_Swap_UpdatesGetCertificate(t *testing.T) {
	first := mustGenerateCert(t)
	second := mustGenerateCert(t)

	p := NewCertProvider(first)

	before, _ := p.GetCertificate(nil)
	p.Swap(second)
	after, _ := p.GetCertificate(nil)

	if before == after {
		t.Error("GetCertificate returned same pointer after Swap")
	}
}

func TestCertProvider_Current_ReturnsActiveCert(t *testing.T) {
	cert := mustGenerateCert(t)
	p := NewCertProvider(cert)

	current := p.Current()
	if len(current.Certificate) == 0 {
		t.Error("Current() returned certificate with no DER bytes")
	}
}

func TestCertProvider_Swap_Current_Consistent(t *testing.T) {
	first := mustGenerateCert(t)
	second := mustGenerateCert(t)

	p := NewCertProvider(first)
	p.Swap(second)

	fromGet, _ := p.GetCertificate(nil)
	fromCurrent := p.Current()

	if len(fromGet.Certificate) == 0 || len(fromCurrent.Certificate) == 0 {
		t.Fatal("empty certificate after Swap")
	}
	// Both views of the certificate should have the same raw DER bytes.
	if string(fromGet.Certificate[0]) != string(fromCurrent.Certificate[0]) {
		t.Error("GetCertificate and Current() disagree after Swap")
	}
}

func TestCertProvider_ConcurrentSwap_NoDataRace(t *testing.T) {
	first := mustGenerateCert(t)
	second := mustGenerateCert(t)
	p := NewCertProvider(first)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			p.Swap(second)
		}()
		go func() {
			defer wg.Done()
			_, _ = p.GetCertificate(nil)
		}()
	}
	wg.Wait()
}
