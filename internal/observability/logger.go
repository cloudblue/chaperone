// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package observability provides logging, metrics, and tracing primitives
// for the Chaperone proxy. It includes a RedactingHandler that wraps slog
// to prevent sensitive credential values from appearing in log output.
package observability

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// RedactedValue is the replacement string for sensitive data in logs.
const RedactedValue = "[REDACTED]"

// MinSecretLength is the minimum length of a secret value for value-based
// scanning. Shorter values risk excessive false positives (e.g., a 2-char
// API key matching random text).
const MinSecretLength = 8

// secretValuesKey is an unexported type for the context key that stores
// known secret values for value-based redaction, preventing collisions
// with keys from other packages.
type secretValuesKey struct{}

// WithSecretValue appends a secret value to the context for value-based
// redaction. The RedactingHandler will scan all string attributes and
// messages for these values and replace them with [REDACTED].
//
// Typically called in request middleware after extracting sensitive headers:
//
//	ctx = observability.WithSecretValue(ctx, req.Header.Get("Authorization"))
func WithSecretValue(ctx context.Context, secret string) context.Context {
	existing := SecretValues(ctx)
	updated := make([]string, len(existing)+1)
	copy(updated, existing)
	updated[len(existing)] = secret
	return context.WithValue(ctx, secretValuesKey{}, updated)
}

// SecretValues retrieves the list of known secret values from the context.
// Returns nil if no secrets have been stored.
func SecretValues(ctx context.Context) []string {
	vals, _ := ctx.Value(secretValuesKey{}).([]string)
	return vals
}

// RedactingHandler is an slog.Handler that intercepts log records and
// redacts sensitive information using five complementary layers:
//
//  1. Key-based: Attributes whose key matches a known sensitive header name
//     (case-insensitive) have their value replaced with [REDACTED].
//  2. Type-based: Attributes whose value is an http.Header have sensitive
//     entries redacted before serialization.
//  3. Value-based: Attributes whose string value contains a known secret
//     (from context) have their entire value replaced with [REDACTED].
//  4. Message: The log message itself is scanned for known secret values
//     and any occurrences are replaced with [REDACTED].
//  5. Body suppression: Attributes with body-related keys (e.g., "original_body",
//     "request_body") are redacted when body logging is disabled.
//
// This handler is a defense-in-depth safety net per Design Spec Section 5.3.
type RedactingHandler struct {
	inner              slog.Handler
	sensitive          map[string]struct{}
	bodyLoggingEnabled bool
}

// NewRedactingHandler creates a RedactingHandler wrapping the given inner
// handler. The sensitiveHeaders list defines which attribute key names
// trigger key-based redaction (matched case-insensitively).
// When bodyLoggingEnabled is false, attributes with body-related keys
// (e.g., "original_body", "request_body") are redacted as a safety net.
func NewRedactingHandler(inner slog.Handler, sensitiveHeaders []string, bodyLoggingEnabled bool) *RedactingHandler {
	s := make(map[string]struct{}, len(sensitiveHeaders))
	for _, h := range sensitiveHeaders {
		s[strings.ToLower(h)] = struct{}{}
	}
	return &RedactingHandler{
		inner:              inner,
		sensitive:          s,
		bodyLoggingEnabled: bodyLoggingEnabled,
	}
}

// Enabled reports whether the inner handler handles records at the given level.
func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle redacts sensitive information from the record and delegates to the
// inner handler.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	// Collect secret values from context for value-based scanning
	secrets := SecretValues(ctx)

	// Layer 4: Redact secrets from message
	msg := h.redactMessage(r.Message, secrets)

	// Layers 1–3, 5: Redact attributes
	redacted := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		redacted = append(redacted, h.redactAttr(a, secrets))
		return true
	})

	// Build new record with redacted attrs
	newRecord := slog.NewRecord(r.Time, r.Level, msg, r.PC)
	newRecord.AddAttrs(redacted...)

	return h.inner.Handle(ctx, newRecord)
}

// WithAttrs returns a new handler with pre-set attributes, redacting any
// sensitive values. This ensures that attributes set via logger.With() are
// also protected.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.redactAttr(a, nil)
	}
	return &RedactingHandler{
		inner:              h.inner.WithAttrs(redacted),
		sensitive:          h.sensitive,
		bodyLoggingEnabled: h.bodyLoggingEnabled,
	}
}

// WithGroup returns a new handler with the given group name. The group
// structure is preserved and redaction applies within the group.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		inner:              h.inner.WithGroup(name),
		sensitive:          h.sensitive,
		bodyLoggingEnabled: h.bodyLoggingEnabled,
	}
}

// redactAttr applies attribute redaction layers (1–3, 5, and group recursion) to a single attr.
func (h *RedactingHandler) redactAttr(a slog.Attr, secrets []string) slog.Attr {
	// Layer 1: Key-based — sensitive header name as key
	if h.isSensitiveKey(a.Key) {
		return slog.String(a.Key, RedactedValue)
	}

	// Layer 2: Type-based — value is http.Header
	if a.Value.Kind() == slog.KindAny {
		if headers, ok := a.Value.Any().(http.Header); ok {
			return slog.Any(a.Key, h.redactHTTPHeader(headers))
		}
	}

	// Layer 3: Value-based — string value contains known secret
	if a.Value.Kind() == slog.KindString {
		if h.containsSecret(a.Value.String(), secrets) {
			return slog.String(a.Key, RedactedValue)
		}
	}

	// Layer 5: Body key suppression — redact body content when body logging disabled
	if !h.bodyLoggingEnabled && isBodyKey(a.Key) {
		return slog.String(a.Key, RedactedValue)
	}

	// Handle groups recursively
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		redacted := make([]slog.Attr, len(attrs))
		for i, ga := range attrs {
			redacted[i] = h.redactAttr(ga, secrets)
		}
		return slog.Group(a.Key, attrsToAny(redacted)...)
	}

	return a
}

// isSensitiveKey returns true if the key matches a known sensitive header
// name (case-insensitive).
func (h *RedactingHandler) isSensitiveKey(key string) bool {
	_, ok := h.sensitive[strings.ToLower(key)]
	return ok
}

// redactHTTPHeader returns a copy of the header map with sensitive entries
// replaced by [REDACTED]. Non-sensitive entries are preserved.
func (h *RedactingHandler) redactHTTPHeader(headers http.Header) http.Header {
	result := make(http.Header, len(headers))
	for key, values := range headers {
		if h.isSensitiveKey(key) {
			result[key] = []string{RedactedValue}
		} else {
			copied := make([]string, len(values))
			copy(copied, values)
			result[key] = copied
		}
	}
	return result
}

// containsSecret returns true if the string value contains any of the
// known secret values from context. Secrets shorter than MinSecretLength
// are skipped to avoid false positives.
func (h *RedactingHandler) containsSecret(value string, secrets []string) bool {
	for _, s := range secrets {
		if len(s) < MinSecretLength {
			continue
		}
		if strings.Contains(value, s) {
			return true
		}
	}
	return false
}

// redactMessage scans the log message for known secret values and replaces
// any occurrences with [REDACTED].
func (h *RedactingHandler) redactMessage(msg string, secrets []string) string {
	for _, s := range secrets {
		if len(s) < MinSecretLength {
			continue
		}
		if strings.Contains(msg, s) {
			msg = strings.ReplaceAll(msg, s, RedactedValue)
		}
	}
	return msg
}

// bodyKeys is the set of slog attribute keys that contain request/response
// body content. When body logging is disabled, these keys are redacted.
// This is a safety net: code like NormalizeError can always log bodies at
// DEBUG level, and the handler decides whether the content passes through.
var bodyKeys = map[string]struct{}{
	"original_body": {},
	"request_body":  {},
	"response_body": {},
	"body":          {},
}

// isBodyKey returns true if the attribute key is a known body content key.
func isBodyKey(key string) bool {
	_, ok := bodyKeys[strings.ToLower(key)]
	return ok
}

// attrsToAny converts a slice of slog.Attr to a slice of any for use
// with slog.Group().
func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, a := range attrs {
		result[i] = a
	}
	return result
}

// NewLogger creates a configured slog.Logger with redaction enabled.
// It wraps a JSON handler writing to w at the given level, with the
// RedactingHandler providing defense-in-depth credential protection.
// When bodyLoggingEnabled is false, body-related attributes are redacted.
//
// Usage in main:
//
//	logger := observability.NewLogger(os.Stdout, logLevel, sensitiveHeaders, false)
//	slog.SetDefault(logger)
func NewLogger(w io.Writer, level slog.Level, sensitiveHeaders []string, bodyLoggingEnabled bool) *slog.Logger {
	inner := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	handler := NewRedactingHandler(inner, sensitiveHeaders, bodyLoggingEnabled)
	return slog.New(handler)
}
