// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileStore_SaveAndLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	store := NewFileStore(path)
	ctx := context.Background()

	const want = "rt_abc123_secret"
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Errorf("Load() = %q, want %q", got, want)
	}
}

func TestFileStore_Load_MissingFile_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent")
	store := NewFileStore(path)

	_, err := store.Load(context.Background())
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load() error chain should contain os.ErrNotExist, got: %v", err)
	}
}

func TestFileStore_Save_CreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "dir", "token")
	store := NewFileStore(path)

	if err := store.Save(context.Background(), "token-value"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s, got error: %v", path, err)
	}
}

func TestFileStore_Save_OverwritesExistingToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	store := NewFileStore(path)
	ctx := context.Background()

	if err := store.Save(ctx, "first-token"); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save(ctx, "second-token"); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "second-token" {
		t.Errorf("Load() = %q, want %q", got, "second-token")
	}
}

func TestFileStore_Save_EmptyToken_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	store := NewFileStore(path)

	err := store.Save(context.Background(), "")
	if err == nil {
		t.Fatal("Save(\"\") expected error, got nil")
	}
}

func TestFileStore_NewFileStore_EmptyPath_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewFileStore(\"\") should panic")
		}
	}()

	NewFileStore("")
}

func TestFileStore_Load_TrimsTrailingNewlines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"trailing LF", "my-token\n", "my-token"},
		{"trailing CRLF", "my-token\r\n", "my-token"},
		{"no trailing newline", "my-token", "my-token"},
		{"interior spaces preserved", "my token with spaces\n", "my token with spaces"},
		{"leading space preserved", " leading-space\n", " leading-space"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("writing test file: %v", err)
			}

			store := NewFileStore(path)
			got, err := store.Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFileStore_Save_FilePermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}

	path := filepath.Join(t.TempDir(), "token")
	store := NewFileStore(path)

	if err := store.Save(context.Background(), "secret-token"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != fs.FileMode(0o600) {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestFileStore_ConcurrentSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	store := NewFileStore(path)
	ctx := context.Background()

	const written = "token-from-goroutine"

	// Seed with an initial value so Load never hits a missing file.
	if err := store.Save(ctx, "initial"); err != nil {
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
					if err := store.Save(ctx, written); err != nil {
						errs[id] = err
						return
					}
				} else {
					if _, err := store.Load(ctx); err != nil {
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
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("final Load() error = %v", err)
	}
	if got != written && got != "initial" {
		t.Errorf("final token = %q, want %q or %q", got, written, "initial")
	}
}
