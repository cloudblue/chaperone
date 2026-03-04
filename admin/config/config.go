// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// EnvPrefix is the environment variable prefix for admin portal configuration.
const EnvPrefix = "CHAPERONE_ADMIN"

// Config holds the admin portal configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Scraper  ScraperConfig  `yaml:"scraper"`
	Session  SessionConfig  `yaml:"session"`
	Audit    AuditConfig    `yaml:"audit"`
	Log      LogConfig      `yaml:"log"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// DatabaseConfig configures the SQLite database.
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// ScraperConfig configures the proxy metrics scraper.
type ScraperConfig struct {
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
}

// SessionConfig configures session management.
type SessionConfig struct {
	MaxAge      Duration `yaml:"max_age"`
	IdleTimeout Duration `yaml:"idle_timeout"`
}

// AuditConfig configures the audit log.
type AuditConfig struct {
	RetentionDays *int `yaml:"retention_days"`
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Duration is a time.Duration that unmarshals from YAML duration strings
// like "10s", "5m", "24h".
type Duration time.Duration

// Unwrap returns the underlying time.Duration.
func (d Duration) Unwrap() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	dur, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}
