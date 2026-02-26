// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"net/url"
	"os"
	"testing"
	"time"

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

// testConfig returns a valid proxy.Config with all required fields populated.
// Tests should use this as a base and override only the fields they need.
//
// This ensures all tests pass NewServer validation (no zero timeouts, no
// missing required fields) while keeping test code concise.
func testConfig() proxy.Config {
	return proxy.Config{
		Addr:             ":0",
		Version:          "test",
		HeaderPrefix:     "X-Connect",
		TraceHeader:      "Connect-Request-ID",
		TLS:              &proxy.TLSConfig{Enabled: false},
		AllowList:        testAllowList(),
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     30 * time.Second,
		IdleTimeout:      120 * time.Second,
		KeepAliveTimeout: 30 * time.Second,
		PluginTimeout:    10 * time.Second,
		ConnectTimeout:   5 * time.Second,
		ShutdownTimeout:  30 * time.Second,
	}
}

// mustNewServer creates a proxy.Server from the given config, failing the
// test immediately if the config is invalid. Use for tests that are not
// testing config validation itself.
func mustNewServer(t *testing.T, cfg proxy.Config) *proxy.Server {
	t.Helper()
	srv, err := proxy.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed with valid config: %v", err)
	}
	return srv
}

// mustNewServerForTarget creates a server config that allows requests to the
// target URL's exact host:port. This keeps integration tests aligned with the
// production allow-list port filtering rules.
func mustNewServerForTarget(t *testing.T, cfg proxy.Config, targetURL string) *proxy.Server {
	t.Helper()

	host := mustTargetHostPort(t, targetURL)
	cfg.AllowList = map[string][]string{host: {"/**"}}
	return mustNewServer(t, cfg)
}

func mustTargetHostPort(t *testing.T, targetURL string) string {
	t.Helper()

	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("failed to parse target URL %q: %v", targetURL, err)
	}

	host := parsed.Host
	if host == "" {
		t.Fatalf("target URL %q is missing host", targetURL)
	}

	return host
}
