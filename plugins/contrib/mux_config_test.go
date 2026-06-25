// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/cloudblue/chaperone/sdk"
)

// namedStubProvider is a minimal sdk.CredentialProvider used to assert which
// provider the mux registered for a given route.
type namedStubProvider struct {
	name string
}

func (p *namedStubProvider) GetCredentials(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
	return &sdk.Credential{Headers: map[string]string{"X-Provider": p.name}}, nil
}

// --- spec-mandated tests ---

func TestLoadMuxFromConfig_ForwardAndCredentials_Exclusive(t *testing.T) {
	_, err := LoadMuxFromConfig(MuxConfig{
		Routes: []MuxRouteConfig{
			{
				Match:       MatchConfig{VendorID: "x"},
				Forward:     "company-b",
				Credentials: &CredentialsConfig{Type: "oauth2"},
			},
		},
	}, nil)
	if err == nil {
		t.Fatal("expected mutual-exclusion error, got nil")
	}
	if !strings.Contains(err.Error(), "routes[0]") {
		t.Errorf("error message %q must reference routes[0]", err.Error())
	}
}

func TestLoadMuxFromConfig_ForwardOnly_RegistersForwardAction(t *testing.T) {
	m, err := LoadMuxFromConfig(MuxConfig{
		Routes: []MuxRouteConfig{
			{Match: MatchConfig{VendorID: "x"}, Forward: "company-b"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("LoadMuxFromConfig: %v", err)
	}
	if len(m.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(m.entries))
	}
	fa, ok := m.entries[0].action.(ForwardAction)
	if !ok {
		t.Errorf("action = %T, want ForwardAction", m.entries[0].action)
	}
	if fa.Target != "company-b" {
		t.Errorf("Target = %q, want %q", fa.Target, "company-b")
	}
}

// --- table-driven validation error matrix ---

func TestLoadMuxFromConfig_ValidationErrors(t *testing.T) {
	providers := map[string]sdk.CredentialProvider{
		"oauth2": &namedStubProvider{name: "oauth2"},
	}

	tests := []struct {
		name        string
		cfg         MuxConfig
		wantErrSubs []string // all substrings must appear in the error message
	}{
		{
			name: "neither forward nor credentials",
			cfg: MuxConfig{Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "x"}},
			}},
			wantErrSubs: []string{"routes[0]", "forward", "credentials"},
		},
		{
			name: "both forward and credentials",
			cfg: MuxConfig{Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "x"}, Forward: "t", Credentials: &CredentialsConfig{Type: "oauth2"}},
			}},
			wantErrSubs: []string{"routes[0]", "forward", "credentials"},
		},
		{
			name: "unknown credentials provider type",
			cfg: MuxConfig{Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "x"}, Credentials: &CredentialsConfig{Type: "saml"}},
			}},
			wantErrSubs: []string{"routes[0]", `"saml"`},
		},
		{
			name: "empty credentials.type",
			cfg: MuxConfig{Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "x"}, Credentials: &CredentialsConfig{Type: ""}},
			}},
			wantErrSubs: []string{"routes[0]", "credentials.type"},
		},
		{
			name: "first invalid route reported when multiple invalid",
			cfg: MuxConfig{Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "x"}}, // neither -> error at index 0
				{Match: MatchConfig{VendorID: "y"}, Forward: "t", Credentials: &CredentialsConfig{Type: "oauth2"}},
			}},
			wantErrSubs: []string{"routes[0]"},
		},
		{
			name: "fallback with forward is disallowed",
			cfg: MuxConfig{
				Fallback: &MuxFallbackConfig{Forward: "t"},
			},
			wantErrSubs: []string{"fallback", "forward"},
		},
		{
			name: "fallback with both forward and credentials",
			cfg: MuxConfig{
				Fallback: &MuxFallbackConfig{
					Forward:     "t",
					Credentials: &CredentialsConfig{Type: "oauth2"},
				},
			},
			wantErrSubs: []string{"fallback"},
		},
		{
			name: "fallback credentials unknown type",
			cfg: MuxConfig{
				Fallback: &MuxFallbackConfig{Credentials: &CredentialsConfig{Type: "saml"}},
			},
			wantErrSubs: []string{"fallback", `"saml"`},
		},
		{
			name: "fallback credentials empty type",
			cfg: MuxConfig{
				Fallback: &MuxFallbackConfig{Credentials: &CredentialsConfig{Type: ""}},
			},
			wantErrSubs: []string{"fallback", "credentials.type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadMuxFromConfig(tt.cfg, providers)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			msg := err.Error()
			for _, sub := range tt.wantErrSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("error %q missing substring %q", msg, sub)
				}
			}
		})
	}
}

// --- table-driven successful construction matrix ---

func TestLoadMuxFromConfig_SuccessfulConstruction(t *testing.T) {
	oauthProv := &namedStubProvider{name: "oauth2"}
	bearerProv := &namedStubProvider{name: "bearer"}
	fallbackProv := &namedStubProvider{name: "fallback"}
	providers := map[string]sdk.CredentialProvider{
		"oauth2":   oauthProv,
		"bearer":   bearerProv,
		"fallback": fallbackProv,
	}

	t.Run("single forward route", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{
			Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "v"}, Forward: "company-b"},
			},
		}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if len(m.entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(m.entries))
		}
		fa, ok := m.entries[0].action.(ForwardAction)
		if !ok {
			t.Fatalf("action = %T, want ForwardAction", m.entries[0].action)
		}
		if fa.Target != "company-b" {
			t.Errorf("Target = %q, want %q", fa.Target, "company-b")
		}
	})

	t.Run("single credential route", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{
			Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "v"}, Credentials: &CredentialsConfig{Type: "oauth2"}},
			},
		}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if len(m.entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(m.entries))
		}
		ca, ok := m.entries[0].action.(CredentialAction)
		if !ok {
			t.Fatalf("action = %T, want CredentialAction", m.entries[0].action)
		}
		if ca.Provider != oauthProv {
			t.Errorf("Provider = %v, want oauthProv", ca.Provider)
		}
	})

	t.Run("mixed forward and credential routes preserve order", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{
			Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "v1"}, Forward: "f1"},
				{Match: MatchConfig{VendorID: "v2"}, Credentials: &CredentialsConfig{Type: "oauth2"}},
				{Match: MatchConfig{VendorID: "v3"}, Forward: "f3"},
				{Match: MatchConfig{VendorID: "v4"}, Credentials: &CredentialsConfig{Type: "bearer"}},
			},
		}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if len(m.entries) != 4 {
			t.Fatalf("entries = %d, want 4", len(m.entries))
		}
		// Check order preserved by index and action type
		if fa, ok := m.entries[0].action.(ForwardAction); !ok || fa.Target != "f1" {
			t.Errorf("entries[0] = %v, want ForwardAction{Target:f1}", m.entries[0].action)
		}
		if ca, ok := m.entries[1].action.(CredentialAction); !ok || ca.Provider != oauthProv {
			t.Errorf("entries[1] = %v, want CredentialAction{oauth2}", m.entries[1].action)
		}
		if fa, ok := m.entries[2].action.(ForwardAction); !ok || fa.Target != "f3" {
			t.Errorf("entries[2] = %v, want ForwardAction{Target:f3}", m.entries[2].action)
		}
		if ca, ok := m.entries[3].action.(CredentialAction); !ok || ca.Provider != bearerProv {
			t.Errorf("entries[3] = %v, want CredentialAction{bearer}", m.entries[3].action)
		}
		// Indexes should be 0..3
		for i, e := range m.entries {
			if e.index != i {
				t.Errorf("entries[%d].index = %d, want %d", i, e.index, i)
			}
		}
	})

	t.Run("full match dimensions propagated", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{
			Routes: []MuxRouteConfig{
				{
					Match: MatchConfig{
						VendorID:      "vendor",
						MarketplaceID: "mp",
						ProductID:     "prod",
						EnvironmentID: "env",
						TargetURL:     "*.example.com/**",
						Data:          map[string]string{"ResellerId": "r-*", "Region": "us"},
					},
					Forward: "target",
				},
			},
		}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		r := m.entries[0].route
		if r.VendorID != "vendor" || r.MarketplaceID != "mp" || r.ProductID != "prod" ||
			r.EnvironmentID != "env" || r.TargetURL != "*.example.com/**" {
			t.Errorf("route fields not propagated: %+v", r)
		}
		if got, want := r.Data["ResellerId"], "r-*"; got != want {
			t.Errorf("Data[ResellerId] = %q, want %q", got, want)
		}
		if got, want := r.Data["Region"], "us"; got != want {
			t.Errorf("Data[Region] = %q, want %q", got, want)
		}
	})

	t.Run("fallback with credentials sets default", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{
			Fallback: &MuxFallbackConfig{Credentials: &CredentialsConfig{Type: "fallback"}},
		}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if m.fallback != fallbackProv {
			t.Errorf("fallback = %v, want fallbackProv", m.fallback)
		}
		if len(m.entries) != 0 {
			t.Errorf("entries = %d, want 0", len(m.entries))
		}
	})

	t.Run("empty routes plus fallback", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{
			Routes:   nil,
			Fallback: &MuxFallbackConfig{Credentials: &CredentialsConfig{Type: "fallback"}},
		}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if len(m.entries) != 0 {
			t.Errorf("entries = %d, want 0", len(m.entries))
		}
		if m.fallback != fallbackProv {
			t.Errorf("fallback = %v, want fallbackProv", m.fallback)
		}
	})

	t.Run("empty everything yields valid empty mux", func(t *testing.T) {
		m, err := LoadMuxFromConfig(MuxConfig{}, providers)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if len(m.entries) != 0 {
			t.Errorf("entries = %d, want 0", len(m.entries))
		}
		if m.fallback != nil {
			t.Errorf("fallback = %v, want nil", m.fallback)
		}
	})

	t.Run("nil providers map with only forward routes is fine", func(t *testing.T) {
		// Forwards don't need providers — nil map should be acceptable.
		m, err := LoadMuxFromConfig(MuxConfig{
			Routes: []MuxRouteConfig{
				{Match: MatchConfig{VendorID: "v"}, Forward: "f"},
			},
		}, nil)
		if err != nil {
			t.Fatalf("LoadMuxFromConfig: %v", err)
		}
		if len(m.entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(m.entries))
		}
	})
}

// --- YAML roundtrip end-to-end test ---

func TestLoadMuxFromConfig_YAMLRoundtrip(t *testing.T) {
	const doc = `
routes:
  - match:
      vendor_id: "acme"
      product_id: "WIDGET"
    credentials:
      type: oauth2
  - match:
      vendor_id: "globex"
    forward: company-b
  - match:
      vendor_id: "migrated"
      data:
        ResellerId: "legacy-*"
    forward: company-b
fallback:
  credentials:
    type: fallback
`
	var cfg MuxConfig
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	oauthProv := &namedStubProvider{name: "oauth2"}
	fallbackProv := &namedStubProvider{name: "fallback"}
	providers := map[string]sdk.CredentialProvider{
		"oauth2":   oauthProv,
		"fallback": fallbackProv,
	}

	m, err := LoadMuxFromConfig(cfg, providers)
	if err != nil {
		t.Fatalf("LoadMuxFromConfig: %v", err)
	}

	if len(m.entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(m.entries))
	}

	// Entry 0: acme + WIDGET → credentials oauth2
	{
		e := m.entries[0]
		if e.route.VendorID != "acme" || e.route.ProductID != "WIDGET" {
			t.Errorf("entry[0] route = %+v", e.route)
		}
		ca, ok := e.action.(CredentialAction)
		if !ok || ca.Provider != oauthProv {
			t.Errorf("entry[0] action = %T %v, want CredentialAction{oauth2}", e.action, e.action)
		}
	}
	// Entry 1: globex forward
	{
		e := m.entries[1]
		if e.route.VendorID != "globex" || e.route.TargetURL != "" {
			t.Errorf("entry[1] route = %+v", e.route)
		}
		fa, ok := e.action.(ForwardAction)
		if !ok || fa.Target != "company-b" {
			t.Errorf("entry[1] action = %T %v, want ForwardAction{company-b}", e.action, e.action)
		}
	}
	// Entry 2: migrated forward with Data
	{
		e := m.entries[2]
		if e.route.VendorID != "migrated" {
			t.Errorf("entry[2] route.VendorID = %q, want %q", e.route.VendorID, "migrated")
		}
		if got, want := e.route.Data["ResellerId"], "legacy-*"; got != want {
			t.Errorf("entry[2] Data[ResellerId] = %q, want %q", got, want)
		}
		fa, ok := e.action.(ForwardAction)
		if !ok || fa.Target != "company-b" {
			t.Errorf("entry[2] action = %T %v, want ForwardAction{company-b}", e.action, e.action)
		}
	}
	if m.fallback != fallbackProv {
		t.Errorf("fallback = %v, want fallbackProv", m.fallback)
	}

	// Behavioral check: send a tx that matches entry 1 and verify RouteRequest
	// returns a ForwardAction to "company-b".
	tx := sdk.TransactionContext{
		VendorID:  "globex",
		TargetURL: "https://api.globex.com/v1/things",
	}
	ra, err := m.RouteRequest(context.Background(), tx, nil)
	if err != nil {
		t.Fatalf("RouteRequest: %v", err)
	}
	if ra == nil || ra.ForwardTo != "company-b" {
		t.Errorf("RouteRequest = %+v, want ForwardTo=company-b", ra)
	}

	// And a tx that matches the credential route (entry 0) — RouteRequest
	// must return nil so the credential path runs.
	txCred := sdk.TransactionContext{VendorID: "acme", ProductID: "WIDGET"}
	ra2, err := m.RouteRequest(context.Background(), txCred, nil)
	if err != nil {
		t.Fatalf("RouteRequest: %v", err)
	}
	if ra2 != nil {
		t.Errorf("RouteRequest = %+v, want nil for credential match", ra2)
	}
}
