// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cloudblue/chaperone/admin/store"
)

func newAuditTestMux(t *testing.T) (*http.ServeMux, *store.Store) {
	t.Helper()
	st := openTestStore(t)
	h := NewAuditHandler(st)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, st
}

func seedAuditData(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "admin", "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	inst, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	entries := []struct {
		action     string
		instanceID *int64
		detail     string
	}{
		{"instance.create", &inst.ID, "Created instance proxy-1 at 10.0.0.1:9090"},
		{"instance.update", &inst.ID, "Updated instance proxy-1"},
		{"user.login", nil, "User admin logged in"},
	}
	for _, e := range entries {
		if err := st.InsertAuditEntry(ctx, user.ID, e.action, e.instanceID, e.detail); err != nil {
			t.Fatalf("InsertAuditEntry() error = %v", err)
		}
	}
	return user.ID
}

func TestAuditList_Empty_ReturnsEmptyPage(t *testing.T) {
	t.Parallel()
	mux, _ := newAuditTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var page store.AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("Total = %d, want 0", page.Total)
	}
	if len(page.Items) != 0 {
		t.Errorf("len(Items) = %d, want 0", len(page.Items))
	}
}

func TestAuditList_ReturnsEntries(t *testing.T) {
	t.Parallel()
	mux, st := newAuditTestMux(t)
	seedAuditData(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var page store.AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want 3", page.Total)
	}
	if len(page.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(page.Items))
	}
}

func TestAuditList_FilterByAction(t *testing.T) {
	t.Parallel()
	mux, st := newAuditTestMux(t)
	seedAuditData(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?action=user.login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var page store.AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestAuditList_FilterByUser(t *testing.T) {
	t.Parallel()
	mux, st := newAuditTestMux(t)
	userID := seedAuditData(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?user="+itoa(userID), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var page store.AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want 3", page.Total)
	}
}

func TestAuditList_FullTextSearch(t *testing.T) {
	t.Parallel()
	mux, st := newAuditTestMux(t)
	seedAuditData(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?q=proxy-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var page store.AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	// "proxy-1" appears in instance.create and instance.update details.
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2", page.Total)
	}
}

func TestAuditList_Pagination(t *testing.T) {
	t.Parallel()
	mux, st := newAuditTestMux(t)
	seedAuditData(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?page=1&per_page=2", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var page store.AuditPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want 3", page.Total)
	}
	if len(page.Items) != 2 {
		t.Errorf("len(Items) = %d, want 2", len(page.Items))
	}
	if page.Page != 1 {
		t.Errorf("Page = %d, want 1", page.Page)
	}
}

func TestAuditList_InvalidPage_Returns400(t *testing.T) {
	t.Parallel()
	mux, _ := newAuditTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?page=abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuditList_InvalidPerPage_Returns400(t *testing.T) {
	t.Parallel()
	mux, _ := newAuditTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?per_page=999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuditList_InvalidUserID_Returns400(t *testing.T) {
	t.Parallel()
	mux, _ := newAuditTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?user=notanumber", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuditList_InvalidFromDate_Returns400(t *testing.T) {
	t.Parallel()
	mux, _ := newAuditTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/audit?from=not-a-date", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
