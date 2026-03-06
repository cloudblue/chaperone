// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateInstance_Success(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	inst, err := st.CreateInstance(context.Background(), "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	if inst.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if inst.Name != "proxy-1" {
		t.Errorf("Name = %q, want %q", inst.Name, "proxy-1")
	}
	if inst.Address != "10.0.0.1:9090" {
		t.Errorf("Address = %q, want %q", inst.Address, "10.0.0.1:9090")
	}
	if inst.Status != "unknown" {
		t.Errorf("Status = %q, want %q", inst.Status, "unknown")
	}
}

func TestCreateInstance_DuplicateAddress_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090"); err != nil {
		t.Fatalf("first CreateInstance() error = %v", err)
	}

	_, err := st.CreateInstance(ctx, "proxy-2", "10.0.0.1:9090")
	if !errors.Is(err, ErrDuplicateAddress) {
		t.Errorf("error = %v, want %v", err, ErrDuplicateAddress)
	}
}

func TestGetInstance_Exists_ReturnsInstance(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	got, err := st.GetInstance(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Name != "proxy-1" {
		t.Errorf("Name = %q, want %q", got.Name, "proxy-1")
	}
}

func TestGetInstance_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	_, err := st.GetInstance(context.Background(), 999)
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("error = %v, want %v", err, ErrInstanceNotFound)
	}
}

func TestListInstances_Empty_ReturnsNil(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	instances, err := st.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances() error = %v", err)
	}
	if instances != nil {
		t.Errorf("expected nil, got %v", instances)
	}
}

func TestListInstances_Multiple_ReturnsSortedByName(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if _, err := st.CreateInstance(ctx, name, name+":9090"); err != nil {
			t.Fatalf("CreateInstance(%q) error = %v", name, err)
		}
	}

	instances, err := st.ListInstances(ctx)
	if err != nil {
		t.Fatalf("ListInstances() error = %v", err)
	}
	if len(instances) != 3 {
		t.Fatalf("len = %d, want 3", len(instances))
	}

	want := []string{"alpha", "bravo", "charlie"}
	for i, inst := range instances {
		if inst.Name != want[i] {
			t.Errorf("instances[%d].Name = %q, want %q", i, inst.Name, want[i])
		}
	}
}

func TestUpdateInstance_Success(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateInstance(ctx, "old-name", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	updated, err := st.UpdateInstance(ctx, created.ID, "new-name", "10.0.0.2:9090")
	if err != nil {
		t.Fatalf("UpdateInstance() error = %v", err)
	}

	if updated.Name != "new-name" {
		t.Errorf("Name = %q, want %q", updated.Name, "new-name")
	}
	if updated.Address != "10.0.0.2:9090" {
		t.Errorf("Address = %q, want %q", updated.Address, "10.0.0.2:9090")
	}
}

func TestUpdateInstance_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	_, err := st.UpdateInstance(context.Background(), 999, "name", "addr:9090")
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("error = %v, want %v", err, ErrInstanceNotFound)
	}
}

func TestUpdateInstance_DuplicateAddress_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090"); err != nil {
		t.Fatalf("CreateInstance(proxy-1) error = %v", err)
	}
	inst2, err := st.CreateInstance(ctx, "proxy-2", "10.0.0.2:9090")
	if err != nil {
		t.Fatalf("CreateInstance(proxy-2) error = %v", err)
	}

	_, err = st.UpdateInstance(ctx, inst2.ID, "proxy-2", "10.0.0.1:9090")
	if !errors.Is(err, ErrDuplicateAddress) {
		t.Errorf("error = %v, want %v", err, ErrDuplicateAddress)
	}
}

func TestDeleteInstance_Success(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	if err := st.DeleteInstance(ctx, created.ID); err != nil {
		t.Fatalf("DeleteInstance() error = %v", err)
	}

	_, err = st.GetInstance(ctx, created.ID)
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("after delete: error = %v, want %v", err, ErrInstanceNotFound)
	}
}

func TestDeleteInstance_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)

	err := st.DeleteInstance(context.Background(), 999)
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("error = %v, want %v", err, ErrInstanceNotFound)
	}
}

func TestSetInstanceHealthy_UpdatesStatusAndVersion(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	if err := st.SetInstanceHealthy(ctx, created.ID, "1.2.3"); err != nil {
		t.Fatalf("SetInstanceHealthy() error = %v", err)
	}

	got, err := st.GetInstance(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Status != "healthy" {
		t.Errorf("Status = %q, want %q", got.Status, "healthy")
	}
	if got.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", got.Version, "1.2.3")
	}
	if got.LastSeenAt == nil {
		t.Error("LastSeenAt should not be nil after healthy poll")
	}
}

func TestSetInstanceUnreachable_UpdatesStatus(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	created, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	if err := st.SetInstanceUnreachable(ctx, created.ID); err != nil {
		t.Fatalf("SetInstanceUnreachable() error = %v", err)
	}

	got, err := st.GetInstance(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetInstance() error = %v", err)
	}
	if got.Status != "unreachable" {
		t.Errorf("Status = %q, want %q", got.Status, "unreachable")
	}
}
