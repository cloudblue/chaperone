// Copyright 2024-2026 CloudBlue
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Chaperone egress proxy.
package main

import (
	"fmt"
	"os"
)

// Version information (set via ldflags during build)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	fmt.Printf("Chaperone Egress Proxy\n")
	fmt.Printf("Version: %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate)
	fmt.Println()
	fmt.Println("🚧 Work in Progress - PoC Phase")

	os.Exit(0)
}
