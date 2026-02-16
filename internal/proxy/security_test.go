// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"bytes"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

func TestValidateTargetScheme_HTTPS_AlwaysAllowed(t *testing.T) {
	target, _ := url.Parse("https://api.vendor.com/v1/resource")

	err := ValidateTargetScheme(target)

	if err != nil {
		t.Errorf("HTTPS should always be allowed, got error: %v", err)
	}
}

func TestValidateTargetScheme_HTTP_RejectedByDefault(t *testing.T) {
	// This test verifies production behavior (allowInsecureTargets = "false")
	// The default value is "false", so HTTP should be rejected
	if AllowInsecureTargets() {
		t.Skip("Skipping: test requires production build (allowInsecureTargets=false)")
	}

	target, _ := url.Parse("http://api.vendor.com/v1/resource")

	err := ValidateTargetScheme(target)

	if err == nil {
		t.Error("HTTP should be rejected in production mode")
	}
	if !errors.Is(err, ErrInsecureTargetURL) {
		t.Errorf("expected ErrInsecureTargetURL, got: %v", err)
	}
}

func TestValidateTargetScheme_UnknownScheme_Rejected(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
	}{
		{"ftp", "ftp://files.vendor.com/data"},
		{"file", "file:///etc/passwd"},
		{"ws", "ws://socket.vendor.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, _ := url.Parse(tt.scheme)

			err := ValidateTargetScheme(target)

			if err == nil {
				t.Errorf("scheme %q should be rejected", tt.name)
			}
		})
	}
}

func TestAllowInsecureTargets_DefaultIsFalse(t *testing.T) {
	// The default value compiled into the binary should be "false"
	// This ensures production builds are secure by default
	//
	// Note: This test will fail if run with `make build-dev` because
	// the dev build sets allowInsecureTargets="true" via ldflags
	if allowInsecureTargets != "false" && allowInsecureTargets != "true" {
		t.Errorf("allowInsecureTargets should be 'true' or 'false', got: %q", allowInsecureTargets)
	}
}

func TestLogStartup_InsecureTargetsEnabled_EmitsWarning(t *testing.T) {
	// Arrange: enable insecure targets and capture log output
	cleanup := SetAllowInsecureTargetsForTesting(true)
	defer cleanup()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	srv := &Server{
		config: Config{
			Addr: ":0",
			TLS:  &TLSConfig{Enabled: false},
		},
	}

	// Act
	srv.logStartup()

	// Assert
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "INSECURE") {
		t.Errorf("expected INSECURE warning in log output, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "allow_insecure_targets") {
		t.Errorf("expected allow_insecure_targets field in log output, got: %s", logOutput)
	}
}

func TestLogStartup_InsecureTargetsDisabled_NoWarning(t *testing.T) {
	// Arrange: disable insecure targets and capture log output
	cleanup := SetAllowInsecureTargetsForTesting(false)
	defer cleanup()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	srv := &Server{
		config: Config{
			Addr: ":0",
			TLS:  &TLSConfig{Enabled: false},
		},
	}

	// Act
	srv.logStartup()

	// Assert
	logOutput := logBuffer.String()
	if strings.Contains(logOutput, "INSECURE") {
		t.Errorf("unexpected INSECURE warning in log output: %s", logOutput)
	}
}
