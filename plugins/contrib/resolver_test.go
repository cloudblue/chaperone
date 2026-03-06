// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

// mockResolver is a test KeyResolver that returns a fixed key or error.
type mockResolver struct {
	key string
	err error
}

func (m *mockResolver) ResolveKey(_ context.Context, _ sdk.TransactionContext) (string, error) {
	return m.key, m.err
}

func TestResolveFromContext_PresentValidString(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{"TenantID": "contoso.onmicrosoft.com"},
	}

	got, err := ResolveFromContext(context.Background(), tx, "TenantID", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "contoso.onmicrosoft.com" {
		t.Errorf("got %q, want %q", got, "contoso.onmicrosoft.com")
	}
}

func TestResolveFromContext_PresentEmptyString_ReturnsErrInvalidContextData(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{"TenantID": ""},
	}

	resolver := &mockResolver{key: "should-not-be-used"}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}
}

func TestResolveFromContext_PresentWrongType_ReturnsErrInvalidContextData(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{"TenantID": float64(12345)},
	}

	resolver := &mockResolver{key: "should-not-be-used"}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrInvalidContextData) {
		t.Errorf("error = %v, want errors.Is(ErrInvalidContextData)", err)
	}
}

func TestResolveFromContext_AbsentWithResolver(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{"OtherField": "value"},
	}

	resolver := &mockResolver{key: "resolved-tenant"}

	got, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "resolved-tenant" {
		t.Errorf("got %q, want %q", got, "resolved-tenant")
	}
}

func TestResolveFromContext_AbsentWithoutResolver_ReturnsErrMissingContextData(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{},
	}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrMissingContextData) {
		t.Errorf("error = %v, want errors.Is(ErrMissingContextData)", err)
	}
}

func TestResolveFromContext_NilDataWithResolver(t *testing.T) {
	tx := sdk.TransactionContext{Data: nil}

	resolver := &mockResolver{key: "resolved-tenant"}

	got, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "resolved-tenant" {
		t.Errorf("got %q, want %q", got, "resolved-tenant")
	}
}

func TestResolveFromContext_NilDataWithoutResolver(t *testing.T) {
	tx := sdk.TransactionContext{Data: nil}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrMissingContextData) {
		t.Errorf("error = %v, want errors.Is(ErrMissingContextData)", err)
	}
}

func TestResolveFromContext_ResolverReturnsEmptyString_ReturnsErrMissingContextData(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{},
	}

	resolver := &mockResolver{key: ""}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err == nil {
		t.Fatal("expected error for empty resolver return")
	}

	if !errors.Is(err, ErrMissingContextData) {
		t.Errorf("error = %v, want errors.Is(ErrMissingContextData)", err)
	}
}

func TestResolveFromContext_ResolverErrorWrappedWithFieldContext(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{},
	}

	resolverErr := fmt.Errorf("lookup failed: %w", ErrTenantNotFound)
	resolver := &mockResolver{err: resolverErr}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "resolving TenantID") {
		t.Errorf("error = %q, want containing 'resolving TenantID'", err.Error())
	}
}

func TestResolveFromContext_ResolverErrorPropagated(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{},
	}

	resolverErr := fmt.Errorf("lookup failed: %w", ErrTenantNotFound)
	resolver := &mockResolver{err: resolverErr}

	_, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("error = %v, want errors.Is(ErrTenantNotFound)", err)
	}
}

func TestResolveFromContext_PresentOverrideIgnoresResolver(t *testing.T) {
	tx := sdk.TransactionContext{
		Data: map[string]any{"TenantID": "explicit-tenant"},
	}

	resolver := &mockResolver{key: "resolved-tenant"}

	got, err := ResolveFromContext(context.Background(), tx, "TenantID", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "explicit-tenant" {
		t.Errorf("got %q, want %q — tx.Data override should win over resolver", got, "explicit-tenant")
	}
}
