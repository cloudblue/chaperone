// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package timing

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWithTiming_AddsServerTimingHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	wrapped := WithTiming(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Server-Timing header should be present
	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present")
	}

	// Should contain all three components
	if !strings.Contains(header, "plugin;dur=") {
		t.Errorf("Header = %q, want to contain 'plugin;dur='", header)
	}
	if !strings.Contains(header, "upstream;dur=") {
		t.Errorf("Header = %q, want to contain 'upstream;dur='", header)
	}
	if !strings.Contains(header, "overhead;dur=") {
		t.Errorf("Header = %q, want to contain 'overhead;dur='", header)
	}
}

func TestWithTiming_RecorderInContext(t *testing.T) {
	var retrievedRecorder *Recorder

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retrievedRecorder = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrapped := WithTiming(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if retrievedRecorder == nil {
		t.Fatal("Recorder should be available in context")
	}
}

func TestWithTiming_RecordedDurationsReflectedInHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := FromContext(r.Context())
		if recorder == nil {
			t.Fatal("no recorder in context")
		}
		recorder.RecordPlugin(100 * time.Millisecond)
		recorder.RecordUpstream(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	wrapped := WithTiming(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	header := rec.Header().Get("Server-Timing")
	if !strings.Contains(header, "plugin;dur=100.00") {
		t.Errorf("Header = %q, want to contain 'plugin;dur=100.00'", header)
	}
	if !strings.Contains(header, "upstream;dur=200.00") {
		t.Errorf("Header = %q, want to contain 'upstream;dur=200.00'", header)
	}
}

func TestWithTiming_HeaderAddedOnError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := FromContext(r.Context())
		recorder.RecordPlugin(50 * time.Millisecond)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
	})

	wrapped := WithTiming(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present on error responses")
	}
	if !strings.Contains(header, "plugin;dur=50.00") {
		t.Errorf("Header = %q, want to contain 'plugin;dur=50.00'", header)
	}
}

func TestWithTiming_SupportsStreaming(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("chunk1"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.Write([]byte("chunk2"))
	})

	wrapped := WithTiming(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Header should still be present with streaming
	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present with streaming")
	}

	// Body should contain both chunks
	body := rec.Body.String()
	if body != "chunk1chunk2" {
		t.Errorf("body = %q, want 'chunk1chunk2'", body)
	}
}

func TestWithTiming_WriteWithoutExplicitWriteHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write without calling WriteHeader - should default to 200
		w.Write([]byte("direct write"))
	})

	wrapped := WithTiming(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	header := rec.Header().Get("Server-Timing")
	if header == "" {
		t.Fatal("Server-Timing header should be present even without explicit WriteHeader")
	}
}

func TestTimingResponseWriter_Unwrap(t *testing.T) {
	underlying := httptest.NewRecorder()
	tw := &timingResponseWriter{
		ResponseWriter: underlying,
		recorder:       New(),
	}

	unwrapped := tw.Unwrap()

	if unwrapped != underlying {
		t.Error("Unwrap should return the underlying ResponseWriter")
	}
}
