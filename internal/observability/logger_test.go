// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// --- Helpers ---

// captureLog runs fn with a RedactingHandler writing to a buffer and returns
// the raw JSON bytes produced. It uses the given sensitive headers and secret
// values (via context). Body logging is disabled by default (safe mode).
func captureLog(t *testing.T, sensitiveHeaders []string, secretValues []string, fn func(logger *slog.Logger, ctx context.Context)) []byte {
	t.Helper()
	return captureLogWithBodyLogging(t, sensitiveHeaders, secretValues, false, fn)
}

// captureLogWithBodyLogging is like captureLog but allows controlling the
// bodyLoggingEnabled flag.
func captureLogWithBodyLogging(t *testing.T, sensitiveHeaders []string, secretValues []string, bodyLoggingEnabled bool, fn func(logger *slog.Logger, ctx context.Context)) []byte {
	t.Helper()

	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler := NewRedactingHandler(inner, sensitiveHeaders, bodyLoggingEnabled)
	logger := slog.New(handler)

	ctx := context.Background()
	for _, v := range secretValues {
		ctx = WithSecretValue(ctx, v)
	}

	fn(logger, ctx)
	return buf.Bytes()
}

// assertNotContains fails if the raw log output contains the forbidden string.
// This is a byte-level guarantee: the sensitive value must NOT appear anywhere
// in the serialized output.
func assertNotContains(t *testing.T, output []byte, forbidden string) {
	t.Helper()
	if bytes.Contains(output, []byte(forbidden)) {
		t.Errorf("log output contains forbidden value %q:\n%s", forbidden, output)
	}
}

// assertContains fails if the raw log output does NOT contain the expected string.
func assertContains(t *testing.T, output []byte, expected string) {
	t.Helper()
	if !bytes.Contains(output, []byte(expected)) {
		t.Errorf("log output missing expected value %q:\n%s", expected, output)
	}
}

// defaultSensitive returns the standard sensitive header list for tests.
func defaultSensitive() []string {
	return []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"Set-Cookie",
		"X-API-Key",
		"X-Auth-Token",
	}
}

// --- Key-Based Redaction Tests ---

func TestRedactingHandler_KeyMatching_RedactsSensitiveKeys(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		wantRedacted bool
	}{
		{"authorization key", "Authorization", "Bearer secret-token", true},
		{"cookie key", "Cookie", "session=abc123", true},
		{"x-api-key key", "X-API-Key", "api-key-value", true},
		{"x-auth-token key", "X-Auth-Token", "auth-token-value", true},
		{"proxy-authorization key", "Proxy-Authorization", "Basic creds", true},
		{"set-cookie key", "Set-Cookie", "token=xyz", true},
		{"safe key preserved", "Content-Type", "application/json", false},
		{"trace id preserved", "X-Trace-ID", "abc123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				logger.InfoContext(ctx, "test", tt.key, tt.value)
			})

			if tt.wantRedacted {
				assertNotContains(t, output, tt.value)
				assertContains(t, output, RedactedValue)
			} else {
				assertContains(t, output, tt.value)
			}
		})
	}
}

func TestRedactingHandler_KeyMatching_CaseInsensitive(t *testing.T) {
	secret := "Bearer super-secret-token-xyz"
	cases := []string{"authorization", "AUTHORIZATION", "Authorization", "aUtHoRiZaTiOn"}

	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				logger.InfoContext(ctx, "test", key, secret)
			})

			assertNotContains(t, output, secret)
			assertContains(t, output, RedactedValue)
		})
	}
}

// --- Type-Based Redaction (http.Header) Tests ---

func TestRedactingHandler_HTTPHeader_RedactsSensitiveEntries(t *testing.T) {
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		headers := http.Header{
			"Authorization": []string{"Bearer secret-jwt-token"},
			"Content-Type":  []string{"application/json"},
			"Cookie":        []string{"session=private-cookie"},
			"X-Trace-ID":    []string{"trace-123"},
		}
		logger.InfoContext(ctx, "request received", "headers", headers)
	})

	// Sensitive values must not appear
	assertNotContains(t, output, "secret-jwt-token")
	assertNotContains(t, output, "private-cookie")

	// Safe values must appear
	assertContains(t, output, "application/json")
	assertContains(t, output, "trace-123")

	// Redacted marker must appear
	assertContains(t, output, RedactedValue)
}

// --- Value-Based Redaction (Context Secrets) Tests ---

func TestRedactingHandler_ValueBased_RedactsKnownSecretValues(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		secret       string
		wantRedacted bool
	}{
		{
			name:         "secret value under arbitrary key",
			key:          "some_debug_field",
			value:        "Bearer eyJhbGciOiJIUzI1NiJ9.secret",
			secret:       "Bearer eyJhbGciOiJIUzI1NiJ9.secret",
			wantRedacted: true,
		},
		{
			name:         "secret value as substring",
			key:          "data",
			value:        "prefix-my-api-key-12345-suffix",
			secret:       "my-api-key-12345",
			wantRedacted: true,
		},
		{
			name:         "non-secret value not redacted",
			key:          "data",
			value:        "this is safe data",
			secret:       "completely-different-secret",
			wantRedacted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), []string{tt.secret}, func(logger *slog.Logger, ctx context.Context) {
				logger.InfoContext(ctx, "event", tt.key, tt.value)
			})

			if tt.wantRedacted {
				assertNotContains(t, output, tt.secret)
				assertContains(t, output, RedactedValue)
			} else {
				assertContains(t, output, tt.value)
			}
		})
	}
}

func TestRedactingHandler_ValueBased_MultipleSecrets(t *testing.T) {
	secret1 := "Bearer token-alpha-secret"
	secret2 := "api-key-beta-9876"

	output := captureLog(t, defaultSensitive(), []string{secret1, secret2}, func(logger *slog.Logger, ctx context.Context) {
		logger.InfoContext(ctx, "event",
			"field1", "contains "+secret1+" here",
			"field2", secret2,
			"safe", "no secrets here",
		)
	})

	assertNotContains(t, output, secret1)
	assertNotContains(t, output, secret2)
	assertContains(t, output, "no secrets here")
}

func TestRedactingHandler_ValueBased_ShortSecretsIgnored(t *testing.T) {
	// Secrets shorter than MinSecretLength should NOT be used for value scanning
	// to avoid false positives (e.g., a 2-char API key matching random text).
	shortSecret := "ab"
	safeValue := "alphabet"

	output := captureLog(t, defaultSensitive(), []string{shortSecret}, func(logger *slog.Logger, ctx context.Context) {
		logger.InfoContext(ctx, "event", "data", safeValue)
	})

	// "alphabet" contains "ab" but should NOT be redacted (short secret ignored)
	assertContains(t, output, safeValue)
}

func TestRedactingHandler_ValueBased_NoContextSecrets_PassesThrough(t *testing.T) {
	// When no secret values are in context, value-based scanning is skipped
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		logger.InfoContext(ctx, "event", "data", "Bearer some-token-value")
	})

	// Without context secrets, arbitrary values pass through
	assertContains(t, output, "Bearer some-token-value")
}

// --- Message Redaction Tests ---

func TestRedactingHandler_Message_RedactsSecretInMessage(t *testing.T) {
	secret := "super-secret-credential-xyz"

	output := captureLog(t, defaultSensitive(), []string{secret}, func(logger *slog.Logger, ctx context.Context) {
		logger.InfoContext(ctx, "auth failed with "+secret)
	})

	assertNotContains(t, output, secret)
	assertContains(t, output, RedactedValue)
}

// --- Group and WithAttrs Tests ---

func TestRedactingHandler_WithGroup_RedactsInsideGroup(t *testing.T) {
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		grouped := logger.WithGroup("request")
		grouped.InfoContext(ctx, "incoming", "Authorization", "Bearer grouped-secret")
	})

	assertNotContains(t, output, "grouped-secret")
	assertContains(t, output, RedactedValue)
}

func TestRedactingHandler_WithAttrs_RedactsPresetAttrs(t *testing.T) {
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		withAttrs := logger.With("Authorization", "Bearer preset-secret")
		withAttrs.InfoContext(ctx, "event1")
		withAttrs.InfoContext(ctx, "event2")
	})

	assertNotContains(t, output, "preset-secret")

	// Should appear twice (two log lines), each with redaction
	count := bytes.Count(output, []byte(RedactedValue))
	if count < 2 {
		t.Errorf("expected at least 2 redacted markers, got %d\noutput: %s", count, output)
	}
}

// --- All Log Levels Tests ---

func TestRedactingHandler_AllLevels_RedactionApplies(t *testing.T) {
	levels := []struct {
		name  string
		logFn func(*slog.Logger, context.Context, string, ...any)
	}{
		{"debug", (*slog.Logger).DebugContext},
		{"info", (*slog.Logger).InfoContext},
		{"warn", (*slog.Logger).WarnContext},
		{"error", (*slog.Logger).ErrorContext},
	}

	for _, lvl := range levels {
		t.Run(lvl.name, func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				lvl.logFn(logger, ctx, "test", "Authorization", "Bearer level-secret")
			})

			assertNotContains(t, output, "level-secret")
			assertContains(t, output, RedactedValue)
		})
	}
}

// --- Edge Cases ---

func TestRedactingHandler_EmptyRecord_NoError(t *testing.T) {
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		logger.InfoContext(ctx, "empty event")
	})

	assertContains(t, output, "empty event")
}

func TestRedactingHandler_NilContext_NoError(t *testing.T) {
	// Using Background context (no secrets)
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		logger.Info("no context event", "Authorization", "Bearer nil-ctx-secret")
	})

	assertNotContains(t, output, "nil-ctx-secret")
	assertContains(t, output, RedactedValue)
}

func TestRedactingHandler_NonStringAttr_PassesThrough(t *testing.T) {
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		logger.InfoContext(ctx, "numbers", "count", 42, "rate", 3.14, "flag", true)
	})

	assertContains(t, output, "42")
	assertContains(t, output, "3.14")
}

// --- Output Capture Negative Tests (Byte-Level Security Guarantees) ---

func TestRedactingHandler_ByteLevel_SecretNeverInOutput(t *testing.T) {
	// This is the strongest test: the raw bytes of the secret must
	// NOT appear anywhere in the serialized JSON output, regardless
	// of how they were logged.
	secrets := []string{
		"Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature",
		"api-key-sk_live_1234567890abcdef",
		"session-cookie-value-very-private",
	}

	for _, secret := range secrets {
		t.Run("direct_attr", func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), []string{secret}, func(logger *slog.Logger, ctx context.Context) {
				logger.InfoContext(ctx, "event", "token", secret)
			})
			assertNotContains(t, output, secret)
		})

		t.Run("in_message", func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), []string{secret}, func(logger *slog.Logger, ctx context.Context) {
				logger.InfoContext(ctx, "failed: "+secret)
			})
			assertNotContains(t, output, secret)
		})

		t.Run("via_header_key", func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				logger.InfoContext(ctx, "event", "Authorization", secret)
			})
			assertNotContains(t, output, secret)
		})

		t.Run("via_http_header", func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				h := http.Header{"Authorization": []string{secret}}
				logger.InfoContext(ctx, "event", "headers", h)
			})
			assertNotContains(t, output, secret)
		})
	}
}

// --- Context Helper Tests ---

func TestWithSecretValue_StoresAndRetrieves(t *testing.T) {
	ctx := context.Background()
	ctx = WithSecretValue(ctx, "secret-1")
	ctx = WithSecretValue(ctx, "secret-2")

	values := SecretValues(ctx)
	if len(values) != 2 {
		t.Fatalf("expected 2 secret values, got %d", len(values))
	}
	if values[0] != "secret-1" || values[1] != "secret-2" {
		t.Errorf("unexpected values: %v", values)
	}
}

func TestSecretValues_EmptyContext_ReturnsNil(t *testing.T) {
	values := SecretValues(context.Background())
	if values != nil {
		t.Errorf("expected nil for empty context, got %v", values)
	}
}

// --- NewLogger Factory Tests ---

func TestNewLogger_ReturnsWorkingLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo, defaultSensitive(), false)

	logger.Info("test", "Authorization", "Bearer factory-secret")

	assertNotContains(t, buf.Bytes(), "factory-secret")
	assertContains(t, buf.Bytes(), RedactedValue)
}

func TestNewLogger_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelWarn, defaultSensitive(), false)

	logger.Info("should not appear")
	logger.Warn("should appear")

	if bytes.Contains(buf.Bytes(), []byte("should not appear")) {
		t.Error("info message should have been filtered by level")
	}
	assertContains(t, buf.Bytes(), "should appear")
}

// --- Combined Layers Test ---

func TestRedactingHandler_CombinedLayers_AllCatch(t *testing.T) {
	// Test that all four layers work together:
	// 1. Key-based: "Authorization" key
	// 2. Type-based: http.Header value
	// 3. Value-based: known secret in arbitrary key
	// 4. Message: known secret in message text

	keySecret := "Bearer key-based-secret"
	headerSecret := "Bearer header-type-secret"
	valueSecret := "Bearer value-based-secret-long-enough"

	output := captureLog(t, defaultSensitive(), []string{valueSecret}, func(logger *slog.Logger, ctx context.Context) {
		// Layer 1: key-based
		logger.InfoContext(ctx, "event1", "Authorization", keySecret)

		// Layer 2: type-based
		h := http.Header{"Authorization": []string{headerSecret}}
		logger.InfoContext(ctx, "event2", "headers", h)

		// Layer 3: value-based (arbitrary key)
		logger.InfoContext(ctx, "event3", "debug_field", valueSecret)

		// Layer 4: message
		logger.InfoContext(ctx, "event4: "+valueSecret)
	})

	assertNotContains(t, output, keySecret)
	assertNotContains(t, output, headerSecret)
	assertNotContains(t, output, valueSecret)
}

// --- Body Logging Gate Tests ---

func TestRedactingHandler_BodyLogging_DisabledByDefault(t *testing.T) {
	// When body logging is disabled (default), body-related keys are redacted
	tests := []struct {
		name string
		key  string
	}{
		{"original_body", "original_body"},
		{"request_body", "request_body"},
		{"response_body", "response_body"},
		{"body", "body"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "this is sensitive body content with PII"
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				logger.DebugContext(ctx, "error captured", tt.key, body)
			})

			assertNotContains(t, output, body)
			assertContains(t, output, RedactedValue)
			// The key itself should still appear (only the value is redacted)
			assertContains(t, output, tt.key)
		})
	}
}

func TestRedactingHandler_BodyLogging_EnabledPassesThrough(t *testing.T) {
	// When body logging is enabled, body content passes through
	body := "upstream error: invalid subscription ID XYZ-123"

	output := captureLogWithBodyLogging(t, defaultSensitive(), nil, true, func(logger *slog.Logger, ctx context.Context) {
		logger.DebugContext(ctx, "error captured", "original_body", body)
	})

	assertContains(t, output, body)
}

func TestRedactingHandler_BodyLogging_CaseInsensitiveKeys(t *testing.T) {
	body := "secret body data"

	cases := []string{"original_body", "Original_Body", "ORIGINAL_BODY", "Body", "BODY"}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
				logger.DebugContext(ctx, "test", key, body)
			})

			assertNotContains(t, output, body)
		})
	}
}

func TestRedactingHandler_BodyLogging_NonBodyKeysUnaffected(t *testing.T) {
	// Keys that contain "body" as substring but aren't body keys should pass through
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		logger.DebugContext(ctx, "test",
			"somebody", "this should pass through",
			"body_count", 42,
			"antibody", "safe value",
		)
	})

	assertContains(t, output, "this should pass through")
	assertContains(t, output, "safe value")
}

func TestRedactingHandler_BodyLogging_DisabledStillAllowsHeaderRedaction(t *testing.T) {
	// Body logging disabled should not interfere with other redaction layers
	output := captureLog(t, defaultSensitive(), nil, func(logger *slog.Logger, ctx context.Context) {
		logger.DebugContext(ctx, "test",
			"Authorization", "Bearer secret-token",
			"original_body", "body content here",
			"Content-Type", "application/json",
		)
	})

	assertNotContains(t, output, "secret-token")
	assertNotContains(t, output, "body content here")
	assertContains(t, output, "application/json")
}

func TestNewLogger_BodyLoggingFlag(t *testing.T) {
	// Verify NewLogger correctly passes the body logging flag
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelDebug, defaultSensitive(), false)

	logger.Debug("test", "original_body", "secret body data")

	assertNotContains(t, buf.Bytes(), "secret body data")
	assertContains(t, buf.Bytes(), RedactedValue)
}

func TestNewLogger_BodyLoggingEnabled(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelDebug, defaultSensitive(), true)

	logger.Debug("test", "original_body", "visible body data")

	assertContains(t, buf.Bytes(), "visible body data")
}

// --- Fuzz Tests ---

func FuzzRedactingHandler_KeyMatching(f *testing.F) {
	// Seed with known sensitive header names and variations
	f.Add("Authorization", "Bearer token123")
	f.Add("authorization", "secret")
	f.Add("AUTHORIZATION", "value")
	f.Add("Cookie", "session=abc")
	f.Add("X-API-Key", "key-value")
	f.Add("Content-Type", "application/json")
	f.Add("X-Custom-Header", "safe-value")
	f.Add("", "empty-key")
	f.Add("Authorization\x00", "null-byte-key")

	sensitive := defaultSensitive()
	sensitiveSet := make(map[string]struct{}, len(sensitive))
	for _, h := range sensitive {
		sensitiveSet[strings.ToLower(h)] = struct{}{}
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		if key == "" || value == "" {
			return // skip degenerate cases
		}

		output := captureLog(t, sensitive, nil, func(logger *slog.Logger, ctx context.Context) {
			logger.InfoContext(ctx, "fuzz", key, value)
		})

		_, isSensitive := sensitiveSet[strings.ToLower(key)]
		if isSensitive {
			// Sensitive key: value MUST NOT appear in output
			assertNotContains(t, output, value)
		}
		// Non-sensitive key: we don't assert presence because JSON encoding
		// may transform the value (escaping, etc.)
	})
}

func FuzzRedactingHandler_ValueScanning(f *testing.F) {
	f.Add("Bearer eyJhbGciOiJIUzI1NiJ9", "debug_data", "contains Bearer eyJhbGciOiJIUzI1NiJ9 here")
	f.Add("api-key-1234567890", "field", "api-key-1234567890")
	f.Add("short", "field", "this contains short value")

	sensitive := defaultSensitive()

	f.Fuzz(func(t *testing.T, secret, key, value string) {
		if key == "" || value == "" || secret == "" {
			return
		}

		var secrets []string
		if len(secret) >= MinSecretLength {
			secrets = []string{secret}
		}

		output := captureLog(t, sensitive, secrets, func(logger *slog.Logger, ctx context.Context) {
			logger.InfoContext(ctx, "fuzz", key, value)
		})

		// If the secret is long enough and appears in the value, it must be redacted
		if len(secret) >= MinSecretLength && strings.Contains(value, secret) {
			assertNotContains(t, output, secret)
		}
	})
}
