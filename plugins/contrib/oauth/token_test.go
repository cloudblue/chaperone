// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestNewTokenManager_NilLogger_LazyResolution(t *testing.T) {
	tm := newTokenManager("https://example.com/token", nil, func(_ context.Context) (*cachedToken, error) {
		return nil, nil
	})

	// logger field must be nil — no eager slog.Default() at construction.
	if tm.logger != nil {
		t.Error("logger field should be nil when not provided; lazy resolution via log()")
	}
	if tm.log() != slog.Default() {
		t.Error("log() should return slog.Default() when logger is nil")
	}
}

func TestNewTokenManager_ExplicitLogger_UsesExplicitLogger(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	tm := newTokenManager("https://example.com/token", custom, func(_ context.Context) (*cachedToken, error) {
		return nil, nil
	})

	if tm.log() != custom {
		t.Error("log() should return the explicitly provided logger")
	}
}
