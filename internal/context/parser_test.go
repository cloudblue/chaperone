// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

func TestParseContext_ValidHeaders_ReturnsContext(t *testing.T) {
	tests := []struct {
		want    *sdk.TransactionContext
		headers map[string]string
		name    string
		prefix  string
	}{
		{
			name: "all standard headers with default prefix",
			headers: map[string]string{
				"X-Connect-Target-URL":      "https://api.vendor.com/v1/resource",
				"X-Connect-Environment-ID":  "production",
				"X-Connect-Marketplace-ID":  "US",
				"X-Connect-Vendor-ID":       "vendor-123",
				"X-Connect-Product-ID":      "product-456",
				"X-Connect-Subscription-ID": "sub-789",
				"X-Connect-Context-Data":    base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`)),
				"Connect-Request-ID":        "trace-abc-123",
			},
			prefix: "X-Connect",
			want: &sdk.TransactionContext{
				TargetURL:      "https://api.vendor.com/v1/resource",
				EnvironmentID:  "production",
				MarketplaceID:  "US",
				VendorID:       "vendor-123",
				ProductID:      "product-456",
				SubscriptionID: "sub-789",
				TraceID:        "trace-abc-123",
				Data:           map[string]any{"key": "value"},
			},
		},
		{
			name: "minimal required headers only Target-URL",
			headers: map[string]string{
				"X-Connect-Target-URL": "https://api.vendor.com/v1",
			},
			prefix: "X-Connect",
			want: &sdk.TransactionContext{
				TargetURL: "https://api.vendor.com/v1",
			},
		},
		{
			name: "custom prefix",
			headers: map[string]string{
				"X-Custom-Target-URL":     "https://api.example.com/path",
				"X-Custom-Vendor-ID":      "custom-vendor",
				"X-Custom-Marketplace-ID": "EU",
			},
			prefix: "X-Custom",
			want: &sdk.TransactionContext{
				TargetURL:     "https://api.example.com/path",
				VendorID:      "custom-vendor",
				MarketplaceID: "EU",
			},
		},
		{
			name: "empty Context-Data header results in nil Data map",
			headers: map[string]string{
				"X-Connect-Target-URL":   "https://api.vendor.com/v1",
				"X-Connect-Context-Data": "",
			},
			prefix: "X-Connect",
			want: &sdk.TransactionContext{
				TargetURL: "https://api.vendor.com/v1",
			},
		},
		{
			name: "empty JSON object in Context-Data",
			headers: map[string]string{
				"X-Connect-Target-URL":   "https://api.vendor.com/v1",
				"X-Connect-Context-Data": base64.StdEncoding.EncodeToString([]byte(`{}`)),
			},
			prefix: "X-Connect",
			want: &sdk.TransactionContext{
				TargetURL: "https://api.vendor.com/v1",
				Data:      map[string]any{},
			},
		},
		{
			name: "headers are case insensitive",
			headers: map[string]string{
				"x-connect-target-url": "https://api.vendor.com/v1",
				"X-CONNECT-VENDOR-ID":  "vendor-upper",
			},
			prefix: "X-Connect",
			want: &sdk.TransactionContext{
				TargetURL: "https://api.vendor.com/v1",
				VendorID:  "vendor-upper",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got, err := ParseContext(req, tt.prefix, "Connect-Request-ID")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertContextEqual(t, tt.want, got)
		})
	}
}

func TestParseContext_MissingTargetURL_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		prefix  string
	}{
		{
			name:    "no headers at all",
			headers: map[string]string{},
			prefix:  "X-Connect",
		},
		{
			name: "other headers present but no Target-URL",
			headers: map[string]string{
				"X-Connect-Vendor-ID":      "vendor-123",
				"X-Connect-Marketplace-ID": "US",
			},
			prefix: "X-Connect",
		},
		{
			name: "wrong prefix for Target-URL",
			headers: map[string]string{
				"X-Other-Target-URL": "https://api.vendor.com/v1",
			},
			prefix: "X-Connect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got, err := ParseContext(req, tt.prefix, "Connect-Request-ID")
			if err == nil {
				t.Fatalf("expected error, got nil with context: %+v", got)
			}

			if !errors.Is(err, ErrHeaderRequired) {
				t.Errorf("error type = %v, want %v", err, ErrHeaderRequired)
			}
		})
	}
}

func TestParseContext_InvalidContextData_ReturnsError(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		prefix      string
		wantErr     error
		errContains string
	}{
		{
			name: "malformed Base64 in Context-Data",
			headers: map[string]string{
				"X-Connect-Target-URL":   "https://api.vendor.com/v1",
				"X-Connect-Context-Data": "not-valid-base64!!!",
			},
			prefix:      "X-Connect",
			wantErr:     ErrInvalidContextData,
			errContains: "base64",
		},
		{
			name: "invalid JSON in Context-Data",
			headers: map[string]string{
				"X-Connect-Target-URL":   "https://api.vendor.com/v1",
				"X-Connect-Context-Data": base64.StdEncoding.EncodeToString([]byte(`{invalid json`)),
			},
			prefix:      "X-Connect",
			wantErr:     ErrInvalidContextData,
			errContains: "JSON",
		},
		{
			name: "JSON array instead of object in Context-Data",
			headers: map[string]string{
				"X-Connect-Target-URL":   "https://api.vendor.com/v1",
				"X-Connect-Context-Data": base64.StdEncoding.EncodeToString([]byte(`["array", "not", "object"]`)),
			},
			prefix:      "X-Connect",
			wantErr:     ErrInvalidContextData,
			errContains: "JSON",
		},
		{
			name: "JSON primitive instead of object in Context-Data",
			headers: map[string]string{
				"X-Connect-Target-URL":   "https://api.vendor.com/v1",
				"X-Connect-Context-Data": base64.StdEncoding.EncodeToString([]byte(`"just a string"`)),
			},
			prefix:      "X-Connect",
			wantErr:     ErrInvalidContextData,
			errContains: "JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got, err := ParseContext(req, tt.prefix, "Connect-Request-ID")
			if err == nil {
				t.Fatalf("expected error, got nil with context: %+v", got)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error type = %v, want %v", err, tt.wantErr)
			}

			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error message %q should contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestParseContext_DefaultPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1")
	req.Header.Set("X-Connect-Vendor-ID", "vendor-123")

	// Using default prefix constant
	got, err := ParseContext(req, "X-Connect", "Connect-Request-ID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.TargetURL != "https://api.vendor.com/v1" {
		t.Errorf("TargetURL = %q, want %q", got.TargetURL, "https://api.vendor.com/v1")
	}
	if got.VendorID != "vendor-123" {
		t.Errorf("VendorID = %q, want %q", got.VendorID, "vendor-123")
	}
}

func TestParseContext_ComplexContextData(t *testing.T) {
	complexData := `{
		"nested": {"key": "value"},
		"array": [1, 2, 3],
		"number": 42,
		"boolean": true,
		"null_value": null
	}`
	encoded := base64.StdEncoding.EncodeToString([]byte(complexData))

	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1")
	req.Header.Set("X-Connect-Context-Data", encoded)

	got, err := ParseContext(req, "X-Connect", "Connect-Request-ID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify nested object
	nested, ok := got.Data["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested should be map[string]any, got %T", got.Data["nested"])
	}
	if nested["key"] != "value" {
		t.Errorf("nested.key = %v, want %q", nested["key"], "value")
	}

	// Verify array
	arr, ok := got.Data["array"].([]any)
	if !ok {
		t.Fatalf("array should be []any, got %T", got.Data["array"])
	}
	if len(arr) != 3 {
		t.Errorf("array length = %d, want 3", len(arr))
	}

	// Verify number (JSON numbers decode as float64)
	if got.Data["number"] != float64(42) {
		t.Errorf("number = %v, want 42", got.Data["number"])
	}

	// Verify boolean
	if got.Data["boolean"] != true {
		t.Errorf("boolean = %v, want true", got.Data["boolean"])
	}

	// Verify null becomes nil
	if got.Data["null_value"] != nil {
		t.Errorf("null_value = %v, want nil", got.Data["null_value"])
	}
}

func TestParseContext_TraceIDExtraction(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		traceHeader string
		wantTraceID string
	}{
		{
			name: "trace ID from default header",
			headers: map[string]string{
				"X-Connect-Target-URL": "https://api.vendor.com/v1",
				"Connect-Request-ID":   "trace-12345",
			},
			traceHeader: "Connect-Request-ID",
			wantTraceID: "trace-12345",
		},
		{
			name: "no trace ID header present",
			headers: map[string]string{
				"X-Connect-Target-URL": "https://api.vendor.com/v1",
			},
			traceHeader: "Connect-Request-ID",
			wantTraceID: "",
		},
		{
			name: "custom trace header",
			headers: map[string]string{
				"X-Connect-Target-URL": "https://api.vendor.com/v1",
				"X-My-Trace-ID":        "custom-trace-456",
			},
			traceHeader: "X-My-Trace-ID",
			wantTraceID: "custom-trace-456",
		},
		{
			name: "custom trace header with default also present",
			headers: map[string]string{
				"X-Connect-Target-URL": "https://api.vendor.com/v1",
				"Connect-Request-ID":   "default-trace",
				"X-Custom-Correlation": "custom-correlation-789",
			},
			traceHeader: "X-Custom-Correlation",
			wantTraceID: "custom-correlation-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got, err := ParseContext(req, "X-Connect", tt.traceHeader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.TraceID != tt.wantTraceID {
				t.Errorf("TraceID = %q, want %q", got.TraceID, tt.wantTraceID)
			}
		})
	}
}

// assertContextEqual compares two TransactionContext structs field by field.
func assertContextEqual(t *testing.T, want, got *sdk.TransactionContext) {
	t.Helper()

	if got.TargetURL != want.TargetURL {
		t.Errorf("TargetURL = %q, want %q", got.TargetURL, want.TargetURL)
	}
	if got.EnvironmentID != want.EnvironmentID {
		t.Errorf("EnvironmentID = %q, want %q", got.EnvironmentID, want.EnvironmentID)
	}
	if got.MarketplaceID != want.MarketplaceID {
		t.Errorf("MarketplaceID = %q, want %q", got.MarketplaceID, want.MarketplaceID)
	}
	if got.VendorID != want.VendorID {
		t.Errorf("VendorID = %q, want %q", got.VendorID, want.VendorID)
	}
	if got.ProductID != want.ProductID {
		t.Errorf("ProductID = %q, want %q", got.ProductID, want.ProductID)
	}
	if got.SubscriptionID != want.SubscriptionID {
		t.Errorf("SubscriptionID = %q, want %q", got.SubscriptionID, want.SubscriptionID)
	}
	if got.TraceID != want.TraceID {
		t.Errorf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}

	// Compare Data maps
	if want.Data == nil && got.Data != nil {
		t.Errorf("Data = %v, want nil", got.Data)
	}
	if want.Data != nil {
		if got.Data == nil {
			t.Errorf("Data = nil, want %v", want.Data)
		} else if len(want.Data) != len(got.Data) {
			t.Errorf("Data length = %d, want %d", len(got.Data), len(want.Data))
		}
		for k, v := range want.Data {
			if got.Data[k] != v {
				t.Errorf("Data[%q] = %v, want %v", k, got.Data[k], v)
			}
		}
	}
}
