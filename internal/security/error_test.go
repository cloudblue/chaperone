// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
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

// TestNormalizeError_ContentLengthHeader_MatchesBody verifies that
// NormalizeError updates the Content-Length HEADER (not just the struct field).
// httputil.ReverseProxy copies resp.Header to the client via copyHeader —
// if the header is stale, the client receives a wrong Content-Length which
// causes the ReverseProxy to panic with ErrAbortHandler.
//
// Regression test for: Content-Length header mismatch causing
// "net/http: abort Handler" panic in production.
func TestNormalizeError_ContentLengthHeader_MatchesBody(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"401 small body", 401, `{"error":"unauthorized"}`},
		{"500 large body", 500, strings.Repeat("stack trace line\n", 100)},
		{"403 empty body", 403, ""},
		{"502 exact match risk", 502, strings.Repeat("x", 80)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: create response WITH a Content-Length header,
			// as real http.Transport responses always have.
			resp := createMockResponseWithContentLengthHeader(tt.status, tt.body)
			defer resp.Body.Close()

			originalHeaderLen := resp.Header.Get("Content-Length")

			// Act
			err := NormalizeError(resp, "trace-regression")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			body, _ := io.ReadAll(resp.Body)
			headerCL := resp.Header.Get("Content-Length")

			// The Content-Length HEADER must match the actual body length.
			if headerCL == "" {
				t.Fatal("Content-Length header missing after NormalizeError")
			}

			headerLen, err := strconv.Atoi(headerCL)
			if err != nil {
				t.Fatalf("Content-Length header is not a valid integer: %q", headerCL)
			}

			if headerLen != len(body) {
				t.Errorf("Content-Length header = %d, actual body length = %d (original was %s)",
					headerLen, len(body), originalHeaderLen)
			}

			// Struct field must also match.
			if resp.ContentLength != int64(len(body)) {
				t.Errorf("resp.ContentLength = %d, actual body length = %d",
					resp.ContentLength, len(body))
			}
		})
	}
}

// TestNormalizeError_ReverseProxy_NoPanic is an integration test that
// reproduces the exact production failure: httputil.ReverseProxy panics
// with ErrAbortHandler when ModifyResponse (via NormalizeError) changes
// the body without updating the Content-Length header.
func TestNormalizeError_ReverseProxy_NoPanic(t *testing.T) {
	// Upstream server returns a 401 with a known body.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		// Body intentionally different size from what NormalizeError will produce.
		w.Write([]byte(`{"error":"invalid credentials","details":"token expired at 2026-02-23T17:00:00Z"}`))
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ModifyResponse = func(resp *http.Response) error {
		return NormalizeError(resp, "trace-proxy-test")
	}

	// Proxy server that forwards to upstream.
	proxyServer := httptest.NewServer(proxy)
	defer proxyServer.Close()

	// Act: make a request through the proxy.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, proxyServer.URL+"/api/v1/customers", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request through proxy failed: %v", err)
	}
	defer resp.Body.Close()

	// Assert: we should get a valid response (not a connection reset).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Verify Content-Length header matches actual body.
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		clInt, _ := strconv.Atoi(cl)
		if clInt != len(body) {
			t.Errorf("Content-Length header = %d, body length = %d", clInt, len(body))
		}
	}

	// Body should be sanitized.
	if !strings.Contains(string(body), "Request rejected by upstream service") {
		t.Errorf("expected sanitized body, got: %s", string(body))
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

	// Verify JSON format: {"error": "...", "trace_id": "...", "status": N}
	if !strings.Contains(bodyStr, `"error"`) {
		t.Error("JSON response should contain 'error' field")
	}
	if !strings.Contains(bodyStr, `"trace_id"`) {
		t.Error("JSON response should contain 'trace_id' field")
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
	if testing.Short() {
		t.Skip("skipping large body test in short mode")
	}

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

func TestTruncateForLog_RuneSafe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ASCII under limit",
			input: "short string",
			want:  "short string",
		},
		{
			name:  "ASCII at limit",
			input: strings.Repeat("a", maxLogBodyLength),
			want:  strings.Repeat("a", maxLogBodyLength),
		},
		{
			name:  "ASCII over limit",
			input: strings.Repeat("a", maxLogBodyLength+100),
			want:  strings.Repeat("a", maxLogBodyLength) + "... [truncated]",
		},
		{
			name: "multi-byte runes not split",
			// 'é' is 2 bytes, '日' is 3 bytes, '🎉' is 4 bytes
			// Fill with 3-byte runes so a naive byte slice would split mid-rune
			input: strings.Repeat("日", maxLogBodyLength),
			want:  strings.Repeat("日", maxLogBodyLength/3) + "... [truncated]",
		},
		{
			name:  "4-byte emoji not split",
			input: strings.Repeat("🎉", maxLogBodyLength),
			want:  strings.Repeat("🎉", maxLogBodyLength/4) + "... [truncated]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateForLog(tt.input)
			if got != tt.want {
				t.Errorf("truncateForLog() = %d bytes, want %d bytes", len(got), len(tt.want))
			}
		})
	}
}

func TestTruncateForLog_ValidUTF8(t *testing.T) {
	// A string of multi-byte runes that would be split by naive byte truncation
	input := strings.Repeat("日本語", 500) // 4500 runes, 13500 bytes
	result := truncateForLog(input)

	// Result minus the suffix must be valid UTF-8 (no broken runes)
	trimmed := strings.TrimSuffix(result, "... [truncated]")
	for i, r := range trimmed {
		if r == '\uFFFD' {
			t.Errorf("invalid UTF-8 replacement character at byte %d", i)
		}
	}
	// Result byte length (excluding suffix) must not exceed maxLogBodyLength
	if len(trimmed) > maxLogBodyLength {
		t.Errorf("truncated body is %d bytes, exceeds limit %d", len(trimmed), maxLogBodyLength)
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

func TestNormalizeError_OversizedBody_LimitedToMaxRead(t *testing.T) {
	// Arrange: body larger than maxReadBodySize (1 MB)
	oversizedBody := strings.Repeat("A", 2*1024*1024) // 2 MB
	resp := createMockResponse(500, oversizedBody)
	defer resp.Body.Close()

	// Act
	err := NormalizeError(resp, "trace-oversized")

	// Assert - should succeed without OOM
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	// Body should be replaced with a small sanitized JSON, not the 2 MB original
	if len(body) > 1024 {
		t.Errorf("sanitized body should be small JSON, got %d bytes", len(body))
	}
	if !strings.Contains(string(body), "Upstream service error") {
		t.Errorf("body should contain generic error message, got: %s", string(body))
	}
}

func TestCaptureAndReplaceBody_ExactlyAtLimit_ReadsAll(t *testing.T) {
	// Arrange: body exactly at the 1 MB limit
	exactBody := strings.Repeat("B", 1024*1024) // exactly 1 MB
	resp := createMockResponse(500, exactBody)
	defer resp.Body.Close()

	// Act
	captured, err := captureAndReplaceBody(resp)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured) != 1024*1024 {
		t.Errorf("captured body length = %d, want %d", len(captured), 1024*1024)
	}
}

func TestCaptureAndReplaceBody_OverLimit_Truncated(t *testing.T) {
	// Arrange: body larger than 1 MB limit
	oversizedBody := strings.Repeat("C", 2*1024*1024) // 2 MB
	resp := createMockResponse(500, oversizedBody)
	defer resp.Body.Close()

	// Act
	captured, err := captureAndReplaceBody(resp)

	// Assert - should read at most maxReadBodySize bytes
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured) != 1024*1024 {
		t.Errorf("captured body length = %d, want %d (maxReadBodySize)", len(captured), 1024*1024)
	}
}

func TestCaptureAndReplaceBody_NilBody_ReturnsNil(t *testing.T) {
	// Arrange
	resp := &http.Response{
		StatusCode: 500,
		Header:     make(http.Header),
		Body:       nil,
	}

	// Act
	captured, err := captureAndReplaceBody(resp)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured != nil {
		t.Errorf("captured should be nil for nil body, got %d bytes", len(captured))
	}
}

func TestCaptureAndReplaceBody_SmallBody_ReadsAll(t *testing.T) {
	// Arrange: small body well under limit
	smallBody := "small error message"
	resp := createMockResponse(500, smallBody)
	defer resp.Body.Close()

	// Act
	captured, err := captureAndReplaceBody(resp)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(captured) != smallBody {
		t.Errorf("captured = %q, want %q", string(captured), smallBody)
	}

	// Verify body is still readable after capture
	replaced, _ := io.ReadAll(resp.Body)
	if string(replaced) != smallBody {
		t.Errorf("replaced body = %q, want %q", string(replaced), smallBody)
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

// createMockResponseWithContentLengthHeader creates a mock HTTP response
// with the Content-Length header set, matching what http.Transport returns
// for real upstream responses. This is critical for testing because
// httputil.ReverseProxy uses resp.Header (not resp.ContentLength) when
// writing to the client.
func createMockResponseWithContentLengthHeader(statusCode int, body string) *http.Response {
	h := make(http.Header)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("Content-Type", "text/plain")
	return &http.Response{
		StatusCode:    statusCode,
		Status:        http.StatusText(statusCode),
		Header:        h,
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
	}
}
