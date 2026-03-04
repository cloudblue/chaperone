// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

// validConfig returns a Config with all fields set to valid values.
// Tests mutate a single field to test specific validation rules.
func validConfig() *Config {
	return &Config{
		Server:  ServerConfig{Addr: "127.0.0.1:8080"},
		Database: DatabaseConfig{Path: "./test.db"},
		Scraper: ScraperConfig{
			Interval: Duration(10 * time.Second),
			Timeout:  Duration(5 * time.Second),
		},
		Session: SessionConfig{
			MaxAge:      Duration(24 * time.Hour),
			IdleTimeout: Duration(2 * time.Hour),
		},
		Audit: AuditConfig{RetentionDays: 90},
		Log:   LogConfig{Level: "info", Format: "json"},
	}
}

func TestValidate_ValidConfig_NoError(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := validConfig()

	// Act
	err := cfg.Validate()

	// Assert
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidAddr_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		addr string
	}{
		{"empty addr", ""},
		{"missing port", "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			cfg := validConfig()
			cfg.Server.Addr = tt.addr

			// Act
			err := cfg.Validate()

			// Assert
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestValidate_EmptyDatabasePath_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := validConfig()
	cfg.Database.Path = ""

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestValidate_TimeoutGteInterval_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		timeout  time.Duration
	}{
		{"timeout equals interval", 10 * time.Second, 10 * time.Second},
		{"timeout exceeds interval", 10 * time.Second, 15 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			cfg := validConfig()
			cfg.Scraper.Interval = Duration(tt.interval)
			cfg.Scraper.Timeout = Duration(tt.timeout)

			// Act
			err := cfg.Validate()

			// Assert
			if err == nil {
				t.Error("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "timeout must be less than") {
				t.Errorf("error = %q, want to contain %q", err.Error(), "timeout must be less than")
			}
		})
	}
}

func TestValidate_NegativeRetention_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := validConfig()
	cfg.Audit.RetentionDays = -1

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "retention_days") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "retention_days")
	}
}

func TestValidate_ZeroRetention_NoError(t *testing.T) {
	t.Parallel()

	// Arrange — 0 means "keep forever"
	cfg := validConfig()
	cfg.Audit.RetentionDays = 0

	// Act
	err := cfg.Validate()

	// Assert
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_UnknownLogLevel_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := validConfig()
	cfg.Log.Level = "trace"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown level") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "unknown level")
	}
}

func TestValidate_UnknownLogFormat_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := validConfig()
	cfg.Log.Format = "xml"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "unknown format")
	}
}

func TestValidate_MultipleErrors_ReturnsAllErrors(t *testing.T) {
	t.Parallel()

	// Arrange — multiple invalid fields
	cfg := &Config{
		Server:   ServerConfig{Addr: ""},
		Database: DatabaseConfig{Path: ""},
		Scraper: ScraperConfig{
			Interval: Duration(10 * time.Second),
			Timeout:  Duration(10 * time.Second),
		},
		Session: SessionConfig{
			MaxAge:      Duration(24 * time.Hour),
			IdleTimeout: Duration(2 * time.Hour),
		},
		Audit: AuditConfig{RetentionDays: -1},
		Log:   LogConfig{Level: "bad", Format: "bad"},
	}

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	checks := []string{"server.addr", "database.path", "timeout must be less than", "retention_days", "unknown level", "unknown format"}
	for _, check := range checks {
		if !strings.Contains(msg, check) {
			t.Errorf("error = %q, want to contain %q", msg, check)
		}
	}
}
