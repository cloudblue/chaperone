// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudblue/chaperone/plugins/contrib"
)

// FileStore is a file-backed [TokenStore] that stores one refresh token per
// tenant at baseDir/{tenantID}.
//
// Writes are atomic: the token is written to a temporary file in the same
// directory, synced to disk, and then renamed to the target path. This
// prevents corruption from a crash mid-write.
//
// TenantIDs are validated against [validTenantID] to prevent path traversal,
// regardless of whether the caller has already validated them.
//
// It is safe for concurrent use from multiple goroutines.
type FileStore struct {
	baseDir string
}

// NewFileStore creates a FileStore rooted at baseDir.
// It panics if baseDir is empty.
func NewFileStore(baseDir string) *FileStore {
	if baseDir == "" {
		panic("microsoft.NewFileStore: baseDir must not be empty")
	}
	return &FileStore{baseDir: baseDir}
}

// Load retrieves the current refresh token for the given tenant.
// Returns [contrib.ErrTenantNotFound] if no token file exists.
func (f *FileStore) Load(_ context.Context, tenantID string) (string, error) {
	if err := validateTenantID(tenantID); err != nil {
		return "", err
	}

	path := f.tokenPath(tenantID)

	data, err := os.ReadFile(path) // #nosec G304 -- path is built from validated tenantID under baseDir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("no token for tenant %s: %w",
				tenantID, contrib.ErrTenantNotFound)
		}
		return "", fmt.Errorf("loading token for tenant %s: %w", tenantID, err)
	}

	return strings.TrimRight(string(data), "\r\n"), nil
}

// Save persists a rotated refresh token to disk atomically.
func (f *FileStore) Save(_ context.Context, tenantID, refreshToken string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}
	if refreshToken == "" {
		return errors.New("refusing to save empty refresh token")
	}

	path := f.tokenPath(tenantID)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating token directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".token-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file for token: %w", err)
	}
	tmpPath := tmp.Name()

	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(refreshToken); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing token to temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing token to disk: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting token file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil { // #nosec G703 -- tenantID is validated
		return fmt.Errorf("renaming token file: %w", err)
	}

	tmpPath = "" // Prevent deferred cleanup.
	return nil
}

func (f *FileStore) tokenPath(tenantID string) string {
	return filepath.Join(f.baseDir, tenantID)
}

// validateTenantID rejects tenant IDs that could cause path traversal.
// This is defense-in-depth: RefreshTokenSource also validates, but FileStore
// is a public type and must not rely on callers for safety.
func validateTenantID(tenantID string) error {
	if !validTenantID.MatchString(tenantID) {
		display := tenantID
		if len(display) > 64 {
			display = display[:64] + "..."
		}
		return fmt.Errorf("invalid tenant ID %q: must match %s",
			display, validTenantID.String())
	}
	return nil
}
