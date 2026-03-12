// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// logEntry represents a parsed JSON log line for test assertions.
type logEntry struct {
	Time       string `json:"time"`
	Level      string `json:"level"`
	Msg        string `json:"msg"`
	TraceID    string `json:"trace_id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	LatencyMs  int64  `json:"latency_ms"`
	VendorID   string `json:"vendor_id"`
	ClientIP   string `json:"client_ip"`
	RemoteAddr string `json:"remote_addr"`
}

// parseLogEntry parses a single JSON log line from the buffer.
func parseLogEntry(t *testing.T, data []byte) logEntry {
	t.Helper()
	var entry logEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v\nraw: %s", err, data)
	}
	return entry
}

// --- ClientIP Tests ---

func TestClientIP_XForwardedFor_ReturnsFirst(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")

	got := ClientIP(r)

	if got != "203.0.113.1" {
		t.Errorf("got %q, want %q", got, "203.0.113.1")
	}
}

func TestClientIP_XRealIP_ReturnsValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Real-IP", "198.51.100.5")

	got := ClientIP(r)

	if got != "198.51.100.5" {
		t.Errorf("got %q, want %q", got, "198.51.100.5")
	}
}

func TestClientIP_NoProxyHeaders_ReturnsEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"

	got := ClientIP(r)

	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestClientIP_XForwardedFor_TrimsPrecedence(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", " 203.0.113.1 , 10.0.0.1")
	r.Header.Set("X-Real-IP", "198.51.100.5")

	// X-Forwarded-For takes precedence
	got := ClientIP(r)

	if got != "203.0.113.1" {
		t.Errorf("got %q, want %q", got, "203.0.113.1")
	}
}

// --- ResponseCapturer Tests ---

func TestResponseCapturer_CapturesStatusCode(t *testing.T) {
	w := httptest.NewRecorder()
	capturer := NewResponseCapturer(w)

	capturer.WriteHeader(http.StatusNotFound)

	if capturer.Status() != http.StatusNotFound {
		t.Errorf("got %d, want %d", capturer.Status(), http.StatusNotFound)
	}
}

func TestResponseCapturer_DefaultsTo200(t *testing.T) {
	w := httptest.NewRecorder()
	capturer := NewResponseCapturer(w)

	// Write body without explicit WriteHeader
	_, _ = capturer.Write([]byte("OK"))

	if capturer.Status() != http.StatusOK {
		t.Errorf("got %d, want %d", capturer.Status(), http.StatusOK)
	}
}

func TestResponseCapturer_ImplementsFlusher(t *testing.T) {
	w := httptest.NewRecorder()
	capturer := NewResponseCapturer(w)

	// Verify the Flush method exists and doesn't panic.
	// ResponseCapturer embeds http.ResponseWriter, so we check
	// via the concrete method rather than interface assertion.
	capturer.Flush() // should not panic
}

// --- RequestLoggerMiddleware Tests ---

func TestRequestLoggerMiddleware_LogsRequestFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", inner)
	r := httptest.NewRequest(http.MethodPost, "/proxy", nil)
	r = r.WithContext(WithTraceID(r.Context(), "test-trace-123"))
	r.Header.Set("X-Connect-Vendor-ID", "microsoft")
	r.RemoteAddr = "10.0.0.1:54321"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	entry := parseLogEntry(t, buf.Bytes())
	if entry.Msg != "request completed" {
		t.Errorf("msg = %q, want %q", entry.Msg, "request completed")
	}
	if entry.TraceID != "test-trace-123" {
		t.Errorf("trace_id = %q, want %q", entry.TraceID, "test-trace-123")
	}
	if entry.Method != "POST" {
		t.Errorf("method = %q, want %q", entry.Method, "POST")
	}
	if entry.Path != "/proxy" {
		t.Errorf("path = %q, want %q", entry.Path, "/proxy")
	}
	if entry.Status != http.StatusOK {
		t.Errorf("status = %d, want %d", entry.Status, http.StatusOK)
	}
	if entry.LatencyMs < 0 {
		t.Errorf("latency_ms = %d, want >= 0", entry.LatencyMs)
	}
	if entry.VendorID != "microsoft" {
		t.Errorf("vendor_id = %q, want %q", entry.VendorID, "microsoft")
	}
	if entry.ClientIP != "" {
		t.Errorf("client_ip = %q, want empty (no proxy headers)", entry.ClientIP)
	}
	if entry.RemoteAddr != "10.0.0.1:54321" {
		t.Errorf("remote_addr = %q, want %q", entry.RemoteAddr, "10.0.0.1:54321")
	}
}

func TestRequestLoggerMiddleware_CapturesErrorStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r = r.WithContext(WithTraceID(r.Context(), "err-trace"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	entry := parseLogEntry(t, buf.Bytes())
	if entry.Status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", entry.Status, http.StatusBadGateway)
	}
}

func TestRequestLoggerMiddleware_NoTraceID_LogsEmpty(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	// Deliberately no trace ID in context
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	entry := parseLogEntry(t, buf.Bytes())
	if entry.TraceID != "" {
		t.Errorf("trace_id = %q, want empty", entry.TraceID)
	}
}

func TestRequestLoggerMiddleware_LogsOnPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Wrap with panic recovery INSIDE request logger, so logger still fires
	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", panicRecoveryForTest(panicky))
	r := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	r = r.WithContext(WithTraceID(r.Context(), "panic-trace"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	if buf.Len() == 0 {
		t.Fatal("expected log output even after panic recovery")
	}
	entry := parseLogEntry(t, buf.Bytes())
	if entry.TraceID != "panic-trace" {
		t.Errorf("trace_id = %q, want %q", entry.TraceID, "panic-trace")
	}
}

func TestRequestLoggerMiddleware_ClientIPFromXForwardedFor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", inner)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	entry := parseLogEntry(t, buf.Bytes())
	if entry.ClientIP != "203.0.113.50" {
		t.Errorf("client_ip = %q, want %q", entry.ClientIP, "203.0.113.50")
	}
}

// TestRequestLoggerMiddleware_TraceIDFromOuterMiddleware verifies that when
// TraceIDMiddleware wraps RequestLoggerMiddleware (the real production order),
// the logged trace_id reflects the value set by TraceIDMiddleware via context.
func TestRequestLoggerMiddleware_TraceIDFromOuterMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Production order: TraceID (outermost) → Logger → handler
	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", inner)
	handler = TraceIDMiddleware("X-Trace-ID", handler)

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("X-Trace-ID", "from-upstream-abc")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	entry := parseLogEntry(t, buf.Bytes())
	if entry.TraceID != "from-upstream-abc" {
		t.Errorf("trace_id = %q, want %q (should come from TraceIDMiddleware)", entry.TraceID, "from-upstream-abc")
	}
}

// TestRequestLoggerMiddleware_TraceIDGenerated verifies that when no trace
// header is provided, the generated UUIDv4 appears in the log output.
func TestRequestLoggerMiddleware_TraceIDGenerated(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Production order: TraceID (outermost) → Logger → handler
	handler := RequestLoggerMiddleware(logger, "X-Connect-Vendor-ID", inner)
	handler = TraceIDMiddleware("X-Trace-ID", handler)

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No X-Trace-ID header — should generate UUIDv4
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	entry := parseLogEntry(t, buf.Bytes())
	if entry.TraceID == "" {
		t.Error("trace_id should not be empty when TraceIDMiddleware generates one")
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "full URL with path",
			input: "https://api.vendor.com/v1/users",
			want:  "api.vendor.com",
		},
		{
			name:  "URL with port",
			input: "https://api.vendor.com:8443/v1",
			want:  "api.vendor.com:8443",
		},
		{
			name:  "URL with query string",
			input: "https://api.vendor.com/v1?key=secret",
			want:  "api.vendor.com",
		},
		{
			name:  "URL without path",
			input: "https://api.vendor.com",
			want:  "api.vendor.com",
		},
		{
			name:  "empty URL returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "invalid URL returns empty",
			input: "://invalid",
			want:  "",
		},
		{
			name:  "path-only URL returns empty",
			input: "/just/a/path",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHost(tt.input)
			if got != tt.want {
				t.Errorf("extractHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// panicRecoveryForTest wraps a handler with basic panic recovery for testing.
func panicRecoveryForTest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
