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

// maxResourceFilenameLen is the maximum length of a sanitized resource
// filename. This prevents ENAMETOOLONG from the OS on crafted inputs.
const maxResourceFilenameLen = 200

// FileStore is a file-backed [TokenStore] that organizes refresh tokens in a
// directory tree: baseDir/{tenantID}/{sanitizedResource}.
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

// Load retrieves the current refresh token for the given tenant and resource.
// Returns [contrib.ErrTenantNotFound] if no token file exists.
func (f *FileStore) Load(_ context.Context, tenantID, resource string) (string, error) {
	if err := validateTenantID(tenantID); err != nil {
		return "", err
	}
	if err := validateResource(resource); err != nil {
		return "", err
	}

	path := f.tokenPath(tenantID, resource)

	data, err := os.ReadFile(path) // #nosec G304 -- path is built from validated tenantID and sanitized resource under baseDir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("no token for tenant %s, resource %s: %w",
				tenantID, resource, contrib.ErrTenantNotFound)
		}
		return "", fmt.Errorf("loading token for tenant %s: %w", tenantID, err)
	}

	return strings.TrimRight(string(data), "\r\n"), nil
}

// Save persists a rotated refresh token to disk atomically.
func (f *FileStore) Save(_ context.Context, tenantID, resource, refreshToken string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}
	if err := validateResource(resource); err != nil {
		return err
	}
	if refreshToken == "" {
		return errors.New("refusing to save empty refresh token")
	}

	path := f.tokenPath(tenantID, resource)
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
	if err := os.Rename(tmpPath, path); err != nil { // #nosec G703 -- tenantID is validated, resource is sanitized
		return fmt.Errorf("renaming token file: %w", err)
	}

	tmpPath = "" // Prevent deferred cleanup.
	return nil
}

func (f *FileStore) tokenPath(tenantID, resource string) string {
	return filepath.Join(f.baseDir, tenantID, sanitizeResource(resource))
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

// validateResource rejects empty or scheme-only resource strings that would
// produce an empty filename after sanitization, collapsing the file path
// into the tenant directory itself.
func validateResource(resource string) error {
	if resource == "" {
		return errors.New("resource must not be empty")
	}

	name := sanitizeResource(resource)
	if name == "" {
		return fmt.Errorf("resource %q produces an empty filename after sanitization", resource)
	}
	if len(name) > maxResourceFilenameLen {
		return fmt.Errorf("sanitized resource filename exceeds %d bytes", maxResourceFilenameLen)
	}
	return nil
}

// sanitizeResource converts a resource URL into a filesystem-safe filename.
// It strips the URL scheme and replaces characters outside [a-zA-Z0-9._-]
// with underscores.
//
// Note: different resource URLs can theoretically produce the same filename
// (e.g., "https://api.example.com/v1" and "https://api.example.com?v1" both
// become "api.example.com_v1"). This is acceptable for real Microsoft
// resource URIs, which are clean hostnames like "https://graph.microsoft.com".
func sanitizeResource(resource string) string {
	s := strings.TrimPrefix(resource, "https://")
	s = strings.TrimPrefix(s, "http://")

	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '-' {
			return r
		}
		return '_'
	}, s)
}
