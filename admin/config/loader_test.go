// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoad_NoFile_AppliesDefaults(t *testing.T) {
	t.Parallel()

	// Arrange — point to a non-existent file
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")

	// Act
	cfg, err := Load(path)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1:8080" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, "127.0.0.1:8080")
	}
	if cfg.Database.Path != "./chaperone-admin.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./chaperone-admin.db")
	}
	if cfg.Scraper.Interval.Unwrap() != 10*time.Second {
		t.Errorf("Scraper.Interval = %v, want %v", cfg.Scraper.Interval.Unwrap(), 10*time.Second)
	}
	if cfg.Scraper.Timeout.Unwrap() != 5*time.Second {
		t.Errorf("Scraper.Timeout = %v, want %v", cfg.Scraper.Timeout.Unwrap(), 5*time.Second)
	}
	if cfg.Session.MaxAge.Unwrap() != 24*time.Hour {
		t.Errorf("Session.MaxAge = %v, want %v", cfg.Session.MaxAge.Unwrap(), 24*time.Hour)
	}
	if cfg.Session.IdleTimeout.Unwrap() != 2*time.Hour {
		t.Errorf("Session.IdleTimeout = %v, want %v", cfg.Session.IdleTimeout.Unwrap(), 2*time.Hour)
	}
	if cfg.Audit.RetentionDays != 90 {
		t.Errorf("Audit.RetentionDays = %d, want %d", cfg.Audit.RetentionDays, 90)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
}

func TestLoad_ValidYAML_ParsesAllFields(t *testing.T) {
	t.Parallel()

	// Arrange
	path := writeTestConfig(t, `
server:
  addr: "0.0.0.0:9090"
database:
  path: "/var/lib/admin.db"
scraper:
  interval: "30s"
  timeout: "10s"
session:
  max_age: "12h"
  idle_timeout: "1h"
audit:
  retention_days: 30
log:
  level: "debug"
  format: "text"
`)

	// Act
	cfg, err := Load(path)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Addr != "0.0.0.0:9090" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, "0.0.0.0:9090")
	}
	if cfg.Database.Path != "/var/lib/admin.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/var/lib/admin.db")
	}
	if cfg.Scraper.Interval.Unwrap() != 30*time.Second {
		t.Errorf("Scraper.Interval = %v, want %v", cfg.Scraper.Interval.Unwrap(), 30*time.Second)
	}
	if cfg.Scraper.Timeout.Unwrap() != 10*time.Second {
		t.Errorf("Scraper.Timeout = %v, want %v", cfg.Scraper.Timeout.Unwrap(), 10*time.Second)
	}
	if cfg.Session.MaxAge.Unwrap() != 12*time.Hour {
		t.Errorf("Session.MaxAge = %v, want %v", cfg.Session.MaxAge.Unwrap(), 12*time.Hour)
	}
	if cfg.Session.IdleTimeout.Unwrap() != 1*time.Hour {
		t.Errorf("Session.IdleTimeout = %v, want %v", cfg.Session.IdleTimeout.Unwrap(), 1*time.Hour)
	}
	if cfg.Audit.RetentionDays != 30 {
		t.Errorf("Audit.RetentionDays = %d, want %d", cfg.Audit.RetentionDays, 30)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "text")
	}
}

func TestLoad_EnvOverrides_AllFields(t *testing.T) {
	// Not parallel — modifies environment via t.Setenv.

	// Arrange
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")
	t.Setenv("CHAPERONE_ADMIN_SERVER_ADDR", "0.0.0.0:3000")
	t.Setenv("CHAPERONE_ADMIN_DATABASE_PATH", "/tmp/test.db")
	t.Setenv("CHAPERONE_ADMIN_SCRAPER_INTERVAL", "20s")
	t.Setenv("CHAPERONE_ADMIN_SCRAPER_TIMEOUT", "8s")
	t.Setenv("CHAPERONE_ADMIN_SESSION_MAX_AGE", "48h")
	t.Setenv("CHAPERONE_ADMIN_SESSION_IDLE_TIMEOUT", "4h")
	t.Setenv("CHAPERONE_ADMIN_AUDIT_RETENTION_DAYS", "60")
	t.Setenv("CHAPERONE_ADMIN_LOG_LEVEL", "WARN")
	t.Setenv("CHAPERONE_ADMIN_LOG_FORMAT", "TEXT")

	// Act
	cfg, err := Load(path)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Addr != "0.0.0.0:3000" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, "0.0.0.0:3000")
	}
	if cfg.Database.Path != "/tmp/test.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/tmp/test.db")
	}
	if cfg.Scraper.Interval.Unwrap() != 20*time.Second {
		t.Errorf("Scraper.Interval = %v, want %v", cfg.Scraper.Interval.Unwrap(), 20*time.Second)
	}
	if cfg.Scraper.Timeout.Unwrap() != 8*time.Second {
		t.Errorf("Scraper.Timeout = %v, want %v", cfg.Scraper.Timeout.Unwrap(), 8*time.Second)
	}
	if cfg.Session.MaxAge.Unwrap() != 48*time.Hour {
		t.Errorf("Session.MaxAge = %v, want %v", cfg.Session.MaxAge.Unwrap(), 48*time.Hour)
	}
	if cfg.Session.IdleTimeout.Unwrap() != 4*time.Hour {
		t.Errorf("Session.IdleTimeout = %v, want %v", cfg.Session.IdleTimeout.Unwrap(), 4*time.Hour)
	}
	if cfg.Audit.RetentionDays != 60 {
		t.Errorf("Audit.RetentionDays = %d, want %d", cfg.Audit.RetentionDays, 60)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "warn")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "text")
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	path := writeTestConfig(t, `{{{invalid yaml`)

	// Act
	_, err := Load(path)

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestLoad_EnvOverride_InvalidDuration_ReturnsError(t *testing.T) {
	// Not parallel — modifies environment.

	// Arrange
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")
	t.Setenv("CHAPERONE_ADMIN_SCRAPER_INTERVAL", "not-a-duration")

	// Act
	_, err := Load(path)

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestApplyDefaults_ZeroConfig_SetsAllDefaults(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &Config{}

	// Act
	applyDefaults(cfg)

	// Assert
	if cfg.Server.Addr != "127.0.0.1:8080" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, "127.0.0.1:8080")
	}
	if cfg.Database.Path != "./chaperone-admin.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./chaperone-admin.db")
	}
	if cfg.Scraper.Interval.Unwrap() != 10*time.Second {
		t.Errorf("Scraper.Interval = %v, want 10s", cfg.Scraper.Interval.Unwrap())
	}
	if cfg.Scraper.Timeout.Unwrap() != 5*time.Second {
		t.Errorf("Scraper.Timeout = %v, want 5s", cfg.Scraper.Timeout.Unwrap())
	}
	if cfg.Session.MaxAge.Unwrap() != 24*time.Hour {
		t.Errorf("Session.MaxAge = %v, want 24h", cfg.Session.MaxAge.Unwrap())
	}
	if cfg.Session.IdleTimeout.Unwrap() != 2*time.Hour {
		t.Errorf("Session.IdleTimeout = %v, want 2h", cfg.Session.IdleTimeout.Unwrap())
	}
	if cfg.Audit.RetentionDays != 90 {
		t.Errorf("Audit.RetentionDays = %d, want 90", cfg.Audit.RetentionDays)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
}

func TestResolveConfigPath_ExplicitPath_ReturnsSame(t *testing.T) {
	t.Parallel()

	// Act
	got := resolveConfigPath("/custom/config.yaml")

	// Assert
	if got != "/custom/config.yaml" {
		t.Errorf("resolveConfigPath() = %q, want %q", got, "/custom/config.yaml")
	}
}

func TestResolveConfigPath_EnvVar_ReturnsEnvValue(t *testing.T) {
	// Not parallel — modifies environment.
	t.Setenv("CHAPERONE_ADMIN_CONFIG", "/env/config.yaml")

	// Act
	got := resolveConfigPath("")

	// Assert
	if got != "/env/config.yaml" {
		t.Errorf("resolveConfigPath() = %q, want %q", got, "/env/config.yaml")
	}
}

func TestResolveConfigPath_Default_ReturnsDefault(t *testing.T) {
	t.Parallel()

	// Act
	got := resolveConfigPath("")

	// Assert
	if got != "chaperone-admin.yaml" {
		t.Errorf("resolveConfigPath() = %q, want %q", got, "chaperone-admin.yaml")
	}
}
