// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"unicode/utf8"
)

// ErrorResponse is the sanitized JSON response format for error responses.
// Per Design Spec Section 5.3 (Error Masking).
type ErrorResponse struct {
	Error   string `json:"error"`
	TraceID string `json:"trace_id"`
	Status  int    `json:"status"`
}

// NormalizeError intercepts upstream 4xx/5xx errors and replaces the body
// with a sanitized JSON response. The original body is logged at DEBUG level
// for Distributor troubleshooting.
//
// Per Design Spec Section 5.3 (Error Masking):
//   - Upstream 400/500 errors are intercepted
//   - Body is replaced with generic error
//   - Original stack traces logged locally but never returned to client
//
// The response is modified in-place:
//   - Body is replaced with sanitized JSON
//   - Content-Type is set to application/json
//   - Content-Length is updated
func NormalizeError(resp *http.Response, traceID string) error {
	if !isErrorResponse(resp) {
		return nil
	}

	// Capture original body for logging before replacement
	originalBody, err := captureAndReplaceBody(resp)
	if err != nil {
		return fmt.Errorf("reading original body: %w", err)
	}

	// Log original body at DEBUG level for Distributor troubleshooting
	slog.Debug("original upstream error body",
		"trace_id", traceID,
		"status", resp.StatusCode,
		"original_body", truncateForLog(string(originalBody)),
	)

	// Create sanitized response
	errorMsg := getErrorMessage(resp.StatusCode)
	sanitized := ErrorResponse{
		Error:   errorMsg,
		TraceID: traceID,
		Status:  resp.StatusCode,
	}

	sanitizedBody, err := json.Marshal(sanitized)
	if err != nil {
		return fmt.Errorf("marshaling sanitized response: %w", err)
	}

	// Replace response body and update all length indicators.
	// Both resp.ContentLength (struct field) and the Content-Length header
	// must be updated: httputil.ReverseProxy copies resp.Header to the
	// client via copyHeader, so a stale Content-Length header causes a
	// size mismatch → write error → ErrAbortHandler panic.
	resp.Body = io.NopCloser(bytes.NewReader(sanitizedBody))
	resp.ContentLength = int64(len(sanitizedBody))
	resp.Header.Set("Content-Length", strconv.Itoa(len(sanitizedBody)))
	resp.Header.Set("Content-Type", "application/json")

	return nil
}

// isErrorResponse returns true if the response has a 4xx or 5xx status code.
func isErrorResponse(resp *http.Response) bool {
	return resp.StatusCode >= 400 && resp.StatusCode < 600
}

// getErrorMessage returns the appropriate generic error message based on status code.
func getErrorMessage(statusCode int) string {
	if statusCode >= 500 {
		return "Upstream service error"
	}
	return "Request rejected by upstream service"
}

// maxReadBodySize is the maximum number of bytes to read from an upstream
// error response body. Prevents OOM from malicious or misbehaving upstreams.
// Set to 1 MB — well above the log truncation limit (1 KB), but bounded.
const maxReadBodySize = 1 << 20 // 1 MB

// captureAndReplaceBody reads the original body (up to maxReadBodySize) and
// replaces it for re-reading.
func captureAndReplaceBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReadBodySize))
	if err != nil {
		return nil, err
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("closing original body: %w", err)
	}

	// Replace body with copy for any subsequent reads
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// truncateForLog truncates a string to a reasonable length for logging.
// Prevents log bloat from large error bodies while preserving useful context.
// Uses rune-aware truncation to avoid splitting multi-byte UTF-8 characters.
const maxLogBodyLength = 1024

func truncateForLog(s string) string {
	if len(s) <= maxLogBodyLength {
		return s
	}
	// Walk back from the cut point to find a valid rune boundary.
	i := maxLogBodyLength
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i] + "... [truncated]"
}
