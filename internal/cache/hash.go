// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package cache provides utilities for cache key generation and
// credential caching for the Fast Path strategy.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudblue/chaperone/sdk"
)

// emptyHash is a pre-computed hash for nil contexts.
// Using a distinctive canonical form instead of empty string.
var emptyHash = computeHash("<nil>")

// HashContext computes a deterministic SHA-256 hash of a TransactionContext.
//
// This hash is used as the cache key for Fast Path credential caching.
// The hash is:
//   - Deterministic: same input always produces the same output
//   - Collision-resistant: different inputs produce different outputs
//   - Order-invariant: map key order does not affect the result
//
// The following fields are included in the hash:
//   - TargetURL
//   - EnvironmentID
//   - MarketplaceID
//   - VendorID
//   - ProductID
//   - SubscriptionID
//   - Data (canonicalized JSON)
//
// Note: TraceID is explicitly excluded because it's a per-request
// correlation ID that should not affect cache lookup.
//
// Returns a 64-character lowercase hex string (SHA-256) and an error
// if the Data field cannot be serialized to JSON.
func HashContext(ctx *sdk.TransactionContext) (string, error) {
	if ctx == nil {
		return emptyHash, nil
	}

	canonical, err := buildCanonical(ctx)
	if err != nil {
		return "", fmt.Errorf("building canonical form: %w", err)
	}
	return computeHash(canonical), nil
}

// buildCanonical creates a deterministic string representation of the context.
//
// Format: field1:value1|field2:value2|...
// Fields are in a fixed order. The Data map is serialized as sorted JSON.
func buildCanonical(ctx *sdk.TransactionContext) (string, error) {
	// Serialize Data map with sorted keys
	dataStr, err := canonicalizeData(ctx.Data)
	if err != nil {
		return "", fmt.Errorf("canonicalizing data: %w", err)
	}

	// Fixed field order for determinism - preallocate with known capacity
	parts := []string{
		"target:" + ctx.TargetURL,
		"environment:" + ctx.EnvironmentID,
		"marketplace:" + ctx.MarketplaceID,
		"vendor:" + ctx.VendorID,
		"product:" + ctx.ProductID,
		"subscription:" + ctx.SubscriptionID,
		"data:" + dataStr,
	}

	return strings.Join(parts, "|"), nil
}

// canonicalizeData converts a map to a deterministic JSON string.
//
// Returns empty string for nil or empty maps, otherwise returns
// JSON with keys sorted alphabetically at all nesting levels.
//
// Returns an error if the map contains values that cannot be serialized
// to JSON (e.g., channels, functions, cyclic structures).
func canonicalizeData(data map[string]any) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	// Use json.Marshal which produces deterministic output for
	// primitive types, but we need to sort map keys ourselves
	sorted := sortMapKeys(data)

	// Marshal to JSON - this handles nested structures
	bytes, err := json.Marshal(sorted)
	if err != nil {
		return "", fmt.Errorf("marshaling data to JSON: %w", err)
	}

	return string(bytes), nil
}

// sortMapKeys recursively creates a new map with sorted keys.
//
// This ensures deterministic JSON output regardless of Go's
// random map iteration order.
func sortMapKeys(data map[string]any) map[string]any {
	result := make(map[string]any, len(data))

	// Get sorted keys
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Copy values, recursively sorting nested maps
	for _, k := range keys {
		v := data[k]
		switch val := v.(type) {
		case map[string]any:
			result[k] = sortMapKeys(val)
		case []any:
			result[k] = sortSlice(val)
		default:
			result[k] = v
		}
	}

	return result
}

// sortSlice recursively processes slice elements to sort any nested maps.
func sortSlice(slice []any) []any {
	result := make([]any, len(slice))
	for i, v := range slice {
		switch val := v.(type) {
		case map[string]any:
			result[i] = sortMapKeys(val)
		case []any:
			result[i] = sortSlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// computeHash calculates SHA-256 and returns hex-encoded string.
func computeHash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}
