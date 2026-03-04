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
		cfg.Server.Addr = "127.0.0.1:8080"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./chaperone-admin.db"
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
	if cfg.Audit.RetentionDays == 0 {
		cfg.Audit.RetentionDays = 90
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
}

func applyEnvOverrides(cfg *Config) error {
	var errs []error

	if v := getEnv("SERVER_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := getEnv("DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := getEnv("SCRAPER_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("SCRAPER_INTERVAL: %w", err))
		} else {
			cfg.Scraper.Interval = Duration(d)
		}
	}
	if v := getEnv("SCRAPER_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("SCRAPER_TIMEOUT: %w", err))
		} else {
			cfg.Scraper.Timeout = Duration(d)
		}
	}
	if v := getEnv("SESSION_MAX_AGE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("SESSION_MAX_AGE: %w", err))
		} else {
			cfg.Session.MaxAge = Duration(d)
		}
	}
	if v := getEnv("SESSION_IDLE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("SESSION_IDLE_TIMEOUT: %w", err))
		} else {
			cfg.Session.IdleTimeout = Duration(d)
		}
	}
	if v := getEnv("AUDIT_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("AUDIT_RETENTION_DAYS: %w", err))
		} else {
			cfg.Audit.RetentionDays = n
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

func getEnv(key string) string {
	return os.Getenv(EnvPrefix + "_" + key)
}
