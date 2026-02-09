// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package config

import "time"

// Config is the root configuration structure for Chaperone.
type Config struct {
	// Server holds the server binding configuration.
	Server ServerConfig `yaml:"server"`
	// Upstream holds the upstream proxy configuration.
	Upstream UpstreamConfig `yaml:"upstream"`
	// Observability holds logging and profiling configuration.
	Observability ObservabilityConfig `yaml:"observability"`
}

// ServerConfig holds the server binding and TLS configuration.
type ServerConfig struct {
	// Addr is the traffic port address (e.g., ":443").
	Addr string `yaml:"addr"`
	// AdminAddr is the management/metrics port address (e.g., ":9090").
	AdminAddr string `yaml:"admin_addr"`
	// TLS holds the mTLS configuration.
	TLS TLSConfig `yaml:"tls"`
}

// TLSConfig holds the TLS/mTLS configuration.
type TLSConfig struct {
	// Enabled controls whether TLS/mTLS is active.
	// Pointer to distinguish "not set" (nil → default true) from "explicitly false".
	// Default: true.
	Enabled *bool `yaml:"enabled"`
	// CertFile is the path to the server certificate PEM file.
	CertFile string `yaml:"cert_file"`
	// KeyFile is the path to the server private key PEM file.
	KeyFile string `yaml:"key_file"`
	// CAFile is the path to the CA certificate PEM file for client verification.
	CAFile string `yaml:"ca_file"`
	// AutoRotate enables automatic certificate rotation.
	// Pointer to distinguish "not set" (nil) from "explicitly false".
	// Default: true (per Design Spec).
	AutoRotate *bool `yaml:"auto_rotate"`
}

// UpstreamConfig holds the upstream proxy configuration.
type UpstreamConfig struct {
	// HeaderPrefix is the prefix for context headers (default: "X-Connect").
	// This enables ADR-005 decoupled naming.
	HeaderPrefix string `yaml:"header_prefix"`
	// TraceHeader is the correlation ID header name (default: "Connect-Request-ID").
	TraceHeader string `yaml:"trace_header"`
	// AllowList maps hosts to allowed path patterns.
	// Security: This MUST be explicitly configured; empty means deny all.
	AllowList map[string][]string `yaml:"allow_list"`
	// Timeouts holds the upstream connection timeouts.
	Timeouts TimeoutConfig `yaml:"timeouts"`
}

// TimeoutConfig holds the timeout configuration for upstream connections.
type TimeoutConfig struct {
	// Connect is the connection establishment timeout.
	Connect time.Duration `yaml:"connect"`
	// Read is the maximum time waiting for response headers.
	Read time.Duration `yaml:"read"`
	// Write is the maximum time for writing the response.
	Write time.Duration `yaml:"write"`
	// Idle is the keep-alive connection timeout.
	Idle time.Duration `yaml:"idle"`
}

// ObservabilityConfig holds logging and profiling configuration.
type ObservabilityConfig struct {
	// LogLevel is the logging level (debug, info, warn, error).
	LogLevel string `yaml:"log_level"`
	// EnableProfiling enables the /debug/pprof endpoint on the admin port.
	// Security: This should be false in production.
	EnableProfiling bool `yaml:"enable_profiling"`
	// EnableBodyLogging allows request/response bodies to appear in debug logs.
	// Security: This MUST only be enabled via the CHAPERONE_OBSERVABILITY_ENABLE_BODY_LOGGING
	// environment variable, not via config file. A startup warning is emitted when enabled.
	// Per Design Spec Section 5.3 (Body Safety).
	EnableBodyLogging bool `yaml:"-"`
	// SensitiveHeaders is the list of additional headers to redact from logs
	// and strip from responses. These are merged with the built-in defaults
	// (Authorization, Proxy-Authorization, Cookie, Set-Cookie, X-API-Key,
	// X-Auth-Token) which are always included. Duplicate entries are
	// deduplicated case-insensitively.
	SensitiveHeaders []string `yaml:"sensitive_headers"`
}
