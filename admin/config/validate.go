// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// Validate checks the configuration for required fields and valid values.
func (c *Config) Validate() error {
	var errs []error

	if c.Server.Addr == "" {
		errs = append(errs, errors.New("server.addr is required"))
	} else if _, _, err := net.SplitHostPort(c.Server.Addr); err != nil {
		errs = append(errs, fmt.Errorf("server.addr: %w", err))
	}

	if c.Database.Path == "" {
		errs = append(errs, errors.New("database.path is required"))
	}

	if c.Scraper.Interval.Unwrap() < 1*time.Second {
		errs = append(errs, errors.New("scraper.interval must be at least 1s"))
	}
	if c.Scraper.Timeout.Unwrap() < 1*time.Second {
		errs = append(errs, errors.New("scraper.timeout must be at least 1s"))
	}
	if c.Scraper.Timeout.Unwrap() >= c.Scraper.Interval.Unwrap() {
		errs = append(errs, errors.New("scraper.timeout must be less than scraper.interval"))
	}

	if c.Session.MaxAge.Unwrap() < 1*time.Minute {
		errs = append(errs, errors.New("session.max_age must be at least 1m"))
	}
	if c.Session.IdleTimeout.Unwrap() < 1*time.Minute {
		errs = append(errs, errors.New("session.idle_timeout must be at least 1m"))
	}

	if *c.Audit.RetentionDays < 0 {
		errs = append(errs, errors.New("audit.retention_days must be non-negative (0 = keep forever)"))
	}

	switch c.Log.Level {
	case "debug", DefaultLogLevel, "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level: unknown level %q (valid: debug, info, warn, error)", c.Log.Level))
	}

	switch c.Log.Format {
	case DefaultLogFormat, LogFormatText:
	default:
		errs = append(errs, fmt.Errorf("log.format: unknown format %q (valid: json, text)", c.Log.Format))
	}

	return errors.Join(errs...)
}
