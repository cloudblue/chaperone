// Copyright 2024-2026 CloudBlue
// SPDX-License-Identifier: Apache-2.0

package reference_test

import (
	"testing"

	"github.com/cloudblue/chaperone/plugins/reference"
	"github.com/cloudblue/chaperone/sdk/compliance"
)

func TestReferencePluginCompliance(t *testing.T) {
	// Test with a non-existent file to verify error handling
	plugin := reference.New("testdata/credentials.json")
	compliance.VerifyContract(t, plugin)
}
