// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Load reads configuration from the given path, applies defaults and
// environment variable overrides, and validates the result.
func Load(path string) (*Config, error) {
	path = resolveConfigPath(path)

	cfg := &Config{}
	if err := loadYAML(path, cfg); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading config %s: %w", path, err)
		}
		// Config file not found — proceed with defaults + env overrides.
	}

	applyDefaults(cfg)

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, fmt.Errorf("applying env overrides: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func resolveConfigPath(path string) string {
	if path != "" {
		return path
	}
	if v := os.Getenv(EnvPrefix + "_CONFIG"); v != "" {
		return v
	}
	return "chaperone-admin.yaml"
}

func loadYAML(path string, cfg *Config) error {
	// #nosec G304 -- path comes from trusted sources: CLI flag, env var, or hardcoded default.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = DefaultAddr
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = DefaultDBPath
	}
	if cfg.Scraper.Interval == 0 {
		cfg.Scraper.Interval = Duration(10 * time.Second)
	}
	if cfg.Scraper.Timeout == 0 {
		cfg.Scraper.Timeout = Duration(5 * time.Second)
	}
	if cfg.Session.MaxAge == 0 {
		cfg.Session.MaxAge = Duration(24 * time.Hour)
	}
	if cfg.Session.IdleTimeout == 0 {
		cfg.Session.IdleTimeout = Duration(2 * time.Hour)
	}
	if cfg.Audit.RetentionDays == nil {
		cfg.Audit.RetentionDays = intPtr(90)
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = DefaultLogLevel
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = DefaultLogFormat
	}
}

func applyEnvOverrides(cfg *Config) error {
	var errs []error

	if v := getEnv("SERVER_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := getEnv("SERVER_SECURE_COOKIES"); v != "" {
		cfg.Server.SecureCookies = v == "true" || v == "1"
	}
	if v := getEnv("DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}

	parseDuration(&cfg.Scraper.Interval, "SCRAPER_INTERVAL", &errs)
	parseDuration(&cfg.Scraper.Timeout, "SCRAPER_TIMEOUT", &errs)
	parseDuration(&cfg.Session.MaxAge, "SESSION_MAX_AGE", &errs)
	parseDuration(&cfg.Session.IdleTimeout, "SESSION_IDLE_TIMEOUT", &errs)

	if v := getEnv("AUDIT_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("AUDIT_RETENTION_DAYS: %w", err))
		} else {
			cfg.Audit.RetentionDays = &n
		}
	}
	if v := getEnv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
	if v := getEnv("LOG_FORMAT"); v != "" {
		cfg.Log.Format = strings.ToLower(v)
	}

	return errors.Join(errs...)
}

func parseDuration(dst *Duration, envKey string, errs *[]error) {
	v := getEnv(envKey)
	if v == "" {
		return
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", envKey, err))
		return
	}
	*dst = Duration(d)
}

func getEnv(key string) string {
	return os.Getenv(EnvPrefix + "_" + key)
}

func intPtr(v int) *int { return &v }
