// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/admin/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open(%q) failed: %v", dbPath, err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func newTestHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	st := openTestStore(t)
	h := NewInstanceHandler(st, 2*time.Second)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestListInstances_Empty_ReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %s, want []", body)
	}
}

func TestCreateInstance_Success_Returns201(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"name":"proxy-1","address":"10.0.0.1:9090"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var inst store.Instance
	if err := json.NewDecoder(rec.Body).Decode(&inst); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if inst.Name != "proxy-1" {
		t.Errorf("Name = %q, want %q", inst.Name, "proxy-1")
	}
	if inst.Address != "10.0.0.1:9090" {
		t.Errorf("Address = %q, want %q", inst.Address, "10.0.0.1:9090")
	}
}

func TestCreateInstance_DuplicateAddress_Returns409(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"name":"proxy-1","address":"10.0.0.1:9090"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Second create with same address.
	req = httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	assertErrorCode(t, rec, "DUPLICATE_ADDRESS")
}

func TestCreateInstance_MissingName_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"name":"","address":"10.0.0.1:9090"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	assertErrorCode(t, rec, "VALIDATION_ERROR")
}

func TestCreateInstance_MissingAddress_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"name":"proxy-1","address":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateInstance_InvalidJSON_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetInstance_Exists_Returns200(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	// Create first.
	body := `{"name":"proxy-1","address":"10.0.0.1:9090"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var created store.Instance
	json.NewDecoder(rec.Body).Decode(&created)

	// Get by ID.
	req = httptest.NewRequest(http.MethodGet, "/api/instances/1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetInstance_NotFound_Returns404(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instances/999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	assertErrorCode(t, rec, "INSTANCE_NOT_FOUND")
}

func TestGetInstance_InvalidID_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/instances/abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateInstance_Success_Returns200(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	// Create.
	create := `{"name":"proxy-1","address":"10.0.0.1:9090"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(create))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Update.
	update := `{"name":"proxy-1-updated","address":"10.0.0.2:9090"}`
	req = httptest.NewRequest(http.MethodPut, "/api/instances/1", strings.NewReader(update))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var inst store.Instance
	json.NewDecoder(rec.Body).Decode(&inst)
	if inst.Name != "proxy-1-updated" {
		t.Errorf("Name = %q, want %q", inst.Name, "proxy-1-updated")
	}
}

func TestDeleteInstance_Success_Returns204(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	// Create.
	create := `{"name":"proxy-1","address":"10.0.0.1:9090"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(create))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/api/instances/1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify gone.
	req = httptest.NewRequest(http.MethodGet, "/api/instances/1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("after delete: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteInstance_NotFound_Returns404(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/instances/999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestTestConnection_Success(t *testing.T) {
	t.Parallel()

	// Start a fake proxy admin server.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/_ops/health":
			w.Write([]byte(`{"status":"alive"}`))
		case "/_ops/version":
			w.Write([]byte(`{"version":"1.2.3"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer proxy.Close()

	mux := newTestHandler(t)
	addr := strings.TrimPrefix(proxy.URL, "http://")
	body := `{"address":"` + addr + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/instances/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	json.NewDecoder(rec.Body).Decode(&result)

	if !result.OK {
		t.Error("expected ok=true")
	}
	if result.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", result.Version, "1.2.3")
	}
}

func TestTestConnection_Unreachable(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"address":"127.0.0.1:1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	json.NewDecoder(rec.Body).Decode(&result)

	if result.OK {
		t.Error("expected ok=false for unreachable address")
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestTestConnection_EmptyAddress_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"address":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateInstance_InvalidAddress_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	tests := []struct {
		name    string
		address string
	}{
		{"no port", "not-a-host-port"},
		{"empty host", ":9090"},
		{"non-numeric port", "example.com:abc"},
		{"port zero", "example.com:0"},
		{"port too large", "example.com:70000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"name":"proxy-1","address":"` + tt.address + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("address %q: status = %d, want %d", tt.address, rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestTestConnection_InvalidAddress_Returns400(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	tests := []struct {
		name    string
		address string
	}{
		{"no port", "no-port-here"},
		{"empty host", ":9090"},
		{"non-numeric port", "example.com:abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"address":"` + tt.address + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/instances/test", strings.NewReader(body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("address %q: status = %d, want %d", tt.address, rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCreateInstance_WhitespaceTrimmed(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	body := `{"name":"  proxy-1  ","address":"  10.0.0.1:9090  "}`
	req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var inst store.Instance
	if err := json.NewDecoder(rec.Body).Decode(&inst); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if inst.Name != "proxy-1" {
		t.Errorf("Name = %q, want %q (should be trimmed)", inst.Name, "proxy-1")
	}
	if inst.Address != "10.0.0.1:9090" {
		t.Errorf("Address = %q, want %q (should be trimmed)", inst.Address, "10.0.0.1:9090")
	}
}

func TestListInstances_AfterCreate_ReturnsInstances(t *testing.T) {
	t.Parallel()
	mux := newTestHandler(t)

	// Create two instances.
	for _, name := range []string{"alpha", "bravo"} {
		body := `{"name":"` + name + `","address":"` + name + `:9090"}`
		req := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s: status = %d", name, rec.Code)
		}
	}

	// List.
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var instances []store.Instance
	json.NewDecoder(rec.Body).Decode(&instances)
	if len(instances) != 2 {
		t.Errorf("len = %d, want 2", len(instances))
	}
}

// assertErrorCode checks that the response body contains the expected error code.
func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}
	if resp.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q", resp.Error.Code, wantCode)
	}
}
