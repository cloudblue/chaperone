// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package chaperone

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/internal/telemetry"
	"github.com/cloudblue/chaperone/sdk"
)

// --- Test helpers ---

// writeTestConfig writes a minimal YAML config for testing and returns the path.
// The config uses TLS disabled and a random free port for isolation.
func writeTestConfig(t *testing.T, serverAddr, adminAddr string) string {
	t.Helper()

	content := fmt.Sprintf(`server:
  addr: "%s"
  admin_addr: "%s"
  tls:
    enabled: false
upstream:
  header_prefix: "X-Connect"
  allow_list:
    "example.com":
      - "/api/**"
`, serverAddr, adminAddr)

	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// freeAddr returns a free TCP address on localhost.
func freeAddr(t *testing.T) string {
	t.Helper()

	lc := &net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// waitForHealth polls the health endpoint until it responds or 5s elapses.
func waitForHealth(t *testing.T, addr string) {
	t.Helper()

	const timeout = 5 * time.Second
	healthURL := fmt.Sprintf("http://%s/_ops/health", addr)
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			cancel()
			time.Sleep(50 * time.Millisecond)
			continue
		}

		resp, err := client.Do(req)
		cancel()
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("health endpoint at %s did not become ready within %v", addr, timeout)
}

// safeLogBuf is a thread-safe writer that captures log output for tests.
// strings.Builder is not safe for concurrent use, so this wrapper
// synchronises Write and String with a mutex.
type safeLogBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeLogBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeLogBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// --- Option tests ---

func TestWithConfigPath_SetsConfigPath(t *testing.T) {
	cfg := &runConfig{}
	opt := WithConfigPath("/etc/chaperone.yaml")
	opt(cfg)

	if cfg.configPath != "/etc/chaperone.yaml" {
		t.Errorf("got configPath %q, want %q", cfg.configPath, "/etc/chaperone.yaml")
	}
}

func TestWithVersion_SetsVersion(t *testing.T) {
	cfg := &runConfig{}
	opt := WithVersion("1.2.3")
	opt(cfg)

	if cfg.version != "1.2.3" {
		t.Errorf("got version %q, want %q", cfg.version, "1.2.3")
	}
}

func TestWithLogOutput_SetsLogOutput(t *testing.T) {
	cfg := &runConfig{}
	opt := WithLogOutput(io.Discard)
	opt(cfg)

	if cfg.logOutput != io.Discard {
		t.Errorf("got logOutput %v, want io.Discard", cfg.logOutput)
	}
}

func TestWithBuildInfo_SetsCommitAndBuildDate(t *testing.T) {
	cfg := &runConfig{}
	opt := WithBuildInfo("abc1234", "2026-02-13")
	opt(cfg)

	if cfg.commit != "abc1234" {
		t.Errorf("got commit %q, want %q", cfg.commit, "abc1234")
	}
	if cfg.buildDate != "2026-02-13" {
		t.Errorf("got buildDate %q, want %q", cfg.buildDate, "2026-02-13")
	}
}

// --- parseLogLevel tests ---

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // Default
		{"", slog.LevelInfo},        // Empty defaults to info
		{"DEBUG", slog.LevelInfo},   // Case-sensitive, falls to default
		{"INFO", slog.LevelInfo},    // Case-sensitive, falls to default
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("level_%s", tt.input), func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// --- Run() integration tests ---

// stubPlugin is a minimal Plugin for testing that does no credential injection.
type stubPlugin struct{}

func (p *stubPlugin) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	return nil, nil
}

func (p *stubPlugin) SignCSR(_ context.Context, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *stubPlugin) ModifyResponse(_ context.Context, _ sdk.TransactionContext, _ *http.Response) (*sdk.ResponseAction, error) {
	return nil, nil
}

func TestRun_StartsServerAndRespondsToHealth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, &stubPlugin{},
			WithConfigPath(configPath),
			WithVersion("test-1.0.0"),
			WithLogOutput(io.Discard),
		)
	}()

	// Wait for the server to become ready.
	waitForHealth(t, serverAddr)

	// Hit health endpoint and verify response body.
	healthURL := fmt.Sprintf("http://%s/_ops/health", serverAddr)
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "alive") {
		t.Errorf("health body %q does not contain 'alive'", string(body))
	}

	// Shut down via context cancellation.
	cancel()

	select {
	case runErr := <-errCh:
		if runErr != nil {
			t.Errorf("Run() returned unexpected error: %v", runErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRun_NilPlugin_StartsWithoutCredentialInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, nil,
			WithConfigPath(configPath),
			WithLogOutput(io.Discard),
		)
	}()

	waitForHealth(t, serverAddr)

	// Server should be running without a plugin.
	healthURL := fmt.Sprintf("http://%s/_ops/health", serverAddr)
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}

	cancel()

	select {
	case runErr := <-errCh:
		if runErr != nil {
			t.Errorf("Run() returned unexpected error: %v", runErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRun_InvalidConfigPath_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := Run(ctx, nil,
		WithConfigPath("/nonexistent/config.yaml"),
		WithLogOutput(io.Discard),
	)
	if err == nil {
		t.Fatal("Run() with invalid config path should return an error")
	}
	if !strings.Contains(err.Error(), "loading configuration") {
		t.Errorf("error %q should mention 'loading configuration'", err.Error())
	}
}

func TestRun_AdminServerRespondsToHealth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, nil,
			WithConfigPath(configPath),
			WithLogOutput(io.Discard),
		)
	}()

	waitForHealth(t, serverAddr)

	// Verify admin server health endpoint.
	adminHealthURL := fmt.Sprintf("http://%s/_ops/health", adminAddr)
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, adminHealthURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin health request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("admin health got status %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRun_ContextCancellation_GracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, nil,
			WithConfigPath(configPath),
			WithLogOutput(io.Discard),
		)
	}()

	waitForHealth(t, serverAddr)

	// Cancel and measure shutdown time.
	start := time.Now()
	cancel()

	select {
	case err := <-errCh:
		elapsed := time.Since(start)
		if err != nil {
			t.Errorf("Run() returned unexpected error: %v", err)
		}
		// Graceful shutdown should complete well within the default timeout.
		if elapsed > 5*time.Second {
			t.Errorf("shutdown took %v, expected < 5s", elapsed)
		}
		t.Logf("graceful shutdown completed in %v", elapsed)
	case <-time.After(15 * time.Second):
		t.Fatal("Run() did not return within 15s after context cancellation")
	}
}

func TestRun_WithBuildInfo_IncludesMetadataInLogs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	// Capture log output to verify build info appears.
	var logBuf safeLogBuf

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, nil,
			WithConfigPath(configPath),
			WithVersion("2.0.0"),
			WithBuildInfo("abc1234", "2026-02-13"),
			WithLogOutput(&logBuf),
		)
	}()

	waitForHealth(t, serverAddr)
	cancel()

	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	logOutput := logBuf.String()
	for _, want := range []string{"abc1234", "2026-02-13", "2.0.0"} {
		if !strings.Contains(logOutput, want) {
			t.Errorf("log output should contain %q, got:\n%s", want, logOutput)
		}
	}
}

func TestRun_WithBuildInfo_OmittedWhenEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	var logBuf safeLogBuf

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, nil,
			WithConfigPath(configPath),
			WithLogOutput(&logBuf),
		)
	}()

	waitForHealth(t, serverAddr)
	cancel()

	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	logOutput := logBuf.String()
	// When WithBuildInfo is not called, commit/build_date should not appear.
	if strings.Contains(logOutput, "build_date") {
		t.Errorf("log output should NOT contain 'build_date' when WithBuildInfo is not used, got:\n%s", logOutput)
	}
}

func TestShutdownAdminServer_ReleasesPort(t *testing.T) {
	// Verify shutdownAdminServer properly shuts down a running admin server
	// and releases its port for reuse.
	adminAddr := freeAddr(t)

	adminSrv := telemetry.NewAdminServer(adminAddr)
	telemetry.RegisterPprofHandlers(adminSrv.Mux(), false)

	if err := adminSrv.Start(); err != nil {
		t.Fatalf("failed to start admin server: %v", err)
	}

	// Verify it's actually listening.
	healthURL := fmt.Sprintf("http://%s/_ops/health", adminAddr)
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin server not responding: %v", err)
	}
	resp.Body.Close()

	// Now call shutdownAdminServer and verify port is released.
	shutdownAdminServer(adminSrv)

	lc := &net.ListenConfig{}
	l, listenErr := lc.Listen(context.Background(), "tcp", adminAddr)
	if listenErr != nil {
		t.Errorf("admin port %s still in use after shutdownAdminServer — cleanup failed: %v",
			adminAddr, listenErr)
	} else {
		l.Close()
		t.Logf("admin port %s released after shutdownAdminServer — cleanup OK", adminAddr)
	}
}

func TestRun_ProxyPortInUse_CleansUpAdminServer(t *testing.T) {
	// Verify that when the proxy server fails to bind (port already in use),
	// Run() cleans up the admin server instead of leaking it.
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	serverAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	configPath := writeTestConfig(t, serverAddr, adminAddr)

	// Occupy the proxy port so srv.Start() will fail with "address already in use".
	lc := &net.ListenConfig{}
	blocker, err := lc.Listen(context.Background(), "tcp", serverAddr)
	if err != nil {
		t.Fatalf("failed to occupy proxy port: %v", err)
	}
	defer blocker.Close()

	// Run() should return an error because the proxy port is occupied.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := Run(ctx, nil,
		WithConfigPath(configPath),
		WithLogOutput(io.Discard),
	)
	if runErr == nil {
		t.Fatal("Run() should return an error when proxy port is in use")
	}
	if !strings.Contains(runErr.Error(), "server error") {
		t.Errorf("error %q should mention 'server error'", runErr.Error())
	}

	// The admin port should be released — if the admin server leaked, this
	// listen call would fail with "address already in use".
	adminListener, listenErr := lc.Listen(context.Background(), "tcp", adminAddr)
	if listenErr != nil {
		t.Errorf("admin port %s still in use after Run() returned — admin server leaked: %v",
			adminAddr, listenErr)
	} else {
		adminListener.Close()
	}
}
