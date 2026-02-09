// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package config provides configuration loading and validation for Chaperone.
// It supports YAML configuration files with environment variable overrides
// following 12-Factor App methodology.
package config

import (
	"strings"
	"time"
)

// Default server configuration values.
const (
	// DefaultServerAddr is the default traffic port address.
	DefaultServerAddr = ":443"
	// DefaultAdminAddr is the default management/metrics port address.
	// Bound to localhost only for security - admin endpoints should not be exposed to the network.
	DefaultAdminAddr = "127.0.0.1:9090"
)

// Default TLS configuration values.
const (
	// DefaultTLSEnabled enables TLS/mTLS by default.
	DefaultTLSEnabled = true
	// DefaultCertFile is the default path to the server certificate.
	DefaultCertFile = "/certs/server.crt"
	// DefaultKeyFile is the default path to the server private key.
	DefaultKeyFile = "/certs/server.key"
	// DefaultCAFile is the default path to the CA certificate.
	DefaultCAFile = "/certs/ca.crt"
	// DefaultAutoRotate enables certificate auto-rotation by default.
	DefaultAutoRotate = true
)

// Default upstream configuration values.
const (
	// DefaultHeaderPrefix is the prefix for context headers (ADR-005).
	DefaultHeaderPrefix = "X-Connect"
	// DefaultTraceHeader is the correlation ID header name (ADR-005).
	DefaultTraceHeader = "Connect-Request-ID"
)

// Default timeout configuration values.
const (
	// DefaultConnectTimeout is the connection establishment timeout.
	DefaultConnectTimeout = 5 * time.Second
	// DefaultReadTimeout is the maximum time waiting for response headers.
	DefaultReadTimeout = 30 * time.Second
	// DefaultWriteTimeout is the maximum time for writing the response.
	DefaultWriteTimeout = 30 * time.Second
	// DefaultIdleTimeout is the keep-alive connection timeout.
	DefaultIdleTimeout = 120 * time.Second
)

// Default observability configuration values.
const (
	// DefaultLogLevel is the default logging level.
	DefaultLogLevel = "info"
	// DefaultEnableProfiling is the secure default for profiling (disabled).
	DefaultEnableProfiling = false
)

// defaultSensitiveHeaders returns the list of headers that MUST be redacted
// in logs. This is a security-critical default per Design Spec Section 5.3.
// Returns a new copy each time to prevent accidental mutation.
func defaultSensitiveHeaders() []string {
	return []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"Set-Cookie",
		"X-API-Key",
		"X-Auth-Token",
	}
}

// MergeSensitiveHeaders returns the built-in security-critical headers merged
// with any additional headers, deduplicated case-insensitively.
// Built-in defaults always come first in the result.
//
// This ensures security-critical defaults (Authorization, Cookie, etc.)
// are never silently dropped when a Distributor adds custom headers.
// Per Design Spec Section 5.3: these form a "strict Redact List".
func MergeSensitiveHeaders(extra []string) []string {
	base := defaultSensitiveHeaders()
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, h := range base {
		key := strings.ToLower(h)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			merged = append(merged, h)
		}
	}
	for _, h := range extra {
		key := strings.ToLower(h)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			merged = append(merged, h)
		}
	}
	return merged
}

// ValidLogLevels is the list of accepted log level values.
var ValidLogLevels = []string{"debug", "info", "warn", "error"}
