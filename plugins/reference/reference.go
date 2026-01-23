// Copyright 2024-2026 CloudBlue
// SPDX-License-Identifier: Apache-2.0

// Package reference provides a default Plugin implementation for testing
// and simple deployments.
//
// This plugin reads credentials from a JSON file, making it suitable for:
//   - Local development and testing
//   - Simple deployments with static credentials
//   - As a template for building custom plugins
//
// For production deployments with dynamic credentials (Vault, OAuth2 refresh),
// Distributors should implement their own Plugin.
package reference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/cloudblue/chaperone/sdk"
)

// Plugin is a reference implementation that reads credentials from a JSON file.
//
// The JSON file should have the following structure:
//
//	{
//	  "vendors": {
//	    "vendor-id": {
//	      "auth_type": "bearer",
//	      "token": "your-api-token",
//	      "ttl_minutes": 60
//	    }
//	  }
//	}
type Plugin struct {
	cache map[string]*vendorCredentials

	// CredentialsFile is the path to the JSON credentials file.
	CredentialsFile string

	// mu protects the credentials cache.
	mu sync.RWMutex
}

type vendorCredentials struct {
	AuthType   string `json:"auth_type"`   // "bearer", "api_key", "basic"
	Token      string `json:"token"`       // Token or API key value
	HeaderName string `json:"header_name"` // Custom header name (for api_key type)
	TTLMinutes int    `json:"ttl_minutes"` // Cache TTL in minutes
}

type credentialsFile struct {
	Vendors map[string]*vendorCredentials `json:"vendors"`
}

// New creates a new reference Plugin with the specified credentials file.
func New(credentialsFile string) *Plugin {
	return &Plugin{
		CredentialsFile: credentialsFile,
		cache:           make(map[string]*vendorCredentials),
	}
}

// GetCredentials retrieves credentials for the specified vendor from the JSON file.
//
// Supported auth_type values:
//   - "bearer": Sets Authorization: Bearer <token>
//   - "api_key": Sets custom header (default X-API-Key) to <token>
//   - "basic": Sets Authorization: Basic <token> (token should be base64 encoded)
func (p *Plugin) GetCredentials(ctx context.Context, tx sdk.TransactionContext, req *http.Request) (*sdk.Credential, error) {
	if tx.VendorID == "" {
		return nil, fmt.Errorf("vendor ID is required")
	}

	vendorCreds, err := p.loadVendorCredentials(tx.VendorID)
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials for vendor %s: %w", tx.VendorID, err)
	}

	headers := make(map[string]string)

	switch vendorCreds.AuthType {
	case "bearer":
		headers["Authorization"] = "Bearer " + vendorCreds.Token
	case "api_key":
		headerName := vendorCreds.HeaderName
		if headerName == "" {
			headerName = "X-API-Key"
		}
		headers[headerName] = vendorCreds.Token
	case "basic":
		headers["Authorization"] = "Basic " + vendorCreds.Token
	default:
		return nil, fmt.Errorf("unsupported auth_type: %s", vendorCreds.AuthType)
	}

	ttl := time.Duration(vendorCreds.TTLMinutes) * time.Minute
	if ttl == 0 {
		ttl = 60 * time.Minute // Default 1 hour
	}

	return &sdk.Credential{
		Headers:   headers,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

// SignCSR is not implemented in the reference plugin.
// Production deployments should implement this to integrate with their CA.
func (p *Plugin) SignCSR(ctx context.Context, csrPEM []byte) ([]byte, error) {
	return nil, fmt.Errorf("certificate signing not implemented in reference plugin; configure your CA integration")
}

// ModifyResponse is a no-op in the reference plugin.
// Override in custom plugins to modify vendor responses.
func (p *Plugin) ModifyResponse(ctx context.Context, tx sdk.TransactionContext, resp *http.Response) error {
	// No modifications in reference implementation
	return nil
}

func (p *Plugin) loadVendorCredentials(vendorID string) (*vendorCredentials, error) {
	// Check cache first
	p.mu.RLock()
	if creds, ok := p.cache[vendorID]; ok {
		p.mu.RUnlock()
		return creds, nil
	}
	p.mu.RUnlock()

	// Load from file
	data, err := os.ReadFile(p.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var cf credentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	creds, ok := cf.Vendors[vendorID]
	if !ok {
		return nil, fmt.Errorf("no credentials found for vendor: %s", vendorID)
	}

	// Cache the credentials
	p.mu.Lock()
	p.cache[vendorID] = creds
	p.mu.Unlock()

	return creds, nil
}

// ReloadCredentials clears the cache, forcing a reload from the file.
// Call this after updating the credentials file.
func (p *Plugin) ReloadCredentials() {
	p.mu.Lock()
	p.cache = make(map[string]*vendorCredentials)
	p.mu.Unlock()
}
