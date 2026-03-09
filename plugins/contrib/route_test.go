// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

func TestRoute_Specificity(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		want  int
	}{
		{
			name:  "empty route has zero specificity",
			route: Route{},
			want:  0,
		},
		{
			name:  "one field set",
			route: Route{VendorID: "acme"},
			want:  1,
		},
		{
			name:  "two fields set",
			route: Route{EnvironmentID: "prod", VendorID: "acme"},
			want:  2,
		},
		{
			name:  "all three original fields set",
			route: Route{EnvironmentID: "prod", VendorID: "acme", TargetURL: "api.acme.com/**"},
			want:  3,
		},
		{
			name:  "marketplace only",
			route: Route{MarketplaceID: "MP-12345"},
			want:  1,
		},
		{
			name:  "product only",
			route: Route{ProductID: "MICROSOFT_SAAS"},
			want:  1,
		},
		{
			name:  "marketplace and product",
			route: Route{MarketplaceID: "MP-*", ProductID: "MICROSOFT_SAAS"},
			want:  2,
		},
		{
			name:  "all five fields set",
			route: Route{VendorID: "acme", MarketplaceID: "MP-*", ProductID: "SKU-*", EnvironmentID: "prod", TargetURL: "api.acme.com/**"},
			want:  5,
		},
		{
			name:  "only target URL",
			route: Route{TargetURL: "*.vendor.com/**"},
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Specificity()
			if got != tt.want {
				t.Errorf("Route.Specificity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRoute_Matches_VendorID(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "exact vendor match",
			route: Route{VendorID: "acme-corp"},
			tx:    sdk.TransactionContext{VendorID: "acme-corp"},
			want:  true,
		},
		{
			name:  "glob vendor match",
			route: Route{VendorID: "microsoft-*"},
			tx:    sdk.TransactionContext{VendorID: "microsoft-azure"},
			want:  true,
		},
		{
			name:  "glob vendor no match",
			route: Route{VendorID: "microsoft-*"},
			tx:    sdk.TransactionContext{VendorID: "google-cloud"},
			want:  false,
		},
		{
			name:  "empty vendor in route matches any",
			route: Route{},
			tx:    sdk.TransactionContext{VendorID: "any-vendor"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Matches(tt.tx)
			if got != tt.want {
				t.Errorf("Route.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoute_Matches_EnvironmentID(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "exact environment match",
			route: Route{EnvironmentID: "production"},
			tx:    sdk.TransactionContext{EnvironmentID: "production"},
			want:  true,
		},
		{
			name:  "environment mismatch",
			route: Route{EnvironmentID: "production"},
			tx:    sdk.TransactionContext{EnvironmentID: "staging"},
			want:  false,
		},
		{
			name:  "empty environment in route matches any",
			route: Route{},
			tx:    sdk.TransactionContext{EnvironmentID: "staging"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Matches(tt.tx)
			if got != tt.want {
				t.Errorf("Route.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoute_Matches_MarketplaceID(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "exact marketplace match",
			route: Route{MarketplaceID: "MP-12345"},
			tx:    sdk.TransactionContext{MarketplaceID: "MP-12345"},
			want:  true,
		},
		{
			name:  "glob marketplace match",
			route: Route{MarketplaceID: "MP-*"},
			tx:    sdk.TransactionContext{MarketplaceID: "MP-12345"},
			want:  true,
		},
		{
			name:  "marketplace mismatch",
			route: Route{MarketplaceID: "MP-12345"},
			tx:    sdk.TransactionContext{MarketplaceID: "MP-67890"},
			want:  false,
		},
		{
			name:  "empty marketplace in route matches any",
			route: Route{},
			tx:    sdk.TransactionContext{MarketplaceID: "MP-12345"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Matches(tt.tx)
			if got != tt.want {
				t.Errorf("Route.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoute_Matches_ProductID(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "exact product match",
			route: Route{ProductID: "MICROSOFT_SAAS"},
			tx:    sdk.TransactionContext{ProductID: "MICROSOFT_SAAS"},
			want:  true,
		},
		{
			name:  "glob product match",
			route: Route{ProductID: "MICROSOFT_*"},
			tx:    sdk.TransactionContext{ProductID: "MICROSOFT_SAAS"},
			want:  true,
		},
		{
			name:  "product mismatch",
			route: Route{ProductID: "MICROSOFT_SAAS"},
			tx:    sdk.TransactionContext{ProductID: "AZURE"},
			want:  false,
		},
		{
			name:  "empty product in route matches any",
			route: Route{},
			tx:    sdk.TransactionContext{ProductID: "MICROSOFT_SAAS"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Matches(tt.tx)
			if got != tt.want {
				t.Errorf("Route.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoute_Matches_TargetURL(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "glob target URL match with scheme stripped",
			route: Route{TargetURL: "*.graph.microsoft.com/**"},
			tx:    sdk.TransactionContext{TargetURL: "https://api.graph.microsoft.com/v1/users"},
			want:  true,
		},
		{
			name:  "target URL mismatch",
			route: Route{TargetURL: "*.graph.microsoft.com/**"},
			tx:    sdk.TransactionContext{TargetURL: "https://api.other.com/v1/data"},
			want:  false,
		},
		{
			name:  "exact target URL match",
			route: Route{TargetURL: "api.vendor.com/v1/status"},
			tx:    sdk.TransactionContext{TargetURL: "https://api.vendor.com/v1/status"},
			want:  true,
		},
		{
			name:  "target URL without scheme in context",
			route: Route{TargetURL: "api.vendor.com/v1/status"},
			tx:    sdk.TransactionContext{TargetURL: "api.vendor.com/v1/status"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Matches(tt.tx)
			if got != tt.want {
				t.Errorf("Route.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoute_Matches_MultipleFields(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "both fields match",
			route: Route{EnvironmentID: "production", VendorID: "microsoft-*"},
			tx:    sdk.TransactionContext{EnvironmentID: "production", VendorID: "microsoft-azure"},
			want:  true,
		},
		{
			name:  "vendor matches but environment does not",
			route: Route{EnvironmentID: "production", VendorID: "microsoft-*"},
			tx:    sdk.TransactionContext{EnvironmentID: "staging", VendorID: "microsoft-azure"},
			want:  false,
		},
		{
			name:  "environment matches but vendor does not",
			route: Route{EnvironmentID: "production", VendorID: "microsoft-*"},
			tx:    sdk.TransactionContext{EnvironmentID: "production", VendorID: "google-cloud"},
			want:  false,
		},
		{
			name: "all three original fields match",
			route: Route{
				EnvironmentID: "production",
				VendorID:      "microsoft-*",
				TargetURL:     "*.graph.microsoft.com/**",
			},
			tx: sdk.TransactionContext{
				EnvironmentID: "production",
				VendorID:      "microsoft-azure",
				TargetURL:     "https://api.graph.microsoft.com/v1/users",
			},
			want: true,
		},
		{
			name: "two of three original fields match",
			route: Route{
				EnvironmentID: "production",
				VendorID:      "microsoft-*",
				TargetURL:     "*.graph.microsoft.com/**",
			},
			tx: sdk.TransactionContext{
				EnvironmentID: "production",
				VendorID:      "microsoft-azure",
				TargetURL:     "https://api.other.com/v1/data",
			},
			want: false,
		},
		{
			name: "marketplace and product both match",
			route: Route{
				MarketplaceID: "MP-*",
				ProductID:     "MICROSOFT_SAAS",
			},
			tx: sdk.TransactionContext{
				MarketplaceID: "MP-12345",
				ProductID:     "MICROSOFT_SAAS",
			},
			want: true,
		},
		{
			name: "marketplace matches but product does not",
			route: Route{
				MarketplaceID: "MP-*",
				ProductID:     "MICROSOFT_SAAS",
			},
			tx: sdk.TransactionContext{
				MarketplaceID: "MP-12345",
				ProductID:     "AZURE",
			},
			want: false,
		},
		{
			name: "all five fields match",
			route: Route{
				VendorID:      "microsoft-*",
				MarketplaceID: "MP-*",
				ProductID:     "MICROSOFT_*",
				EnvironmentID: "production",
				TargetURL:     "*.graph.microsoft.com/**",
			},
			tx: sdk.TransactionContext{
				VendorID:      "microsoft-azure",
				MarketplaceID: "MP-12345",
				ProductID:     "MICROSOFT_SAAS",
				EnvironmentID: "production",
				TargetURL:     "https://api.graph.microsoft.com/v1/users",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.route.Matches(tt.tx)
			if got != tt.want {
				t.Errorf("Route.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripScheme(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https scheme",
			input: "https://api.example.com/v1",
			want:  "api.example.com/v1",
		},
		{
			name:  "http scheme",
			input: "http://api.example.com/v1",
			want:  "api.example.com/v1",
		},
		{
			name:  "no scheme",
			input: "api.example.com/v1",
			want:  "api.example.com/v1",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripScheme(tt.input)
			if got != tt.want {
				t.Errorf("stripScheme(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
