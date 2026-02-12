// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusCapturingResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	wrapped.WriteHeader(http.StatusCreated)

	if wrapped.Status != http.StatusCreated {
		t.Errorf("expected status 201, got %d", wrapped.Status)
	}
	if !wrapped.WroteHeader {
		t.Error("expected WroteHeader to be true")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected underlying status 201, got %d", rec.Code)
	}
}

func TestStatusCapturingResponseWriter_WriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	wrapped.WriteHeader(http.StatusCreated)
	wrapped.WriteHeader(http.StatusOK) // Should be ignored

	if wrapped.Status != http.StatusCreated {
		t.Errorf("expected status 201, got %d", wrapped.Status)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected underlying status 201, got %d", rec.Code)
	}
}

func TestStatusCapturingResponseWriter_Write_ImplicitHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	n, err := wrapped.Write([]byte("hello"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
	if wrapped.Status != http.StatusOK {
		t.Errorf("expected implicit status 200, got %d", wrapped.Status)
	}
	if !wrapped.WroteHeader {
		t.Error("expected WroteHeader to be true after Write")
	}
}

func TestStatusCapturingResponseWriter_Flush(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	// Should not panic
	wrapped.Flush()
}

func TestStatusCapturingResponseWriter_Hijack_NotSupported(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	_, _, err := wrapped.Hijack()
	if err == nil {
		t.Error("expected error when underlying writer doesn't support Hijack")
	}
	if !errors.Is(err, ErrHijackNotSupported) {
		t.Errorf("expected ErrHijackNotSupported, got %v", err)
	}
}

// mockHijacker implements http.Hijacker for testing.
type mockHijacker struct {
	http.ResponseWriter
	hijackCalled bool
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijackCalled = true
	return nil, nil, nil
}

func TestStatusCapturingResponseWriter_Hijack_Supported(t *testing.T) {
	mock := &mockHijacker{ResponseWriter: httptest.NewRecorder()}
	wrapped := NewStatusCapturingResponseWriter(mock)

	_, _, err := wrapped.Hijack()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !mock.hijackCalled {
		t.Error("expected Hijack to be called on underlying writer")
	}
}

// mockPusher implements http.Pusher for testing.
type mockPusher struct {
	http.ResponseWriter
	pushCalled bool
	pushTarget string
}

func (m *mockPusher) Push(target string, opts *http.PushOptions) error {
	m.pushCalled = true
	m.pushTarget = target
	return nil
}

func TestStatusCapturingResponseWriter_Push_Supported(t *testing.T) {
	mock := &mockPusher{ResponseWriter: httptest.NewRecorder()}
	wrapped := NewStatusCapturingResponseWriter(mock)

	err := wrapped.Push("/resource", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !mock.pushCalled {
		t.Error("expected Push to be called on underlying writer")
	}
	if mock.pushTarget != "/resource" {
		t.Errorf("expected target '/resource', got %s", mock.pushTarget)
	}
}

func TestStatusCapturingResponseWriter_Push_NotSupported(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	err := wrapped.Push("/resource", nil)
	if !errors.Is(err, http.ErrNotSupported) {
		t.Errorf("expected http.ErrNotSupported, got %v", err)
	}
}

func TestStatusCapturingResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	unwrapped := wrapped.Unwrap()
	if unwrapped != rec {
		t.Error("expected Unwrap to return underlying writer")
	}
}

func TestStatusCapturingResponseWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := NewStatusCapturingResponseWriter(rec)

	if wrapped.Status != http.StatusOK {
		t.Errorf("expected default status 200, got %d", wrapped.Status)
	}
	if wrapped.WroteHeader {
		t.Error("expected WroteHeader to be false initially")
	}
}
