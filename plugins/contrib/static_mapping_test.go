// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/cloudblue/chaperone/sdk"
)

func TestStaticMapping_SingleRuleMatch(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "contoso-eu.onmicrosoft.com"},
	})

	tx := sdk.TransactionContext{MarketplaceID: "EU-germany"}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "contoso-eu.onmicrosoft.com" {
		t.Errorf("got %q, want %q", got, "contoso-eu.onmicrosoft.com")
	}
}

func TestStaticMapping_NoMatch_ReturnsErrNoMappingMatch(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "contoso-eu"},
		{MarketplaceID: "US-*", Key: "contoso-us"},
	})

	tx := sdk.TransactionContext{MarketplaceID: "AP-japan"}

	_, err := sm.ResolveKey(context.Background(), tx)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrNoMappingMatch) {
		t.Errorf("error = %v, want errors.Is(ErrNoMappingMatch)", err)
	}
}

func TestStaticMapping_SpecificityRanking(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "eu-generic"},
		{MarketplaceID: "EU-*", VendorID: "acme", Key: "eu-acme"},
	})

	tx := sdk.TransactionContext{
		MarketplaceID: "EU-germany",
		VendorID:      "acme",
	}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "eu-acme" {
		t.Errorf("got %q, want %q (higher specificity should win)", got, "eu-acme")
	}
}

func TestStaticMapping_SpecificityRanking_ThreeFields(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "spec-1"},
		{MarketplaceID: "EU-*", VendorID: "acme", Key: "spec-2"},
		{MarketplaceID: "EU-*", VendorID: "acme",
			TargetURL: "*.graph.microsoft.com/**", Key: "spec-3"},
	})

	tx := sdk.TransactionContext{
		MarketplaceID: "EU-germany",
		VendorID:      "acme",
		TargetURL:     "https://api.graph.microsoft.com/v1/users",
	}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "spec-3" {
		t.Errorf("got %q, want %q (highest specificity should win)", got, "spec-3")
	}
}

func TestStaticMapping_TieBreaking_FirstRegisteredWins(t *testing.T) {
	capture := &mappingLogCapture{}
	logger := slog.New(capture)

	sm := NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "first-registered"},
		{MarketplaceID: "EU-*", Key: "second-registered"},
	}, WithMappingLogger(logger))

	tx := sdk.TransactionContext{MarketplaceID: "EU-germany"}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "first-registered" {
		t.Errorf("got %q, want %q (first registered should win tie)", got, "first-registered")
	}

	if !capture.hasWarning() {
		t.Error("expected warning log for tie-breaking")
	}
}

func TestStaticMapping_CatchAllWildcard(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "eu-tenant"},
		{Key: "default-tenant"}, // all fields empty = catch-all
	})

	tx := sdk.TransactionContext{MarketplaceID: "AP-japan"}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "default-tenant" {
		t.Errorf("got %q, want %q (catch-all should match)", got, "default-tenant")
	}
}

func TestStaticMapping_CatchAllLosesToSpecific(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{Key: "default-tenant"},
		{MarketplaceID: "EU-*", Key: "eu-tenant"},
	})

	tx := sdk.TransactionContext{MarketplaceID: "EU-germany"}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "eu-tenant" {
		t.Errorf("got %q, want %q (specific rule should win over catch-all)", got, "eu-tenant")
	}
}

func TestStaticMapping_GlobMatching_TargetURL(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{TargetURL: "*.graph.microsoft.com/**", Key: "graph-tenant"},
	})

	tx := sdk.TransactionContext{
		TargetURL: "https://api.graph.microsoft.com/v1/users",
	}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "graph-tenant" {
		t.Errorf("got %q, want %q", got, "graph-tenant")
	}
}

func TestStaticMapping_AllFieldsMatch(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{
			VendorID:      "acme",
			MarketplaceID: "EU-*",
			EnvironmentID: "prod",
			ProductID:     "sku-*",
			TargetURL:     "api.acme.com/**",
			Key:           "full-match-tenant",
		},
	})

	tx := sdk.TransactionContext{
		VendorID:      "acme",
		MarketplaceID: "EU-germany",
		EnvironmentID: "prod",
		ProductID:     "sku-100",
		TargetURL:     "https://api.acme.com/v1/data",
	}

	got, err := sm.ResolveKey(context.Background(), tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "full-match-tenant" {
		t.Errorf("got %q, want %q", got, "full-match-tenant")
	}
}

func TestStaticMapping_PartialFieldMismatch(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{
		{
			VendorID:      "acme",
			MarketplaceID: "EU-*",
			Key:           "should-not-match",
		},
	})

	tx := sdk.TransactionContext{
		VendorID:      "different-vendor",
		MarketplaceID: "EU-germany",
	}

	_, err := sm.ResolveKey(context.Background(), tx)
	if !errors.Is(err, ErrNoMappingMatch) {
		t.Errorf("error = %v, want errors.Is(ErrNoMappingMatch)", err)
	}
}

func TestNewStaticMapping_EmptyKeyPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty Key")
		}
	}()

	NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "valid"},
		{MarketplaceID: "US-*", Key: ""},
	})
}

func TestNewStaticMapping_EmptyKeyPanics_IncludesIndex(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty Key")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		if msg != "contrib.NewStaticMapping: rule at index 2 has empty Key" {
			t.Errorf("panic message = %q, want index 2", msg)
		}
	}()

	NewStaticMapping([]MappingRule{
		{MarketplaceID: "EU-*", Key: "valid"},
		{MarketplaceID: "US-*", Key: "also-valid"},
		{MarketplaceID: "AP-*", Key: ""}, // index 2
	})
}

func TestMappingRule_Specificity(t *testing.T) {
	tests := []struct {
		name string
		rule MappingRule
		want int
	}{
		{
			name: "empty rule",
			rule: MappingRule{Key: "k"},
			want: 0,
		},
		{
			name: "one field",
			rule: MappingRule{MarketplaceID: "EU-*", Key: "k"},
			want: 1,
		},
		{
			name: "two fields",
			rule: MappingRule{MarketplaceID: "EU-*", VendorID: "acme", Key: "k"},
			want: 2,
		},
		{
			name: "all five fields",
			rule: MappingRule{
				VendorID: "v", MarketplaceID: "m", EnvironmentID: "e",
				ProductID: "p", TargetURL: "t", Key: "k",
			},
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rule.Specificity()
			if got != tt.want {
				t.Errorf("Specificity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStaticMapping_EmptyRules_ReturnsErrNoMappingMatch(t *testing.T) {
	sm := NewStaticMapping([]MappingRule{})

	tx := sdk.TransactionContext{MarketplaceID: "EU-germany"}

	_, err := sm.ResolveKey(context.Background(), tx)
	if !errors.Is(err, ErrNoMappingMatch) {
		t.Errorf("error = %v, want errors.Is(ErrNoMappingMatch)", err)
	}
}

// mappingLogCapture captures log entries for testing tie-breaking warnings.
type mappingLogCapture struct {
	warned bool
}

func (lc *mappingLogCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (lc *mappingLogCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		lc.warned = true
	}
	return nil
}

func (lc *mappingLogCapture) WithAttrs(_ []slog.Attr) slog.Handler { return lc }
func (lc *mappingLogCapture) WithGroup(_ string) slog.Handler      { return lc }
func (lc *mappingLogCapture) hasWarning() bool                     { return lc.warned }
