// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package keyvault provides an Azure Key Vault-backed implementation of
// microsoft.TokenStore for persisting Microsoft SAM refresh tokens.
//
// The store maps each tenant to a single Key Vault secret named
// prefix+hex(sha256(tenantID)). The original tenantID is preserved as a
// "tenantID" tag on the secret for operator visibility.
//
// Secret rotation is handled by Key Vault: each Save creates a new secret
// version, Load always reads the latest version.
package keyvault

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/cloudblue/chaperone/plugins/contrib"
	"github.com/cloudblue/chaperone/plugins/contrib/microsoft"
)

// defaultPrefix is prepended to every secret name. Override via Config.Prefix
// to namespace multiple environments in a shared vault.
const defaultPrefix = "chaperone-rt-"

// secretNotFoundCode is the ErrorCode returned by Key Vault when a secret
// does not exist in the vault.
const secretNotFoundCode = "SecretNotFound"

// validTenantID mirrors microsoft.validTenantID, which is unexported. Any
// change there should be reflected here. It matches Azure AD tenant
// identifiers (GUIDs, domain names) and rejects path separators, query
// strings, and fragments.
var validTenantID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*$`)

// Config configures a Key Vault-backed TokenStore.
type Config struct {
	// VaultURL is the Key Vault URL, e.g. "https://myvault.vault.azure.net/".
	// Required.
	VaultURL string

	// Credential authenticates to Key Vault. Any azcore.TokenCredential works;
	// azidentity.NewDefaultAzureCredential, NewManagedIdentityCredential, and
	// NewWorkloadIdentityCredential are typical. Required.
	Credential azcore.TokenCredential

	// Prefix is prepended to every secret name. Defaults to "chaperone-rt-".
	// Override to namespace multiple environments in a shared vault.
	Prefix string

	// ClientOptions are passed through to azsecrets.NewClient. Optional.
	ClientOptions *azsecrets.ClientOptions

	// Logger for debug/warning messages. If nil, slog.Default() is used
	// (resolved lazily, matching the RefreshTokenSource pattern).
	Logger *slog.Logger
}

// secretsClient is the narrow subset of *azsecrets.Client that Store uses.
// Declaring it as a local interface enables fake-based unit testing without
// pulling in the full SDK client.
type secretsClient interface {
	GetSecret(ctx context.Context, name, version string,
		opts *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
	SetSecret(ctx context.Context, name string, params azsecrets.SetSecretParameters,
		opts *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
}

// Store persists Microsoft SAM refresh tokens in Azure Key Vault.
// Safe for concurrent use.
type Store struct {
	client secretsClient
	prefix string
	logger *slog.Logger
}

// Compile-time check that Store implements microsoft.TokenStore.
var _ microsoft.TokenStore = (*Store)(nil)

// NewStore constructs a Store from a Config. Returns an error if VaultURL or
// Credential is missing, or if the underlying azsecrets client fails to
// initialize.
func NewStore(cfg Config) (*Store, error) {
	if cfg.VaultURL == "" {
		return nil, errors.New("keyvault: VaultURL is required")
	}
	if cfg.Credential == nil {
		return nil, errors.New("keyvault: Credential is required")
	}

	client, err := azsecrets.NewClient(cfg.VaultURL, cfg.Credential, cfg.ClientOptions)
	if err != nil {
		return nil, fmt.Errorf("keyvault: creating azsecrets client: %w", err)
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}

	return &Store{
		client: client,
		prefix: prefix,
		logger: cfg.Logger,
	}, nil
}

// newStoreWithClient is the test-only constructor that accepts a preconstructed
// secretsClient. The public NewStore wraps azsecrets.NewClient and delegates
// here. Kept unexported so production callers can only reach it via NewStore.
func newStoreWithClient(client secretsClient, logger *slog.Logger) *Store {
	return &Store{client: client, prefix: defaultPrefix, logger: logger}
}

// log returns the configured logger, or slog.Default() if none was set.
// Called at log-emit time so the current global default is always used
// when no explicit logger is provided.
func (s *Store) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

// Load retrieves the current refresh token for the given tenant.
// Returns an error wrapping [contrib.ErrTenantNotFound] if the tenant has no
// stored token.
func (s *Store) Load(ctx context.Context, tenantID string) (string, error) {
	if err := validateTenantID(tenantID); err != nil {
		return "", err
	}

	name := secretName(s.prefix, tenantID)

	resp, err := s.client.GetSecret(ctx, name, "", nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) &&
			respErr.StatusCode == http.StatusNotFound &&
			respErr.ErrorCode == secretNotFoundCode {
			s.log().LogAttrs(ctx, slog.LevelDebug, "no refresh token in key vault for tenant",
				slog.String("tenant_id", tenantID),
				slog.String("secret_name", name))
			return "", fmt.Errorf("no token for tenant %s: %w",
				tenantID, contrib.ErrTenantNotFound)
		}
		return "", fmt.Errorf("loading token for tenant %s: %w", tenantID, err)
	}

	if resp.Value == nil {
		return "", fmt.Errorf("key vault returned nil secret value for tenant %s", tenantID)
	}

	return *resp.Value, nil
}

// Save persists a rotated refresh token to Key Vault. Each call creates a
// new secret version; Load always reads the latest.
func (s *Store) Save(ctx context.Context, tenantID, refreshToken string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}
	if refreshToken == "" {
		return errors.New("refusing to save empty refresh token")
	}

	name := secretName(s.prefix, tenantID)
	tenantIDCopy := tenantID
	managedBy := "chaperone"

	params := azsecrets.SetSecretParameters{
		Value: &refreshToken,
		Tags: map[string]*string{
			"tenantID":  &tenantIDCopy,
			"managedBy": &managedBy,
		},
	}

	if _, err := s.client.SetSecret(ctx, name, params, nil); err != nil {
		return fmt.Errorf("saving token for tenant %s: %w", tenantID, err)
	}

	return nil
}

// validateTenantID rejects tenant IDs that could cause path traversal or that
// don't match Azure AD's tenant ID grammar. Defense in depth: the outer
// RefreshTokenSource already validates, but Store is a public type and must
// not rely on callers for safety.
func validateTenantID(tenantID string) error {
	if !validTenantID.MatchString(tenantID) {
		display := tenantID
		if len(display) > 64 {
			display = display[:64] + "..."
		}
		return fmt.Errorf("keyvault: invalid tenant ID %q: must match %s",
			display, validTenantID.String())
	}
	return nil
}
