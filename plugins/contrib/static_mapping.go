// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/cloudblue/chaperone/sdk"
)

// MappingRule maps a pattern of transaction context fields to a credential key.
// Non-empty fields must all match (glob patterns supported). Empty fields are
// wildcards.
type MappingRule struct {
	VendorID      string // glob pattern
	MarketplaceID string // glob pattern
	EnvironmentID string // glob pattern
	ProductID     string // glob pattern
	TargetURL     string // glob pattern (scheme stripped before matching)
	Key           string // resolved credential key
}

// Specificity returns the number of non-empty matching fields (excluding Key).
// Range: 0-5. A higher specificity means a more specific match.
func (r MappingRule) Specificity() int {
	n := 0
	if r.VendorID != "" {
		n++
	}
	if r.MarketplaceID != "" {
		n++
	}
	if r.EnvironmentID != "" {
		n++
	}
	if r.ProductID != "" {
		n++
	}
	if r.TargetURL != "" {
		n++
	}
	return n
}

// Matches reports whether the rule matches the given transaction context.
// Every non-empty field must match for the rule to match overall.
func (r MappingRule) Matches(tx sdk.TransactionContext) bool {
	if r.VendorID != "" && !GlobMatch(r.VendorID, tx.VendorID, '/') {
		return false
	}
	if r.MarketplaceID != "" && !GlobMatch(r.MarketplaceID, tx.MarketplaceID, '/') {
		return false
	}
	if r.EnvironmentID != "" && !GlobMatch(r.EnvironmentID, tx.EnvironmentID, '/') {
		return false
	}
	if r.ProductID != "" && !GlobMatch(r.ProductID, tx.ProductID, '/') {
		return false
	}
	if r.TargetURL != "" && !matchTargetURL(r.TargetURL, tx.TargetURL) {
		return false
	}
	return true
}

// Compile-time check that StaticMapping implements KeyResolver.
var _ KeyResolver = (*StaticMapping)(nil)

// StaticMapping implements KeyResolver with a declarative rule table.
//
// Rules are evaluated by specificity: a rule with more non-empty fields
// wins over one with fewer. When multiple rules match with equal specificity,
// the first registered rule wins and a warning is logged.
//
// StaticMapping is safe for concurrent use. Rules are set at construction
// time and only read during ResolveKey.
type StaticMapping struct {
	rules  []MappingRule
	logger *slog.Logger
}

// StaticMappingOption configures a StaticMapping at construction time.
type StaticMappingOption func(*StaticMapping)

// WithMappingLogger sets the logger used for tie-breaking warnings.
func WithMappingLogger(l *slog.Logger) StaticMappingOption {
	return func(sm *StaticMapping) { sm.logger = l }
}

// NewStaticMapping creates a StaticMapping from the given rules.
// Panics if any rule has an empty Key field (catches misconfiguration
// at startup, not at request time).
func NewStaticMapping(rules []MappingRule, opts ...StaticMappingOption) *StaticMapping {
	for i, r := range rules {
		if r.Key == "" {
			panic("contrib.NewStaticMapping: rule at index " +
				strconv.Itoa(i) + " has empty Key")
		}
	}

	sm := &StaticMapping{
		rules:  make([]MappingRule, len(rules)),
		logger: slog.Default(),
	}
	copy(sm.rules, rules)

	for _, opt := range opts {
		opt(sm)
	}

	return sm
}

// ResolveKey finds the best matching rule for the transaction context and
// returns its Key. Returns ErrNoMappingMatch if no rule matches.
func (sm *StaticMapping) ResolveKey(_ context.Context, tx sdk.TransactionContext) (string, error) {
	var (
		best     *MappingRule
		bestSpec int
		tied     bool
	)

	for i := range sm.rules {
		r := &sm.rules[i]
		if !r.Matches(tx) {
			continue
		}

		spec := r.Specificity()

		if best == nil || spec > bestSpec {
			best = r
			bestSpec = spec
			tied = false
		} else if spec == bestSpec {
			tied = true
		}
	}

	if best == nil {
		return "", ErrNoMappingMatch
	}

	if tied {
		sm.logger.Warn("multiple mapping rules matched with equal specificity, using first registered",
			"vendor_id", tx.VendorID,
			"marketplace_id", tx.MarketplaceID,
			"environment_id", tx.EnvironmentID,
			"resolved_key", best.Key,
		)
	}

	return best.Key, nil
}
