// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauthutil

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudblue/chaperone/plugins/contrib"
)

// recordedLog is one captured log record, flattened to text for searching.
type recordedLog struct {
	level   slog.Level
	message string
	attrs   map[string]string
}

// logRecorder is an slog.Handler that records entries at or above minLevel.
type logRecorder struct {
	minLevel slog.Level

	mu      sync.Mutex
	entries []recordedLog
}

func (lr *logRecorder) Enabled(_ context.Context, level slog.Level) bool {
	return level >= lr.minLevel
}

func (lr *logRecorder) Handle(_ context.Context, r slog.Record) error {
	entry := recordedLog{level: r.Level, message: r.Message, attrs: make(map[string]string)}
	r.Attrs(func(a slog.Attr) bool {
		entry.attrs[a.Key] = a.Value.String()
		return true
	})
	lr.mu.Lock()
	defer lr.mu.Unlock()
	lr.entries = append(lr.entries, entry)
	return nil
}

func (lr *logRecorder) WithAttrs(_ []slog.Attr) slog.Handler { return lr }
func (lr *logRecorder) WithGroup(_ string) slog.Handler      { return lr }

// warn returns the single WARN entry, failing the test if there is not
// exactly one.
func (lr *logRecorder) warn(t *testing.T) recordedLog {
	t.Helper()
	lr.mu.Lock()
	defer lr.mu.Unlock()
	var warns []recordedLog
	for _, e := range lr.entries {
		if e.level == slog.LevelWarn {
			warns = append(warns, e)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("got %d WARN log entries, want 1: %+v", len(warns), warns)
	}
	return warns[0]
}

// text returns everything that was logged, as one searchable string.
func (lr *logRecorder) text() string {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	var b strings.Builder
	for _, e := range lr.entries {
		b.WriteString(e.message)
		for k, v := range e.attrs {
			b.WriteString(" " + k + "=" + v)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// exchangeAgainst runs a refresh token exchange against a token endpoint that
// answers with the given status and JSON body, logging at minLevel and above.
func exchangeAgainst(t *testing.T, minLevel slog.Level, status int, contentType, body string) (*logRecorder, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	rec := &logRecorder{minLevel: minLevel}
	f := NewTokenFetcher(TokenFetcher{
		TokenURL:     srv.URL + "/contoso/oauth2/token",
		ClientID:     "client-id",
		ClientSecret: "client-secret-value",
		Logger:       slog.New(rec),
	})

	form := f.BuildForm("refresh_token")
	form.Set("refresh_token", "refresh-token-value")

	_, err := f.Exchange(context.Background(), form)
	if err == nil {
		t.Fatal("Exchange() error = nil, want an error")
	}
	return rec, err
}

// entraMFARequired is the shape of a Microsoft Entra ID error response when
// the refresh token no longer satisfies a multi-factor authentication policy.
const entraMFARequired = `{
  "error": "interaction_required",
  "error_description": "AADSTS50076: Due to a configuration change made by your administrator, or because you moved to a new location, you must use multi-factor authentication to access '00000003-0000-0000-c000-000000000000'. Trace ID: 11111111-2222-3333-4444-555555555555 Correlation ID: 66666666-7777-8888-9999-000000000000 Timestamp: 2026-11-18 16:07:12Z",
  "error_codes": [50076],
  "trace_id": "11111111-2222-3333-4444-555555555555"
}`

// The reason a token endpoint rejects an exchange must be visible without
// enabling debug logging.
func TestExchange_Rejected_LogsReasonAtWarn(t *testing.T) {
	rec, err := exchangeAgainst(t, slog.LevelInfo, http.StatusBadRequest, "application/json", entraMFARequired)

	warn := rec.warn(t)
	if warn.message != "token endpoint rejected the token request" {
		t.Errorf("WARN message = %q", warn.message)
	}
	if got := warn.attrs["oauth_error"]; got != "interaction_required" {
		t.Errorf("oauth_error = %q, want %q", got, "interaction_required")
	}
	if got := warn.attrs["oauth_error_code"]; got != "AADSTS50076" {
		t.Errorf("oauth_error_code = %q, want %q", got, "AADSTS50076")
	}
	if got := warn.attrs["status"]; got != "400" {
		t.Errorf("status = %q, want 400", got)
	}
	if !strings.HasSuffix(warn.attrs["token_url"], "/contoso/oauth2/token") {
		t.Errorf("token_url = %q, want the token endpoint URL", warn.attrs["token_url"])
	}

	// The same reason travels in the error, so whoever logs the error (the
	// Core logs it as "plugin error") reports it too.
	for _, want := range []string{"400", "interaction_required", "AADSTS50076"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// At normal log levels only the two codes are reported: not the description,
// not any other body field, not anything the request carried.
func TestExchange_Rejected_NormalLevel_ReportsOnlyTheCodes(t *testing.T) {
	rec, err := exchangeAgainst(t, slog.LevelInfo, http.StatusBadRequest, "application/json", entraMFARequired)

	reported := rec.text() + err.Error()
	for _, forbidden := range []string{
		"Due to a configuration change",        // error_description text
		"11111111-2222-3333-4444-555555555555", // trace ID
		"00000003-0000-0000-c000-000000000000", // resource named in the description
		"refresh-token-value",
		"client-secret-value",
	} {
		if strings.Contains(reported, forbidden) {
			t.Errorf("log or error contains %q:\n%s", forbidden, reported)
		}
	}
}

// Debug logging keeps the full description, as before.
func TestExchange_Rejected_DebugLevel_KeepsDescription(t *testing.T) {
	rec, _ := exchangeAgainst(t, slog.LevelDebug, http.StatusBadRequest, "application/json", entraMFARequired)

	if !strings.Contains(rec.text(), "oauth_error_description=AADSTS50076: Due to a configuration change") {
		t.Errorf("debug log does not contain the error description:\n%s", rec.text())
	}
	// The WARN entry is the same at any level.
	if _, ok := rec.warn(t).attrs["oauth_error_description"]; ok {
		t.Error("WARN entry carries oauth_error_description; it must stay at debug level")
	}
}

func TestExchange_Rejected_ByStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantMessage string
		wantErr     error
		wantCode    string
	}{
		{
			name:        "401 invalid_client",
			status:      http.StatusUnauthorized,
			body:        `{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided."}`,
			wantMessage: "token endpoint rejected the client credentials",
			wantErr:     contrib.ErrInvalidCredentials,
			wantCode:    "invalid_client",
		},
		{
			name:        "400 invalid_grant",
			status:      http.StatusBadRequest,
			body:        `{"error":"invalid_grant","error_description":"The refresh token is expired."}`,
			wantMessage: "token endpoint rejected the token request",
			wantCode:    "invalid_grant",
		},
		{
			name:        "503 temporarily_unavailable",
			status:      http.StatusServiceUnavailable,
			body:        `{"error":"temporarily_unavailable"}`,
			wantMessage: "token endpoint unavailable",
			wantErr:     contrib.ErrTokenEndpointUnavailable,
			wantCode:    "temporarily_unavailable",
		},
		{
			name:        "429 without body",
			status:      http.StatusTooManyRequests,
			body:        ``,
			wantMessage: "token endpoint unavailable",
			wantErr:     contrib.ErrTokenEndpointUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := exchangeAgainst(t, slog.LevelInfo, tt.status, "application/json", tt.body)

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErr)
			}

			warn := rec.warn(t)
			if warn.message != tt.wantMessage {
				t.Errorf("WARN message = %q, want %q", warn.message, tt.wantMessage)
			}
			if got := warn.attrs["oauth_error"]; got != tt.wantCode {
				t.Errorf("oauth_error = %q, want %q", got, tt.wantCode)
			}
			if tt.wantCode != "" && !strings.Contains(err.Error(), tt.wantCode) {
				t.Errorf("error %q does not contain %q", err, tt.wantCode)
			}
		})
	}
}

// An "error" value that is not a short identifier is not reported: it could
// be free text or an echoed credential.
func TestExchange_Rejected_UnsafeErrorValue_IsNotReported(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"free text", "the refresh token rt-secret-123 is not valid"},
		{"log injection", "invalid_grant\\nlevel=ERROR msg=forged"},
		{"longer than an error code", strings.Repeat("a", 65)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"error":"` + tt.value + `","error_description":"no provider code here"}`
			rec, err := exchangeAgainst(t, slog.LevelInfo, http.StatusBadRequest, "application/json", body)

			warn := rec.warn(t)
			if got, ok := warn.attrs["oauth_error"]; ok {
				t.Errorf("oauth_error = %q, want it omitted", got)
			}
			if got, ok := warn.attrs["oauth_error_code"]; ok {
				t.Errorf("oauth_error_code = %q, want it omitted", got)
			}
			if strings.Contains(err.Error(), "oauth_error") {
				t.Errorf("error %q reports an unsafe oauth_error", err)
			}
			if warn.attrs["status"] != "400" {
				t.Errorf("status = %q, want 400", warn.attrs["status"])
			}
		})
	}
}

// A body that is not an OAuth2 error document (a gateway's HTML page) still
// produces the WARN entry, with the status only.
func TestExchange_Rejected_NonJSONBody_LogsStatusOnly(t *testing.T) {
	rec, err := exchangeAgainst(t, slog.LevelInfo, http.StatusBadRequest, "text/html",
		"<html><body>Bad Request: refresh_token=refresh-token-value</body></html>")

	warn := rec.warn(t)
	if _, ok := warn.attrs["oauth_error"]; ok {
		t.Errorf("oauth_error = %q, want it omitted", warn.attrs["oauth_error"])
	}
	if strings.Contains(rec.text()+err.Error(), "refresh-token-value") {
		t.Error("log or error contains the response body")
	}
}

func TestRejectionReason(t *testing.T) {
	tests := []struct {
		name             string
		in               OAuthErrorResponse
		wantCode         string
		wantProviderCode string
	}{
		{"empty", OAuthErrorResponse{}, "", ""},
		{"standard code only", OAuthErrorResponse{Error: "invalid_grant"}, "invalid_grant", ""},
		{
			"entra code at the start of the description",
			OAuthErrorResponse{Error: "invalid_grant", Description: "AADSTS700082: The refresh token has expired due to inactivity."},
			"invalid_grant", "AADSTS700082",
		},
		{
			"entra code without a standard code",
			OAuthErrorResponse{Description: "AADSTS90002: Tenant not found."},
			"", "AADSTS90002",
		},
		{
			"description without a provider code",
			OAuthErrorResponse{Error: "access_denied", Description: "insufficient scope"},
			"access_denied", "",
		},
		{
			"provider code must be a whole word",
			OAuthErrorResponse{Error: "invalid_grant", Description: "XAADSTS50076 AADSTS12"},
			"invalid_grant", "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, providerCode := rejectionReason(tt.in)
			if code != tt.wantCode || providerCode != tt.wantProviderCode {
				t.Errorf("rejectionReason() = (%q, %q), want (%q, %q)",
					code, providerCode, tt.wantCode, tt.wantProviderCode)
			}
		})
	}
}
