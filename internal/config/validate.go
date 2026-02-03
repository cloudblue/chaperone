// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
)

// Validation errors.
var (
	// ErrMissingAllowList is returned when allow_list is not configured.
	ErrMissingAllowList = errors.New("upstream.allow_list is required")
	// ErrEmptyAllowList is returned when allow_list is empty.
	ErrEmptyAllowList = errors.New("upstream.allow_list must not be empty")
	// ErrInvalidLogLevel is returned when log_level is not a valid value.
	ErrInvalidLogLevel = errors.New("observability.log_level must be one of: debug, info, warn, error")
	// ErrInvalidServerAddr is returned when server.addr is invalid.
	ErrInvalidServerAddr = errors.New("server.addr is invalid")
	// ErrInvalidAdminAddr is returned when server.admin_addr is invalid.
	ErrInvalidAdminAddr = errors.New("server.admin_addr is invalid")
	// ErrInvalidTimeout is returned when a timeout value is invalid.
	ErrInvalidTimeout = errors.New("timeout must be positive")
	// ErrMissingTLSCertFile is returned when TLS is enabled but cert_file is missing.
	ErrMissingTLSCertFile = errors.New("server.tls.cert_file is required when TLS is enabled")
	// ErrMissingTLSKeyFile is returned when TLS is enabled but key_file is missing.
	ErrMissingTLSKeyFile = errors.New("server.tls.key_file is required when TLS is enabled")
	// ErrMissingTLSCAFile is returned when TLS is enabled but ca_file is missing.
	ErrMissingTLSCAFile = errors.New("server.tls.ca_file is required when TLS is enabled")
	// ErrTLSFileNotFound is returned when a TLS file does not exist.
	ErrTLSFileNotFound = errors.New("TLS file not found")
)

// Validate validates the configuration and returns an error if invalid.
// It checks required fields, value constraints, and security requirements.
func Validate(cfg *Config) error {
	var errs []error

	// Validate server configuration
	if err := validateServerConfig(&cfg.Server); err != nil {
		errs = append(errs, err)
	}

	// Validate upstream configuration
	if err := validateUpstreamConfig(&cfg.Upstream); err != nil {
		errs = append(errs, err)
	}

	// Validate observability configuration
	if err := validateObservabilityConfig(&cfg.Observability); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// validateServerConfig validates the server configuration section.
func validateServerConfig(cfg *ServerConfig) error {
	var errs []error

	if err := validateAddress(cfg.Addr, "server.addr"); err != nil {
		errs = append(errs, err)
	}

	if err := validateAddress(cfg.AdminAddr, "server.admin_addr"); err != nil {
		errs = append(errs, err)
	}

	if err := validateTLSConfig(&cfg.TLS); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// validateAddress validates a network address (host:port or :port format).
func validateAddress(addr, fieldName string) error {
	if addr == "" {
		return nil // Empty is valid, will use default
	}

	// Host can be empty for ":port" format
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s split failed: %w", fieldName, ErrInvalidServerAddr)
	}

	// Port must be a valid number
	if portStr == "" {
		return fmt.Errorf("%s: %w: empty port", fieldName, ErrInvalidServerAddr)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("%s: %w: port must be numeric", fieldName, ErrInvalidServerAddr)
	}

	if port < 0 || port > 65535 {
		return fmt.Errorf("%s: %w: port must be 0-65535", fieldName, ErrInvalidServerAddr)
	}

	return nil
}

// validateTLSConfig validates the TLS configuration when TLS is enabled.
// It checks that required file paths are provided and that the files exist.
func validateTLSConfig(cfg *TLSConfig) error {
	// If TLS is explicitly disabled, skip validation
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}

	// TLS is enabled (either explicitly or by default), validate file paths
	var errs []error

	if cfg.CertFile == "" {
		errs = append(errs, ErrMissingTLSCertFile)
	} else if err := validateFileExists(cfg.CertFile, "server.tls.cert_file"); err != nil {
		errs = append(errs, err)
	}

	if cfg.KeyFile == "" {
		errs = append(errs, ErrMissingTLSKeyFile)
	} else if err := validateFileExists(cfg.KeyFile, "server.tls.key_file"); err != nil {
		errs = append(errs, err)
	}

	if cfg.CAFile == "" {
		errs = append(errs, ErrMissingTLSCAFile)
	} else if err := validateFileExists(cfg.CAFile, "server.tls.ca_file"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// validateFileExists checks that a file exists at the given path.
func validateFileExists(path, fieldName string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%s: %w: %s", fieldName, ErrTLSFileNotFound, path)
	}
	return nil
}

// validateUpstreamConfig validates the upstream configuration section.
func validateUpstreamConfig(cfg *UpstreamConfig) error {
	var errs []error

	// Security: allow_list is required
	if cfg.AllowList == nil {
		errs = append(errs, ErrMissingAllowList)
	} else if len(cfg.AllowList) == 0 {
		errs = append(errs, ErrEmptyAllowList)
	}

	// Validate timeouts
	if err := validateTimeouts(&cfg.Timeouts); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// validateTimeouts validates timeout configuration values.
func validateTimeouts(cfg *TimeoutConfig) error {
	var errs []error

	if cfg.Connect < 0 {
		errs = append(errs, fmt.Errorf("upstream.timeouts.connect: %w", ErrInvalidTimeout))
	}
	if cfg.Read < 0 {
		errs = append(errs, fmt.Errorf("upstream.timeouts.read: %w", ErrInvalidTimeout))
	}
	if cfg.Write < 0 {
		errs = append(errs, fmt.Errorf("upstream.timeouts.write: %w", ErrInvalidTimeout))
	}
	if cfg.Idle < 0 {
		errs = append(errs, fmt.Errorf("upstream.timeouts.idle: %w", ErrInvalidTimeout))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// validateObservabilityConfig validates the observability configuration section.
func validateObservabilityConfig(cfg *ObservabilityConfig) error {
	if cfg.LogLevel != "" && !slices.Contains(ValidLogLevels, cfg.LogLevel) {
		return fmt.Errorf("%w: got %q", ErrInvalidLogLevel, cfg.LogLevel)
	}

	return nil
}
