// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

//go:build tools

// Package-level dependency anchors. This file ensures that go mod tidy
// retains dependencies declared in go.mod that are used by sub-packages
// (oauth/, microsoft/) but not yet by the root contrib package itself.
//
// golang.org/x/sync — singleflight for token deduplication (used by oauth/ and microsoft/).
package contrib

import _ "golang.org/x/sync/singleflight"
