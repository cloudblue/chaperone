// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package keyvault

import "testing"

func TestSecretName_Deterministic(t *testing.T) {
	const (
		prefix   = "chaperone-rt-"
		tenantID = "contoso.onmicrosoft.com"
	)

	first := secretName(prefix, tenantID)
	second := secretName(prefix, tenantID)

	if first != second {
		t.Errorf("secretName() not deterministic: %q != %q", first, second)
	}
}

func TestSecretName_CollisionSafety(t *testing.T) {
	// These two tenants would collide under a naive "dot→hyphen" mapping
	// but must map to distinct secret names under SHA-256 hex.
	const (
		a = "my-a.b"
		b = "my.a-b"
	)

	nameA := secretName(defaultPrefix, a)
	nameB := secretName(defaultPrefix, b)

	if nameA == nameB {
		t.Errorf("collision: secretName(%q) == secretName(%q) == %q", a, b, nameA)
	}
}

func TestSecretName_ValidKeyVaultName(t *testing.T) {
	tenants := []string{
		"contoso.onmicrosoft.com",
		"my-company.onmicrosoft.com",
		"00000000-0000-0000-0000-000000000000",
		"common",
		"organizations",
		"consumers",
		"a",
		"tenant.with.many.dots.example.com",
	}

	for _, tenant := range tenants {
		name := secretName(defaultPrefix, tenant)
		if !isValidKeyVaultSecretName(name) {
			t.Errorf("secretName(%q) = %q, not a valid Key Vault secret name", tenant, name)
		}
	}
}

func TestSecretName_RespectsPrefix(t *testing.T) {
	const tenantID = "contoso.onmicrosoft.com"

	envA := secretName("envA-", tenantID)
	envB := secretName("envB-", tenantID)

	if envA == envB {
		t.Errorf("prefixes ignored: %q == %q", envA, envB)
	}
	if got := envA[:5]; got != "envA-" {
		t.Errorf("envA name = %q, want prefix %q", envA, "envA-")
	}
	if got := envB[:5]; got != "envB-" {
		t.Errorf("envB name = %q, want prefix %q", envB, "envB-")
	}
}

func TestSecretName_LengthWithinKeyVaultLimit(t *testing.T) {
	// Even with a long prefix and domain tenantID, the resulting name must
	// fit within Key Vault's 127-character secret name limit.
	const longPrefix = "chaperone-rt-very-long-prefix-for-testing-"

	name := secretName(longPrefix, "very-long-tenant-name.onmicrosoft.com")

	if len(name) > 127 {
		t.Errorf("secretName length = %d, exceeds Key Vault limit of 127", len(name))
	}
}
