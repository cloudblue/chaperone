// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"sync"
	"time"
)

// RateLimiter tracks failed login attempts per IP using a sliding window.
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time
	maxAttempts int
	window      time.Duration
	now         func() time.Time // injectable clock for testing
}

// NewRateLimiter creates a rate limiter that allows maxAttempts failures
// within the given window duration per IP.
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
		now:         time.Now,
	}
}

// Allow returns true if the IP has not exceeded the failure limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.prune(ip)
	return len(rl.attempts[ip]) < rl.maxAttempts
}

// Record logs a failed login attempt for the given IP.
func (rl *RateLimiter) Record(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.attempts[ip] = append(rl.attempts[ip], rl.now())
}

// Reset clears the failure counter for an IP (called on successful login).
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.attempts, ip)
}

// prune removes attempts older than the sliding window. Must be called under lock.
func (rl *RateLimiter) prune(ip string) {
	attempts := rl.attempts[ip]
	if len(attempts) == 0 {
		return
	}

	cutoff := rl.now().Add(-rl.window)
	i := 0
	for i < len(attempts) && attempts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		rl.attempts[ip] = attempts[i:]
	}
	if len(rl.attempts[ip]) == 0 {
		delete(rl.attempts, ip)
	}
}

// Sweep removes all expired entries across all IPs.
// Call periodically from a background goroutine to prevent unbounded growth
// from IPs that record failures but never return.
func (rl *RateLimiter) Sweep() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := rl.now().Add(-rl.window)
	for ip, attempts := range rl.attempts {
		i := 0
		for i < len(attempts) && attempts[i].Before(cutoff) {
			i++
		}
		if i == len(attempts) {
			delete(rl.attempts, ip)
		} else if i > 0 {
			rl.attempts[ip] = attempts[i:]
		}
	}
}
