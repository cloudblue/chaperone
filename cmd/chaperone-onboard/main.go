// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Command chaperone-onboard performs a one-time OAuth2 authorization code flow
// to obtain a refresh token for seeding a TokenStore.
//
// This is a setup-time utility, not a runtime dependency of the proxy.
// It ships as a separate binary alongside chaperone.
//
// Usage:
//
//	chaperone-onboard oauth       # Generic OAuth2 (any provider)
//	chaperone-onboard microsoft   # Microsoft SAM shortcut
//	chaperone-onboard -version    # Show version
package main

import (
	"errors"
	"fmt"
	"os"
)

// Version information (set via ldflags during build).
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

const envClientSecret = "CHAPERONE_ONBOARD_CLIENT_SECRET" // #nosec G101 -- env var name, not a credential

// Sentinel errors for exit code mapping.
var (
	errUsage          = errors.New("usage error")
	errConsentTimeout = errors.New("consent timeout")
	errExchangeFailed = errors.New("token exchange failed")
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitCode(err))
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return errUsage
	}

	switch args[0] {
	case "oauth":
		return oauthCmd(args[1:])
	case "microsoft":
		return microsoftCmd(args[1:])
	case "-version", "--version", "version":
		fmt.Fprintf(os.Stderr, "chaperone-onboard\nVersion: %s\nCommit: %s\nBuilt: %s\n",
			Version, GitCommit, BuildDate)
		return nil
	case "-h", "-help", "--help", "help":
		printUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\n\n", args[0]) // #nosec G705 -- CLI stderr output, not web
		printUsage()
		return errUsage
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: chaperone-onboard <command> [options]

Perform a one-time OAuth2 authorization code flow to obtain a refresh token.

Commands:
  oauth       Generic OAuth2 (any provider)
  microsoft   Microsoft Secure Application Model shortcut

Options:
  -version   Show version and exit

Client secret: read from %s env var.
`, envClientSecret)
}

// exitCode maps errors to exit codes per the plan:
// 0=success, 1=usage/validation error, 2=consent timeout, 3=token exchange failed, 4=internal error.
func exitCode(err error) int {
	switch {
	case errors.Is(err, errUsage):
		return 1
	case errors.Is(err, errConsentTimeout):
		return 2
	case errors.Is(err, errExchangeFailed):
		return 3
	default:
		return 4
	}
}
