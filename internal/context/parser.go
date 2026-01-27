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

// DefaultHeaderPrefix is the default prefix for Connect headers.
const DefaultHeaderPrefix = "X-Connect"

// DefaultTraceHeader is the default header name for trace/correlation IDs.
const DefaultTraceHeader = "Connect-Request-ID"

// Sentinel errors for context parsing.
var (
	// ErrTargetURLRequired is returned when the Target-URL header is missing.
	ErrTargetURLRequired = errors.New("Target-URL header is required")

	// ErrInvalidContextData is returned when Context-Data cannot be decoded.
	ErrInvalidContextData = errors.New("invalid Context-Data")
)

// ParseContext extracts transaction context from HTTP request headers.
//
// It reads headers with the specified prefix (e.g., "X-Connect-Target-URL")
// and populates a TransactionContext struct. The Context-Data header is
// expected to be a Base64-encoded JSON object.
//
// Parameters:
//   - req: The incoming HTTP request
//   - prefix: Header prefix (e.g., "X-Connect")
//
// Returns an error if:
//   - Target-URL header is missing (required)
//   - Context-Data contains invalid Base64
//   - Context-Data contains invalid JSON
func ParseContext(req *http.Request, prefix string) (*sdk.TransactionContext, error) {
	ctx := &sdk.TransactionContext{}

	// Extract required Target-URL
	targetURL := req.Header.Get(prefix + "-Target-URL")
	if targetURL == "" {
		return nil, fmt.Errorf("%w: %s-Target-URL", ErrTargetURLRequired, prefix)
	}
	ctx.TargetURL = targetURL

	// Extract optional standard headers
	ctx.MarketplaceID = req.Header.Get(prefix + "-Marketplace-ID")
	ctx.VendorID = req.Header.Get(prefix + "-Vendor-ID")
	ctx.ProductID = req.Header.Get(prefix + "-Product-ID")
	ctx.SubscriptionID = req.Header.Get(prefix + "-Subscription-ID")

	// Extract trace ID (using default trace header)
	ctx.TraceID = req.Header.Get(DefaultTraceHeader)

	// Decode optional Context-Data (Base64-encoded JSON)
	contextData := req.Header.Get(prefix + "-Context-Data")
	if contextData != "" {
		data, err := decodeContextData(contextData)
		if err != nil {
			return nil, err
		}
		ctx.Data = data
	}

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
