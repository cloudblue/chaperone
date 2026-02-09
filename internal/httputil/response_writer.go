// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package httputil provides HTTP utilities shared across the codebase.
package httputil

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// ErrHijackNotSupported is returned when the underlying ResponseWriter
// does not implement http.Hijacker.
var ErrHijackNotSupported = errors.New("httputil: hijack not supported")

// StatusCapturingResponseWriter wraps http.ResponseWriter to capture the status code.
// It properly implements http.Flusher, http.Hijacker, http.Pusher, and Unwrap
// for streaming, WebSocket support, HTTP/2 server push, and Go 1.20+
// http.ResponseController compatibility.
type StatusCapturingResponseWriter struct {
	http.ResponseWriter
	Status      int
	WroteHeader bool
}

// NewStatusCapturingResponseWriter creates a new response writer wrapper.
func NewStatusCapturingResponseWriter(w http.ResponseWriter) *StatusCapturingResponseWriter {
	return &StatusCapturingResponseWriter{
		ResponseWriter: w,
		Status:         http.StatusOK,
	}
}

// WriteHeader captures the status code and delegates to the underlying writer.
// Per http.ResponseWriter contract, only the first call takes effect.
func (w *StatusCapturingResponseWriter) WriteHeader(code int) {
	if w.WroteHeader {
		return // Silently ignore duplicate WriteHeader calls
	}
	w.Status = code
	w.WroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

// Write ensures WriteHeader is called (with 200) if not already called,
// then delegates to the underlying writer.
func (w *StatusCapturingResponseWriter) Write(b []byte) (int, error) {
	if !w.WroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for streaming response support.
func (w *StatusCapturingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket and HTTP upgrade support.
func (w *StatusCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, ErrHijackNotSupported
}

// Push implements http.Pusher for HTTP/2 server push support.
func (w *StatusCapturingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// Unwrap returns the underlying ResponseWriter for Go 1.20+ http.ResponseController.
// This enables SetReadDeadline/SetWriteDeadline to work through the wrapper.
func (w *StatusCapturingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
