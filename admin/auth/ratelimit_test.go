// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(3, time.Minute)

	for i := range 3 {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		rl.Record("1.2.3.4")
	}
}

func TestRateLimiter_BlocksAtLimit(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(3, time.Minute)

	for range 3 {
		rl.Record("1.2.3.4")
	}

	if rl.Allow("1.2.3.4") {
		t.Error("should be blocked after 3 failures")
	}
}

func TestRateLimiter_DifferentIPs_Independent(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(2, time.Minute)

	rl.Record("1.1.1.1")
	rl.Record("1.1.1.1")

	if rl.Allow("1.1.1.1") {
		t.Error("1.1.1.1 should be blocked")
	}
	if !rl.Allow("2.2.2.2") {
		t.Error("2.2.2.2 should be allowed (separate IP)")
	}
}

func TestRateLimiter_ResetClearsCounter(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(2, time.Minute)

	rl.Record("1.2.3.4")
	rl.Record("1.2.3.4")
	rl.Reset("1.2.3.4")

	if !rl.Allow("1.2.3.4") {
		t.Error("should be allowed after reset")
	}
}

func TestRateLimiter_SlidingWindow_PrunesOldAttempts(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(2, time.Minute)

	now := time.Now()
	rl.now = func() time.Time { return now }

	rl.Record("1.2.3.4")
	rl.Record("1.2.3.4")

	// Advance past the window.
	rl.now = func() time.Time { return now.Add(61 * time.Second) }

	if !rl.Allow("1.2.3.4") {
		t.Error("old attempts should be pruned; IP should be allowed")
	}
}

func TestRateLimiter_PartialPrune_KeepsRecentAttempts(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(2, time.Minute)

	now := time.Now()
	rl.now = func() time.Time { return now }
	rl.Record("1.2.3.4") // t=0

	rl.now = func() time.Time { return now.Add(50 * time.Second) }
	rl.Record("1.2.3.4") // t=50s

	// At t=61s, the first attempt is pruned but the second is still within window.
	rl.now = func() time.Time { return now.Add(61 * time.Second) }

	if !rl.Allow("1.2.3.4") {
		t.Error("should be allowed (only 1 recent attempt after prune)")
	}

	rl.Record("1.2.3.4") // second recent attempt

	if rl.Allow("1.2.3.4") {
		t.Error("should be blocked (2 recent attempts)")
	}
}
