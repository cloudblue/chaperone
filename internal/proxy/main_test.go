// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"os"
	"testing"

	"github.com/cloudblue/chaperone/internal/proxy"
)

// TestMain runs before all tests in the package.
// It enables insecure HTTP targets for testing purposes only.
func TestMain(m *testing.M) {
	// Allow HTTP targets during tests (they use httptest.NewServer which is HTTP)
	cleanup := proxy.SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	os.Exit(m.Run())
}
