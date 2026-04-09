// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"
)

func createTestUser(t *testing.T, st *Store) int64 {
	t.Helper()
	user, err := st.CreateUser(context.Background(), "testuser", "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return user.ID
}

func createTestInstance(t *testing.T, st *Store) int64 {
	t.Helper()
	inst, err := st.CreateInstance(context.Background(), "test-proxy", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	return inst.ID
}

func TestInsertAuditEntry_Success(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)
	instID := createTestInstance(t, st)

	err := st.InsertAuditEntry(ctx, userID, "instance.create", &instID, "Created instance test-proxy at 10.0.0.1:9090")
	if err != nil {
		t.Fatalf("InsertAuditEntry() error = %v", err)
	}
}

func TestInsertAuditEntry_NilInstanceID(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	err := st.InsertAuditEntry(ctx, userID, "user.login", nil, "User logged in")
	if err != nil {
		t.Fatalf("InsertAuditEntry() error = %v", err)
	}
}

func TestListAuditEntries_Empty_ReturnsEmptyPage(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	page, err := st.ListAuditEntries(ctx, AuditFilter{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 0 {
		t.Errorf("Total = %d, want 0", page.Total)
	}
	if len(page.Items) != 0 {
		t.Errorf("len(Items) = %d, want 0", len(page.Items))
	}
	if page.Page != 1 {
		t.Errorf("Page = %d, want 1", page.Page)
	}
}

func TestListAuditEntries_ReturnsEntriesOrderedByCreatedAtDesc(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	actions := []string{"user.login", "instance.create", "instance.delete"}
	for _, action := range actions {
		if err := st.InsertAuditEntry(ctx, userID, action, nil, "detail for "+action); err != nil {
			t.Fatalf("InsertAuditEntry(%s) error = %v", action, err)
		}
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("Total = %d, want 3", page.Total)
	}

	// Most recent first.
	if page.Items[0].Action != "instance.delete" {
		t.Errorf("Items[0].Action = %q, want %q", page.Items[0].Action, "instance.delete")
	}
	if page.Items[2].Action != "user.login" {
		t.Errorf("Items[2].Action = %q, want %q", page.Items[2].Action, "user.login")
	}
}

func TestListAuditEntries_JoinsUsername(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	if err := st.InsertAuditEntry(ctx, userID, "user.login", nil, "Login"); err != nil {
		t.Fatalf("InsertAuditEntry() error = %v", err)
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Items[0].Username != "testuser" {
		t.Errorf("Username = %q, want %q", page.Items[0].Username, "testuser")
	}
}

func TestListAuditEntries_FilterByAction(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	for _, action := range []string{"user.login", "instance.create", "user.login"} {
		if err := st.InsertAuditEntry(ctx, userID, action, nil, "detail"); err != nil {
			t.Fatalf("InsertAuditEntry() error = %v", err)
		}
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{Action: "user.login", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2", page.Total)
	}
	for _, item := range page.Items {
		if item.Action != "user.login" {
			t.Errorf("Action = %q, want %q", item.Action, "user.login")
		}
	}
}

func TestListAuditEntries_FilterByUserID(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	user2, createErr := st.CreateUser(ctx, "other", "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234")
	if createErr != nil {
		t.Fatalf("CreateUser() error = %v", createErr)
	}

	if insertErr := st.InsertAuditEntry(ctx, userID, "user.login", nil, "u1"); insertErr != nil {
		t.Fatal(insertErr)
	}
	if insertErr := st.InsertAuditEntry(ctx, user2.ID, "user.login", nil, "u2"); insertErr != nil {
		t.Fatal(insertErr)
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{UserID: &userID, Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestListAuditEntries_FilterByInstanceID(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)
	instID := createTestInstance(t, st)

	if err := st.InsertAuditEntry(ctx, userID, "instance.create", &instID, "with instance"); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertAuditEntry(ctx, userID, "user.login", nil, "without instance"); err != nil {
		t.Fatal(err)
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{InstanceID: &instID, Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestListAuditEntries_FilterByDateRange(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	// Insert entries with explicit timestamps via raw SQL.
	for _, ts := range []string{"2026-01-01 00:00:00", "2026-02-01 00:00:00", "2026-03-01 00:00:00"} {
		_, err := st.db.ExecContext(ctx,
			`INSERT INTO audit_log (user_id, action, detail, created_at) VALUES (?, 'user.login', ?, ?)`,
			userID, "entry at "+ts, ts)
		if err != nil {
			t.Fatalf("inserting audit entry: %v", err)
		}
	}

	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)

	page, err := st.ListAuditEntries(ctx, AuditFilter{From: &from, To: &to, Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestListAuditEntries_FullTextSearch(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	entries := []struct{ action, detail string }{
		{"instance.create", "Created instance production-proxy at 10.0.0.1:9090"},
		{"instance.create", "Created instance staging-proxy at 10.0.0.2:9090"},
		{"user.login", "User logged in from 192.168.1.1"},
	}
	for _, e := range entries {
		if err := st.InsertAuditEntry(ctx, userID, e.action, nil, e.detail); err != nil {
			t.Fatalf("InsertAuditEntry() error = %v", err)
		}
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{Query: "production", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
	if page.Total > 0 && page.Items[0].Detail != "Created instance production-proxy at 10.0.0.1:9090" {
		t.Errorf("Detail = %q, unexpected", page.Items[0].Detail)
	}
}

func TestListAuditEntries_Pagination(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	for i := 0; i < 5; i++ {
		if err := st.InsertAuditEntry(ctx, userID, "user.login", nil, "entry"); err != nil {
			t.Fatal(err)
		}
	}

	page1, err := st.ListAuditEntries(ctx, AuditFilter{Page: 1, PerPage: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(page1.Items))
	}
	if page1.Total != 5 {
		t.Errorf("page 1 Total = %d, want 5", page1.Total)
	}

	page3, err := st.ListAuditEntries(ctx, AuditFilter{Page: 3, PerPage: 2})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3.Items) != 1 {
		t.Errorf("page 3 len = %d, want 1", len(page3.Items))
	}
}

func TestListAuditEntries_CombinedFilters(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)
	instID := createTestInstance(t, st)

	if err := st.InsertAuditEntry(ctx, userID, "instance.create", &instID, "Created production-proxy"); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertAuditEntry(ctx, userID, "instance.delete", &instID, "Deleted production-proxy"); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertAuditEntry(ctx, userID, "user.login", nil, "Login"); err != nil {
		t.Fatal(err)
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{
		Action:     "instance.create",
		InstanceID: &instID,
		Query:      "production",
		Page:       1,
		PerPage:    20,
	})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestDeleteAuditEntriesBefore_DeletesOldEntries(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()
	userID := createTestUser(t, st)

	// Insert old and new entries via raw SQL for controlled timestamps.
	_, insertErr := st.db.ExecContext(ctx,
		`INSERT INTO audit_log (user_id, action, detail, created_at) VALUES (?, 'user.login', 'old', '2025-01-01 00:00:00')`,
		userID)
	if insertErr != nil {
		t.Fatal(insertErr)
	}
	if recentErr := st.InsertAuditEntry(ctx, userID, "user.login", nil, "recent"); recentErr != nil {
		t.Fatal(recentErr)
	}

	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deleted, err := st.DeleteAuditEntriesBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeleteAuditEntriesBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	page, err := st.ListAuditEntries(ctx, AuditFilter{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestFtsQuote_EscapesSpecialCharacters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"simple", `"simple"`},
		{"proxy-1", `"proxy-1"`},
		{"", `""`},
		{`has "quotes"`, `"has ""quotes"""`},
		{`double "" already`, `"double """" already"`},
		{"AND OR NOT", `"AND OR NOT"`},
		{"prefix*", `"prefix*"`},
		{"NEAR/2", `"NEAR/2"`},
		{`back\slash`, `"back\slash"`},
	}
	for _, tt := range tests {
		got := ftsQuote(tt.input)
		if got != tt.want {
			t.Errorf("ftsQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDeleteAuditEntriesBefore_NothingToDelete(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	deleted, err := st.DeleteAuditEntriesBefore(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("DeleteAuditEntriesBefore() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}
