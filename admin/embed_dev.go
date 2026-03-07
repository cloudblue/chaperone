// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

//go:build dev

package admin

import (
	"fmt"
	"io/fs"
	"os"
)

const devUIDir = "admin/ui/dist"

// loadUIAssets serves the Vue SPA from the filesystem during development.
// This assumes the working directory is the repository root (e.g., via
// make run-admin). For hot reload, run "pnpm dev" separately and use
// the Vite dev server proxy instead.
func loadUIAssets() (fs.FS, error) {
	if _, err := os.Stat(devUIDir); err != nil {
		return nil, fmt.Errorf("UI dist directory not found at %s: run 'make build-admin-ui' first: %w", devUIDir, err)
	}
	return os.DirFS(devUIDir), nil
}
