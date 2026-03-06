// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package microsoft

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/cloudblue/chaperone/plugins/contrib"
	"github.com/cloudblue/chaperone/plugins/contrib/internal/oauthutil"
	"github.com/cloudblue/chaperone/sdk"
)

// validTenantID matches Azure AD tenant identifiers: GUIDs, domain names
// (alphanumeric with dots and hyphens), or the literal "common"/"organizations"/
// "consumers". It rejects path separators, query strings, and fragments.
var validTenantID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*$`)

const (
	// defaultTokenEndpoint is the public Azure AD v1 token endpoint.
	defaultTokenEndpoint = "https://login.microsoftonline.com" // #nosec G101 -- URL endpoint, not a credential

	// defaultMaxPoolSize is the maximum number of per-tenant entries kept in
	// the LRU pool. Each entry owns a singleflight group and a per-resource
	// access token cache.
	defaultMaxPoolSize = 10_000

	// defaultExpiryMargin matches the Python connector's 300-second margin.
	defaultExpiryMargin = 5 * time.Minute

	// maxResourcesPerTenant is the target bound for the per-resource access
	// token cache within each tenant entry. When the cache reaches this size,
	// expired entries are purged before inserting. If all entries are still
	// valid, the new entry is inserted anyway (soft bound) — well-known
	// Microsoft resources (Graph, Management, Partner Center) are a small,
	// fixed set, so exceeding 100 in practice requires adversarial input.
	maxResourcesPerTenant = 100
)

// Config configures a Microsoft Secure Application Model refresh token source.
type Config struct {
	// TokenEndpoint is the base URL for the Microsoft token service.
	// Default is "https://login.microsoftonline.com".
	// Override for sovereign clouds (e.g., "https://login.microsoftonline.us").
	TokenEndpoint string

	// ClientID is the Azure AD application (client) ID.
	// A single app registration is shared across all tenants.
	ClientID string

	// ClientSecret is the Azure AD application secret.
	ClientSecret string // #nosec G117 -- config struct holds secrets by design

	// Store provides per-tenant refresh token persistence. Because Azure AD
	// refresh tokens are MRRTs (Multi-Resource Refresh Tokens), a single
	// refresh token per tenant suffices for all resources.
	Store TokenStore

	// MaxPoolSize is the maximum number of per-tenant entries in the LRU
	// pool. Default is 10,000.
	MaxPoolSize int

	// ExpiryMargin is subtracted from the token's expires_in before setting
	// ExpiresAt on the credential. Default is 5 minutes.
	ExpiryMargin time.Duration

	// HTTPClient is used for token requests. If nil, a default client with
	// 10s timeout and TLS 1.3 minimum is used.
	HTTPClient *http.Client

	// Logger for debug, warning, and error messages. If nil, slog.Default()
	// is used.
	Logger *slog.Logger

	// KeyResolver resolves the tenant ID from transaction context when
	// TenantID is not present in tx.Data. If nil, TenantID must always
	// be provided in tx.Data.
	KeyResolver contrib.KeyResolver

	// OnSaveError is an optional callback invoked when a rotated refresh
	// token fails to persist. This allows operators to hook metrics or
	// alerting for this degraded state. The current request still succeeds
	// (returning the access token), but subsequent refreshes may fail if the
	// old token has been invalidated by the IdP.
	// If nil, only logging occurs.
	OnSaveError func(ctx context.Context, tenantID, resource string, err error)
}

// Compile-time check that RefreshTokenSource implements CredentialProvider.
var _ sdk.CredentialProvider = (*RefreshTokenSource)(nil)

// RefreshTokenSource implements [sdk.CredentialProvider] for the Microsoft
// Secure Application Model (delegated refresh token grant).
//
// It extracts TenantID and Resource from [sdk.TransactionContext].Data,
// looks up (or creates) a per-tenant entry in a bounded LRU pool, and
// manages the refresh lifecycle directly. Access tokens are cached per
// (tenant, resource) pair. Refresh tokens are loaded from the [TokenStore]
// on each exchange (no in-memory refresh token cache) because Azure AD
// refresh tokens are Multi-Resource Refresh Tokens (MRRTs) — a single token
// per tenant serves all resources.
//
// A single ClientID + ClientSecret pair is shared across all tenants.
// Per-tenant state (refresh tokens) is managed by the [TokenStore].
//
// It is safe for concurrent use from multiple goroutines.
type RefreshTokenSource struct {
	tokenEndpoint string
	clientID      string
	clientSecret  string
	store         TokenStore
	keyResolver   contrib.KeyResolver
	maxPoolSize   int
	expiryMargin  time.Duration
	httpClient    *http.Client
	logger        *slog.Logger
	onSaveError   func(ctx context.Context, tenantID, resource string, err error)

	mu   sync.Mutex
	pool map[string]*list.Element // keyed by tenantID
	lru  *list.List
}

// tenantEntry holds per-tenant state: a singleflight group for exchange
// deduplication and a per-resource access token cache.
type tenantEntry struct {
	tenantID string
	group    singleflight.Group

	mu     sync.RWMutex
	tokens map[string]*cachedToken // resource -> cached access token
}

// cachedToken holds a fetched access token and its computed expiration.
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// NewRefreshTokenSource creates a new Microsoft refresh token source.
func NewRefreshTokenSource(cfg Config) *RefreshTokenSource {
	endpoint := cfg.TokenEndpoint
	if endpoint == "" {
		endpoint = defaultTokenEndpoint
	}

	maxPool := cfg.MaxPoolSize
	if maxPool <= 0 {
		maxPool = defaultMaxPoolSize
	}

	margin := cfg.ExpiryMargin
	if margin == 0 {
		margin = defaultExpiryMargin
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	client := cfg.HTTPClient
	if client == nil {
		client = oauthutil.DefaultHTTPClient()
	}

	return &RefreshTokenSource{
		tokenEndpoint: endpoint,
		clientID:      cfg.ClientID,
		clientSecret:  cfg.ClientSecret,
		store:         cfg.Store,
		keyResolver:   cfg.KeyResolver,
		maxPoolSize:   maxPool,
		expiryMargin:  margin,
		httpClient:    client,
		logger:        logger,
		onSaveError:   cfg.OnSaveError,
		pool:          make(map[string]*list.Element),
		lru:           list.New(),
	}
}

// GetCredentials extracts TenantID and Resource from the transaction context,
// resolves a per-tenant entry from the pool, checks the per-resource access
// token cache, and if needed exchanges the tenant's MRRT for a new access
// token via the token endpoint.
//
// Returns a cacheable credential (Fast Path) with an Authorization: Bearer
// header and ExpiresAt adjusted by the configured expiry margin.
func (s *RefreshTokenSource) GetCredentials(
	ctx context.Context,
	tx sdk.TransactionContext,
	_ *http.Request,
) (*sdk.Credential, error) {
	tenantID, err := contrib.ResolveFromContext(ctx, tx, "TenantID", s.keyResolver)
	if err != nil {
		return nil, err
	}

	if !validTenantID.MatchString(tenantID) {
		return nil, fmt.Errorf("TenantID contains invalid characters: %w",
			contrib.ErrInvalidContextData)
	}

	resource, err := contrib.ResolveFromContext(ctx, tx, "Resource", nil)
	if err != nil {
		return nil, err
	}

	entry := s.getOrCreate(ctx, tenantID)

	token, expiresAt, err := s.getAccessToken(ctx, entry, resource)
	if err != nil {
		return nil, err
	}

	return &sdk.Credential{
		Headers:   map[string]string{"Authorization": "Bearer " + token},
		ExpiresAt: expiresAt,
	}, nil
}

// getAccessToken returns a valid access token for the given resource,
// fetching a new one via singleflight if the cache is stale.
func (s *RefreshTokenSource) getAccessToken(
	ctx context.Context,
	entry *tenantEntry,
	resource string,
) (string, time.Time, error) {
	// Check cache.
	entry.mu.RLock()
	if ct, ok := entry.tokens[resource]; ok && time.Now().Before(ct.expiresAt) {
		entry.mu.RUnlock()
		s.logger.LogAttrs(ctx, slog.LevelDebug, "access token cache hit",
			slog.String("tenant_id", entry.tenantID),
			slog.String("resource", resource))
		return ct.accessToken, ct.expiresAt, nil
	}
	entry.mu.RUnlock()

	s.logger.LogAttrs(ctx, slog.LevelDebug, "access token cache miss",
		slog.String("tenant_id", entry.tenantID),
		slog.String("resource", resource))

	// Use context.WithoutCancel so that a single caller's cancellation
	// does not poison all coalesced singleflight waiters.
	result, err, _ := entry.group.Do(resource, func() (any, error) {
		return s.exchange(context.WithoutCancel(ctx), entry.tenantID, resource)
	})
	if err != nil {
		return "", time.Time{}, err
	}

	ct, ok := result.(*cachedToken)
	if !ok {
		return "", time.Time{}, fmt.Errorf("unexpected singleflight result type %T", result)
	}

	// Cache the result, enforcing the per-tenant resource bound.
	entry.mu.Lock()
	if len(entry.tokens) >= maxResourcesPerTenant {
		purgeExpiredTokens(entry)
	}
	entry.tokens[resource] = ct
	entry.mu.Unlock()

	return ct.accessToken, ct.expiresAt, nil
}

// exchange performs the actual token exchange: load refresh token from store,
// exchange for an access token for the given resource, save the rotated
// refresh token back to the store.
//
// Concurrent exchanges for different resources on the same tenant are safe
// because Azure AD refresh tokens are MRRTs and the IdP does not revoke the
// old token upon exchange. Both exchanges use the same refresh token, each
// receives a (potentially different) rotated token, and Save is last-write-wins.
// If Microsoft ever changes this policy, exchanges should be serialized per
// tenant (e.g., via a per-tenant mutex around the Load→Exchange→Save sequence).
func (s *RefreshTokenSource) exchange(
	ctx context.Context,
	tenantID, resource string,
) (*cachedToken, error) {
	refreshToken, err := s.store.Load(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("loading refresh token: %w", err)
	}

	tokenURL := fmt.Sprintf("%s/%s/oauth2/token", s.tokenEndpoint, tenantID)

	fetcher := oauthutil.NewTokenFetcher(oauthutil.TokenFetcher{
		TokenURL:     tokenURL,
		ClientID:     s.clientID,
		ClientSecret: s.clientSecret,
		ExpiryMargin: s.expiryMargin,
		Client:       s.httpClient,
		Logger:       s.logger,
	})

	form := fetcher.BuildForm("refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("resource", resource)

	result, err := fetcher.Exchange(ctx, form)
	if err != nil {
		return nil, err
	}

	if result.RefreshToken != "" {
		if saveErr := s.store.Save(ctx, tenantID, result.RefreshToken); saveErr != nil {
			s.logger.LogAttrs(ctx, slog.LevelError, "failed to save rotated refresh token",
				slog.String("tenant_id", tenantID),
				slog.String("resource", resource),
				slog.String("error", saveErr.Error()))

			if s.onSaveError != nil {
				s.onSaveError(ctx, tenantID, resource, saveErr)
			}
		}
	}

	return &cachedToken{
		accessToken: result.AccessToken,
		expiresAt:   result.ExpiresAt,
	}, nil
}

// getOrCreate returns an existing per-tenant entry or creates a new one.
// On pool capacity overflow, the least recently used entry is evicted.
func (s *RefreshTokenSource) getOrCreate(ctx context.Context, tenantID string) *tenantEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if elem, ok := s.pool[tenantID]; ok {
		s.lru.MoveToFront(elem)
		entry, _ := elem.Value.(*tenantEntry)
		return entry
	}

	entry := &tenantEntry{
		tenantID: tenantID,
		tokens:   make(map[string]*cachedToken),
	}

	// Evict LRU if at capacity.
	if s.lru.Len() >= s.maxPoolSize {
		s.evictLRU(ctx)
	}

	elem := s.lru.PushFront(entry)
	s.pool[tenantID] = elem

	return entry
}

// evictLRU removes the least recently used entry from the pool.
// Caller must hold s.mu.
func (s *RefreshTokenSource) evictLRU(ctx context.Context) {
	back := s.lru.Back()
	if back == nil {
		return
	}

	entry, _ := back.Value.(*tenantEntry)
	s.logger.LogAttrs(ctx, slog.LevelDebug, "evicting LRU pool entry",
		slog.String("tenant_id", entry.tenantID))

	delete(s.pool, entry.tenantID)
	s.lru.Remove(back)
}

// purgeExpiredTokens removes expired entries from the tenant's resource cache.
// Caller must hold entry.mu for writing.
func purgeExpiredTokens(entry *tenantEntry) {
	now := time.Now()
	for resource, ct := range entry.tokens {
		if !now.Before(ct.expiresAt) {
			delete(entry.tokens, resource)
		}
	}
}
