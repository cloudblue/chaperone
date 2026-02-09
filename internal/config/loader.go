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
	applyServerDefaults(&cfg.Server)
	applyUpstreamDefaults(&cfg.Upstream)
	applyObservabilityDefaults(&cfg.Observability)
}

// applyServerDefaults applies default values for server configuration.
func applyServerDefaults(cfg *ServerConfig) {
	if cfg.Addr == "" {
		cfg.Addr = DefaultServerAddr
	}
	if cfg.AdminAddr == "" {
		cfg.AdminAddr = DefaultAdminAddr
	}
	if cfg.ShutdownTimeout == nil {
		cfg.ShutdownTimeout = durationPtr(DefaultShutdownTimeout)
	}
	applyTLSDefaults(&cfg.TLS)
}

// applyTLSDefaults applies default values for TLS configuration.
func applyTLSDefaults(cfg *TLSConfig) {
	// Enabled: nil means not set, so apply default (true for security)
	if cfg.Enabled == nil {
		tlsEnabled := DefaultTLSEnabled
		cfg.Enabled = &tlsEnabled
	}
	if cfg.CertFile == "" {
		cfg.CertFile = DefaultCertFile
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = DefaultKeyFile
	}
	if cfg.CAFile == "" {
		cfg.CAFile = DefaultCAFile
	}
	// AutoRotate: nil means not set, so apply default (true)
	if cfg.AutoRotate == nil {
		autoRotate := DefaultAutoRotate
		cfg.AutoRotate = &autoRotate
	}
}

// applyUpstreamDefaults applies default values for upstream configuration.
func applyUpstreamDefaults(cfg *UpstreamConfig) {
	if cfg.HeaderPrefix == "" {
		cfg.HeaderPrefix = DefaultHeaderPrefix
	}
	if cfg.TraceHeader == "" {
		cfg.TraceHeader = DefaultTraceHeader
	}
	applyTimeoutDefaults(&cfg.Timeouts)
}

// applyTimeoutDefaults applies default values for timeout configuration.
// Only fills nil pointers — explicitly set values (including zero) are preserved
// and validated later.
func applyTimeoutDefaults(cfg *TimeoutConfig) {
	if cfg.Connect == nil {
		cfg.Connect = durationPtr(DefaultConnectTimeout)
	}
	if cfg.Read == nil {
		cfg.Read = durationPtr(DefaultReadTimeout)
	}
	if cfg.Write == nil {
		cfg.Write = durationPtr(DefaultWriteTimeout)
	}
	if cfg.Idle == nil {
		cfg.Idle = durationPtr(DefaultIdleTimeout)
	}
	if cfg.KeepAlive == nil {
		cfg.KeepAlive = durationPtr(DefaultKeepAlive)
	}
	if cfg.Plugin == nil {
		cfg.Plugin = durationPtr(DefaultPluginTimeout)
	}
}

// applyObservabilityDefaults applies default values for observability configuration.
func applyObservabilityDefaults(cfg *ObservabilityConfig) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
	// EnableProfiling defaults to false (secure default), which is Go zero value

	// Security: Always merge user-provided sensitive headers with mandatory
	// defaults. This prevents silent credential leaks when a Distributor adds
	// custom headers without realizing the defaults would be dropped.
	// Per Design Spec Section 5.3: the default list is a "strict Redact List".
	cfg.SensitiveHeaders = MergeSensitiveHeaders(
		cfg.SensitiveHeaders,
	)
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
	if v := getEnv("SERVER_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "SERVER_SHUTDOWN_TIMEOUT", v, err)
		}
		cfg.Server.ShutdownTimeout = &d
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
	overrides := []struct {
		envKey string
		target **time.Duration
	}{
		{"UPSTREAM_TIMEOUTS_CONNECT", &cfg.Upstream.Timeouts.Connect},
		{"UPSTREAM_TIMEOUTS_READ", &cfg.Upstream.Timeouts.Read},
		{"UPSTREAM_TIMEOUTS_WRITE", &cfg.Upstream.Timeouts.Write},
		{"UPSTREAM_TIMEOUTS_IDLE", &cfg.Upstream.Timeouts.Idle},
		{"UPSTREAM_TIMEOUTS_KEEP_ALIVE", &cfg.Upstream.Timeouts.KeepAlive},
		{"UPSTREAM_TIMEOUTS_PLUGIN", &cfg.Upstream.Timeouts.Plugin},
	}
	for _, o := range overrides {
		if err := applyDurationEnvOverride(o.envKey, o.target); err != nil {
			return err
		}
	}
	return nil
}

// applyDurationEnvOverride reads a single duration env var and writes it to target.
func applyDurationEnvOverride(envKey string, target **time.Duration) error {
	v := getEnv(envKey)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, envKey, v, err)
	}
	*target = &d
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
	// Security: Body logging can ONLY be enabled via env var, not config file.
	// The yaml:"-" tag on EnableBodyLogging prevents YAML from setting it.
	// Per Design Spec Section 5.3 (Body Safety).
	if v := getEnv("OBSERVABILITY_ENABLE_BODY_LOGGING"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid %s_%s value %q: %w", EnvPrefix, "OBSERVABILITY_ENABLE_BODY_LOGGING", v, err)
		}
		cfg.Observability.EnableBodyLogging = b
	}
	return nil
}

// getEnv retrieves an environment variable with the CHAPERONE_ prefix.
func getEnv(key string) string {
	return os.Getenv(EnvPrefix + "_" + strings.ToUpper(key))
}
