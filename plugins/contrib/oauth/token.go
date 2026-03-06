// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// cachedToken holds a fetched access token and its computed expiration.
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// tokenManager handles caching and deduplicating token requests.
// The actual fetch logic is delegated to a grant-type-specific function.
type tokenManager struct {
	tokenURL  string
	logger    *slog.Logger
	fetchFunc func(ctx context.Context) (*cachedToken, error)

	mu    sync.RWMutex
	token *cachedToken

	group singleflight.Group
}

func newTokenManager(
	tokenURL string,
	logger *slog.Logger,
	fetchFunc func(context.Context) (*cachedToken, error),
) *tokenManager {
	return &tokenManager{
		tokenURL:  tokenURL,
		logger:    logger,
		fetchFunc: fetchFunc,
	}
}

// getToken returns a valid access token, fetching a new one if needed.
// Concurrent callers are deduplicated via singleflight.
func (tm *tokenManager) getToken(ctx context.Context) (string, time.Time, error) {
	tm.mu.RLock()
	if tm.token != nil && time.Now().Before(tm.token.expiresAt) {
		t := tm.token
		tm.mu.RUnlock()
		tm.logger.LogAttrs(ctx, slog.LevelDebug, "token cache hit",
			slog.String("token_url", tm.tokenURL))
		return t.accessToken, t.expiresAt, nil
	}
	tm.mu.RUnlock()

	tm.logger.LogAttrs(ctx, slog.LevelDebug, "token cache miss",
		slog.String("token_url", tm.tokenURL))

	// Use context.WithoutCancel so that a single caller's cancellation
	// does not poison all coalesced singleflight waiters.
	result, err, _ := tm.group.Do("token", func() (any, error) {
		cached, fetchErr := tm.fetchFunc(context.WithoutCancel(ctx))
		if fetchErr != nil {
			return nil, fetchErr
		}

		tm.mu.Lock()
		tm.token = cached
		tm.mu.Unlock()

		tm.logger.LogAttrs(ctx, slog.LevelDebug, "token fetched",
			slog.String("token_url", tm.tokenURL),
			slog.Time("expires_at", cached.expiresAt))

		return cached, nil
	})
	if err != nil {
		return "", time.Time{}, err
	}

	t, ok := result.(*cachedToken)
	if !ok {
		return "", time.Time{}, fmt.Errorf("unexpected singleflight result type %T", result)
	}
	return t.accessToken, t.expiresAt, nil
}
