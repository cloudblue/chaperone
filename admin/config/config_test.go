// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDuration_UnmarshalYAML_ValidDuration_ParsesCorrectly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"seconds", `"10s"`, 10 * time.Second},
		{"minutes", `"5m"`, 5 * time.Minute},
		{"hours", `"24h"`, 24 * time.Hour},
		{"milliseconds", `"500ms"`, 500 * time.Millisecond},
		{"compound", `"1h30m"`, 90 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			var d Duration

			// Act
			err := yaml.Unmarshal([]byte(tt.input), &d)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.Unwrap() != tt.want {
				t.Errorf("duration = %v, want %v", d.Unwrap(), tt.want)
			}
		})
	}
}

func TestDuration_UnmarshalYAML_InvalidDuration_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"no unit", `"10"`},
		{"invalid unit", `"10x"`},
		{"empty", `""`},
		{"text", `"forever"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			var d Duration

			// Act
			err := yaml.Unmarshal([]byte(tt.input), &d)

			// Assert
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestDuration_Unwrap_ReturnsDuration(t *testing.T) {
	t.Parallel()

	// Arrange
	d := Duration(42 * time.Second)

	// Act
	got := d.Unwrap()

	// Assert
	if got != 42*time.Second {
		t.Errorf("Unwrap() = %v, want %v", got, 42*time.Second)
	}
}

func TestDuration_String_FormatsCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	d := Duration(90 * time.Second)

	// Act
	got := d.String()

	// Assert
	if got != "1m30s" {
		t.Errorf("String() = %q, want %q", got, "1m30s")
	}
}

func TestDuration_MarshalYAML_FormatsCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	d := Duration(10 * time.Second)

	// Act
	got, err := d.MarshalYAML()

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "10s" {
		t.Errorf("MarshalYAML() = %q, want %q", got, "10s")
	}
}
