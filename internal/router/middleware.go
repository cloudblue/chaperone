// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/cloudblue/chaperone/internal/observability"
)

// AllowListMiddleware validates incoming requests against the allow list
// before passing them to the next handler.
type AllowListMiddleware struct {
	validator    *AllowListValidator
	headerPrefix string
	next         http.Handler
}

// NewAllowListMiddleware creates a new allow list middleware.
//
// Parameters:
//   - allowList: The host-to-paths mapping from configuration
//   - headerPrefix: The prefix for context headers (e.g., "X-Connect")
//   - next: The next handler in the chain
func NewAllowListMiddleware(allowList map[string][]string, headerPrefix string, next http.Handler) *AllowListMiddleware {
	return &AllowListMiddleware{
		validator:    NewAllowListValidator(allowList),
		headerPrefix: headerPrefix,
		next:         next,
	}
}

// ServeHTTP implements http.Handler.
// It extracts the target URL from the request header, validates it against
// the allow list, and either passes the request to the next handler or
// returns an error response.
func (m *AllowListMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetURL := r.Header.Get(m.headerPrefix + "-Target-URL")

	// Missing target URL is a client error
	if targetURL == "" {
		slog.Warn("missing target URL header",
			"trace_id", observability.TraceIDFromContext(r.Context()),
			"header", m.headerPrefix+"-Target-URL",
			"remote_addr", r.RemoteAddr,
		)
		respondError(w, http.StatusBadRequest, "missing Target-URL header")
		return
	}

	// Validate target URL against allow list
	if err := m.validator.Validate(targetURL); err != nil {
		// Log the host but not the full URL to avoid leaking query params
		slog.Warn("allow list validation failed",
			"trace_id", observability.TraceIDFromContext(r.Context()),
			"error", err.Error(),
			"remote_addr", r.RemoteAddr,
		)

		// Return appropriate status code based on error type
		switch {
		case errors.Is(err, ErrHostNotAllowed):
			respondError(w, http.StatusForbidden, "host not allowed")
		case errors.Is(err, ErrPathNotAllowed):
			respondError(w, http.StatusForbidden, "path not allowed")
		case errors.Is(err, ErrEmptyAllowList):
			respondError(w, http.StatusForbidden, "no routes configured")
		default:
			// Invalid URL or other validation errors
			respondError(w, http.StatusBadRequest, "invalid target URL")
		}
		return
	}

	slog.Debug("allow list validation passed",
		"trace_id", observability.TraceIDFromContext(r.Context()),
		"target_host", extractHostFromURL(targetURL),
	)

	// Validation passed, continue to next handler
	m.next.ServeHTTP(w, r)
}

// extractHostFromURL parses a URL string and returns only the host portion.
// Returns an empty string if the URL is invalid or has no host.
func extractHostFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// errorResponse is the JSON structure for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// respondError writes a JSON error response with the given status code and message.
func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := errorResponse{Error: message}
	_ = json.NewEncoder(w).Encode(resp) // Error ignored: client may have disconnected
}
