// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package keyvault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/cloudblue/chaperone/plugins/contrib"
)

// fakeSecretsClient is an in-memory secretsClient implementation for tests.
// Safe for concurrent use.
type fakeSecretsClient struct {
	mu      sync.Mutex
	secrets map[string]fakeSecret // key: secret name (post-encoding)

	getErr error
	setErr error

	getCalls atomic.Int32
	setCalls atomic.Int32
}

type fakeSecret struct {
	value string
	tags  map[string]*string
}

func newFakeSecretsClient() *fakeSecretsClient {
	return &fakeSecretsClient{secrets: make(map[string]fakeSecret)}
}

func (f *fakeSecretsClient) GetSecret(
	_ context.Context, name, _ string, _ *azsecrets.GetSecretOptions,
) (azsecrets.GetSecretResponse, error) {
	f.getCalls.Add(1)
	if f.getErr != nil {
		return azsecrets.GetSecretResponse{}, f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.secrets[name]
	if !ok {
		return azsecrets.GetSecretResponse{}, &azcore.ResponseError{
			StatusCode: http.StatusNotFound,
			ErrorCode:  secretNotFoundCode,
		}
	}
	val := s.value
	return azsecrets.GetSecretResponse{
		Secret: azsecrets.Secret{Value: &val, Tags: s.tags},
	}, nil
}

func (f *fakeSecretsClient) SetSecret(
	_ context.Context, name string, params azsecrets.SetSecretParameters, _ *azsecrets.SetSecretOptions,
) (azsecrets.SetSecretResponse, error) {
	f.setCalls.Add(1)
	if f.setErr != nil {
		return azsecrets.SetSecretResponse{}, f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[name] = fakeSecret{value: *params.Value, tags: params.Tags}
	return azsecrets.SetSecretResponse{
		Secret: azsecrets.Secret{Value: params.Value, Tags: params.Tags},
	}, nil
}

func (f *fakeSecretsClient) get(name string) (fakeSecret, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.secrets[name]
	return s, ok
}

func newTestStore(t *testing.T) (*Store, *fakeSecretsClient) {
	t.Helper()
	fake := newFakeSecretsClient()
	return newStoreWithClient(fake, nil), fake
}

func TestStore_SaveAndLoad_RoundTrip(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	const (
		tenant = "contoso.onmicrosoft.com"
		token  = "rt_abc123_secret"
	)

	if err := store.Save(ctx, tenant, token); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load(ctx, tenant)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != token {
		t.Errorf("Load() = %q, want %q", got, token)
	}
}

func TestStore_Load_SecretNotFound_ReturnsErrTenantNotFound(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.Load(context.Background(), "contoso.onmicrosoft.com")
	if err == nil {
		t.Fatal("Load() expected error on empty vault, got nil")
	}
	if !errors.Is(err, contrib.ErrTenantNotFound) {
		t.Errorf("Load() error should wrap contrib.ErrTenantNotFound, got: %v", err)
	}
}

func TestStore_Load_OtherAPIError_Propagated(t *testing.T) {
	store, fake := newTestStore(t)

	sentinel := errors.New("transient network failure")
	fake.getErr = sentinel

	_, err := store.Load(context.Background(), "contoso.onmicrosoft.com")
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Load() should wrap sentinel error, got: %v", err)
	}
	if errors.Is(err, contrib.ErrTenantNotFound) {
		t.Error("Load() should NOT wrap ErrTenantNotFound for a non-404 error")
	}
}

func TestStore_Load_WrongErrorCode_NotTreatedAsNotFound(t *testing.T) {
	store, fake := newTestStore(t)

	// 404 with a different ErrorCode must NOT be mapped to ErrTenantNotFound
	// — only the specific SecretNotFound code signals "no token for tenant".
	fake.getErr = &azcore.ResponseError{
		StatusCode: http.StatusNotFound,
		ErrorCode:  "Forbidden",
	}

	_, err := store.Load(context.Background(), "contoso.onmicrosoft.com")
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if errors.Is(err, contrib.ErrTenantNotFound) {
		t.Error("Load() should only map SecretNotFound to ErrTenantNotFound")
	}
}

func TestStore_Load_NilSecretValue_ReturnsError(t *testing.T) {
	store := newStoreWithClient(nilValueSecretsClient{}, nil)

	_, err := store.Load(context.Background(), "contoso.onmicrosoft.com")
	if err == nil {
		t.Fatal("Load() expected error on nil secret value, got nil")
	}
}

// nilValueSecretsClient returns a GetSecret response whose Value pointer is
// nil — a possible defect in a live Key Vault response. Store.Load must
// detect this rather than panic on a nil dereference.
type nilValueSecretsClient struct{}

func (nilValueSecretsClient) GetSecret(
	_ context.Context, _, _ string, _ *azsecrets.GetSecretOptions,
) (azsecrets.GetSecretResponse, error) {
	return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{Value: nil}}, nil
}

func (nilValueSecretsClient) SetSecret(
	_ context.Context, _ string, _ azsecrets.SetSecretParameters, _ *azsecrets.SetSecretOptions,
) (azsecrets.SetSecretResponse, error) {
	return azsecrets.SetSecretResponse{}, nil
}

func TestStore_Save_EmptyToken_ReturnsError(t *testing.T) {
	store, fake := newTestStore(t)

	err := store.Save(context.Background(), "contoso.onmicrosoft.com", "")
	if err == nil {
		t.Fatal("Save() expected error for empty token, got nil")
	}
	if fake.setCalls.Load() != 0 {
		t.Errorf("Save() with empty token called SetSecret %d times; want 0",
			fake.setCalls.Load())
	}
}

func TestStore_Save_SetsTenantIDTag(t *testing.T) {
	store, fake := newTestStore(t)

	const tenant = "contoso.onmicrosoft.com"
	if err := store.Save(context.Background(), tenant, "rt_xyz"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	sec, ok := fake.get(secretName(defaultPrefix, tenant))
	if !ok {
		t.Fatal("secret not stored under expected name")
	}

	gotTenant, ok := sec.tags["tenantID"]
	if !ok || gotTenant == nil {
		t.Fatal("secret missing tenantID tag")
	}
	if *gotTenant != tenant {
		t.Errorf("tenantID tag = %q, want %q", *gotTenant, tenant)
	}

	managedBy, ok := sec.tags["managedBy"]
	if !ok || managedBy == nil || *managedBy != "chaperone" {
		t.Errorf("managedBy tag = %v, want \"chaperone\"", managedBy)
	}
}

func TestStore_Save_SetSecretError_Propagated(t *testing.T) {
	store, fake := newTestStore(t)

	sentinel := errors.New("throttled")
	fake.setErr = sentinel

	err := store.Save(context.Background(), "contoso.onmicrosoft.com", "rt_abc")
	if err == nil {
		t.Fatal("Save() expected error when SetSecret fails, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Save() should wrap sentinel error, got: %v", err)
	}
}

func TestStore_ValidateTenantID_TruncatesLongInvalidID(t *testing.T) {
	store, _ := newTestStore(t)

	// Invalid char triggers failure; length > 64 triggers the truncation branch.
	longInvalid := strings.Repeat("a", 70) + "/bad"

	_, err := store.Load(context.Background(), longInvalid)
	if err == nil {
		t.Fatal("Load() expected validation error, got nil")
	}
	// Error message must not contain the full invalid string and must end
	// with the truncation marker inside the quoted tenant ID.
	if !strings.Contains(err.Error(), `..."`) {
		t.Errorf("Load() error message missing truncation marker: %v", err)
	}
	if strings.Contains(err.Error(), "/bad") {
		t.Error("Load() error message leaked the tail of a long invalid tenantID")
	}
}

func TestStore_Save_RotationCreatesNewValue(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	const tenant = "contoso.onmicrosoft.com"

	if err := store.Save(ctx, tenant, "rt_v1"); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save(ctx, tenant, "rt_v2"); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := store.Load(ctx, tenant)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "rt_v2" {
		t.Errorf("Load() after rotation = %q, want %q", got, "rt_v2")
	}
}

func TestStore_LoadSave_ValidatesTenantID(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
	}{
		{"empty", ""},
		{"path traversal", "../evil"},
		{"leading slash", "/etc"},
		{"contains slash", "tenant/sub"},
		{"contains space", "ten ant"},
		{"starts with dot", ".hidden"},
		{"null byte", "tenant\x00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, fake := newTestStore(t)
			ctx := context.Background()

			if _, err := store.Load(ctx, tt.tenantID); err == nil {
				t.Error("Load() expected validation error, got nil")
			}
			if err := store.Save(ctx, tt.tenantID, "rt_abc"); err == nil {
				t.Error("Save() expected validation error, got nil")
			}
			if fake.getCalls.Load() != 0 {
				t.Errorf("invalid tenantID reached GetSecret (%d calls)", fake.getCalls.Load())
			}
			if fake.setCalls.Load() != 0 {
				t.Errorf("invalid tenantID reached SetSecret (%d calls)", fake.setCalls.Load())
			}
		})
	}
}

func TestStore_SecretNameIsValidKeyVaultName(t *testing.T) {
	// Tenant IDs with dots must produce names using only [0-9a-zA-Z-].
	tenants := []string{
		"contoso.onmicrosoft.com",
		"my-company.onmicrosoft.com",
		"00000000-0000-0000-0000-000000000000",
		"common",
	}

	for _, tenant := range tenants {
		name := secretName(defaultPrefix, tenant)
		if !isValidKeyVaultSecretName(name) {
			t.Errorf("secretName(%q) = %q, not a valid Key Vault secret name", tenant, name)
		}
	}
}

func TestStore_ConcurrentSaveLoad(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	const tenant = "contoso.onmicrosoft.com"

	// Seed so Load never hits ErrTenantNotFound.
	if err := store.Save(ctx, tenant, "rt_seed"); err != nil {
		t.Fatalf("seed Save() error = %v", err)
	}

	const (
		goroutines = 10
		iterations = 50
	)

	allowedValues := map[string]bool{"rt_seed": true}
	for g := range goroutines {
		allowedValues[fmt.Sprintf("rt_v%d", g)] = true
	}

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for g := range goroutines {
		go func() {
			defer wg.Done()
			value := fmt.Sprintf("rt_v%d", g)
			for range iterations {
				if err := store.Save(ctx, tenant, value); err != nil {
					t.Errorf("Save() error = %v", err)
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range iterations {
				got, err := store.Load(ctx, tenant)
				if err != nil {
					t.Errorf("Load() error = %v", err)
					return
				}
				if !allowedValues[got] {
					t.Errorf("Load() = %q, not one of the written values", got)
					return
				}
			}
		}()
	}

	wg.Wait()
}

func TestNewStore_MissingVaultURL(t *testing.T) {
	_, err := NewStore(Config{Credential: &fakeTokenCredential{}})
	if err == nil {
		t.Fatal("NewStore() expected error for missing VaultURL, got nil")
	}
	if !strings.Contains(err.Error(), "VaultURL") {
		t.Errorf("error = %q, want substring %q", err.Error(), "VaultURL")
	}
}

func TestNewStore_MissingCredential(t *testing.T) {
	_, err := NewStore(Config{VaultURL: "https://myvault.vault.azure.net/"})
	if err == nil {
		t.Fatal("NewStore() expected error for missing Credential, got nil")
	}
	if !strings.Contains(err.Error(), "Credential") {
		t.Errorf("error = %q, want substring %q", err.Error(), "Credential")
	}
}

func TestNewStore_DefaultPrefix(t *testing.T) {
	s, err := NewStore(Config{
		VaultURL:   "https://myvault.vault.azure.net/",
		Credential: &fakeTokenCredential{},
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if s.prefix != defaultPrefix {
		t.Errorf("prefix = %q, want %q", s.prefix, defaultPrefix)
	}
}

func TestNewStore_CustomPrefix(t *testing.T) {
	const customPrefix = "env-staging-"
	s, err := NewStore(Config{
		VaultURL:   "https://myvault.vault.azure.net/",
		Credential: &fakeTokenCredential{},
		Prefix:     customPrefix,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if s.prefix != customPrefix {
		t.Errorf("prefix = %q, want %q", s.prefix, customPrefix)
	}
}

func TestStore_CustomLogger_EmitsOnNotFound(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := newStoreWithClient(newFakeSecretsClient(), custom)

	if _, err := store.Load(context.Background(), "contoso.onmicrosoft.com"); err == nil {
		t.Fatal("Load() expected ErrTenantNotFound, got nil")
	}

	got := buf.String()
	if got == "" {
		t.Fatal("expected configured logger to receive a log line, got none")
	}
	if !strings.Contains(got, "contoso.onmicrosoft.com") {
		t.Errorf("log output missing tenant_id; got: %s", got)
	}
}

func TestStore_NilLogger_LazyResolvesToDefault(t *testing.T) {
	// This test mutates slog.Default() for the duration of the run. Do NOT
	// mark it t.Parallel(): any sibling test that logs or reads slog.Default()
	// would race with this one.
	//
	// Rewire slog.Default to capture output, run Load, and verify the
	// message landed on the global default — confirming nil config.Logger
	// means "use slog.Default() at emit time".
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	store := newStoreWithClient(newFakeSecretsClient(), nil)

	if _, err := store.Load(context.Background(), "contoso.onmicrosoft.com"); err == nil {
		t.Fatal("Load() expected ErrTenantNotFound, got nil")
	}

	if !strings.Contains(buf.String(), "contoso.onmicrosoft.com") {
		t.Errorf("expected default logger to receive log line; got: %q", buf.String())
	}
}

// fakeTokenCredential implements azcore.TokenCredential for NewStore tests
// that don't make any network calls.
type fakeTokenCredential struct{}

func (*fakeTokenCredential) GetToken(
	_ context.Context, _ policy.TokenRequestOptions,
) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, errors.New("fake credential not usable")
}

// isValidKeyVaultSecretName validates that name matches Key Vault's secret
// name constraints: 1–127 chars of [0-9a-zA-Z-].
func isValidKeyVaultSecretName(name string) bool {
	if len(name) < 1 || len(name) > 127 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
