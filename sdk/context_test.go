// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package sdk

import (
	"errors"
	"testing"
)

func TestDataString_PresentValidString(t *testing.T) {
	tx := TransactionContext{
		Data: map[string]any{"TenantID": "contoso.onmicrosoft.com"},
	}

	got, ok, err := tx.DataString("TenantID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected field to be present")
	}

	if got != "contoso.onmicrosoft.com" {
		t.Errorf("got %q, want %q", got, "contoso.onmicrosoft.com")
	}
}

func TestDataString_PresentEmptyString_ReturnsErrInvalidContextData(t *testing.T) {
	tx := TransactionContext{
		Data: map[string]any{"TenantID": ""},
	}

	_, ok, err := tx.DataString("TenantID")
	if err == nil {
		t.Fatal("expected error")
	}
	if !ok {
		t.Fatal("expected field to be present")
	}

	if !errors.Is(err, ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}
}

func TestDataString_PresentWrongType_ReturnsErrInvalidContextData(t *testing.T) {
	tx := TransactionContext{
		Data: map[string]any{"TenantID": float64(12345)},
	}

	_, ok, err := tx.DataString("TenantID")
	if err == nil {
		t.Fatal("expected error")
	}
	if !ok {
		t.Fatal("expected field to be present")
	}

	if !errors.Is(err, ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}
}

func TestDataString_Absent_ReturnsNotPresent(t *testing.T) {
	tx := TransactionContext{
		Data: map[string]any{"OtherField": "value"},
	}

	_, ok, err := tx.DataString("TenantID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected field to be absent")
	}
}
