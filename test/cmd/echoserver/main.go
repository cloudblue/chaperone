// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// Package main provides a minimal HTTP echo server for integration testing.
//
// The echo server listens on a configurable port and returns a JSON response
// containing all received request headers, method, path, and host. This allows
// the docker-test target to verify that Chaperone correctly:
//
//   - Proxied the request to the target
//   - Injected the expected credential headers (e.g., Authorization)
//
// This server is intentionally minimal (no external dependencies) and compiles
// to a static binary suitable for distroless containers.
//
// Usage:
//
//	echoserver [-addr :8080]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"time"
)

// echoResponse is the JSON payload returned for every request.
type echoResponse struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Host    string            `json:"host"`
	Headers map[string]string `json:"headers"`
}

func main() {
	addr := flag.String("addr", ":8080", "Listen address")

	flag.Parse()

	http.HandleFunc("/", handleEcho)

	log.SetOutput(os.Stdout)
	log.Printf("echoserver listening on %s", *addr)

	srv := &http.Server{
		Addr:         *addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "echoserver: %v\n", err)
		os.Exit(1)
	}
}

// handleEcho returns a JSON object with the request method, path, host,
// and all received headers (single-value, alphabetically sorted keys).
func handleEcho(w http.ResponseWriter, r *http.Request) {
	headers := make(map[string]string, len(r.Header))

	keys := make([]string, 0, len(r.Header))
	for k := range r.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		headers[k] = r.Header.Get(k)
	}

	resp := echoResponse{
		Method:  r.Method,
		Path:    r.URL.Path,
		Host:    r.Host,
		Headers: headers,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("echoserver: failed to encode response: %v", err)
	}
}
