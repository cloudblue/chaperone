// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"testing"
	"time"
)

func TestRing_PushAndLen(t *testing.T) {
	t.Parallel()
	r := NewRing(5)

	if r.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", r.Len())
	}

	for i := 0; i < 3; i++ {
		r.Push(Snapshot{Time: time.Unix(int64(i), 0)})
	}
	if r.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", r.Len())
	}
}

func TestRing_At_ReturnsOldestFirst(t *testing.T) {
	t.Parallel()
	r := NewRing(5)

	for i := 0; i < 3; i++ {
		r.Push(Snapshot{Time: time.Unix(int64(i+10), 0)})
	}

	if got := r.At(0).Time.Unix(); got != 10 {
		t.Errorf("At(0).Time = %d, want 10", got)
	}
	if got := r.At(2).Time.Unix(); got != 12 {
		t.Errorf("At(2).Time = %d, want 12", got)
	}
}

func TestRing_Wraparound_OverwritesOldest(t *testing.T) {
	t.Parallel()
	r := NewRing(3)

	for i := 0; i < 5; i++ {
		r.Push(Snapshot{Time: time.Unix(int64(i), 0)})
	}

	if r.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", r.Len())
	}
	// After pushing 0,1,2,3,4 into capacity 3, oldest should be 2.
	if got := r.At(0).Time.Unix(); got != 2 {
		t.Errorf("At(0).Time = %d, want 2 (oldest after wrap)", got)
	}
	if got := r.At(2).Time.Unix(); got != 4 {
		t.Errorf("At(2).Time = %d, want 4 (newest)", got)
	}
}

func TestRing_Last_ReturnsNewest(t *testing.T) {
	t.Parallel()
	r := NewRing(5)

	_, ok := r.Last()
	if ok {
		t.Error("Last() on empty ring should return false")
	}

	r.Push(Snapshot{Time: time.Unix(100, 0)})
	r.Push(Snapshot{Time: time.Unix(200, 0)})

	last, ok := r.Last()
	if !ok {
		t.Fatal("Last() returned false on non-empty ring")
	}
	if last.Time.Unix() != 200 {
		t.Errorf("Last().Time = %d, want 200", last.Time.Unix())
	}
}

func TestRing_At_PanicsOnOutOfRange(t *testing.T) {
	t.Parallel()
	r := NewRing(3)
	r.Push(Snapshot{})

	defer func() {
		if recover() == nil {
			t.Error("expected panic for out of range index")
		}
	}()
	r.At(1) // only 1 element, index 1 is out of range
}
