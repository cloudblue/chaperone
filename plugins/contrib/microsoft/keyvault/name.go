// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package keyvault

import (
	"crypto/sha256"
	"encoding/hex"
)

// secretName maps a tenantID to a Key Vault secret name using SHA-256 hex.
// This guarantees a collision-free, always-valid name. Key Vault secret names
// allow only [0-9a-zA-Z-] and cap at 127 chars — a 64-char hex hash plus a
// short prefix fits comfortably and can never collide for distinct tenants.
//
// The original tenantID is preserved in the secret's "tenantID" tag for
// operator visibility in the Azure portal.
func secretName(prefix, tenantID string) string {
	sum := sha256.Sum256([]byte(tenantID))
	return prefix + hex.EncodeToString(sum[:])
}
