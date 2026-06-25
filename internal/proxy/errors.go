// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"encoding/json"
	"net/http"
)

// errorResponse is the JSON structure for error responses returned from
// the proxy. It mirrors the shape used by internal/router so that clients
// observe a consistent error envelope across the request lifecycle.
type errorResponse struct {
	Error string `json:"error"`
}

// respondError writes a JSON error response with the given status code and
// message. It is intentionally a small duplicate of the helper in
// internal/router/middleware.go — the helper is four lines and the duplication
// keeps the proxy package free of an extra dependency on router internals.
func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := errorResponse{Error: message}
	_ = json.NewEncoder(w).Encode(resp) // Error ignored: client may have disconnected
}
