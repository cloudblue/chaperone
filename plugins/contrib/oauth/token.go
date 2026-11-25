// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/cloudblue/chaperone/plugins/contrib/internal/oauthutil"
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
	// discardedAt is when a cached token was last discarded because the
	// target rejected it. Zero if that never happened.
	discardedAt time.Time

	group singleflight.Group

	// now returns the current time. It is time.Now except in tests.
	now func() time.Time
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
		now:       time.Now,
	}
}

// log returns the configured logger, or slog.Default() if none was set.
// Called at log-emit time so the current global default is always used
// when no explicit logger is provided.
func (tm *tokenManager) log() *slog.Logger {
	if tm.logger != nil {
		return tm.logger
	}
	return slog.Default()
}

// getToken returns a valid access token, fetching a new one if needed.
// Concurrent callers are deduplicated via singleflight.
func (tm *tokenManager) getToken(ctx context.Context) (string, time.Time, error) {
	tm.mu.RLock()
	if tm.token != nil && time.Now().Before(tm.token.expiresAt) {
		t := tm.token
		tm.mu.RUnlock()
		tm.log().LogAttrs(ctx, slog.LevelDebug, "token cache hit",
			slog.String("token_url", tm.tokenURL))
		return t.accessToken, t.expiresAt, nil
	}
	tm.mu.RUnlock()

	tm.log().LogAttrs(ctx, slog.LevelDebug, "token cache miss",
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

		tm.log().LogAttrs(ctx, slog.LevelDebug, "token fetched",
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

// discardRejected drops the cached token when resp is a 401 from the target
// for a request that was sent with that token, so the next getToken call
// fetches a new one.
//
// A 401 for any other token is ignored: it belongs to a request that was
// already in flight when the cache was refreshed. Discards are limited to one
// per [oauthutil.RejectionCooldown], so a target that keeps answering 401 to
// fresh tokens cannot force a token request per proxied request.
func (tm *tokenManager) discardRejected(ctx context.Context, resp *http.Response) {
	rejected, ok := oauthutil.RejectedBearerToken(resp)
	if !ok {
		return
	}

	now := tm.now()

	tm.mu.Lock()
	if tm.token == nil || !oauthutil.SameToken(tm.token.accessToken, rejected) {
		tm.mu.Unlock()
		return
	}
	if oauthutil.InCooldown(tm.discardedAt, now) {
		tm.mu.Unlock()
		tm.log().LogAttrs(ctx, slog.LevelDebug, "target rejected the cached access token again; keeping it until the cooldown ends",
			slog.String("token_url", tm.tokenURL),
			slog.Duration("cooldown", oauthutil.RejectionCooldown))
		return
	}
	tm.token = nil
	tm.discardedAt = now
	tm.mu.Unlock()

	tm.log().LogAttrs(ctx, slog.LevelWarn, "target rejected the cached access token; discarded, the next request fetches a new one",
		slog.String("token_url", tm.tokenURL),
		slog.Int("status", resp.StatusCode))
}
