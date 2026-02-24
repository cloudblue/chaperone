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

	"github.com/cloudblue/chaperone/plugins/contrib"
	"github.com/cloudblue/chaperone/plugins/contrib/oauth"
	"github.com/cloudblue/chaperone/sdk"
)

// validTenantID matches Azure AD tenant identifiers: GUIDs, domain names
// (alphanumeric with dots and hyphens), or the literal "common"/"organizations"/
// "consumers". It rejects path separators, query strings, and fragments.
var validTenantID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*$`)

const (
	// defaultTokenEndpoint is the public Azure AD v1 token endpoint.
	defaultTokenEndpoint = "https://login.microsoftonline.com" // #nosec G101 -- URL endpoint, not a credential

	// defaultMaxPoolSize is the maximum number of oauth.RefreshToken instances
	// kept in the LRU pool. Each instance owns its own access token cache and
	// singleflight group.
	defaultMaxPoolSize = 10_000

	// defaultExpiryMargin matches the Python connector's 300-second margin.
	defaultExpiryMargin = 5 * time.Minute
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

	// Store provides per-tenant, per-resource refresh token persistence.
	Store TokenStore

	// MaxPoolSize is the maximum number of oauth.RefreshToken instances in
	// the LRU pool. Default is 10,000.
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
}

// Compile-time check that RefreshTokenSource implements CredentialProvider.
var _ sdk.CredentialProvider = (*RefreshTokenSource)(nil)

// RefreshTokenSource implements [sdk.CredentialProvider] for the Microsoft
// Secure Application Model (delegated refresh token grant).
//
// It extracts TenantID and Resource from [sdk.TransactionContext].Data,
// looks up (or creates) an [oauth.RefreshToken] instance in a bounded LRU
// pool, and delegates token exchange to that instance.
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
	maxPoolSize   int
	expiryMargin  time.Duration
	httpClient    *http.Client
	logger        *slog.Logger

	mu   sync.Mutex
	pool map[poolKey]*list.Element
	lru  *list.List
}

// poolKey identifies a unique oauth.RefreshToken instance in the pool.
type poolKey struct {
	tenantID string
	resource string
}

// poolEntry is stored in each list element.
type poolEntry struct {
	key      poolKey
	instance *oauth.RefreshToken
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

	return &RefreshTokenSource{
		tokenEndpoint: endpoint,
		clientID:      cfg.ClientID,
		clientSecret:  cfg.ClientSecret,
		store:         cfg.Store,
		maxPoolSize:   maxPool,
		expiryMargin:  margin,
		httpClient:    cfg.HTTPClient,
		logger:        logger,
		pool:          make(map[poolKey]*list.Element),
		lru:           list.New(),
	}
}

// GetCredentials extracts TenantID and Resource from the transaction context,
// resolves an oauth.RefreshToken instance from the pool, and delegates the
// token exchange to it.
//
// Returns a cacheable credential (Fast Path) with an Authorization: Bearer
// header and ExpiresAt adjusted by the configured expiry margin.
func (s *RefreshTokenSource) GetCredentials(
	ctx context.Context,
	tx sdk.TransactionContext,
	req *http.Request,
) (*sdk.Credential, error) {
	tenantID, err := extractString(tx.Data, "TenantID")
	if err != nil {
		return nil, err
	}

	if !validTenantID.MatchString(tenantID) {
		return nil, fmt.Errorf("TenantID contains invalid characters: %w",
			contrib.ErrInvalidContextData)
	}

	resource, err := extractString(tx.Data, "Resource")
	if err != nil {
		return nil, err
	}

	instance := s.getOrCreate(ctx, tenantID, resource)

	return instance.GetCredentials(ctx, tx, req)
}

// getOrCreate returns an existing pool instance or creates a new one.
// On pool capacity overflow, the least recently used instance is evicted.
func (s *RefreshTokenSource) getOrCreate(ctx context.Context, tenantID, resource string) *oauth.RefreshToken {
	key := poolKey{tenantID: tenantID, resource: resource}

	s.mu.Lock()
	defer s.mu.Unlock()

	if elem, ok := s.pool[key]; ok {
		s.lru.MoveToFront(elem)
		entry, _ := elem.Value.(*poolEntry)
		return entry.instance
	}

	instance := s.newInstance(tenantID, resource)

	// Evict LRU if at capacity.
	if s.lru.Len() >= s.maxPoolSize {
		s.evictLRU(ctx)
	}

	entry := &poolEntry{key: key, instance: instance}
	elem := s.lru.PushFront(entry)
	s.pool[key] = elem

	return instance
}

// newInstance creates a new oauth.RefreshToken configured for the given
// tenant and resource.
func (s *RefreshTokenSource) newInstance(tenantID, resource string) *oauth.RefreshToken {
	tokenURL := fmt.Sprintf("%s/%s/oauth2/token", s.tokenEndpoint, tenantID)

	adapter := &keyedStoreAdapter{
		store:    s.store,
		tenantID: tenantID,
		resource: resource,
	}

	return oauth.NewRefreshToken(oauth.RefreshTokenConfig{
		TokenURL:     tokenURL,
		ClientID:     s.clientID,
		ClientSecret: s.clientSecret,
		ExtraParams:  map[string]string{"resource": resource},
		Store:        adapter,
		HTTPClient:   s.httpClient,
		Logger:       s.logger,
		ExpiryMargin: s.expiryMargin,
	})
}

// evictLRU removes the least recently used instance from the pool.
// Caller must hold s.mu.
func (s *RefreshTokenSource) evictLRU(ctx context.Context) {
	back := s.lru.Back()
	if back == nil {
		return
	}

	entry, _ := back.Value.(*poolEntry)
	s.logger.LogAttrs(ctx, slog.LevelDebug, "evicting LRU pool instance",
		slog.String("tenant_id", entry.key.tenantID),
		slog.String("resource", entry.key.resource))

	delete(s.pool, entry.key)
	s.lru.Remove(back)
}

// extractString extracts a required string field from the context data map.
func extractString(data map[string]any, key string) (string, error) {
	raw, ok := data[key]
	if !ok {
		return "", fmt.Errorf("%s not present in transaction context: %w",
			key, contrib.ErrMissingContextData)
	}

	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %T: %w",
			key, raw, contrib.ErrInvalidContextData)
	}

	if s == "" {
		return "", fmt.Errorf("%s is empty in transaction context: %w",
			key, contrib.ErrInvalidContextData)
	}

	return s, nil
}

// keyedStoreAdapter bridges a keyed microsoft.TokenStore to the keyless
// oauth.TokenStore interface by pre-binding a specific tenant+resource pair.
type keyedStoreAdapter struct {
	store    TokenStore
	tenantID string
	resource string
}

func (a *keyedStoreAdapter) Load(ctx context.Context) (string, error) {
	return a.store.Load(ctx, a.tenantID, a.resource)
}

func (a *keyedStoreAdapter) Save(ctx context.Context, token string) error {
	return a.store.Save(ctx, a.tenantID, a.resource, token)
}
