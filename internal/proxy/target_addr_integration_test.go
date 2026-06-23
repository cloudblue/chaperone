// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/internal/observability"
	"github.com/cloudblue/chaperone/sdk"
)

// =============================================================================
// Integration tests for the configurable target_addr field (LITE-34062).
//
// These tests verify the three log_target_addr modes end-to-end through the
// full middleware stack (allow-list, request logger, proxy handlers) and
// guard the cross-site consistency invariant — every log line that emits
// target_addr must produce identical content per request.
// =============================================================================

// targetAddrTestSetup runs a request through the full proxy stack with the
// given LogTargetAddrMode and returns the captured JSON log lines.
func targetAddrTestSetup(t *testing.T, mode observability.TargetAddrMode, target, requestURL string) []map[string]any {
	t.Helper()

	getLogs := captureLogsAt(t, &slog.HandlerOptions{Level: slog.LevelDebug})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	plugin := &mockPlugin{
		getCredentialsFn: func(_ context.Context, _ sdk.TransactionContext, _ *http.Request) (*sdk.Credential, error) {
			return &sdk.Credential{
				Headers:   map[string]string{"Authorization": "Bearer test"},
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.Plugin = plugin
	cfg.LogTargetAddrMode = mode
	srv := mustNewServerForTarget(t, cfg, target)
	handler := srv.Handler()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/proxy", nil)
	req.Header.Set("X-Connect-Target-URL", requestURL)
	req.Header.Set("X-Connect-Vendor-ID", "VA-test")
	req.Header.Set("X-Connect-Marketplace-ID", "MP-US")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d. body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	return parseJSONLogLines(t, []byte(getLogs()))
}

func parseJSONLogLines(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// targetAddrFromLogs collects all distinct non-empty target_addr values.
func targetAddrFromLogs(lines []map[string]any) []string {
	var values []string
	seen := map[string]struct{}{}
	for _, line := range lines {
		v, ok := line["target_addr"].(string)
		if !ok || v == "" {
			continue
		}
		if _, dup := seen[v]; !dup {
			seen[v] = struct{}{}
			values = append(values, v)
		}
	}
	return values
}

// joinedLog returns the entire buffer as a single string for substring asserts.
func joinedLog(lines []map[string]any) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		b, _ := json.Marshal(line)
		parts = append(parts, string(b))
	}
	return strings.Join(parts, "\n")
}

// TestLogTargetAddr_HostMode_OnlyAuthority verifies the default mode emits
// only the authority and never lets path/query/userinfo leak into any log line.
func TestLogTargetAddr_HostMode_OnlyAuthority(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	requestURL := backend.URL + "/v1/users/alice@example.com?api_key=SHOULD_NOT_APPEAR&token=SECRET"

	lines := targetAddrTestSetup(t, observability.TargetAddrModeHost, backend.URL, requestURL)

	addrs := targetAddrFromLogs(lines)
	if len(addrs) == 0 {
		t.Fatalf("no target_addr field found in any log line, got %d lines", len(lines))
	}
	for _, v := range addrs {
		if strings.Contains(v, "://") {
			t.Errorf("target_addr in host mode must not contain scheme, got %q", v)
		}
		if strings.Contains(v, "/") {
			t.Errorf("target_addr in host mode must not contain path, got %q", v)
		}
		if strings.Contains(v, "?") {
			t.Errorf("target_addr in host mode must not contain query, got %q", v)
		}
	}

	full := joinedLog(lines)
	for _, leak := range []string{"SHOULD_NOT_APPEAR", "SECRET", "alice@example.com", "/v1/users", "api_key", "token="} {
		if strings.Contains(full, leak) {
			t.Errorf("host mode log must not contain %q, got: %s", leak, full)
		}
	}
}

// TestLogTargetAddr_PathMode_StripsQuery verifies that path mode emits
// scheme+host+path while still stripping the query string.
func TestLogTargetAddr_PathMode_StripsQuery(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	requestURL := backend.URL + "/v1/users?api_key=SHOULD_NOT_APPEAR&token=SECRET"

	lines := targetAddrTestSetup(t, observability.TargetAddrModePath, backend.URL, requestURL)

	addrs := targetAddrFromLogs(lines)
	if len(addrs) == 0 {
		t.Fatalf("no target_addr field found, got %d lines", len(lines))
	}
	for _, v := range addrs {
		if !strings.Contains(v, "://") {
			t.Errorf("target_addr in path mode must contain scheme, got %q", v)
		}
		if !strings.Contains(v, "/v1/users") {
			t.Errorf("target_addr in path mode must contain path /v1/users, got %q", v)
		}
		if strings.Contains(v, "?") {
			t.Errorf("target_addr in path mode must not contain query, got %q", v)
		}
	}

	full := joinedLog(lines)
	for _, leak := range []string{"SHOULD_NOT_APPEAR", "SECRET", "api_key", "token="} {
		if strings.Contains(full, leak) {
			t.Errorf("path mode log must not contain %q (query value), got: %s", leak, full)
		}
	}
}

// TestLogTargetAddr_FullMode_KeepsQueryStripsUserinfo verifies that full
// mode emits the query string and ALWAYS strips userinfo.
func TestLogTargetAddr_FullMode_KeepsQueryStripsUserinfo(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	// Build a target URL with userinfo embedded.
	requestURL := strings.Replace(backend.URL, "://", "://USER:PASS_NEVER_IN_LOGS@", 1) + "/v1/users?key=val&token=APPEARS_HERE"

	lines := targetAddrTestSetup(t, observability.TargetAddrModeFull, backend.URL, requestURL)

	addrs := targetAddrFromLogs(lines)
	if len(addrs) == 0 {
		t.Fatalf("no target_addr field found, got %d lines", len(lines))
	}
	for _, v := range addrs {
		if !strings.Contains(v, "?key=val") {
			t.Errorf("target_addr in full mode must contain query, got %q", v)
		}
		if strings.Contains(v, "USER") || strings.Contains(v, "PASS_NEVER_IN_LOGS") {
			t.Errorf("target_addr in full mode must always strip userinfo, got %q", v)
		}
	}

	full := joinedLog(lines)
	if !strings.Contains(full, "APPEARS_HERE") {
		t.Errorf("full mode must include query value, got: %s", full)
	}
	for _, leak := range []string{"USER", "PASS_NEVER_IN_LOGS"} {
		if strings.Contains(full, leak) {
			t.Errorf("userinfo must NEVER appear in any log mode, found %q in: %s", leak, full)
		}
	}
}

// TestLogTargetAddr_AllSitesAgree guards the consistency invariant: for a
// single request, every log line that reports target_addr must contain the
// same string. A future regression that re-formats the field at one site
// (e.g. a new logger using a stale helper) is caught here.
func TestLogTargetAddr_AllSitesAgree(t *testing.T) {
	for _, mode := range []observability.TargetAddrMode{
		observability.TargetAddrModeHost,
		observability.TargetAddrModePath,
		observability.TargetAddrModeFull,
	} {
		t.Run(string(mode), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			lines := targetAddrTestSetup(t, mode, backend.URL, backend.URL+"/v1/users?key=val")

			addrs := targetAddrFromLogs(lines)
			if len(addrs) < 1 {
				t.Fatalf("expected at least one target_addr value, got 0")
			}
			if len(addrs) > 1 {
				t.Errorf("target_addr must be byte-identical across all log sites for a single request, got %d distinct values: %v",
					len(addrs), addrs)
			}

			// Sanity: at least the request_completed line and the upstream_response
			// line must be present.
			expectedMsgs := []string{"request completed", "upstream response"}
			for _, msg := range expectedMsgs {
				found := false
				for _, line := range lines {
					if line["msg"] == msg {
						if v, _ := line["target_addr"].(string); v == "" {
							t.Errorf("log line %q missing target_addr field", msg)
						}
						found = true
					}
				}
				if !found {
					t.Errorf("expected a log line with msg=%q, none found", msg)
				}
			}
		})
	}
}
