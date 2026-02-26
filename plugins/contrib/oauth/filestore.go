// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileStore is a file-backed [TokenStore] that reads and writes a single
// refresh token to a plain text file.
//
// Writes are atomic: the token is written to a temporary file in the same
// directory, synced to disk, and then renamed to the target path. This
// prevents corruption from a crash mid-write.
//
// It is safe for concurrent use from multiple goroutines.
type FileStore struct {
	path string
}

// NewFileStore creates a FileStore that persists the refresh token at path.
// It panics if path is empty.
func NewFileStore(path string) *FileStore {
	if path == "" {
		panic("oauth.NewFileStore: path must not be empty")
	}
	return &FileStore{path: path}
}

// Load retrieves the current refresh token from disk.
func (f *FileStore) Load(_ context.Context) (string, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return "", fmt.Errorf("loading token from %s: %w", f.path, err)
	}

	return strings.TrimRight(string(data), "\r\n"), nil
}

// Save persists a rotated refresh token to disk atomically.
func (f *FileStore) Save(_ context.Context, refreshToken string) error {
	if refreshToken == "" {
		return errors.New("refusing to save empty refresh token")
	}

	dir := filepath.Dir(f.path)
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
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(refreshToken); err != nil {
		tmp.Close()
		return fmt.Errorf("writing token to temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing token to disk: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("setting token file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, f.path); err != nil { // #nosec G703 -- path is set once at construction time, not from request input
		return fmt.Errorf("renaming token file: %w", err)
	}

	tmpPath = "" // Prevent deferred cleanup.
	return nil
}
