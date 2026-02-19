// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package context provides utilities for parsing and managing transaction
// context from incoming proxy requests.
package context

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cloudblue/chaperone/sdk"
)

// Sentinel errors for context parsing.
var (
	// ErrHeaderRequired is returned when a required context header is missing.
	ErrHeaderRequired = errors.New("required header is missing")

	// ErrInvalidContextData is returned when Context-Data cannot be decoded.
	ErrInvalidContextData = errors.New("invalid Context-Data")
)

// headerDef couples a header suffix with the logic to apply its value
// to a TransactionContext. Adding a new context header means adding one
// entry here — both stripping and parsing are derived from this table.
type headerDef struct {
	// Suffix is appended to the configured prefix (e.g., "-Vendor-ID").
	Suffix string

	// Required causes ParseContext to return an error if the header is absent.
	Required bool

	// Apply stores the header value into the TransactionContext.
	// Return a non-nil error only for values that need validation (e.g., Context-Data).
	Apply func(ctx *sdk.TransactionContext, value string) error
}

// contextHeaders is the single source of truth for all context headers.
// Both HeaderSuffixes (used for stripping) and ParseContext (used for
// extraction) are derived from this table, so they cannot drift apart.
var contextHeaders = []headerDef{
	{
		Suffix:   "-Target-URL",
		Required: true,
		Apply:    func(ctx *sdk.TransactionContext, v string) error { ctx.TargetURL = v; return nil },
	},
	{
		Suffix: "-Vendor-ID",
		Apply:  func(ctx *sdk.TransactionContext, v string) error { ctx.VendorID = v; return nil },
	},
	{
		Suffix: "-Marketplace-ID",
		Apply:  func(ctx *sdk.TransactionContext, v string) error { ctx.MarketplaceID = v; return nil },
	},
	{
		Suffix: "-Product-ID",
		Apply:  func(ctx *sdk.TransactionContext, v string) error { ctx.ProductID = v; return nil },
	},
	{
		Suffix: "-Subscription-ID",
		Apply:  func(ctx *sdk.TransactionContext, v string) error { ctx.SubscriptionID = v; return nil },
	},
	{
		Suffix: "-Context-Data",
		Apply: func(ctx *sdk.TransactionContext, v string) error {
			data, err := decodeContextData(v)
			if err != nil {
				return err
			}
			ctx.Data = data
			return nil
		},
	},
}

// headerSuffixes is pre-computed from contextHeaders at init time to avoid
// a per-request allocation in the hot path (stripContextHeaders / Director).
var headerSuffixes = func() []string {
	out := make([]string, len(contextHeaders))
	for i, h := range contextHeaders {
		out[i] = h.Suffix
	}
	return out
}()

// HeaderSuffixes returns the suffixes of all context headers, derived from
// contextHeaders. Used by the proxy Director to strip these headers before
// forwarding to the target.
func HeaderSuffixes() []string { return headerSuffixes }

// ParseContext extracts transaction context from HTTP request headers.
//
// It reads headers with the specified prefix (e.g., "X-Connect-Target-URL")
// and populates a TransactionContext struct. The header list and parsing
// logic are driven by the contextHeaders table.
//
// Parameters:
//   - req: The incoming HTTP request
//   - prefix: Header prefix (e.g., "X-Connect")
//   - traceHeader: Header name for trace/correlation ID (e.g., "Connect-Request-ID")
//
// Returns an error if:
//   - A required header is missing (e.g., Target-URL)
//   - A header value fails validation (e.g., invalid Base64 in Context-Data)
func ParseContext(req *http.Request, prefix, traceHeader string) (*sdk.TransactionContext, error) {
	ctx := &sdk.TransactionContext{}

	for _, h := range contextHeaders {
		value := req.Header.Get(prefix + h.Suffix)
		if value == "" {
			if h.Required {
				return nil, fmt.Errorf("%w: %s%s", ErrHeaderRequired, prefix, h.Suffix)
			}
			continue
		}
		if err := h.Apply(ctx, value); err != nil {
			return nil, err
		}
	}

	// Extract trace ID using configured header (not a context header — different naming)
	ctx.TraceID = req.Header.Get(traceHeader)

	return ctx, nil
}

// decodeContextData decodes a Base64-encoded JSON object into a map.
//
// Returns an error for invalid Base64 or non-object JSON.
func decodeContextData(encoded string) (map[string]any, error) {
	// Decode Base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 encoding: %w", ErrInvalidContextData, err)
	}

	// Parse JSON
	var data map[string]any
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %w", ErrInvalidContextData, err)
	}

	return data, nil
}
