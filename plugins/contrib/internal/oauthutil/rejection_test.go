// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauthutil

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRejectedBearerToken(t *testing.T) {
	sentWith := func(authorization string) *http.Request {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
			"https://api.vendor.example/v1/orders", http.NoBody)
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		return req
	}

	tests := []struct {
		name      string
		resp      *http.Response
		wantToken string
		wantOK    bool
	}{
		{"401 with bearer token", &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith("Bearer abc.def")}, "abc.def", true},
		{"403 with bearer token", &http.Response{StatusCode: http.StatusForbidden, Request: sentWith("Bearer abc.def")}, "", false},
		{"200 with bearer token", &http.Response{StatusCode: http.StatusOK, Request: sentWith("Bearer abc.def")}, "", false},
		{"401 without Authorization", &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith("")}, "", false},
		{"401 with basic auth", &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith("Basic dXNlcjpwYXNz")}, "", false},
		{"401 with empty bearer token", &http.Response{StatusCode: http.StatusUnauthorized, Request: sentWith("Bearer ")}, "", false},
		{"nil response", nil, "", false},
		{"401 without request", &http.Response{StatusCode: http.StatusUnauthorized}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := RejectedBearerToken(tt.resp)
			if token != tt.wantToken || ok != tt.wantOK {
				t.Errorf("RejectedBearerToken() = (%q, %v), want (%q, %v)", token, ok, tt.wantToken, tt.wantOK)
			}
		})
	}
}

func TestSameToken(t *testing.T) {
	if !SameToken("abc", "abc") {
		t.Error(`SameToken("abc", "abc") = false, want true`)
	}
	if SameToken("abc", "abd") || SameToken("abc", "abcd") || SameToken("", "abc") {
		t.Error("SameToken reported different tokens as equal")
	}
}

func TestInCooldown(t *testing.T) {
	now := time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		last time.Time
		want bool
	}{
		{"no previous discard", time.Time{}, false},
		{"just discarded", now, true},
		{"one second before the cooldown ends", now.Add(-RejectionCooldown + time.Second), true},
		{"exactly at the cooldown", now.Add(-RejectionCooldown), false},
		{"long ago", now.Add(-time.Hour), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InCooldown(tt.last, now); got != tt.want {
				t.Errorf("InCooldown() = %v, want %v", got, tt.want)
			}
		})
	}
}
