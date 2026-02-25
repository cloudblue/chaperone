// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"
)

const defaultMicrosoftEndpoint = "https://login.microsoftonline.com" // #nosec G101 -- URL endpoint, not a credential

// microsoftCmd handles the "microsoft" subcommand for Microsoft SAM consent.
//
//nolint:funlen // CLI command handler, acceptable to be longer
func microsoftCmd(args []string) error {
	fs := flag.NewFlagSet("microsoft", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	tenant := fs.String("tenant", "", "Azure AD tenant ID (GUID or domain, required)")
	clientID := fs.String("client-id", "", "Azure AD application (client) ID (required)")
	resource := fs.String("resource", "", "Target resource URI (e.g. https://graph.microsoft.com) (required)")
	endpoint := fs.String("endpoint", defaultMicrosoftEndpoint, "Token endpoint base URL (override for sovereign clouds)")
	port := fs.Int("port", 0, "Local callback port (default: OS-assigned)")
	timeout := fs.Duration("timeout", 5*time.Minute, "Consent timeout")
	noBrowser := fs.Bool("no-browser", false, "Print authorization URL instead of opening browser")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: chaperone-onboard microsoft [options]

Perform Microsoft Secure Application Model consent using Azure AD's v1 endpoint.

Derives authorization and token URLs from the tenant ID. Uses the v1 endpoint
with the resource parameter. For v2 endpoints (scope-based), use the generic
oauth subcommand with -scope instead.

Required:
  -tenant      Azure AD tenant ID (GUID or domain, e.g. contoso.onmicrosoft.com)
  -client-id   Azure AD application (client) ID
  -resource    Target resource URI (e.g. https://graph.microsoft.com)

Optional:
  -endpoint    Token endpoint base URL (default: https://login.microsoftonline.com;
               override for sovereign clouds, e.g. https://login.microsoftonline.us)
  -port        Local callback port (default: 0 = OS-assigned; use fixed port
               if your app registration requires an exact redirect URI)
  -timeout     Consent timeout (default: 5m)
  -no-browser  Print authorization URL instead of opening browser

Client secret: read from CHAPERONE_ONBOARD_CLIENT_SECRET env var.

Example:
  CHAPERONE_ONBOARD_CLIENT_SECRET=s3cret chaperone-onboard microsoft \
    -tenant contoso.onmicrosoft.com \
    -client-id 12345678-abcd-1234-abcd-1234567890ab \
    -resource https://graph.microsoft.com
`)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errUsage
	}

	// Validate required flags
	if err := validateTenantID(*tenant); err != nil {
		return fmt.Errorf("-tenant: %w", err)
	}
	if err := validateNonEmpty("client-id", *clientID); err != nil {
		return fmt.Errorf("-%w", err)
	}
	if err := validateNonEmpty("resource", *resource); err != nil {
		return fmt.Errorf("-%w", err)
	}

	clientSecret := os.Getenv(envClientSecret)
	if clientSecret == "" {
		return fmt.Errorf("%s environment variable is required", envClientSecret)
	}

	// Derive Azure AD v1 endpoints from tenant ID
	authorizeURL := fmt.Sprintf("%s/%s/oauth2/authorize", *endpoint, *tenant)
	tokenURL := fmt.Sprintf("%s/%s/oauth2/token", *endpoint, *tenant)

	// resource is both an authorize URL param and a token exchange param
	extraAuth := url.Values{}
	extraAuth.Set("resource", *resource)
	extraToken := url.Values{}
	extraToken.Set("resource", *resource)

	cfg := consentConfig{
		authorizeURL:     authorizeURL,
		tokenURL:         tokenURL,
		clientID:         *clientID,
		clientSecret:     clientSecret,
		extraAuthParams:  extraAuth,
		extraTokenParams: extraToken,
		port:             *port,
		timeout:          *timeout,
		noBrowser:        *noBrowser,
		usePKCE:          true, // Azure AD v1 supports PKCE
	}

	refreshToken, err := runConsent(cfg)
	if err != nil {
		return err
	}

	fmt.Print(refreshToken)
	return nil
}
