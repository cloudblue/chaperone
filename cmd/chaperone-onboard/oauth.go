// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// oauthCmd handles the "oauth" subcommand for generic OAuth2 authorization code flow.
//
//nolint:funlen // CLI command handler, acceptable to be longer
func oauthCmd(args []string) error {
	fs := flag.NewFlagSet("oauth", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	authorizeURL := fs.String("authorize-url", "", "Authorization endpoint (required)")
	tokenURL := fs.String("token-url", "", "Token endpoint (required)")
	clientID := fs.String("client-id", "", "OAuth2 client ID (required)")
	scope := fs.String("scope", "", "Space-delimited scopes (e.g. \"openid offline_access\")")
	extraParams := fs.String("extra-params", "", "Extra authorize URL params (key=value,key=value)")
	port := fs.Int("port", 0, "Local callback port (default: OS-assigned)")
	timeout := fs.Duration("timeout", 5*time.Minute, "Consent timeout")
	noBrowser := fs.Bool("no-browser", false, "Print authorization URL instead of opening browser")
	noPKCE := fs.Bool("no-pkce", false, "Disable PKCE for legacy providers")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: chaperone-onboard oauth [options]

Perform a generic OAuth2 authorization code flow with PKCE (S256).

Required:
  -authorize-url   Authorization endpoint (e.g. https://auth.example.com/authorize)
  -token-url       Token endpoint (e.g. https://auth.example.com/token)
  -client-id       OAuth2 client ID

Optional:
  -scope           Space-delimited scopes (e.g. "openid profile offline_access")
  -extra-params    Extra query params for authorize URL (key=value,key=value)
  -port            Local callback port (default: 0 = OS-assigned; use fixed port
                   if your provider requires an exact redirect URI match)
  -timeout         Consent timeout (default: 5m)
  -no-browser      Print authorization URL instead of opening browser
  -no-pkce         Disable PKCE for legacy providers that don't support it

Client secret: read from CHAPERONE_ONBOARD_CLIENT_SECRET env var.

Example:
  CHAPERONE_ONBOARD_CLIENT_SECRET=secret123 chaperone-onboard oauth \
    -authorize-url https://auth.example.com/authorize \
    -token-url https://auth.example.com/token \
    -client-id my-app \
    -scope "openid offline_access"
`)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errUsage
	}

	// Validate required flags
	if err := validateURL(*authorizeURL); err != nil {
		return fmt.Errorf("-authorize-url: %w", err)
	}
	if err := validateURL(*tokenURL); err != nil {
		return fmt.Errorf("-token-url: %w", err)
	}
	if err := validateNonEmpty("client-id", *clientID); err != nil {
		return fmt.Errorf("-%w", err)
	}

	if !isHTTPS(*authorizeURL) || !isHTTPS(*tokenURL) {
		fmt.Fprintf(os.Stderr, "WARNING: Using HTTP URLs. Credentials will be sent in plaintext.\n")
		fmt.Fprintf(os.Stderr, "         Production OAuth2 endpoints should always use HTTPS.\n\n")
	}

	clientSecret := os.Getenv(envClientSecret)
	if clientSecret == "" {
		return fmt.Errorf("%s environment variable is required", envClientSecret)
	}

	cfg := consentConfig{
		authorizeURL: *authorizeURL,
		tokenURL:     *tokenURL,
		clientID:     *clientID,
		clientSecret: clientSecret,
		scopes:       *scope,
		port:         *port,
		timeout:      *timeout,
		noBrowser:    *noBrowser,
		usePKCE:      !*noPKCE,
	}

	if *extraParams != "" {
		parsed, err := parseExtraParams(*extraParams)
		if err != nil {
			return fmt.Errorf("-extra-params: %w", err)
		}
		cfg.extraAuthParams = parsed
	}

	refreshToken, err := runConsent(cfg)
	if err != nil {
		return err
	}

	fmt.Print(refreshToken)
	return nil
}

// parseExtraParams parses a "key=value,key=value" string into url.Values.
func parseExtraParams(s string) (url.Values, error) {
	params := url.Values{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid parameter %q (expected key=value)", pair)
		}
		params.Set(k, v)
	}
	return params, nil
}
