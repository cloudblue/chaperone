// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNormalizeError_4xxError_ReturnsGenericJSON(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		originalBody   string
		traceID        string
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "400 Bad Request",
			statusCode:     400,
			originalBody:   `{"error": "invalid field 'username'", "field": "username"}`,
			traceID:        "trace-123",
			wantStatusCode: 400,
			wantError:      "Request rejected by upstream service",
		},
		{
			name:           "401 Unauthorized",
			statusCode:     401,
			originalBody:   "Authentication failed: invalid token",
			traceID:        "trace-456",
			wantStatusCode: 401,
			wantError:      "Request rejected by upstream service",
		},
		{
			name:           "403 Forbidden",
			statusCode:     403,
			originalBody:   "<html>Access Denied</html>",
			traceID:        "trace-789",
			wantStatusCode: 403,
			wantError:      "Request rejected by upstream service",
		},
		{
			name:           "404 Not Found",
			statusCode:     404,
			originalBody:   "Resource /api/users/123 not found",
			traceID:        "trace-abc",
			wantStatusCode: 404,
			wantError:      "Request rejected by upstream service",
		},
		{
			name:           "429 Too Many Requests",
			statusCode:     429,
			originalBody:   "Rate limit exceeded. Retry after 60 seconds.",
			traceID:        "trace-def",
			wantStatusCode: 429,
			wantError:      "Request rejected by upstream service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resp := createMockResponse(tt.statusCode, tt.originalBody)
			defer resp.Body.Close()

			// Act
			err := NormalizeError(resp, tt.traceID)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			if !strings.Contains(bodyStr, tt.wantError) {
				t.Errorf("body does not contain %q, got: %s", tt.wantError, bodyStr)
			}
			if !strings.Contains(bodyStr, tt.traceID) {
				t.Errorf("body does not contain trace_id %q, got: %s", tt.traceID, bodyStr)
			}
			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q, want %q", resp.Header.Get("Content-Type"), "application/json")
			}
			if resp.Header.Get("X-Error-ID") != tt.traceID {
				t.Errorf("X-Error-ID = %q, want %q", resp.Header.Get("X-Error-ID"), tt.traceID)
			}
		})
	}
}

func TestNormalizeError_5xxError_ReturnsGenericJSON(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		originalBody   string
		traceID        string
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "500 Internal Server Error",
			statusCode:     500,
			originalBody:   "panic: runtime error: index out of range\ngoroutine 1 [running]:\nmain.main()\n\t/app/main.go:42",
			traceID:        "trace-500",
			wantStatusCode: 500,
			wantError:      "Upstream service error",
		},
		{
			name:           "502 Bad Gateway",
			statusCode:     502,
			originalBody:   "nginx: upstream server error",
			traceID:        "trace-502",
			wantStatusCode: 502,
			wantError:      "Upstream service error",
		},
		{
			name:           "503 Service Unavailable",
			statusCode:     503,
			originalBody:   `{"error": "database connection failed", "host": "db-prod-1.internal"}`,
			traceID:        "trace-503",
			wantStatusCode: 503,
			wantError:      "Upstream service error",
		},
		{
			name:           "504 Gateway Timeout",
			statusCode:     504,
			originalBody:   "Request timed out after 30s waiting for backend",
			traceID:        "trace-504",
			wantStatusCode: 504,
			wantError:      "Upstream service error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resp := createMockResponse(tt.statusCode, tt.originalBody)
			defer resp.Body.Close()

			// Act
			err := NormalizeError(resp, tt.traceID)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			if !strings.Contains(bodyStr, tt.wantError) {
				t.Errorf("body does not contain %q, got: %s", tt.wantError, bodyStr)
			}
			if !strings.Contains(bodyStr, tt.traceID) {
				t.Errorf("body does not contain trace_id %q, got: %s", tt.traceID, bodyStr)
			}
		})
	}
}

func TestNormalizeError_SuccessResponse_Passthrough(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		originalBody string
	}{
		{
			name:         "200 OK",
			statusCode:   200,
			originalBody: `{"data": "success"}`,
		},
		{
			name:         "201 Created",
			statusCode:   201,
			originalBody: `{"id": "new-resource-123"}`,
		},
		{
			name:         "204 No Content",
			statusCode:   204,
			originalBody: "",
		},
		{
			name:         "301 Moved Permanently",
			statusCode:   301,
			originalBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resp := createMockResponse(tt.statusCode, tt.originalBody)
			defer resp.Body.Close()
			originalContentType := resp.Header.Get("Content-Type")

			// Act
			err := NormalizeError(resp, "trace-success")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tt.statusCode {
				t.Errorf("status code changed from %d to %d", tt.statusCode, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) != tt.originalBody {
				t.Errorf("body changed from %q to %q", tt.originalBody, string(body))
			}

			if resp.Header.Get("Content-Type") != originalContentType {
				t.Errorf("Content-Type changed from %q to %q", originalContentType, resp.Header.Get("Content-Type"))
			}

			if resp.Header.Get("X-Error-ID") != "" {
				t.Error("X-Error-ID header should not be set for success responses")
			}
		})
	}
}

func TestNormalizeError_StackTracesRemoved(t *testing.T) {
	stackTracePatterns := []string{
		"panic: runtime error",
		"goroutine 1 [running]:",
		".go:42",
		"at /app/main.go",
		"Exception in thread",
		"Traceback (most recent call last):",
		"java.lang.NullPointerException",
	}

	for _, pattern := range stackTracePatterns {
		t.Run("removes "+pattern, func(t *testing.T) {
			// Arrange
			originalBody := "Error occurred\n" + pattern + "\nsome more details"
			resp := createMockResponse(500, originalBody)
			defer resp.Body.Close()

			// Act
			err := NormalizeError(resp, "trace-stack")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			body, _ := io.ReadAll(resp.Body)
			if strings.Contains(string(body), pattern) {
				t.Errorf("response body should not contain stack trace pattern %q, got: %s", pattern, string(body))
			}
		})
	}
}

func TestNormalizeError_InternalDetailsRemoved(t *testing.T) {
	sensitivePatterns := []struct {
		name string
		body string
	}{
		{
			name: "internal IP address",
			body: `{"error": "connection refused", "host": "192.168.1.100:5432"}`,
		},
		{
			name: "database connection string",
			body: "Error connecting to postgres://user:password@db.internal:5432/production",
		},
		{
			name: "file path",
			body: `Error in /home/app/src/handlers/user.go at line 156`,
		},
		{
			name: "internal hostname",
			body: `Failed to connect to api-prod-west-2.internal.company.com`,
		},
	}

	for _, tt := range sensitivePatterns {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resp := createMockResponse(500, tt.body)
			defer resp.Body.Close()

			// Act
			err := NormalizeError(resp, "trace-internal")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) == tt.body {
				t.Errorf("response body should be sanitized, but was unchanged: %s", tt.body)
			}
		})
	}
}

func TestNormalizeError_ContentLengthUpdated(t *testing.T) {
	// Arrange
	originalBody := strings.Repeat("x", 1000)
	resp := createMockResponse(500, originalBody)
	defer resp.Body.Close()

	// Act
	err := NormalizeError(resp, "trace-length")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if resp.ContentLength != int64(len(body)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(body))
	}
}

func TestNormalizeError_JSONResponseFormat(t *testing.T) {
	// Arrange
	resp := createMockResponse(500, "some error")
	defer resp.Body.Close()

	// Act
	err := NormalizeError(resp, "trace-format")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Verify JSON format: {"error": "...", "error_id": "...", "status": N}
	if !strings.Contains(bodyStr, `"error"`) {
		t.Error("JSON response should contain 'error' field")
	}
	if !strings.Contains(bodyStr, `"error_id"`) {
		t.Error("JSON response should contain 'error_id' field")
	}
	if !strings.Contains(bodyStr, `"status"`) {
		t.Error("JSON response should contain 'status' field")
	}
	if !strings.Contains(bodyStr, `"status":500`) {
		t.Errorf("JSON response should contain 'status':500, got: %s", bodyStr)
	}
}

func TestNormalizeError_EmptyBody(t *testing.T) {
	// Arrange
	resp := createMockResponse(500, "")
	defer resp.Body.Close()

	// Act
	err := NormalizeError(resp, "trace-empty")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("response body should not be empty after normalization")
	}
}

func TestNormalizeError_LargeBody(t *testing.T) {
	// Arrange: 1MB body with stack trace
	largeBody := strings.Repeat("stack trace line\n", 65536)
	resp := createMockResponse(500, largeBody)
	defer resp.Body.Close()

	// Act
	err := NormalizeError(resp, "trace-large")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) >= len(largeBody) {
		t.Errorf("response body should be smaller than original, got %d bytes (original: %d)", len(body), len(largeBody))
	}
}

func TestIsErrorResponse_DetectsErrors(t *testing.T) {
	tests := []struct {
		statusCode int
		want       bool
	}{
		{200, false},
		{201, false},
		{204, false},
		{301, false},
		{399, false},
		{400, true},
		{401, true},
		{404, true},
		{499, true},
		{500, true},
		{502, true},
		{503, true},
		{599, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			resp := createMockResponse(tt.statusCode, "body")
			defer resp.Body.Close()
			got := isErrorResponse(resp)
			if got != tt.want {
				t.Errorf("isErrorResponse(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

// createMockResponse creates a mock HTTP response for testing.
func createMockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode:    statusCode,
		Status:        http.StatusText(statusCode),
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
	}
}
