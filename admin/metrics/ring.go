// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

// Ring is a fixed-capacity circular buffer of Snapshots.
// It is not safe for concurrent use; the Collector handles synchronization.
type Ring struct {
	data  []Snapshot
	write int // next write position
	count int // number of stored snapshots
}

// NewRing creates a Ring with the given capacity.
func NewRing(capacity int) *Ring {
	return &Ring{data: make([]Snapshot, capacity)}
}

// Push adds a snapshot, overwriting the oldest if at capacity.
func (r *Ring) Push(s Snapshot) {
	r.data[r.write] = s
	r.write = (r.write + 1) % len(r.data)
	if r.count < len(r.data) {
		r.count++
	}
}

// Len returns the number of stored snapshots.
func (r *Ring) Len() int { return r.count }

// At returns the i-th snapshot where 0 is the oldest.
// Panics if i is out of range.
func (r *Ring) At(i int) Snapshot {
	if i < 0 || i >= r.count {
		panic("ring: index out of range")
	}
	start := (r.write - r.count + len(r.data)) % len(r.data)
	return r.data[(start+i)%len(r.data)]
}

// Last returns the most recent snapshot and true, or a zero Snapshot and false
// if the ring is empty.
func (r *Ring) Last() (Snapshot, bool) {
	if r.count == 0 {
		return Snapshot{}, false
	}
	return r.At(r.count - 1), true
}
