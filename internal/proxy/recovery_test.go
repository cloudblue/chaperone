// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudblue/chaperone/internal/proxy"
)

func TestPanicRecovery_CatchesPanic_ReturnsJSON500(t *testing.T) {
	t.Parallel()

	// Arrange
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := proxy.WithPanicRecovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should return 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Assert - should be JSON content type
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	// Assert - should be valid JSON with error field
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	if body["error"] != "Internal Server Error" {
		t.Errorf("error = %q, want %q", body["error"], "Internal Server Error")
	}

	statusVal, ok := body["status"].(float64) // JSON numbers are float64
	if !ok || int(statusVal) != 500 {
		t.Errorf("status = %v, want 500", body["status"])
	}
}

func TestPanicRecovery_LogsStackTrace(t *testing.T) {
	t.Parallel()

	// Arrange - capture log output
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("stack trace test")
	})

	handler := proxy.WithPanicRecovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - log should contain panic info with stack trace
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "panic recovered") {
		t.Errorf("log should contain 'panic recovered', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "stack trace test") {
		t.Errorf("log should contain panic value, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "stack") {
		t.Errorf("log should contain stack trace, got: %s", logOutput)
	}
}

func TestPanicRecovery_ServerContinuesAfterPanic(t *testing.T) {
	t.Parallel()

	// Arrange - handler that panics on first call, succeeds on second
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			panic("first call panic")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	recovered := proxy.WithPanicRecovery(handler)

	// Act - first request panics
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	recovered.ServeHTTP(rec1, req1)

	// Assert - first request returns 500
	if rec1.Code != http.StatusInternalServerError {
		t.Errorf("first request status = %d, want %d", rec1.Code, http.StatusInternalServerError)
	}

	// Act - second request should succeed (server still running)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	recovered.ServeHTTP(rec2, req2)

	// Assert - second request succeeds
	if rec2.Code != http.StatusOK {
		t.Errorf("second request status = %d, want %d", rec2.Code, http.StatusOK)
	}
}

func TestPanicRecovery_ConcurrentPanics(t *testing.T) {
	t.Parallel()

	// Arrange
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("concurrent panic")
	})

	handler := proxy.WithPanicRecovery(panicHandler)

	// Act - send many concurrent requests that all panic
	const numRequests = 50
	var wg sync.WaitGroup
	results := make([]int, numRequests)

	for i := range numRequests {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			results[idx] = rec.Code
		}(i)
	}
	wg.Wait()

	// Assert - all should return 500, none should crash the server
	for i, code := range results {
		if code != http.StatusInternalServerError {
			t.Errorf("request %d: status = %d, want %d", i, code, http.StatusInternalServerError)
		}
	}
}

func TestPanicRecovery_NoPanic_NormalResponse(t *testing.T) {
	t.Parallel()

	// Arrange
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})

	handler := proxy.WithPanicRecovery(normalHandler)

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - normal response passes through
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if rec.Body.String() != "created" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "created")
	}
}

func TestPanicRecovery_ErrorTypePanic(t *testing.T) {
	t.Parallel()

	// Arrange - panic with an error type instead of string
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})

	handler := proxy.WithPanicRecovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rec, req)

	// Assert - should still return 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
