#!/bin/bash
# Copyright 2026 CloudBlue LLC
# SPDX-License-Identifier: Apache-2.0
#
# scripts/test-distributor-workflow.sh
# Validates that the Distributor "Own Repo" workflow works with current code.
#
# This script creates a temporary external Go module that imports the
# Chaperone public API (chaperone.Run) and SDK, builds a binary, and
# verifies the binary executes correctly.
#
# Usage:
#   ./scripts/test-distributor-workflow.sh
#
# This simulates a Distributor creating their own repo per Design Spec §7.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# Create a workspace under the user's home to avoid Go 1.25+ temp root
# restrictions (Go ignores go.mod in system temp roots).
WORK_DIR="${HOME}/.cache/chaperone-distributor-test"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "=== Distributor 'Own Repo' Workflow Test ==="
echo "  Repo: $REPO_ROOT"
echo "  Work: $WORK_DIR"
echo ""

# --- Step 1: Create external module ---
echo "[1/5] Creating external Go module..."
cd "$WORK_DIR"
go mod init github.com/test/distributor-proxy

# --- Step 2: Write Distributor main.go ---
echo "[2/5] Writing Distributor main.go..."
cat > main.go << 'GOEOF'
// Simulated Distributor main.go — uses chaperone.Run() public API.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudblue/chaperone"
	"github.com/cloudblue/chaperone/sdk"
)

// testPlugin is a minimal Plugin implementation for testing.
type testPlugin struct{}

func (p *testPlugin) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	return &sdk.Credential{
		Headers: map[string]string{"Authorization": "Bearer test-token"},
	}, nil
}

func (p *testPlugin) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *testPlugin) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

func main() {
	// Verify compile-time interface compliance.
	var _ sdk.Plugin = &testPlugin{}

	// If --verify-only flag is passed, just print success and exit.
	if len(os.Args) > 1 && os.Args[1] == "--verify-only" {
		fmt.Println("Distributor build OK — all imports resolved, interface satisfied")
		os.Exit(0)
	}

	// Otherwise, start the proxy (requires config).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := chaperone.Run(ctx, &testPlugin{}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
GOEOF

# --- Step 3: Point to local checkout ---
echo "[3/5] Configuring module replacements (local checkout)..."
go mod edit -replace "github.com/cloudblue/chaperone=$REPO_ROOT"
go mod edit -replace "github.com/cloudblue/chaperone/sdk=$REPO_ROOT/sdk"
go mod tidy

# --- Step 4: Build the binary ---
echo "[4/5] Building Distributor binary..."
go build -o "$WORK_DIR/distributor-proxy" .

# --- Step 5: Verify execution ---
echo "[5/5] Verifying binary execution..."
"$WORK_DIR/distributor-proxy" --verify-only

echo ""
echo "=== PASS: Distributor 'Own Repo' workflow validated ==="
echo ""
echo "This confirms:"
echo "  ✓ SDK module (github.com/cloudblue/chaperone/sdk) is importable"
echo "  ✓ Core module (github.com/cloudblue/chaperone) is importable"
echo "  ✓ chaperone.Run() public API is accessible"
echo "  ✓ sdk.Plugin interface is implementable from external code"
echo "  ✓ Binary compiles and executes successfully"
