// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package telemetry

import "testing"

// ResetMetrics resets all metrics for test isolation.
// This function is exported for use by integration tests in other packages.
//
// Usage:
//
//	func TestSomething(t *testing.T) {
//	    telemetry.ResetMetrics(t)
//	    // ... test code ...
//	}
//
// NOTE: Tests using this must NOT use t.Parallel() because they share
// global Prometheus metrics.
func ResetMetrics(t *testing.T) {
	RequestsTotal.Reset()
	RequestDuration.Reset()
	UpstreamDuration.Reset()
	ActiveConnections.Set(0)

	t.Cleanup(func() {
		RequestsTotal.Reset()
		RequestDuration.Reset()
		UpstreamDuration.Reset()
		ActiveConnections.Set(0)
	})
}
