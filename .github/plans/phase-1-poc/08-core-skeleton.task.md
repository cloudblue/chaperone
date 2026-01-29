# Task: Core Skeleton

**Status:** [x] Completed  
**Priority:** P0  
**Estimated Effort:** L (Large)

## Objective

Implement the basic HTTP server with `httputil.ReverseProxy` that handles incoming requests, parses context, invokes the plugin, and forwards to the target.

## Design Spec Reference

- **Primary:** Section 3.3 - Technology Stack (`net/http/httputil.ReverseProxy`)
- **Primary:** Section 5.1 - API & Routing Specification
- **Primary:** Section 5.2.B - Routing & Injection Logic
- **Related:** Section 8.1 - Resilience & Reliability (Panic Recovery, Timeouts)
- **Related:** Section 5.3 - Security Controls (Allow-list stub)

## Dependencies

- [x] `05-context-parsing.task.md` - Context parser
- [x] `07-reference-plugin.task.md` - Reference plugin for testing

## Acceptance Criteria

- [x] HTTP server starts and listens on configurable port
- [x] Endpoints implemented:
  - [x] `* /proxy` - Main provisioning endpoint (all HTTP methods)
  - [x] `GET /_ops/health` - Returns `{"status": "alive"}`
  - [x] `GET /_ops/version` - Returns version info
- [x] Request lifecycle:
  - [x] Parse `X-Connect-*` headers into TransactionContext
  - [x] Call plugin `GetCredentials`
  - [x] Inject returned headers into outgoing request
  - [x] Forward to target URL via ReverseProxy
  - [x] Return response to caller
- [x] Panic recovery middleware (server doesn't crash on panics)
- [x] Request logging middleware (per Design Spec 8.3.5: trace_id, latency, status, path, client_ip)
- [x] Context propagation:
  - [x] Plugin receives context with request timeout applied
  - [x] Client disconnect cancels context (via `r.Context()`)
  - [x] Context cancellation returns appropriate error (504 Gateway Timeout or early termination)
- [x] Configurable timeouts (hardcoded safe defaults for PoC)
- [x] Structured logging with trace ID
- [x] Tests pass: `go test ./internal/proxy/...`
- [x] Integration test: request → proxy → mock backend → response

## Implementation Hints

### Project Structure

```
internal/
└── proxy/
    ├── server.go       # HTTP server setup
    ├── handler.go      # Request handlers
    ├── middleware.go   # Panic recovery, logging
    ├── proxy.go        # ReverseProxy wrapper
    └── *_test.go       # Tests

cmd/
└── chaperone/
    └── main.go         # Wires everything together
```

### Server Setup

```go
type Server struct {
    addr   string
    plugin sdk.Plugin
    // ... other fields
}

func (s *Server) Start() error {
    mux := http.NewServeMux()
    mux.HandleFunc("/proxy", s.handleProxy)
    mux.HandleFunc("GET /_ops/health", s.handleHealth)
    mux.HandleFunc("GET /_ops/version", s.handleVersion)
    
    handler := s.withMiddleware(mux)
    
    srv := &http.Server{
        Addr:         s.addr,
        Handler:      handler,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  120 * time.Second,
    }
    return srv.ListenAndServe()
}
```

### Proxy Handler Flow

```go
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
    // 1. Generate/extract trace ID
    traceID := r.Header.Get("X-Connect-Request-ID")
    if traceID == "" {
        traceID = uuid.New().String()
    }
    
    // 2. Parse transaction context (note: this is TransactionContext, not context.Context)
    txCtx, err := context.ParseContext(r, "X-Connect")
    if err != nil {
        http.Error(w, "Bad Request", 400)
        return
    }
    
    // 3. Validate target URL (stub for PoC)
    // TODO: implement allow-list in Phase 2
    
    // 4. Create bounded context for plugin execution
    // - r.Context() cancels on client disconnect (net/http behavior)
    // - WithTimeout adds upper bound to prevent hung plugins
    ctx, cancel := context.WithTimeout(r.Context(), s.pluginTimeout) // e.g., 10s
    defer cancel()
    
    // 5. Get credentials from plugin
    cred, err := s.plugin.GetCredentials(ctx, *txCtx, r)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            http.Error(w, "Gateway Timeout", 504)
            return
        }
        if errors.Is(err, context.Canceled) {
            // Client disconnected, don't write response
            return
        }
        http.Error(w, "Internal Server Error", 500)
        return
    }
    
    // 6. Inject headers if returned
    if cred != nil {
        for k, v := range cred.Headers {
            r.Header.Set(k, v)
        }
    }
    
    // 7. Forward via ReverseProxy
    s.reverseProxy.ServeHTTP(w, r)
}
```

### Panic Recovery Middleware

```go
func (s *Server) withPanicRecovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                slog.Error("panic recovered", "error", err)
                http.Error(w, "Internal Server Error", 500)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

### Gotchas

- `ReverseProxy.Director` modifies the request; clone if needed
- Response body must be handled carefully (streaming)
- Timeouts should be shorter on client side than server side
- Don't forget to close response bodies in error paths

## Files to Create/Modify

- [ ] `internal/proxy/server.go` - Server struct and setup
- [ ] `internal/proxy/handler.go` - HTTP handlers
- [ ] `internal/proxy/middleware.go` - Middleware stack
- [ ] `internal/proxy/proxy.go` - ReverseProxy configuration
- [ ] `internal/proxy/server_test.go` - Unit tests
- [ ] `internal/proxy/integration_test.go` - Integration tests
- [ ] `cmd/chaperone/main.go` - Wire server + plugin

## Testing Strategy

### Unit Tests

- Health endpoint returns correct JSON
- Version endpoint returns version info
- Panic recovery catches panics and returns 500
- Invalid requests return 400
- Context timeout returns 504 Gateway Timeout
- Context cancellation handled gracefully (no response written)

### Integration Tests

Use `httptest.Server` as mock backend:

```go
func TestProxy_Integration(t *testing.T) {
    // 1. Create mock backend
    backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify injected headers
        if r.Header.Get("Authorization") == "" {
            t.Error("Authorization header not injected")
        }
        w.WriteHeader(200)
    }))
    defer backend.Close()
    
    // 2. Create proxy with reference plugin
    proxy := NewServer(":0", NewReferencePlugin(...))
    
    // 3. Make request with X-Connect headers
    req := httptest.NewRequest("POST", "/proxy", nil)
    req.Header.Set("X-Connect-Target-URL", backend.URL)
    req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
    
    // 4. Verify response
}
```

## Security Considerations

- Allow-list validation is stubbed (TODO for Phase 2)
- Response sanitization is stubbed (TODO for Phase 2)
- Timeouts prevent resource exhaustion
- Panic recovery prevents server crashes
