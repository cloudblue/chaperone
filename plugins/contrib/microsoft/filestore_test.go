// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cloudblue/chaperone/plugins/contrib"
)

func TestFileStore_SaveAndLoad_RoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir())
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

func TestFileStore_Load_MissingFile_ReturnsErrTenantNotFound(t *testing.T) {
	store := NewFileStore(t.TempDir())

	_, err := store.Load(context.Background(), "contoso.onmicrosoft.com")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
	if !errors.Is(err, contrib.ErrTenantNotFound) {
		t.Errorf("Load() error should wrap contrib.ErrTenantNotFound, got: %v", err)
	}
}

func TestFileStore_Save_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	store := NewFileStore(base)

	const tenant = "contoso.onmicrosoft.com"
	if err := store.Save(context.Background(), tenant, "tok"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Token file should exist directly under baseDir.
	tokenPath := filepath.Join(base, tenant)
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("expected token file at %s, got error: %v", tokenPath, err)
	}
	if info.IsDir() {
		t.Errorf("expected %s to be a file, not a directory", tokenPath)
	}
}

func TestFileStore_Save_MultipleTenants_Isolated(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	if err := store.Save(ctx, "tenant-a", "token-a"); err != nil {
		t.Fatalf("Save(tenant-a) error = %v", err)
	}
	if err := store.Save(ctx, "tenant-b", "token-b"); err != nil {
		t.Fatalf("Save(tenant-b) error = %v", err)
	}

	gotA, err := store.Load(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("Load(tenant-a) error = %v", err)
	}
	gotB, err := store.Load(ctx, "tenant-b")
	if err != nil {
		t.Fatalf("Load(tenant-b) error = %v", err)
	}

	if gotA != "token-a" {
		t.Errorf("tenant-a token = %q, want %q", gotA, "token-a")
	}
	if gotB != "token-b" {
		t.Errorf("tenant-b token = %q, want %q", gotB, "token-b")
	}
}

func TestFileStore_Save_OverwritesExisting(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	tenant := "contoso.onmicrosoft.com"

	if err := store.Save(ctx, tenant, "first"); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save(ctx, tenant, "second"); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := store.Load(ctx, tenant)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "second" {
		t.Errorf("Load() = %q, want %q", got, "second")
	}
}

func TestFileStore_Save_EmptyToken_ReturnsError(t *testing.T) {
	store := NewFileStore(t.TempDir())

	err := store.Save(context.Background(), "contoso.onmicrosoft.com", "")
	if err == nil {
		t.Fatal("Save(\"\") expected error, got nil")
	}
}

func TestFileStore_NewFileStore_EmptyBaseDir_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewFileStore(\"\") should panic")
		}
	}()

	NewFileStore("")
}

func TestFileStore_Save_InvalidTenantID_ReturnsError(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	tests := []struct {
		name     string
		tenantID string
	}{
		{"path traversal", "../evil"},
		{"absolute path", "/etc"},
		{"empty string", ""},
		{"contains slash", "tenant/sub"},
		{"starts with dot", ".hidden"},
		{"contains space", "tenant name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Save(ctx, tt.tenantID, "token")
			if err == nil {
				t.Errorf("Save(tenantID=%q) expected error, got nil", tt.tenantID)
			}
		})
	}
}

func TestFileStore_Load_InvalidTenantID_ReturnsError(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	tests := []struct {
		name     string
		tenantID string
	}{
		{"path traversal", "../evil"},
		{"absolute path", "/etc"},
		{"empty string", ""},
		{"contains slash", "tenant/sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Load(ctx, tt.tenantID)
			if err == nil {
				t.Errorf("Load(tenantID=%q) expected error, got nil", tt.tenantID)
			}
			if errors.Is(err, contrib.ErrTenantNotFound) {
				t.Errorf("Load(tenantID=%q) should not be ErrTenantNotFound, it's a validation error", tt.tenantID)
			}
		})
	}
}

func TestFileStore_Save_FilePermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}

	base := t.TempDir()
	store := NewFileStore(base)
	tenant := "contoso.onmicrosoft.com"

	if err := store.Save(context.Background(), tenant, "secret"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check file permissions.
	tokenPath := store.tokenPath(tenant)
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat(token) error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Errorf("token file permissions = %o, want 0600", perm)
	}
}

func TestFileStore_ConcurrentSaveLoad(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()

	tenant := "contoso.onmicrosoft.com"

	const written = "token-from-goroutine"

	// Seed with an initial value so Load never hits a missing file.
	if err := store.Save(ctx, tenant, "initial"); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	const goroutines = 10
	const iterations = 50

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			for j := range iterations {
				if j%2 == 0 {
					if err := store.Save(ctx, tenant, written); err != nil {
						errs[id] = err
						return
					}
				} else {
					if _, err := store.Load(ctx, tenant); err != nil {
						errs[id] = err
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	// Verify the final value is one of the tokens actually written.
	got, err := store.Load(ctx, tenant)
	if err != nil {
		t.Fatalf("final Load() error = %v", err)
	}
	if got != written && got != "initial" {
		t.Errorf("final token = %q, want %q or %q", got, written, "initial")
	}
}
