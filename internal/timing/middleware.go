// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package timing

import (
	"net/http"
)

// serverTimingHeader is the standard header name per W3C spec.
const serverTimingHeader = "Server-Timing"

// timingResponseWriter wraps http.ResponseWriter to inject the
// Server-Timing header before the first write.
type timingResponseWriter struct {
	http.ResponseWriter
	recorder      *Recorder
	headerWritten bool
}

// WriteHeader injects the Server-Timing header and writes the status code.
// Guarded against duplicate calls to avoid "superfluous WriteHeader" warnings.
func (w *timingResponseWriter) WriteHeader(code int) {
	if w.headerWritten {
		return
	}
	w.headerWritten = true
	if w.recorder != nil {
		w.ResponseWriter.Header().Set(serverTimingHeader, w.recorder.Header())
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write ensures WriteHeader is called (with 200) before writing body.
func (w *timingResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for streaming support.
func (w *timingResponseWriter) Flush() {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter.
// Required for http.ResponseController compatibility.
func (w *timingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// WithTiming wraps a handler to:
// 1. Create a timing Recorder and store it in the request context
// 2. Wrap the ResponseWriter to inject Server-Timing header on response
func WithTiming(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := New()
		ctx := WithRecorder(r.Context(), recorder)
		r = r.WithContext(ctx)

		tw := &timingResponseWriter{
			ResponseWriter: w,
			recorder:       recorder,
		}

		next.ServeHTTP(tw, r)
	})
}
