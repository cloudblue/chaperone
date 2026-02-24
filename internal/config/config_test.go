// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/internal/router"
)

func TestLoad_ValidYAML_ParsesAllFields(t *testing.T) {
	// Arrange - create temporary TLS files
	certFile, keyFile, caFile := createTestTLSFiles(t)

	yamlContent := `
server:
  addr: ":9443"
  admin_addr: ":9191"
  tls:
    cert_file: "` + certFile + `"
    key_file: "` + keyFile + `"
    ca_file: "` + caFile + `"
    auto_rotate: false
upstream:
  header_prefix: "X-Custom"
  trace_header: "Custom-Request-ID"
  allow_list:
    api.vendor.com:
      - "/v1/**"
      - "/v2/products"
  timeouts:
    connect: 10s
    read: 60s
    write: 45s
    idle: 180s
observability:
  log_level: "debug"
  enable_profiling: true
  enable_tracing: true
  sensitive_headers:
    - "X-Secret-Key"
`
	configPath := writeTestConfig(t, yamlContent)

	// Act
	cfg, err := Load(configPath)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Server
	if cfg.Server.Addr != ":9443" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":9443")
	}
	if cfg.Server.AdminAddr != ":9191" {
		t.Errorf("Server.AdminAddr = %q, want %q", cfg.Server.AdminAddr, ":9191")
	}
	if cfg.Server.TLS.CertFile != certFile {
		t.Errorf("Server.TLS.CertFile = %q, want %q", cfg.Server.TLS.CertFile, certFile)
	}
	if cfg.Server.TLS.AutoRotate == nil || *cfg.Server.TLS.AutoRotate != false {
		t.Errorf("Server.TLS.AutoRotate = %v, want false", cfg.Server.TLS.AutoRotate)
	}

	// Upstream
	if cfg.Upstream.HeaderPrefix != "X-Custom" {
		t.Errorf("Upstream.HeaderPrefix = %q, want %q", cfg.Upstream.HeaderPrefix, "X-Custom")
	}
	if cfg.Upstream.TraceHeader != "Custom-Request-ID" {
		t.Errorf("Upstream.TraceHeader = %q, want %q", cfg.Upstream.TraceHeader, "Custom-Request-ID")
	}
	if len(cfg.Upstream.AllowList) != 1 {
		t.Errorf("Upstream.AllowList length = %d, want 1", len(cfg.Upstream.AllowList))
	}
	if paths, ok := cfg.Upstream.AllowList["api.vendor.com"]; !ok || len(paths) != 2 {
		t.Errorf("Upstream.AllowList[api.vendor.com] = %v, want 2 paths", paths)
	}
	if cfg.Upstream.Timeouts.Connect == nil || *cfg.Upstream.Timeouts.Connect != 10*time.Second {
		t.Errorf("Upstream.Timeouts.Connect = %v, want 10s", cfg.Upstream.Timeouts.Connect)
	}

	// Observability
	if cfg.Observability.LogLevel != "debug" {
		t.Errorf("Observability.LogLevel = %q, want %q", cfg.Observability.LogLevel, "debug")
	}
	if cfg.Observability.EnableProfiling != true {
		t.Errorf("Observability.EnableProfiling = %v, want true", cfg.Observability.EnableProfiling)
	}
	if cfg.Observability.EnableTracing != true {
		t.Errorf("Observability.EnableTracing = %v, want true", cfg.Observability.EnableTracing)
	}
	// Sensitive headers: custom "X-Secret-Key" merged with defaults
	defaults := defaultSensitiveHeaders()
	wantLen := len(defaults) + 1 // defaults + "X-Secret-Key"
	if len(cfg.Observability.SensitiveHeaders) != wantLen {
		t.Errorf("Observability.SensitiveHeaders length = %d, want %d (defaults + custom); got %v",
			len(cfg.Observability.SensitiveHeaders), wantLen, cfg.Observability.SensitiveHeaders)
	}
}

func TestLoad_MinimalYAML_AppliesDefaults(t *testing.T) {
	// Arrange - minimal config with only required fields
	// TLS disabled to skip file validation (not testing TLS here)
	yamlContent := `
server:
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	// Act
	cfg, err := Load(configPath)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify defaults applied
	if cfg.Server.Addr != DefaultServerAddr {
		t.Errorf("Server.Addr = %q, want default %q", cfg.Server.Addr, DefaultServerAddr)
	}
	if cfg.Server.AdminAddr != DefaultAdminAddr {
		t.Errorf("Server.AdminAddr = %q, want default %q", cfg.Server.AdminAddr, DefaultAdminAddr)
	}
	if cfg.Upstream.HeaderPrefix != DefaultHeaderPrefix {
		t.Errorf("Upstream.HeaderPrefix = %q, want default %q", cfg.Upstream.HeaderPrefix, DefaultHeaderPrefix)
	}
	if cfg.Upstream.Timeouts.Connect == nil || *cfg.Upstream.Timeouts.Connect != DefaultConnectTimeout {
		t.Errorf("Upstream.Timeouts.Connect = %v, want default %v", cfg.Upstream.Timeouts.Connect, DefaultConnectTimeout)
	}
	if cfg.Observability.LogLevel != DefaultLogLevel {
		t.Errorf("Observability.LogLevel = %q, want default %q", cfg.Observability.LogLevel, DefaultLogLevel)
	}

	// Verify secure defaults for sensitive headers
	if len(cfg.Observability.SensitiveHeaders) == 0 {
		t.Error("Observability.SensitiveHeaders should have secure defaults")
	}
}

func TestLoad_EnvVarOverride_TakesPrecedence(t *testing.T) {
	// Arrange
	// TLS disabled to skip file validation (not testing TLS here)
	yamlContent := `
server:
  addr: ":8080"
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	// Set environment variable to override YAML
	t.Setenv("CHAPERONE_SERVER_ADDR", ":9999")
	t.Setenv("CHAPERONE_UPSTREAM_HEADER_PREFIX", "X-Override")
	t.Setenv("CHAPERONE_OBSERVABILITY_LOG_LEVEL", "error")

	// Act
	cfg, err := Load(configPath)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Addr != ":9999" {
		t.Errorf("Server.Addr = %q, want env override %q", cfg.Server.Addr, ":9999")
	}
	if cfg.Upstream.HeaderPrefix != "X-Override" {
		t.Errorf("Upstream.HeaderPrefix = %q, want env override %q", cfg.Upstream.HeaderPrefix, "X-Override")
	}
	if cfg.Observability.LogLevel != "error" {
		t.Errorf("Observability.LogLevel = %q, want env override %q", cfg.Observability.LogLevel, "error")
	}
}

func TestLoad_NestedEnvVarOverride_UsesUnderscoreSeparator(t *testing.T) {
	// Arrange - create temp TLS files since we're testing TLS env overrides
	certFile, keyFile, caFile := createTestTLSFiles(t)

	yamlContent := `
server:
  tls:
    cert_file: "` + certFile + `"
    key_file: "` + keyFile + `"
    ca_file: "` + caFile + `"
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	// Set nested environment variables
	t.Setenv("CHAPERONE_SERVER_TLS_AUTO_ROTATE", "false")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_CONNECT", "15s")

	// Act
	cfg, err := Load(configPath)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.TLS.AutoRotate == nil || *cfg.Server.TLS.AutoRotate != false {
		t.Errorf("Server.TLS.AutoRotate = %v, want env override false", cfg.Server.TLS.AutoRotate)
	}
	if cfg.Upstream.Timeouts.Connect == nil || *cfg.Upstream.Timeouts.Connect != 15*time.Second {
		t.Errorf("Upstream.Timeouts.Connect = %v, want env override 15s", cfg.Upstream.Timeouts.Connect)
	}
}

func TestLoad_ConfigPathFromEnv_UsesEnvVar(t *testing.T) {
	// Arrange
	// TLS disabled to skip file validation (not testing TLS here)
	yamlContent := `
server:
  addr: ":7777"
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)
	t.Setenv("CHAPERONE_CONFIG", configPath)

	// Act - load with empty path, should use CHAPERONE_CONFIG env
	cfg, err := Load("")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Addr != ":7777" {
		t.Errorf("Server.Addr = %q, want %q from config loaded via CHAPERONE_CONFIG", cfg.Server.Addr, ":7777")
	}
}

func TestLoad_FileNotFound_ReturnsError(t *testing.T) {
	// Act
	_, err := Load("/nonexistent/config.yaml")

	// Assert
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	// Arrange
	invalidYAML := `
server:
  addr: ":8080"
  this is not valid yaml at all!!
`
	configPath := writeTestConfig(t, invalidYAML)

	// Act
	_, err := Load(configPath)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestValidate_MissingAllowList_ReturnsError(t *testing.T) {
	// Arrange
	cfg := &Config{
		Upstream: UpstreamConfig{
			AllowList: nil, // Security: must be explicitly configured
		},
	}
	applyDefaults(cfg)

	// Act
	err := Validate(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing allow_list, got nil")
	}
}

func TestValidate_EmptyAllowList_ReturnsError(t *testing.T) {
	// Arrange
	cfg := &Config{
		Upstream: UpstreamConfig{
			AllowList: map[string][]string{}, // Empty = deny all, must be explicit
		},
	}
	applyDefaults(cfg)

	// Act
	err := Validate(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for empty allow_list, got nil")
	}
}

func TestValidate_InvalidGlobPattern_ReturnsError(t *testing.T) {
	tests := []struct {
		name      string
		allowList map[string][]string
	}{
		{
			name: "invalid domain pattern - partial star",
			allowList: map[string][]string{
				"api*.google.com": {"/v1/**"},
			},
		},
		{
			name: "invalid path pattern - partial star",
			allowList: map[string][]string{
				"api.google.com": {"/v1/cust*/profiles"},
			},
		},
		{
			name: "invalid triple star in domain",
			allowList: map[string][]string{
				"***.google.com": {"/v1/**"},
			},
		},
		{
			name: "invalid triple star in path",
			allowList: map[string][]string{
				"api.google.com": {"/v1/***/profiles"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Upstream: UpstreamConfig{
					AllowList: tt.allowList,
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err == nil {
				t.Fatal("expected error for invalid glob pattern, got nil")
			}
			if !errors.Is(err, router.ErrInvalidGlobPattern) {
				t.Errorf("expected router.ErrInvalidGlobPattern, got %v", err)
			}
		})
	}
}

func TestValidate_ValidGlobPatterns_NoError(t *testing.T) {
	tests := []struct {
		name      string
		allowList map[string][]string
	}{
		{
			name: "exact domain and path",
			allowList: map[string][]string{
				"api.google.com": {"/v1/customers"},
			},
		},
		{
			name: "single star domain pattern",
			allowList: map[string][]string{
				"*.google.com": {"/v1/**"},
			},
		},
		{
			name: "double star domain pattern",
			allowList: map[string][]string{
				"**.amazonaws.com": {"/bucket/**"},
			},
		},
		{
			name: "single star path pattern",
			allowList: map[string][]string{
				"api.google.com": {"/v1/customers/*/profiles"},
			},
		},
		{
			name: "double star path pattern",
			allowList: map[string][]string{
				"api.google.com": {"/v1/**"},
			},
		},
		{
			name: "catch-all path",
			allowList: map[string][]string{
				"api.google.com": {"/**"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsDisabled := false
			cfg := &Config{
				Server: ServerConfig{
					TLS: TLSConfig{
						Enabled: &tlsDisabled,
					},
				},
				Upstream: UpstreamConfig{
					AllowList: tt.allowList,
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err != nil {
				t.Errorf("unexpected error for valid glob pattern: %v", err)
			}
		})
	}
}

func TestValidate_InvalidLogLevel_ReturnsError(t *testing.T) {
	// Arrange
	cfg := &Config{
		Upstream: UpstreamConfig{
			AllowList: map[string][]string{"api.example.com": {"/**"}},
		},
		Observability: ObservabilityConfig{
			LogLevel: "invalid-level",
		},
	}
	applyDefaults(cfg)

	// Act
	err := Validate(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
}

func TestValidate_InvalidServerAddr_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"empty port", ":"},
		{"invalid port number", ":99999"},
		{"negative port", ":-1"},
		{"non-numeric port", ":abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					Addr: tt.addr,
				},
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err == nil {
				t.Errorf("expected error for addr %q, got nil", tt.addr)
			}
		})
	}
}

func TestValidate_ValidConfig_NoError(t *testing.T) {
	// Arrange - create temporary TLS files for validation
	certFile, keyFile, caFile := createTestTLSFiles(t)
	tlsEnabled := true

	cfg := &Config{
		Server: ServerConfig{
			Addr:      ":8443",
			AdminAddr: ":9090",
			TLS: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
		},
		Upstream: UpstreamConfig{
			HeaderPrefix: "X-Connect",
			TraceHeader:  "Connect-Request-ID",
			AllowList:    map[string][]string{"api.example.com": {"/**"}},
			Timeouts: TimeoutConfig{
				Connect: durationPtr(5 * time.Second),
				Read:    durationPtr(30 * time.Second),
				Write:   durationPtr(30 * time.Second),
				Idle:    durationPtr(120 * time.Second),
				Plugin:  durationPtr(10 * time.Second),
			},
		},
		Observability: ObservabilityConfig{
			LogLevel:         "info",
			EnableProfiling:  false,
			EnableTracing:    false,
			SensitiveHeaders: []string{"Authorization"},
		},
	}

	// Act
	err := Validate(cfg)

	// Assert
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_NegativeTimeout_ReturnsError(t *testing.T) {
	// Arrange
	tlsDisabled := false
	cfg := &Config{
		Server: ServerConfig{
			TLS: TLSConfig{Enabled: &tlsDisabled},
		},
		Upstream: UpstreamConfig{
			AllowList: map[string][]string{"api.example.com": {"/**"}},
			Timeouts: TimeoutConfig{
				Connect: durationPtr(-5 * time.Second), // Invalid
			},
		},
	}
	applyDefaults(cfg)

	// Act
	err := Validate(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for negative timeout, got nil")
	}
}

func TestValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}
	tlsDisabled := false

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					TLS: TLSConfig{Enabled: &tlsDisabled},
				},
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
				Observability: ObservabilityConfig{
					LogLevel: level,
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err != nil {
				t.Errorf("expected no error for log level %q, got %v", level, err)
			}
		})
	}
}

// TestValidate_SentinelErrors_WorkWithErrorsIs verifies that validation errors
// can be checked with errors.Is for proper error handling in calling code.
func TestValidate_SentinelErrors_WorkWithErrorsIs(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Config
		wantErr      error
		skipDefaults bool
	}{
		{
			name: "missing allow_list returns ErrMissingAllowList",
			cfg: &Config{
				Upstream: UpstreamConfig{
					AllowList: nil,
				},
			},
			wantErr: ErrMissingAllowList,
		},
		{
			name: "empty allow_list returns ErrEmptyAllowList",
			cfg: &Config{
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{},
				},
			},
			wantErr: ErrEmptyAllowList,
		},
		{
			name: "invalid log level returns ErrInvalidLogLevel",
			cfg: &Config{
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
				Observability: ObservabilityConfig{
					LogLevel: "invalid",
				},
			},
			wantErr: ErrInvalidLogLevel,
		},
		{
			name: "invalid server addr returns ErrInvalidServerAddr",
			cfg: &Config{
				Server: ServerConfig{
					Addr: ":invalid",
				},
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
			},
			wantErr: ErrInvalidServerAddr,
		},
		{
			name: "negative timeout returns ErrInvalidTimeout",
			cfg: &Config{
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
					Timeouts: TimeoutConfig{
						Connect: durationPtr(-1 * time.Second),
					},
				},
			},
			wantErr: ErrInvalidTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.skipDefaults {
				applyDefaults(tt.cfg)
			}

			err := Validate(tt.cfg)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("errors.Is(err, %v) = false, want true\nerror was: %v", tt.wantErr, err)
			}
		})
	}
}

// TestValidate_TLSFileValidation tests that TLS file paths are validated when TLS is enabled.
func TestValidate_TLSFileValidation(t *testing.T) {
	// Create temporary cert files for valid config tests
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "server.crt")
	keyFile := filepath.Join(tmpDir, "server.key")
	caFile := filepath.Join(tmpDir, "ca.crt")

	// Create the test files
	for _, f := range []string{certFile, keyFile, caFile} {
		if err := os.WriteFile(f, []byte("test"), 0o600); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tlsEnabled := true
	tlsDisabled := false

	tests := []struct {
		name    string
		tls     TLSConfig
		wantErr error
	}{
		{
			name: "valid TLS config with existing files",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
			wantErr: nil,
		},
		{
			name: "TLS disabled skips validation",
			tls: TLSConfig{
				Enabled:  &tlsDisabled,
				CertFile: "", // Would fail if validated
				KeyFile:  "",
				CAFile:   "",
			},
			wantErr: nil,
		},
		{
			name: "missing cert file path",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: "",
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
			wantErr: ErrMissingTLSCertFile,
		},
		{
			name: "missing key file path",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: certFile,
				KeyFile:  "",
				CAFile:   caFile,
			},
			wantErr: ErrMissingTLSKeyFile,
		},
		{
			name: "missing CA file path",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   "",
			},
			wantErr: ErrMissingTLSCAFile,
		},
		{
			name: "cert file does not exist",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: "/nonexistent/cert.crt",
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
			wantErr: ErrTLSFileNotFound,
		},
		{
			name: "key file does not exist",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: certFile,
				KeyFile:  "/nonexistent/key.key",
				CAFile:   caFile,
			},
			wantErr: ErrTLSFileNotFound,
		},
		{
			name: "CA file does not exist",
			tls: TLSConfig{
				Enabled:  &tlsEnabled,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   "/nonexistent/ca.crt",
			},
			wantErr: ErrTLSFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					Addr:      ":8443",
					AdminAddr: ":9090",
					TLS:       tt.tls,
				},
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
			}
			// Don't apply defaults - we want to test TLS config as-is

			err := Validate(cfg)

			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("errors.Is(err, %v) = false\nerror was: %v", tt.wantErr, err)
			}
		})
	}
}

// TestLoad_InvalidEnvVarDuration_ReturnsError tests that invalid duration env vars fail loudly.
func TestLoad_InvalidEnvVarDuration_ReturnsError(t *testing.T) {
	// Create a valid base config
	yamlContent := `
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	tests := []struct {
		name   string
		envKey string
		envVal string
	}{
		{"invalid connect timeout", "CHAPERONE_UPSTREAM_TIMEOUTS_CONNECT", "5mn"},
		{"invalid read timeout", "CHAPERONE_UPSTREAM_TIMEOUTS_READ", "not-a-duration"},
		{"invalid write timeout", "CHAPERONE_UPSTREAM_TIMEOUTS_WRITE", "30seconds"},
		{"invalid idle timeout", "CHAPERONE_UPSTREAM_TIMEOUTS_IDLE", "2h30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envVal)

			_, err := Load(configPath)

			if err == nil {
				t.Fatalf("expected error for invalid env var %s=%s, got nil", tt.envKey, tt.envVal)
			}
			// Error message should contain the env var name and value
			if !contains(err.Error(), tt.envKey) {
				t.Errorf("error should mention env var name %q: %v", tt.envKey, err)
			}
		})
	}
}

// TestLoad_InvalidEnvVarBoolean_ReturnsError tests that invalid boolean env vars fail loudly.
func TestLoad_InvalidEnvVarBoolean_ReturnsError(t *testing.T) {
	// Create a valid base config
	yamlContent := `
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	tests := []struct {
		name   string
		envKey string
		envVal string
	}{
		{"invalid TLS enabled", "CHAPERONE_SERVER_TLS_ENABLED", "yes"},
		{"invalid TLS auto rotate", "CHAPERONE_SERVER_TLS_AUTO_ROTATE", "nope"},
		{"invalid profiling", "CHAPERONE_OBSERVABILITY_ENABLE_PROFILING", "enabled"},
		{"invalid tracing", "CHAPERONE_OBSERVABILITY_ENABLE_TRACING", "yes-please"},
		{"invalid body logging", "CHAPERONE_OBSERVABILITY_ENABLE_BODY_LOGGING", "yes-please"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envVal)

			_, err := Load(configPath)

			if err == nil {
				t.Fatalf("expected error for invalid env var %s=%s, got nil", tt.envKey, tt.envVal)
			}
			// Error message should contain the env var name
			if !contains(err.Error(), tt.envKey) {
				t.Errorf("error should mention env var name %q: %v", tt.envKey, err)
			}
		})
	}
}

// contains checks if s contains substr (helper to avoid strings import).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

// TestLoad_EnableBodyLogging_EnvVarEnablesIt verifies that the env var is the
// only way to enable body logging (security-critical: yaml:"-" tag prevents config file).
func TestLoad_EnableBodyLogging_EnvVarEnablesIt(t *testing.T) {
	yamlContent := `
server:
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)
	t.Setenv("CHAPERONE_OBSERVABILITY_ENABLE_BODY_LOGGING", "true")

	cfg, err := Load(configPath)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Observability.EnableBodyLogging {
		t.Error("EnableBodyLogging should be true when env var is set to 'true'")
	}
}

// TestLoad_EnableBodyLogging_YAMLCannotEnableIt verifies that the yaml:"-" tag
// prevents config file from enabling body logging.
func TestLoad_EnableBodyLogging_YAMLCannotEnableIt(t *testing.T) {
	yamlContent := `
server:
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
observability:
  enable_body_logging: true
`
	configPath := writeTestConfig(t, yamlContent)

	cfg, err := Load(configPath)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Observability.EnableBodyLogging {
		t.Error("EnableBodyLogging should be false — yaml:\"-\" tag must prevent config file from setting it")
	}
}

// TestLoad_EnableBodyLogging_DefaultFalse verifies the secure default.
func TestLoad_EnableBodyLogging_DefaultFalse(t *testing.T) {
	yamlContent := `
server:
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	cfg, err := Load(configPath)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Observability.EnableBodyLogging {
		t.Error("EnableBodyLogging should default to false (secure default)")
	}
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDefaultSensitiveHeaders_IncludesSecurityCritical(t *testing.T) {
	// These headers MUST be in the default redaction list per security requirements
	required := []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"Set-Cookie",
		"X-API-Key",
		"X-Auth-Token",
	}

	headers := defaultSensitiveHeaders()
	for _, header := range required {
		found := false
		for _, h := range headers {
			if h == header {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("defaultSensitiveHeaders() missing required header %q", header)
		}
	}
}

func TestDefaultSensitiveHeaders_ReturnsNewCopy(t *testing.T) {
	// Verify that defaultSensitiveHeaders returns a new copy each time
	// to prevent accidental mutation (Issue #3 from PR review)
	headers1 := defaultSensitiveHeaders()
	headers2 := defaultSensitiveHeaders()

	// Modify the first slice
	headers1[0] = "MUTATED"

	// Second slice should be unaffected
	if headers2[0] == "MUTATED" {
		t.Error("defaultSensitiveHeaders() returns same slice, should return new copy")
	}
}

func TestMergeSensitiveHeaders(t *testing.T) {
	defaults := defaultSensitiveHeaders()

	tests := []struct {
		name      string
		extra     []string
		wantLen   int
		wantItems []string // items that must appear in result
	}{
		{
			name:      "nil extra returns defaults only",
			extra:     nil,
			wantLen:   len(defaults),
			wantItems: defaults,
		},
		{
			name:      "empty extra returns defaults only",
			extra:     []string{},
			wantLen:   len(defaults),
			wantItems: defaults,
		},
		{
			name:      "custom header merged with defaults",
			extra:     []string{"X-Vendor-Secret"},
			wantLen:   len(defaults) + 1,
			wantItems: append(defaults, "X-Vendor-Secret"),
		},
		{
			name:      "multiple custom headers merged",
			extra:     []string{"X-Vendor-Secret", "X-Partner-Key"},
			wantLen:   len(defaults) + 2,
			wantItems: []string{"Authorization", "X-Vendor-Secret", "X-Partner-Key"},
		},
		{
			name:      "duplicate of default is deduplicated",
			extra:     []string{"Authorization"},
			wantLen:   len(defaults),
			wantItems: defaults,
		},
		{
			name:      "case-insensitive dedup",
			extra:     []string{"authorization", "COOKIE", "x-api-key"},
			wantLen:   len(defaults),
			wantItems: defaults,
		},
		{
			name:      "duplicates within extra are deduplicated",
			extra:     []string{"X-Custom", "x-custom", "X-CUSTOM"},
			wantLen:   len(defaults) + 1,
			wantItems: []string{"Authorization", "X-Custom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeSensitiveHeaders(tt.extra)

			if len(result) != tt.wantLen {
				t.Errorf("MergeSensitiveHeaders() length = %d, want %d; got %v",
					len(result), tt.wantLen, result)
			}

			for _, want := range tt.wantItems {
				found := false
				for _, got := range result {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("MergeSensitiveHeaders() missing %q in result %v", want, result)
				}
			}
		})
	}
}

func TestMergeSensitiveHeaders_DefaultsAlwaysFirst(t *testing.T) {
	// Verify that built-in defaults always come before extra entries.
	defaults := defaultSensitiveHeaders()
	extra := []string{"X-Custom-One", "X-Custom-Two"}

	result := MergeSensitiveHeaders(extra)

	wantLen := len(defaults) + len(extra)
	if len(result) != wantLen {
		t.Fatalf("expected %d entries, got %d: %v", wantLen, len(result), result)
	}
	// First entries must be the defaults
	for i, d := range defaults {
		if result[i] != d {
			t.Errorf("result[%d] = %q, want default %q", i, result[i], d)
		}
	}
	// Then the extras
	if result[len(defaults)] != "X-Custom-One" || result[len(defaults)+1] != "X-Custom-Two" {
		t.Errorf("extra entries not after defaults: got %v", result)
	}
}

func TestApplyDefaults_SensitiveHeaders_MergesWithDefaults(t *testing.T) {
	// Security: Verify that user-provided sensitive headers are merged
	// with defaults, not used as a replacement.
	cfg := &Config{
		Observability: ObservabilityConfig{
			SensitiveHeaders: []string{"X-Vendor-Secret"},
		},
	}

	applyDefaults(cfg)

	// Must contain all defaults
	defaults := defaultSensitiveHeaders()
	for _, d := range defaults {
		found := false
		for _, h := range cfg.Observability.SensitiveHeaders {
			if h == d {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("applyDefaults() dropped default header %q; got %v",
				d, cfg.Observability.SensitiveHeaders)
		}
	}

	// Must also contain the custom header
	found := false
	for _, h := range cfg.Observability.SensitiveHeaders {
		if h == "X-Vendor-Secret" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("applyDefaults() dropped custom header %q; got %v",
			"X-Vendor-Secret", cfg.Observability.SensitiveHeaders)
	}
}

func TestApplyDefaults_SensitiveHeaders_EmptyUsesDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	defaults := defaultSensitiveHeaders()
	if len(cfg.Observability.SensitiveHeaders) != len(defaults) {
		t.Errorf("expected %d defaults, got %d: %v",
			len(defaults), len(cfg.Observability.SensitiveHeaders),
			cfg.Observability.SensitiveHeaders)
	}
}

// writeTestConfig creates a temporary config file and returns its path.
// The file is automatically cleaned up when the test completes.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	return path
}

// createTestTLSFiles creates temporary TLS files for tests that need TLS validation to pass.
// Returns the paths to cert, key, and CA files.
func createTestTLSFiles(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()

	dir := t.TempDir()
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")
	caFile = filepath.Join(dir, "ca.crt")

	// Create placeholder files (content doesn't matter for validation, only existence)
	for _, f := range []string{certFile, keyFile, caFile} {
		if err := os.WriteFile(f, []byte("placeholder"), 0o600); err != nil {
			t.Fatalf("failed to create test TLS file: %v", err)
		}
	}

	return certFile, keyFile, caFile
}

func TestValidate_InvalidAdminAddr_ReturnsError(t *testing.T) {
	tests := []struct {
		name      string
		adminAddr string
	}{
		{"empty port", ":"},
		{"invalid port number", ":99999"},
		{"negative port", ":-1"},
		{"non-numeric port", ":abc"},
	}
	tlsDisabled := false

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					Addr:      ":8443", // Valid
					AdminAddr: tt.adminAddr,
					TLS:       TLSConfig{Enabled: &tlsDisabled},
				},
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err == nil {
				t.Errorf("expected error for admin_addr %q, got nil", tt.adminAddr)
			}
		})
	}
}

func TestValidate_AllNegativeTimeouts_ReturnsErrors(t *testing.T) {
	// Arrange - test all timeout validations
	tlsDisabled := false
	cfg := &Config{
		Server: ServerConfig{
			TLS: TLSConfig{Enabled: &tlsDisabled},
		},
		Upstream: UpstreamConfig{
			AllowList: map[string][]string{"api.example.com": {"/**"}},
			Timeouts: TimeoutConfig{
				Connect: durationPtr(-1 * time.Second),
				Read:    durationPtr(-2 * time.Second),
				Write:   durationPtr(-3 * time.Second),
				Idle:    durationPtr(-4 * time.Second),
				Plugin:  durationPtr(-5 * time.Second),
			},
		},
	}
	applyDefaults(cfg)

	// Act
	err := Validate(cfg)

	// Assert
	if err == nil {
		t.Fatal("expected error for negative timeouts, got nil")
	}
}

func TestLoad_DefaultConfigPath_WhenNoPathAndNoEnv(t *testing.T) {
	// This tests the resolveConfigPath function's default branch
	// We expect it to fail because ./config.yaml doesn't exist
	_, err := Load("")

	// Should fail because default ./config.yaml doesn't exist
	if err == nil {
		t.Fatal("expected error when default config path doesn't exist")
	}
}

func TestLoad_AllEnvOverrides_Applied(t *testing.T) {
	// Arrange - comprehensive env override test
	// Create temp TLS files for the env var paths
	certFile, keyFile, caFile := createTestTLSFiles(t)

	yamlContent := `
upstream:
  allow_list:
    api.example.com:
      - "/**"
`
	configPath := writeTestConfig(t, yamlContent)

	// Set ALL env overrides (using real temp files for TLS)
	t.Setenv("CHAPERONE_SERVER_ADDR", ":1111")
	t.Setenv("CHAPERONE_SERVER_ADMIN_ADDR", ":2222")
	t.Setenv("CHAPERONE_SERVER_TLS_CERT_FILE", certFile)
	t.Setenv("CHAPERONE_SERVER_TLS_KEY_FILE", keyFile)
	t.Setenv("CHAPERONE_SERVER_TLS_CA_FILE", caFile)
	t.Setenv("CHAPERONE_SERVER_TLS_AUTO_ROTATE", "true")
	t.Setenv("CHAPERONE_UPSTREAM_HEADER_PREFIX", "X-Env")
	t.Setenv("CHAPERONE_UPSTREAM_TRACE_HEADER", "Env-Trace-ID")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_CONNECT", "1s")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_READ", "2s")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_WRITE", "3s")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_IDLE", "4s")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_KEEP_ALIVE", "5s")
	t.Setenv("CHAPERONE_UPSTREAM_TIMEOUTS_PLUGIN", "7s")
	t.Setenv("CHAPERONE_OBSERVABILITY_LOG_LEVEL", "debug")
	t.Setenv("CHAPERONE_OBSERVABILITY_ENABLE_PROFILING", "true")
	t.Setenv("CHAPERONE_OBSERVABILITY_ENABLE_TRACING", "true")

	// Act
	cfg, err := Load(configPath)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all env overrides applied
	if cfg.Server.Addr != ":1111" {
		t.Errorf("Server.Addr = %q, want :1111", cfg.Server.Addr)
	}
	if cfg.Server.AdminAddr != ":2222" {
		t.Errorf("Server.AdminAddr = %q, want :2222", cfg.Server.AdminAddr)
	}
	if cfg.Server.TLS.CertFile != certFile {
		t.Errorf("TLS.CertFile = %q, want %q", cfg.Server.TLS.CertFile, certFile)
	}
	if cfg.Server.TLS.KeyFile != keyFile {
		t.Errorf("TLS.KeyFile = %q, want %q", cfg.Server.TLS.KeyFile, keyFile)
	}
	if cfg.Server.TLS.CAFile != caFile {
		t.Errorf("TLS.CAFile = %q, want %q", cfg.Server.TLS.CAFile, caFile)
	}
	if cfg.Server.TLS.AutoRotate == nil || *cfg.Server.TLS.AutoRotate != true {
		t.Errorf("TLS.AutoRotate = %v, want true", cfg.Server.TLS.AutoRotate)
	}
	if cfg.Upstream.HeaderPrefix != "X-Env" {
		t.Errorf("HeaderPrefix = %q, want X-Env", cfg.Upstream.HeaderPrefix)
	}
	if cfg.Upstream.TraceHeader != "Env-Trace-ID" {
		t.Errorf("TraceHeader = %q, want Env-Trace-ID", cfg.Upstream.TraceHeader)
	}
	if cfg.Upstream.Timeouts.Connect == nil || *cfg.Upstream.Timeouts.Connect != 1*time.Second {
		t.Errorf("Timeouts.Connect = %v, want 1s", cfg.Upstream.Timeouts.Connect)
	}
	if cfg.Upstream.Timeouts.Read == nil || *cfg.Upstream.Timeouts.Read != 2*time.Second {
		t.Errorf("Timeouts.Read = %v, want 2s", cfg.Upstream.Timeouts.Read)
	}
	if cfg.Upstream.Timeouts.Write == nil || *cfg.Upstream.Timeouts.Write != 3*time.Second {
		t.Errorf("Timeouts.Write = %v, want 3s", cfg.Upstream.Timeouts.Write)
	}
	if cfg.Upstream.Timeouts.Idle == nil || *cfg.Upstream.Timeouts.Idle != 4*time.Second {
		t.Errorf("Timeouts.Idle = %v, want 4s", cfg.Upstream.Timeouts.Idle)
	}
	if cfg.Upstream.Timeouts.KeepAlive == nil || *cfg.Upstream.Timeouts.KeepAlive != 5*time.Second {
		t.Errorf("Timeouts.KeepAlive = %v, want 5s", cfg.Upstream.Timeouts.KeepAlive)
	}
	if cfg.Upstream.Timeouts.Plugin == nil || *cfg.Upstream.Timeouts.Plugin != 7*time.Second {
		t.Errorf("Timeouts.Plugin = %v, want 7s", cfg.Upstream.Timeouts.Plugin)
	}
	if cfg.Observability.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.Observability.LogLevel)
	}
	if cfg.Observability.EnableProfiling != true {
		t.Errorf("EnableProfiling = %v, want true", cfg.Observability.EnableProfiling)
	}
	if cfg.Observability.EnableTracing != true {
		t.Errorf("EnableTracing = %v, want true", cfg.Observability.EnableTracing)
	}
}

func TestApplyDefaults_AutoRotate_DefaultsToTrue(t *testing.T) {
	// Arrange - config with no auto_rotate set
	cfg := &Config{}

	// Act
	applyDefaults(cfg)

	// Assert - AutoRotate should default to true per Design Spec
	if cfg.Server.TLS.AutoRotate == nil || *cfg.Server.TLS.AutoRotate != DefaultAutoRotate {
		t.Errorf("TLS.AutoRotate = %v, want default %v", cfg.Server.TLS.AutoRotate, DefaultAutoRotate)
	}
}
func TestApplyDefaults_TLSEnabled_DefaultsToTrue(t *testing.T) {
	// Arrange - config with no tls.enabled set
	cfg := &Config{}

	// Act
	applyDefaults(cfg)

	// Assert - TLS.Enabled should default to true for security
	if cfg.Server.TLS.Enabled == nil || *cfg.Server.TLS.Enabled != DefaultTLSEnabled {
		t.Errorf("TLS.Enabled = %v, want default %v", cfg.Server.TLS.Enabled, DefaultTLSEnabled)
	}
}

func TestEnvOverride_TLSEnabled_DisablesTLS(t *testing.T) {
	// Arrange - create a minimal config file
	content := `
upstream:
  allow_list:
    "example.com":
      - "/api/**"
`
	tmpFile := writeTestConfig(t, content)

	// Set env to disable TLS
	t.Setenv("CHAPERONE_SERVER_TLS_ENABLED", "false")

	// Act
	cfg, err := Load(tmpFile)

	// Assert
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.TLS.Enabled == nil || *cfg.Server.TLS.Enabled != false {
		t.Errorf("TLS.Enabled = %v, want false (env override)", cfg.Server.TLS.Enabled)
	}
}

// TestValidate_EmptyAddress_UsesDefault verifies that empty addresses don't trigger errors
// (they will use defaults).
func TestValidate_EmptyAddress_UsesDefault(t *testing.T) {
	tlsDisabled := false
	cfg := &Config{
		Server: ServerConfig{
			Addr:      "", // Empty - will use default
			AdminAddr: "", // Empty - will use default
			TLS:       TLSConfig{Enabled: &tlsDisabled},
		},
		Upstream: UpstreamConfig{
			AllowList: map[string][]string{"api.example.com": {"/**"}},
		},
	}
	// Don't apply defaults to test the empty address validation path

	err := Validate(cfg)

	if err != nil {
		t.Errorf("unexpected error for empty addresses: %v", err)
	}
}

// TestValidate_MalformedAddress_ReturnsError tests addresses that fail SplitHostPort.
func TestValidate_MalformedAddress_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"missing colon", "localhost8443"},
		{"multiple colons without brackets", "::8443"},
		{"incomplete IPv6", "[::1"},
	}

	tlsDisabled := false
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					Addr: tt.addr,
					TLS:  TLSConfig{Enabled: &tlsDisabled},
				},
				Upstream: UpstreamConfig{
					AllowList: map[string][]string{"api.example.com": {"/**"}},
				},
			}

			err := Validate(cfg)

			if err == nil {
				t.Errorf("expected error for malformed addr %q, got nil", tt.addr)
			}
			if !errors.Is(err, ErrInvalidServerAddr) {
				t.Errorf("expected ErrInvalidServerAddr, got %v", err)
			}
		})
	}
}

// TestValidate_GlobPattern_EmptyPath_Valid tests that empty paths in allow list are valid.
func TestValidate_GlobPattern_EmptyPath_Valid(t *testing.T) {
	tlsDisabled := false
	cfg := &Config{
		Server: ServerConfig{
			TLS: TLSConfig{Enabled: &tlsDisabled},
		},
		Upstream: UpstreamConfig{
			AllowList: map[string][]string{
				"api.example.com": {""}, // Empty path pattern is valid
			},
		},
	}
	applyDefaults(cfg)

	err := Validate(cfg)

	if err != nil {
		t.Errorf("unexpected error for empty path pattern: %v", err)
	}
}

// TestValidate_GlobPattern_MoreInvalidCases tests additional invalid glob patterns
// for comprehensive security coverage.
func TestValidate_GlobPattern_MoreInvalidCases(t *testing.T) {
	tests := []struct {
		name      string
		allowList map[string][]string
	}{
		{
			name: "star in middle of segment",
			allowList: map[string][]string{
				"api.google.com": {"/v1/cust*omers"},
			},
		},
		{
			name: "star at start of segment",
			allowList: map[string][]string{
				"api.google.com": {"/v1/*partial"},
			},
		},
		{
			name: "star at end of segment",
			allowList: map[string][]string{
				"api.google.com": {"/v1/partial*"},
			},
		},
		{
			name: "double star with suffix",
			allowList: map[string][]string{
				"api.google.com": {"/v1/**suffix"},
			},
		},
		{
			name: "quadruple star",
			allowList: map[string][]string{
				"api.google.com": {"/v1/****"},
			},
		},
		{
			name: "partial star in domain",
			allowList: map[string][]string{
				"*api.google.com": {"/v1/**"},
			},
		},
		{
			name: "star suffix in domain",
			allowList: map[string][]string{
				"api*.google.com": {"/v1/**"},
			},
		},
		{
			name: "mixed stars and text",
			allowList: map[string][]string{
				"api.google.com": {"/v1/*test*"},
			},
		},
	}

	tlsDisabled := false
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					TLS: TLSConfig{Enabled: &tlsDisabled},
				},
				Upstream: UpstreamConfig{
					AllowList: tt.allowList,
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err == nil {
				t.Fatal("expected error for invalid glob pattern, got nil")
			}
			if !errors.Is(err, router.ErrInvalidGlobPattern) {
				t.Errorf("expected router.ErrInvalidGlobPattern, got %v", err)
			}
		})
	}
}

// TestValidate_GlobPattern_EdgeCases tests edge cases that should be valid.
func TestValidate_GlobPattern_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		allowList map[string][]string
	}{
		{
			name: "root path only",
			allowList: map[string][]string{
				"api.example.com": {"/"},
			},
		},
		{
			name: "multiple paths for same host",
			allowList: map[string][]string{
				"api.example.com": {"/v1/**", "/v2/**", "/health"},
			},
		},
		{
			name: "multiple hosts",
			allowList: map[string][]string{
				"api.example.com":  {"/v1/**"},
				"data.example.com": {"/query/**"},
			},
		},
		{
			name: "wildcard domain with wildcard path",
			allowList: map[string][]string{
				"*.example.com": {"/**"},
			},
		},
		{
			name: "double star domain with specific path",
			allowList: map[string][]string{
				"**.amazonaws.com": {"/bucket/specific/path"},
			},
		},
		{
			name: "path with many segments",
			allowList: map[string][]string{
				"api.example.com": {"/v1/users/*/profiles/*/settings"},
			},
		},
		{
			name: "double star in middle of path",
			allowList: map[string][]string{
				"api.example.com": {"/v1/**/settings"},
			},
		},
		{
			name: "consecutive single star segments",
			allowList: map[string][]string{
				"api.example.com": {"/v1/*/*/details"},
			},
		},
	}

	tlsDisabled := false
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					TLS: TLSConfig{Enabled: &tlsDisabled},
				},
				Upstream: UpstreamConfig{
					AllowList: tt.allowList,
				},
			}
			applyDefaults(cfg)

			err := Validate(cfg)

			if err != nil {
				t.Errorf("unexpected error for valid glob pattern: %v", err)
			}
		})
	}
}

// TestValidate_MultipleErrors_AllReported tests that multiple validation errors
// are all reported, not just the first one.
func TestValidate_MultipleErrors_AllReported(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Addr:      ":invalid",
			AdminAddr: ":alsoinvalid",
		},
		Upstream: UpstreamConfig{
			AllowList: nil, // Missing
			Timeouts: TimeoutConfig{
				Connect: durationPtr(-1 * time.Second),
			},
		},
		Observability: ObservabilityConfig{
			LogLevel: "notavalidlevel",
		},
	}

	err := Validate(cfg)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Check that multiple errors are reported
	errStr := err.Error()
	if !contains(errStr, "server.addr") {
		t.Error("error should mention server.addr")
	}
	if !contains(errStr, "server.admin_addr") {
		t.Error("error should mention server.admin_addr")
	}
	if !contains(errStr, "allow_list") {
		t.Error("error should mention allow_list")
	}
}

func TestApplyDefaults_EnableTracing_DefaultsToFalse(t *testing.T) {
	// Arrange - config with no enable_tracing set
	cfg := &Config{}

	// Act
	applyDefaults(cfg)

	// Assert - EnableTracing should default to false (opt-in)
	if cfg.Observability.EnableTracing != DefaultEnableTracing {
		t.Errorf("EnableTracing = %v, want default %v", cfg.Observability.EnableTracing, DefaultEnableTracing)
	}
}

func TestLoad_EnableTracing_YAMLEnablesIt(t *testing.T) {
	yamlContent := `
server:
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
observability:
  enable_tracing: true
`
	configPath := writeTestConfig(t, yamlContent)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Observability.EnableTracing {
		t.Error("EnableTracing should be true when set in YAML")
	}
}

func TestLoad_EnableTracing_EnvVarOverridesYAML(t *testing.T) {
	yamlContent := `
server:
  tls:
    enabled: false
upstream:
  allow_list:
    api.example.com:
      - "/**"
observability:
  enable_tracing: true
`
	configPath := writeTestConfig(t, yamlContent)

	// Env var overrides YAML
	t.Setenv("CHAPERONE_OBSERVABILITY_ENABLE_TRACING", "false")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Observability.EnableTracing {
		t.Error("EnableTracing should be false when env var overrides YAML")
	}
}
