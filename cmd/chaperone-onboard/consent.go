// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// consentConfig is the common configuration for both subcommands.
type consentConfig struct {
	authorizeURL     string
	tokenURL         string
	clientID         string
	clientSecret     string
	scopes           string
	extraAuthParams  url.Values
	extraTokenParams url.Values
	port             int
	timeout          time.Duration
	noBrowser        bool
	usePKCE          bool
}

// callbackResult holds the result from the OAuth2 callback.
type callbackResult struct {
	code string
	err  error
}

// thankYouHTML is the response shown to the user's browser after a successful callback.
const thankYouHTML = `<!DOCTYPE html>
<html><head><title>Authorization Complete</title></head>
<body style="font-family:system-ui,sans-serif;text-align:center;padding:2em">
<h2>Authorization complete</h2>
<p>You may close this tab and return to the terminal.</p>
</body></html>`

// runConsent orchestrates the full OAuth2 authorization code flow:
// generate CSRF state and PKCE, start callback server, open browser,
// wait for callback, exchange code for tokens, return refresh token.
//
//nolint:funlen // CLI orchestrator, acceptable to be longer
func runConsent(cfg consentConfig) (string, error) {
	state, err := generateState()
	if err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}

	var verifier, challenge string
	if cfg.usePKCE {
		verifier, challenge, err = generatePKCE()
		if err != nil {
			return "", fmt.Errorf("generating PKCE: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	redirectURI, resultCh, err := startCallbackServer(ctx, cfg.port, state)
	if err != nil {
		return "", fmt.Errorf("starting callback server: %w", err)
	}

	authURL := buildAuthURL(cfg, state, challenge, redirectURI)

	if cfg.noBrowser {
		fmt.Fprintf(os.Stderr, "Open this URL in your browser to authorize:\n\n  %s\n\n", authURL)
	} else {
		fmt.Fprintf(os.Stderr, "Opening browser for authorization...\n")
		if browserErr := openBrowser(authURL); browserErr != nil {
			fmt.Fprintf(os.Stderr, "Could not open browser: %v\n", browserErr)
			fmt.Fprintf(os.Stderr, "Open this URL manually:\n\n  %s\n\n", authURL)
		}
	}

	fmt.Fprintf(os.Stderr, "Waiting for authorization callback (timeout: %s)...\n", cfg.timeout)

	select {
	case result := <-resultCh:
		if result.err != nil {
			return "", result.err
		}
		return finishExchange(ctx, cfg, result.code, redirectURI, verifier)
	case <-ctx.Done():
		return "", errConsentTimeout
	}
}

// finishExchange performs the code-for-token exchange after a successful callback.
func finishExchange(ctx context.Context, cfg consentConfig, code, redirectURI, verifier string) (string, error) {
	fmt.Fprintf(os.Stderr, "  ✓ Authorization code received\n")

	refreshToken, err := exchangeCode(ctx, exchangeConfig{
		tokenURL:     cfg.tokenURL,
		clientID:     cfg.clientID,
		clientSecret: cfg.clientSecret,
		code:         code,
		redirectURI:  redirectURI,
		codeVerifier: verifier,
		extraParams:  cfg.extraTokenParams,
	})
	if err != nil {
		return "", fmt.Errorf("%w: %w", errExchangeFailed, err)
	}
	fmt.Fprintf(os.Stderr, "  ✓ Token exchange complete\n")

	return refreshToken, nil
}

// buildAuthURL constructs the authorization URL with all required parameters.
func buildAuthURL(cfg consentConfig, state, challenge, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", cfg.clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)

	if cfg.scopes != "" {
		params.Set("scope", cfg.scopes)
	}

	if challenge != "" {
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
	}

	for k, vs := range cfg.extraAuthParams {
		for _, v := range vs {
			params.Set(k, v)
		}
	}

	return cfg.authorizeURL + "?" + params.Encode()
}

// startCallbackServer starts a local HTTP server on 127.0.0.1 that waits for
// a single OAuth2 callback. It returns the redirect URI, a channel that will
// receive the authorization code, and any startup error.
//
//nolint:funlen // callback server setup is inherently sequential
func startCallbackServer(ctx context.Context, port int, expectedState string) (redirectURI string, results <-chan callbackResult, err error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lc := &net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return "", nil, fmt.Errorf("listening on %s: %w", addr, err)
	}

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return "", nil, fmt.Errorf("unexpected listener address type")
	}
	redirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", tcpAddr.Port)

	resultCh := make(chan callbackResult, 1)
	server := &http.Server{
		ReadTimeout: 5 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callbackHandler(ctx, server, resultCh, expectedState))
	server.Handler = mux

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			resultCh <- callbackResult{err: fmt.Errorf("callback server error: %w", serveErr)}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.WithoutCancel(ctx))
	}()

	return redirectURI, resultCh, nil
}

// callbackHandler returns an HTTP handler for the OAuth2 redirect callback.
func callbackHandler(
	ctx context.Context,
	server *http.Server,
	resultCh chan<- callbackResult,
	expectedState string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			go func() { _ = server.Shutdown(context.WithoutCancel(ctx)) }()
		}()

		query := r.URL.Query()

		if errParam := query.Get("error"); errParam != "" {
			desc := query.Get("error_description")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Authorization error: %s", html.EscapeString(errParam)) //nolint:gosec // output is HTML-escaped
			if desc != "" {
				resultCh <- callbackResult{err: fmt.Errorf("authorization denied: %s: %s", errParam, desc)}
			} else {
				resultCh <- callbackResult{err: fmt.Errorf("authorization denied: %s", errParam)}
			}
			return
		}

		gotState := query.Get("state")
		if gotState != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "State mismatch — possible CSRF attack")
			resultCh <- callbackResult{err: fmt.Errorf("state mismatch: possible CSRF attack")}
			return
		}

		code := query.Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing authorization code")
			resultCh <- callbackResult{err: fmt.Errorf("callback missing authorization code")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, thankYouHTML)
		resultCh <- callbackResult{code: code}
	}
}

// generateState produces a 32-byte cryptographically random base64url string
// for use as an OAuth2 CSRF state parameter.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generatePKCE generates a PKCE code verifier and its S256 code challenge
// per RFC 7636.
func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("reading random bytes: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}
