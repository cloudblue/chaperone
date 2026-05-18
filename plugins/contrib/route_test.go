// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"
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

// --- Spec tests (verbatim from Task 9 plan) ---

func TestRoute_Matches_DataField_ExactKey(t *testing.T) {
	r := Route{Data: map[string]string{"ResellerId": "migrated-*"}}
	tx := sdk.TransactionContext{Data: map[string]any{"ResellerId": "migrated-42"}}
	if !r.Matches(tx) {
		t.Error("expected match for ResellerId=migrated-42 against migrated-*")
	}
}

func TestRoute_Matches_DataField_Mismatch(t *testing.T) {
	r := Route{Data: map[string]string{"ResellerId": "migrated-*"}}
	tx := sdk.TransactionContext{Data: map[string]any{"ResellerId": "legacy-42"}}
	if r.Matches(tx) {
		t.Error("expected no match for legacy-42")
	}
}

func TestRoute_Matches_DataField_MissingKey_DoesNotMatch(t *testing.T) {
	r := Route{Data: map[string]string{"ResellerId": "migrated-*"}}
	tx := sdk.TransactionContext{Data: map[string]any{}}
	if r.Matches(tx) {
		t.Error("expected no match when key is absent")
	}
}

func TestRoute_Specificity_IncludesDataEntries(t *testing.T) {
	r := Route{VendorID: "microsoft-*", Data: map[string]string{"ResellerId": "x", "TenantId": "y"}}
	if got, want := r.Specificity(), 3; got != want {
		t.Errorf("Specificity = %d, want %d", got, want)
	}
}

// --- Matches table for the Data dimension ---

func TestRoute_Matches_Data_Table(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		tx    sdk.TransactionContext
		want  bool
	}{
		{
			name:  "single Data key with literal exact value matches",
			route: Route{Data: map[string]string{"ResellerId": "migrated-42"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"ResellerId": "migrated-42"}},
			want:  true,
		},
		{
			name:  "single Data key with glob pattern matches",
			route: Route{Data: map[string]string{"ResellerId": "migrated-*"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"ResellerId": "migrated-42"}},
			want:  true,
		},
		{
			name:  "single Data key with wrong value does not match",
			route: Route{Data: map[string]string{"ResellerId": "migrated-42"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"ResellerId": "legacy-42"}},
			want:  false,
		},
		{
			name:  "Data key empty string in tx is invalid and does not match (DataString returns err)",
			route: Route{Data: map[string]string{"ResellerId": "*"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"ResellerId": ""}},
			want:  false,
		},
		{
			name:  "multiple Data keys all match",
			route: Route{Data: map[string]string{"ResellerId": "migrated-*", "TenantId": "abc-*"}},
			tx: sdk.TransactionContext{Data: map[string]any{
				"ResellerId": "migrated-42",
				"TenantId":   "abc-001",
			}},
			want: true,
		},
		{
			name:  "multiple Data keys one mismatch",
			route: Route{Data: map[string]string{"ResellerId": "migrated-*", "TenantId": "abc-*"}},
			tx: sdk.TransactionContext{Data: map[string]any{
				"ResellerId": "migrated-42",
				"TenantId":   "xyz-001",
			}},
			want: false,
		},
		{
			name:  "multiple Data keys one missing",
			route: Route{Data: map[string]string{"ResellerId": "migrated-*", "TenantId": "abc-*"}},
			tx: sdk.TransactionContext{Data: map[string]any{
				"ResellerId": "migrated-42",
			}},
			want: false,
		},
		{
			name: "Data combined with top-level VendorID, both match",
			route: Route{
				VendorID: "microsoft-*",
				Data:     map[string]string{"ResellerId": "migrated-*"},
			},
			tx: sdk.TransactionContext{
				VendorID: "microsoft-azure",
				Data:     map[string]any{"ResellerId": "migrated-42"},
			},
			want: true,
		},
		{
			name: "Data matches but VendorID does not",
			route: Route{
				VendorID: "microsoft-*",
				Data:     map[string]string{"ResellerId": "migrated-*"},
			},
			tx: sdk.TransactionContext{
				VendorID: "google-cloud",
				Data:     map[string]any{"ResellerId": "migrated-42"},
			},
			want: false,
		},
		{
			name: "VendorID matches but Data does not",
			route: Route{
				VendorID: "microsoft-*",
				Data:     map[string]string{"ResellerId": "migrated-*"},
			},
			tx: sdk.TransactionContext{
				VendorID: "microsoft-azure",
				Data:     map[string]any{"ResellerId": "legacy-1"},
			},
			want: false,
		},
		{
			name:  "Data with recursive ** glob matches multi-segment value",
			route: Route{Data: map[string]string{"Scope": "tenant/**"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"Scope": "tenant/abc/sub/123"}},
			want:  true,
		},
		{
			name:  "empty Data map plus top-level VendorID match returns true",
			route: Route{VendorID: "microsoft-*", Data: map[string]string{}},
			tx:    sdk.TransactionContext{VendorID: "microsoft-azure"},
			want:  true,
		},
		{
			name:  "nil Data plus top-level VendorID match returns true",
			route: Route{VendorID: "microsoft-*"},
			tx:    sdk.TransactionContext{VendorID: "microsoft-azure"},
			want:  true,
		},
		{
			name:  "tx Data has key but value is wrong type (int) does not match",
			route: Route{Data: map[string]string{"ResellerId": "migrated-*"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"ResellerId": 42}},
			want:  false,
		},
		{
			name:  "tx Data has key but value is wrong type (bool) does not match",
			route: Route{Data: map[string]string{"Active": "*"}},
			tx:    sdk.TransactionContext{Data: map[string]any{"Active": true}},
			want:  false,
		},
		{
			name:  "tx Data nil with non-empty route Data does not match",
			route: Route{Data: map[string]string{"ResellerId": "migrated-*"}},
			tx:    sdk.TransactionContext{Data: nil},
			want:  false,
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

// --- Specificity table for the Data dimension ---

func TestRoute_Specificity_Data_Table(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		want  int
	}{
		{
			name:  "zero non-empty fields, nil Data",
			route: Route{},
			want:  0,
		},
		{
			name:  "zero non-empty fields, empty Data map",
			route: Route{Data: map[string]string{}},
			want:  0,
		},
		{
			name:  "only Data with one entry",
			route: Route{Data: map[string]string{"ResellerId": "x"}},
			want:  1,
		},
		{
			name:  "only Data with three entries",
			route: Route{Data: map[string]string{"a": "1", "b": "2", "c": "3"}},
			want:  3,
		},
		{
			name:  "VendorID plus two Data entries",
			route: Route{VendorID: "microsoft-*", Data: map[string]string{"a": "1", "b": "2"}},
			want:  3,
		},
		{
			name: "all five top-level fields plus two Data entries",
			route: Route{
				VendorID:      "v",
				MarketplaceID: "m",
				ProductID:     "p",
				TargetURL:     "t",
				EnvironmentID: "e",
				Data:          map[string]string{"a": "1", "b": "2"},
			},
			want: 7,
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

// --- routesMayOverlap regression checks for the Data dimension ---

func TestRoutesMayOverlap_DataDimension(t *testing.T) {
	tests := []struct {
		name string
		a    Route
		b    Route
		want bool
	}{
		{
			name: "both routes same Data key with same literal may overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			b:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			want: true,
		},
		{
			name: "both routes same Data key with disjoint literals do not overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			b:    Route{Data: map[string]string{"ResellerId": "migrated-2"}},
			want: false,
		},
		{
			name: "routes with different Data keys may overlap (no shared dimension)",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			b:    Route{Data: map[string]string{"TenantId": "abc"}},
			want: true,
		},
		{
			name: "same Data key, one glob one literal: conservatively may overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-*"}},
			b:    Route{Data: map[string]string{"ResellerId": "legacy-1"}},
			want: true,
		},
		{
			name: "same Data key, one glob one matching literal: may overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-*"}},
			b:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			want: true,
		},
		{
			name: "one route with Data, other with nil Data: no shared dimension, may overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			b:    Route{VendorID: "microsoft-*"},
			want: true,
		},
		{
			name: "one route with Data, other with empty Data map: no shared dimension, may overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1"}},
			b:    Route{Data: map[string]string{}},
			want: true,
		},
		{
			name: "multi-key Data with one shared key disjoint literal: do not overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1", "TenantId": "abc"}},
			b:    Route{Data: map[string]string{"ResellerId": "migrated-2", "TenantId": "abc"}},
			want: false,
		},
		{
			name: "multi-key Data with all shared keys literal and equal: may overlap",
			a:    Route{Data: map[string]string{"ResellerId": "migrated-1", "TenantId": "abc"}},
			b:    Route{Data: map[string]string{"ResellerId": "migrated-1", "TenantId": "abc"}},
			want: true,
		},
		{
			name: "Data shared key matches, top-level VendorID disjoint literals: do not overlap",
			a: Route{
				VendorID: "microsoft",
				Data:     map[string]string{"ResellerId": "migrated-1"},
			},
			b: Route{
				VendorID: "google",
				Data:     map[string]string{"ResellerId": "migrated-1"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routesMayOverlap(tt.a, tt.b); got != tt.want {
				t.Errorf("routesMayOverlap(%+v, %+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- routeString rendering for Data ---

func TestRouteString_RendersDataEntries(t *testing.T) {
	tests := []struct {
		name  string
		route Route
		// substrings that must appear in the rendered string
		mustContain []string
	}{
		{
			name:        "Data-only single entry",
			route:       Route{Data: map[string]string{"ResellerId": "migrated-*"}},
			mustContain: []string{"Data[ResellerId]=migrated-*"},
		},
		{
			name: "Data combined with VendorID",
			route: Route{
				VendorID: "microsoft-*",
				Data:     map[string]string{"ResellerId": "migrated-*"},
			},
			mustContain: []string{"VendorID=microsoft-*", "Data[ResellerId]=migrated-*"},
		},
		{
			name: "Data with multiple entries is deterministically sorted",
			route: Route{
				Data: map[string]string{"TenantId": "abc", "ResellerId": "migrated-*"},
			},
			// Sorted alphabetically: ResellerId before TenantId.
			mustContain: []string{"Data[ResellerId]=migrated-*", "Data[TenantId]=abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeString(tt.route)
			for _, s := range tt.mustContain {
				if !strings.Contains(got, s) {
					t.Errorf("routeString = %q, want it to contain %q", got, s)
				}
			}
		})
	}

	// Additionally verify deterministic ordering: ResellerId appears
	// before TenantId in the sorted multi-entry case.
	got := routeString(Route{Data: map[string]string{"TenantId": "abc", "ResellerId": "migrated-*"}})
	iReseller := strings.Index(got, "Data[ResellerId]=")
	iTenant := strings.Index(got, "Data[TenantId]=")
	if iReseller == -1 || iTenant == -1 {
		t.Fatalf("routeString = %q, missing expected entries", got)
	}
	if iReseller > iTenant {
		t.Errorf("routeString = %q, expected Data[ResellerId] before Data[TenantId]", got)
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
