// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !dev

package admin

import (
	"embed"
	"io/fs"
)

// uiRawAssets holds the compiled Vue SPA build output.
//
// The ui/dist directory must exist at compile time. Build with:
//
//	cd admin/ui && pnpm install && pnpm build
//
// Or use: make build-admin
//
// For development without building the UI, use: go build -tags dev
//
//go:embed all:ui/dist
var uiRawAssets embed.FS

func loadUIAssets() (fs.FS, error) {
	return fs.Sub(uiRawAssets, "ui/dist")
}
