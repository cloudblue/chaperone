// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestBinary builds the chaperone-onboard binary once and returns the path.
func buildTestBinary(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	projectRoot := filepath.Join(wd, "..", "..")

	tmpBinary := filepath.Join(t.TempDir(), "chaperone-onboard-test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", tmpBinary, "./cmd/chaperone-onboard")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\nOutput: %s", err, output)
	}

	return tmpBinary
}

func TestBuild_VersionFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-version failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	for _, want := range []string{"chaperone-onboard", "Version:", "Commit:", "Built:"} {
		if !strings.Contains(outputStr, want) {
			t.Errorf("version output missing %q\nGot: %s", want, outputStr)
		}
	}
}

func TestBuild_NoSubcommand_ExitsWithUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code when no subcommand given")
	}

	if !strings.Contains(string(output), "Usage:") {
		t.Errorf("expected usage message, got: %s", output)
	}
}

func TestBuild_OAuthMissingFlags_ExitsWithError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "oauth")
	cmd.Env = append(os.Environ(), "CHAPERONE_ONBOARD_CLIENT_SECRET=test")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code when required flags missing")
	}

	if !strings.Contains(string(output), "authorize-url") {
		t.Errorf("expected error about missing authorize-url, got: %s", output)
	}
	assertExitCode(t, err, 1)
}

func TestBuild_OAuthBadExtraParams_ExitsWithUsageCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "oauth",
		"-authorize-url", "https://auth.example.com/authorize",
		"-token-url", "https://auth.example.com/token",
		"-client-id", "test",
		"-extra-params", "badpair")
	cmd.Env = append(os.Environ(), "CHAPERONE_ONBOARD_CLIENT_SECRET=test")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for invalid extra-params")
	}

	if !strings.Contains(string(output), "extra-params") {
		t.Errorf("expected error about extra-params, got: %s", output)
	}
	assertExitCode(t, err, 1)
}

func TestBuild_MicrosoftInvalidTenant_ExitsWithError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "microsoft",
		"-tenant", "../etc/passwd",
		"-client-id", "test",
		"-resource", "https://graph.microsoft.com")
	cmd.Env = append(os.Environ(), "CHAPERONE_ONBOARD_CLIENT_SECRET=test")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for invalid tenant")
	}

	if !strings.Contains(string(output), "invalid tenant") {
		t.Errorf("expected error about invalid tenant, got: %s", output)
	}
	assertExitCode(t, err, 1)
}

// TestBuild_MicrosoftE2E_WithoutResource runs a full end-to-end test without -resource.
//
//nolint:funlen // E2E test, acceptable to be longer
func TestBuild_MicrosoftE2E_WithoutResource(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	binary := buildTestBinary(t)

	// Start mock Microsoft endpoint that does NOT require resource param
	mockProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"access_token":  "mock-access-token",
				"refresh_token": "mock-refresh-token-no-resource",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockProvider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Note: no -resource flag
	cmd := exec.CommandContext(ctx, binary, "microsoft",
		"-tenant", "contoso.onmicrosoft.com",
		"-client-id", "test-app-id",
		"-endpoint", mockProvider.URL,
		"-no-browser",
		"-allow-http",
		"-timeout", "15s")
	cmd.Env = append(os.Environ(), "CHAPERONE_ONBOARD_CLIENT_SECRET=test-secret")

	authURL, stdout := runE2EConsent(t, cmd)

	// Verify auth URL does NOT contain resource param
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}
	state := parsed.Query().Get("state")
	redirectURI := parsed.Query().Get("redirect_uri")
	if state == "" || redirectURI == "" {
		t.Fatalf("missing state or redirect_uri in auth URL: %s", authURL)
	}
	if parsed.Query().Get("resource") != "" {
		t.Errorf("auth URL should not contain resource param when omitted: %s", authURL)
	}

	// Simulate the provider callback
	simulateCallback(ctx, t, redirectURI, state, "msft-auth-code")

	if waitErr := cmd.Wait(); waitErr != nil {
		t.Fatalf("binary exited with error: %v", waitErr)
	}

	if got := stdout.String(); got != "mock-refresh-token-no-resource" {
		t.Errorf("stdout = %q, want mock-refresh-token-no-resource", got)
	}
}

// TestBuild_OAuthE2E runs a full end-to-end test with a mock OAuth2 provider.
//
//nolint:funlen // E2E test, acceptable to be longer
func TestBuild_OAuthE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	binary := buildTestBinary(t)

	// Start mock OAuth2 provider (plain HTTP for testing)
	mockProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"access_token":  "mock-access-token",
				"refresh_token": "mock-refresh-token-oauth",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockProvider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "oauth",
		"-authorize-url", mockProvider.URL+"/authorize",
		"-token-url", mockProvider.URL+"/token",
		"-client-id", "test-app",
		"-scope", "openid offline_access",
		"-no-browser",
		"-allow-http",
		"-timeout", "15s")
	cmd.Env = append(os.Environ(), "CHAPERONE_ONBOARD_CLIENT_SECRET=test-secret")

	authURL, stdout := runE2EConsent(t, cmd)

	// Verify auth URL params
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}
	state := parsed.Query().Get("state")
	redirectURI := parsed.Query().Get("redirect_uri")
	if state == "" || redirectURI == "" {
		t.Fatalf("missing state or redirect_uri in auth URL: %s", authURL)
	}

	// Simulate the provider callback
	simulateCallback(ctx, t, redirectURI, state, "test-auth-code")

	if waitErr := cmd.Wait(); waitErr != nil {
		t.Fatalf("binary exited with error: %v", waitErr)
	}

	if got := stdout.String(); got != "mock-refresh-token-oauth" {
		t.Errorf("stdout = %q, want mock-refresh-token-oauth", got)
	}
}

// TestBuild_MicrosoftE2E runs a full end-to-end test with a mock Microsoft endpoint.
//
//nolint:funlen // E2E test, acceptable to be longer
func TestBuild_MicrosoftE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	binary := buildTestBinary(t)

	// Start mock Microsoft endpoint
	mockProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The microsoft subcommand derives URLs like: {endpoint}/{tenant}/oauth2/token
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			if parseErr := r.ParseForm(); parseErr != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// Verify resource param is present
			if r.FormValue("resource") != "https://graph.microsoft.com" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "missing resource")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"access_token":  "mock-access-token",
				"refresh_token": "mock-refresh-token-msft",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockProvider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "microsoft",
		"-tenant", "contoso.onmicrosoft.com",
		"-client-id", "test-app-id",
		"-resource", "https://graph.microsoft.com",
		"-endpoint", mockProvider.URL,
		"-no-browser",
		"-allow-http",
		"-timeout", "15s")
	cmd.Env = append(os.Environ(), "CHAPERONE_ONBOARD_CLIENT_SECRET=test-secret")

	authURL, stdout := runE2EConsent(t, cmd)

	// Verify Microsoft-specific params
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}
	state := parsed.Query().Get("state")
	redirectURI := parsed.Query().Get("redirect_uri")
	if state == "" || redirectURI == "" {
		t.Fatalf("missing state or redirect_uri in auth URL: %s", authURL)
	}
	if !strings.Contains(authURL, "contoso.onmicrosoft.com") {
		t.Errorf("auth URL missing tenant: %s", authURL)
	}
	if !strings.Contains(authURL, url.QueryEscape("https://graph.microsoft.com")) {
		t.Errorf("auth URL missing resource: %s", authURL)
	}

	// Simulate the provider callback
	simulateCallback(ctx, t, redirectURI, state, "msft-auth-code")

	if waitErr := cmd.Wait(); waitErr != nil {
		t.Fatalf("binary exited with error: %v", waitErr)
	}

	if got := stdout.String(); got != "mock-refresh-token-msft" {
		t.Errorf("stdout = %q, want mock-refresh-token-msft", got)
	}
}

// runE2EConsent starts the binary, reads stderr to find the authorization URL,
// and returns the URL and a builder capturing stdout.
func runE2EConsent(t *testing.T, cmd *exec.Cmd) (string, *strings.Builder) {
	t.Helper()

	var stdout strings.Builder
	cmd.Stdout = &stdout

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	if startErr := cmd.Start(); startErr != nil {
		t.Fatalf("failed to start binary: %v", startErr)
	}

	// Read stderr to find the authorization URL
	scanner := bufio.NewScanner(stderrPipe)
	var authURL string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "http") {
			authURL = strings.TrimSpace(line)
			break
		}
	}

	if authURL == "" {
		// Drain remaining stderr for diagnostics
		var remaining strings.Builder
		for scanner.Scan() {
			remaining.WriteString(scanner.Text() + "\n")
		}
		t.Fatalf("did not find authorization URL in stderr output.\nRemaining stderr: %s", remaining.String())
	}

	return authURL, &stdout
}

// simulateCallback sends a GET request to the callback server with the given
// authorization code and state.
func simulateCallback(ctx context.Context, t *testing.T, redirectURI, state, code string) {
	t.Helper()

	callbackURL := fmt.Sprintf("%s?code=%s&state=%s", redirectURI, url.QueryEscape(code), url.QueryEscape(state))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
	if err != nil {
		t.Fatalf("failed to create callback request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()
}

func TestRun_HelpFlag(t *testing.T) {
	t.Parallel()

	err := run([]string{"-help"})
	if err != nil {
		t.Errorf("run(-help) = %v, want nil", err)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	t.Parallel()

	err := run([]string{"bogus"})
	if err == nil {
		t.Error("run(bogus) = nil, want error")
	}
}

func TestExitCode_Mapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"usage error", errUsage, 1},
		{"wrapped usage error", fmt.Errorf("%w: -authorize-url: URL is required", errUsage), 1},
		{"consent timeout", errConsentTimeout, 2},
		{"exchange failed", errExchangeFailed, 3},
		{"wrapped exchange failed", fmt.Errorf("%w: HTTP 401: invalid_client", errExchangeFailed), 3},
		{"generic error", fmt.Errorf("something else"), 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// assertExitCode checks that an error from exec.Cmd contains the expected exit code.
func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if got := exitErr.ExitCode(); got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
}
