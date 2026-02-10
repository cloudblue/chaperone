// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"runtime"
	"testing"
)

// Package-level sink to prevent compiler optimization of benchmark results.
var benchAllowListSink any

// BenchmarkAllowListValidator benchmarks URL validation against allow list.
// Target: < 10us per pattern, minimal allocs
func BenchmarkAllowListValidator(b *testing.B) {
	allowList := map[string][]string{
		"api.partnercenter.microsoft.com": {"/v1/customers/*", "/v1/invoices/**"},
		"**.amazonaws.com":                {"/my-bucket/**"},
		"api.adobe.com":                   {"/**"},
		"api.google.com":                  {"/v1/users/*", "/v1/products/**"},
	}
	validator := NewAllowListValidator(allowList)

	cases := []struct {
		name string
		url  string
	}{
		{"exact_host_match", "https://api.adobe.com/v1/resource"},
		{"single_star_host", "https://s3.amazonaws.com/my-bucket/file.txt"},
		{"recursive_path", "https://api.partnercenter.microsoft.com/v1/invoices/123/lines/456"},
		{"single_star_path", "https://api.google.com/v1/users/123"},
		{"host_not_allowed", "https://evil.com/hack"},
		{"path_not_allowed", "https://api.google.com/v2/forbidden"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				benchAllowListSink = validator.Validate(tc.url)
			}
		})
	}
}

// BenchmarkAllowListValidator_Parallel tests concurrent URL validation.
func BenchmarkAllowListValidator_Parallel(b *testing.B) {
	allowList := map[string][]string{
		"api.partnercenter.microsoft.com": {"/v1/customers/*", "/v1/invoices/**"},
		"**.amazonaws.com":                {"/my-bucket/**"},
	}
	validator := NewAllowListValidator(allowList)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result := validator.Validate("https://api.partnercenter.microsoft.com/v1/invoices/123/lines")
			runtime.KeepAlive(result)
		}
	})
}

// BenchmarkCheckPathTraversal benchmarks path traversal detection.
// This runs on every request through the allow-list validator and involves
// URL decoding, double-decoding, and string splitting.
func BenchmarkCheckPathTraversal(b *testing.B) {
	cases := []struct {
		name string
		path string
	}{
		{"clean_short", "/v1/customers"},
		{"clean_long", "/v1/customers/123/orders/456/items/789/details"},
		{"traversal_literal", "/v1/../../../etc/passwd"},
		{"traversal_encoded", "/v1/%2e%2e/%2e%2e/etc/passwd"},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				benchAllowListSink = checkPathTraversal(tc.path)
			}
		})
	}
}

// BenchmarkNewAllowListValidator benchmarks validator construction.
// This runs once at startup, not per-request, so less critical.
func BenchmarkNewAllowListValidator(b *testing.B) {
	allowList := map[string][]string{
		"api.partnercenter.microsoft.com": {"/v1/customers/*", "/v1/invoices/**"},
		"**.amazonaws.com":                {"/my-bucket/**"},
		"api.adobe.com":                   {"/**"},
		"api.google.com":                  {"/v1/users/*", "/v1/products/**"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchAllowListSink = NewAllowListValidator(allowList)
	}
}
