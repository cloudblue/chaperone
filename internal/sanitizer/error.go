// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package sanitizer provides response sanitization functionality to prevent
// leakage of sensitive information from upstream error responses.
package sanitizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// ErrorResponse is the sanitized JSON response format for error responses.
// Per Design Spec Section 5.3 (Error Masking).
type ErrorResponse struct {
	Error   string `json:"error"`
	ErrorID string `json:"error_id"`
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
//   - X-Error-ID header is added for correlation
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
		ErrorID: traceID,
		Status:  resp.StatusCode,
	}

	sanitizedBody, err := json.Marshal(sanitized)
	if err != nil {
		return fmt.Errorf("marshaling sanitized response: %w", err)
	}

	// Replace response body
	resp.Body = io.NopCloser(bytes.NewReader(sanitizedBody))
	resp.ContentLength = int64(len(sanitizedBody))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("X-Error-ID", traceID)

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

// captureAndReplaceBody reads the original body and replaces it for re-reading.
func captureAndReplaceBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	// Replace body with copy for any subsequent reads
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// truncateForLog truncates a string to a reasonable length for logging.
// Prevents log bloat from large error bodies while preserving useful context.
const maxLogBodyLength = 1024

func truncateForLog(s string) string {
	if len(s) <= maxLogBodyLength {
		return s
	}
	return s[:maxLogBodyLength] + "... [truncated]"
}
