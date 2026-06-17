// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package renewal

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudblue/chaperone/internal/telemetry"
)

const keyRenewalID = "renewal_id"

// CertSwapper is satisfied by proxy.CertProvider. Defined here to avoid an
// import cycle between the renewal and proxy packages.
type CertSwapper interface {
	Current() tls.Certificate
	Swap(tls.Certificate)
}

// Handler implements the HTTP endpoints for Connect-driven certificate rotation.
type Handler struct {
	manager    *Manager
	provider   CertSwapper
	certFile   string
	keyFile    string
	autoRotate bool
}

// NewHandler returns a Handler wired to the given manager, cert provider, and
// on-disk paths. When autoRotate is false (or provider is nil) both endpoints
// return 501.
func NewHandler(manager *Manager, provider CertSwapper, certFile, keyFile string, autoRotate bool) *Handler {
	// A nil provider means TLS is disabled; renewal is not possible regardless
	// of the autoRotate setting.
	if provider == nil {
		autoRotate = false
	}
	return &Handler{
		manager:    manager,
		provider:   provider,
		certFile:   certFile,
		keyFile:    keyFile,
		autoRotate: autoRotate,
	}
}

// HandlePrepare serves POST /_ops/renew/prepare.
//
// Generates a fresh ECDSA P-256 key pair and CSR whose SANs match the current
// certificate, stores the pending state with a 10-minute TTL, and returns the
// CSR PEM plus a pairing token (renewal_id).
//
// Status codes:
//   - 200 — {"csr": "<PEM>", "renewal_id": "<hex>"}
//   - 501 — cert_management is external; renewal is disabled
func (h *Handler) HandlePrepare(w http.ResponseWriter, r *http.Request) {
	if !h.autoRotate {
		writeJSONError(w, http.StatusNotImplemented, "certificate management is external; renewal is disabled")
		return
	}

	currentCert := h.provider.Current()
	csrPEM, renewalID, err := h.manager.Prepare(currentCert)
	if err != nil {
		if errors.Is(err, ErrRenewalInProgress) {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
		slog.Error("renewal prepare failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "prepare failed")
		return
	}

	pending := h.manager.Pending()
	slog.Info("renewal prepare completed", keyRenewalID, renewalID, "expires_at", pending.ExpiresAt)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"csr":        string(csrPEM),
		keyRenewalID: renewalID,
	})
}

// HandleInstall serves POST /_ops/renew/install.
//
// Validates the renewal_id against the pending state, verifies the signed
// certificate's public key matches the pending private key, hot-swaps the TLS
// listener, and atomically writes the new cert and key to disk.
//
// Status codes:
//   - 202 — certificate installed and TLS listener hot-swapped
//   - 400 — malformed request body
//   - 409 — renewal_id mismatch or pending renewal expired
//   - 422 — certificate public key does not match pending private key
//   - 501 — cert_management is external; renewal is disabled
func (h *Handler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	if !h.autoRotate {
		writeJSONError(w, http.StatusNotImplemented, "certificate management is external; renewal is disabled")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)

	var body struct {
		RenewalID   string `json:"renewal_id"`
		Certificate string `json:"certificate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.RenewalID == "" || body.Certificate == "" {
		writeJSONError(w, http.StatusBadRequest, "renewal_id and certificate are required")
		return
	}

	certPEM := []byte(body.Certificate)
	newCert, keyPEM, err := h.manager.Install(body.RenewalID, certPEM)
	if err == nil {
		defer zeroBytes(keyPEM)
	}
	if err != nil {
		slog.Warn("renewal install rejected", keyRenewalID, body.RenewalID, "error", err)
		telemetry.CertRenewalsTotal.WithLabelValues("failure").Inc()
		switch {
		case errors.Is(err, ErrNoPending), errors.Is(err, ErrRenewalIDMismatch), errors.Is(err, ErrExpired):
			writeJSONError(w, http.StatusConflict, err.Error())
		case errors.Is(err, ErrKeyMismatch), errors.Is(err, ErrInvalidCertificate):
			writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, "install failed")
		}
		return
	}

	// Hot-swap the TLS listener — in-flight connections are unaffected.
	h.provider.Swap(newCert)
	telemetry.CertExpirySeconds.Set(time.Until(newCert.Leaf.NotAfter).Seconds())
	telemetry.CertRenewalsTotal.WithLabelValues("success").Inc()

	// Persist cert and key to disk. Both are written to temp files first;
	// renames are committed only after both writes succeed. A failure here
	// returns 500 so the caller knows persistence did not complete.
	if err := persistPair(h.certFile, certPEM, h.keyFile, keyPEM); err != nil {
		slog.Error("renewal: failed to persist cert/key", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "certificate installed but failed to persist to disk")
		return
	}

	slog.Info("renewal install completed", keyRenewalID, body.RenewalID, "cert_not_after", newCert.Leaf.NotAfter)
	w.WriteHeader(http.StatusAccepted)
}

// writeJSONError writes a JSON {"error": msg} body with the given status code.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// persistPair writes certPEM and keyPEM to temp files in their respective
// directories, then renames both. If the key rename fails after the cert rename
// succeeds, the new cert file is removed so the pair is never left mismatched on
// disk. The caller receives an error and should treat the cert as in-memory only.
func persistPair(certPath string, certPEM []byte, keyPath string, keyPEM []byte) error {
	certTmp, err := writePEMToTemp(certPath, certPEM)
	if err != nil {
		return err
	}
	keyTmp, err := writePEMToTemp(keyPath, keyPEM)
	if err != nil {
		_ = os.Remove(certTmp)
		return err
	}
	if err := os.Rename(certTmp, certPath); err != nil {
		_ = os.Remove(certTmp)
		_ = os.Remove(keyTmp)
		return err
	}
	if err := os.Rename(keyTmp, keyPath); err != nil {
		_ = os.Remove(keyTmp)
		_ = os.Remove(certPath) // roll back cert rename to avoid a mismatched pair on disk
		return err
	}
	return nil
}

// writePEMToTemp writes data to a new temp file in the same directory as
// targetPath and returns the temp file name. The caller must rename or remove it.
func writePEMToTemp(targetPath string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(targetPath), ".pem-*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp) // #nosec G703 -- tmp is the OS-generated temp path, not user-controlled
		return "", err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp) // #nosec G703
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp) // #nosec G703
		return "", err
	}
	return tmp, nil
}
