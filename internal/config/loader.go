// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EnvPrefix is the prefix for environment variable overrides.
const EnvPrefix = "CHAPERONE"

// ConfigEnvVar is the environment variable for the config file path.
const ConfigEnvVar = "CHAPERONE_CONFIG"

// DefaultConfigPath is the default configuration file path.
const DefaultConfigPath = "./config.yaml"

// Load loads configuration from a YAML file with environment variable overrides.
// If configPath is empty, it checks CHAPERONE_CONFIG env var, then falls back to DefaultConfigPath.
// Environment variables take precedence over YAML values (12-Factor App methodology).
func Load(configPath string) (*Config, error) {
	// Resolve config path
	path := resolveConfigPath(configPath)

	// Load YAML file
	cfg, err := loadYAML(path)
	if err != nil {
		return nil, err
	}

	// Apply defaults for unset values
	applyDefaults(cfg)

	// Apply environment variable overrides
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, fmt.Errorf("environment variable override failed: %w", err)
	}

	// Validate the final configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// resolveConfigPath determines the config file path from input, env var, or default.
func resolveConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}

	if envPath := os.Getenv(ConfigEnvVar); envPath != "" {
		return envPath
	}

	return DefaultConfigPath
}

// loadYAML reads and parses a YAML configuration file.
// Security: yaml.v3 is safe and does not execute arbitrary code.
// The config file should have restricted permissions (0600 or 0640).
func loadYAML(path string) (*Config, error) {
	// Security: Check file permissions (warn if world-readable)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("accessing config file %s: %w", path, err)
	}
	// Warn if file is world-readable (permission bits include 0004)
	if info.Mode().Perm()&0o004 != 0 {
		// Log warning but don't fail - some deployments use read-only mounts
		fmt.Fprintf(os.Stderr, "Warning: config file %s is world-readable, consider chmod 600\n", path)
	}

	// #nosec G304 -- path comes from trusted sources: CLI flag, env var, or hardcoded default.
	// This is a CLI tool where the operator has full system access.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return &cfg, nil
}

// applyDefaults applies default values for any unset configuration fields.
func applyDefaults(cfg *Config) {
	// Server defaults
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = DefaultServerAddr
	}
	if cfg.Server.AdminAddr == "" {
		cfg.Server.AdminAddr = DefaultAdminAddr
	}

	// TLS defaults
	// Enabled: nil means not set, so apply default (true for security)
	if cfg.Server.TLS.Enabled == nil {
		tlsEnabled := DefaultTLSEnabled
		cfg.Server.TLS.Enabled = &tlsEnabled
	}
	if cfg.Server.TLS.CertFile == "" {
		cfg.Server.TLS.CertFile = DefaultCertFile
	}
	if cfg.Server.TLS.KeyFile == "" {
		cfg.Server.TLS.KeyFile = DefaultKeyFile
	}
	if cfg.Server.TLS.CAFile == "" {
		cfg.Server.TLS.CAFile = DefaultCAFile
	}
	// AutoRotate: nil means not set, so apply default (true)
	if cfg.Server.TLS.AutoRotate == nil {
		autoRotate := DefaultAutoRotate
		cfg.Server.TLS.AutoRotate = &autoRotate
	}

	// Upstream defaults
	if cfg.Upstream.HeaderPrefix == "" {
		cfg.Upstream.HeaderPrefix = DefaultHeaderPrefix
	}
	if cfg.Upstream.TraceHeader == "" {
		cfg.Upstream.TraceHeader = DefaultTraceHeader
	}

	// Timeout defaults
	if cfg.Upstream.Timeouts.Connect == 0 {
		cfg.Upstream.Timeouts.Connect = DefaultConnectTimeout
	}
	if cfg.Upstream.Timeouts.Read == 0 {
		cfg.Upstream.Timeouts.Read = DefaultReadTimeout
	}
	if cfg.Upstream.Timeouts.Write == 0 {
		cfg.Upstream.Timeouts.Write = DefaultWriteTimeout
	}
	if cfg.Upstream.Timeouts.Idle == 0 {
		cfg.Upstream.Timeouts.Idle = DefaultIdleTimeout
	}

	// Observability defaults
	if cfg.Observability.LogLevel == "" {
		cfg.Observability.LogLevel = DefaultLogLevel
	}
	// EnableProfiling defaults to false (secure default), which is Go zero value

	// Security: Always ensure sensitive headers has secure defaults
	if len(cfg.Observability.SensitiveHeaders) == 0 {
		cfg.Observability.SensitiveHeaders = DefaultSensitiveHeaders()
	}
}

// applyEnvOverrides applies environment variable overrides to the configuration.
// Pattern: CHAPERONE_<SECTION>_<KEY> (uppercase, underscore separator)
// Returns an error if any environment variable has an invalid value.
//
//nolint:gocognit // Explicit mapping is clearer than reflection for maintainability
func applyEnvOverrides(cfg *Config) error {
	if err := applyServerEnvOverrides(cfg); err != nil {
		return err
	}
	if err := applyUpstreamEnvOverrides(cfg); err != nil {
		return err
	}
	if err := applyObservabilityEnvOverrides(cfg); err != nil {
		return err
	}
	return nil
}

// applyServerEnvOverrides applies server-related environment variable overrides.
func applyServerEnvOverrides(cfg *Config) error {
	if v := getEnv("SERVER_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := getEnv("SERVER_ADMIN_ADDR"); v != "" {
		cfg.Server.AdminAddr = v
	}
	return applyTLSEnvOverrides(cfg)
}

// applyTLSEnvOverrides applies TLS-related environment variable overrides.
func applyTLSEnvOverrides(cfg *Config) error {
	if v := getEnv("SERVER_TLS_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "SERVER_TLS_ENABLED", v, err)
		}
		cfg.Server.TLS.Enabled = &b
	}
	if v := getEnv("SERVER_TLS_CERT_FILE"); v != "" {
		cfg.Server.TLS.CertFile = v
	}
	if v := getEnv("SERVER_TLS_KEY_FILE"); v != "" {
		cfg.Server.TLS.KeyFile = v
	}
	if v := getEnv("SERVER_TLS_CA_FILE"); v != "" {
		cfg.Server.TLS.CAFile = v
	}
	if v := getEnv("SERVER_TLS_AUTO_ROTATE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "SERVER_TLS_AUTO_ROTATE", v, err)
		}
		cfg.Server.TLS.AutoRotate = &b
	}
	return nil
}

// applyUpstreamEnvOverrides applies upstream-related environment variable overrides.
func applyUpstreamEnvOverrides(cfg *Config) error {
	if v := getEnv("UPSTREAM_HEADER_PREFIX"); v != "" {
		cfg.Upstream.HeaderPrefix = v
	}
	if v := getEnv("UPSTREAM_TRACE_HEADER"); v != "" {
		cfg.Upstream.TraceHeader = v
	}
	return applyTimeoutEnvOverrides(cfg)
}

// applyTimeoutEnvOverrides applies timeout-related environment variable overrides.
func applyTimeoutEnvOverrides(cfg *Config) error {
	if v := getEnv("UPSTREAM_TIMEOUTS_CONNECT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "UPSTREAM_TIMEOUTS_CONNECT", v, err)
		}
		cfg.Upstream.Timeouts.Connect = d
	}
	if v := getEnv("UPSTREAM_TIMEOUTS_READ"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "UPSTREAM_TIMEOUTS_READ", v, err)
		}
		cfg.Upstream.Timeouts.Read = d
	}
	if v := getEnv("UPSTREAM_TIMEOUTS_WRITE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "UPSTREAM_TIMEOUTS_WRITE", v, err)
		}
		cfg.Upstream.Timeouts.Write = d
	}
	if v := getEnv("UPSTREAM_TIMEOUTS_IDLE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "UPSTREAM_TIMEOUTS_IDLE", v, err)
		}
		cfg.Upstream.Timeouts.Idle = d
	}
	return nil
}

// applyObservabilityEnvOverrides applies observability-related environment variable overrides.
func applyObservabilityEnvOverrides(cfg *Config) error {
	if v := getEnv("OBSERVABILITY_LOG_LEVEL"); v != "" {
		cfg.Observability.LogLevel = v
	}
	if v := getEnv("OBSERVABILITY_ENABLE_PROFILING"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "OBSERVABILITY_ENABLE_PROFILING", v, err)
		}
		cfg.Observability.EnableProfiling = b
	}
	return nil
}

// getEnv retrieves an environment variable with the CHAPERONE_ prefix.
func getEnv(key string) string {
	return os.Getenv(EnvPrefix + "_" + strings.ToUpper(key))
}
