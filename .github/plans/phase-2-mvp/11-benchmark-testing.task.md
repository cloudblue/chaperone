# Task: Benchmark Testing

**Status:** [ ] Not Started
**Priority:** P1
**Estimated Effort:** M

## Objective

Implement comprehensive Go benchmarks for all hot-path components to establish performance baselines and enable regression detection in CI.

## Design Spec Reference

- **Primary:** Section 9.3.A - Benchmark Testing (Go Native)
- **Related:** Section 8.3 - Observability & Telemetry

## Dependencies

- [ ] `09-profiling.task.md` - pprof integration enables deeper analysis
- [ ] Phase 1 completed (components to benchmark exist)

## Acceptance Criteria

- [ ] Benchmark files exist for all hot-path components
- [ ] All benchmarks use `b.ReportAllocs()`
- [ ] Parallel benchmarks use `b.RunParallel()`
- [ ] TLS handshake benchmarks included
- [ ] Benchmark baseline saved to repository
- [ ] Makefile targets: `bench`, `bench-save`, `bench-compare`
- [ ] CI workflow compares benchmarks on PR
- [ ] Target metrics documented and verified
- [ ] Tests pass: `go test ./...`
- [ ] Lint passes: `make lint`

## Hot Path Components to Benchmark

These are the critical per-request operations that directly impact latency:

| Component | Why Benchmark | Target | Location |
|-----------|---------------|--------|----------|
| **Context Parsing** | Runs on every request, extracts headers | < 1μs, 0 allocs | `internal/context/parser.go` |
| **Context Hashing** | Cache key generation | < 5μs, minimal allocs | `internal/cache/hash.go` |
| **Allow-List Matching** | Glob pattern validation | < 10μs per pattern | `internal/router/allowlist.go` |
| **Credential Injection** | Header manipulation | < 1μs, 0 allocs | `internal/proxy/inject.go` |
| **Response Sanitization** | Header stripping (Reflector) | < 1μs, 0 allocs | `internal/sanitizer/reflector.go` |
| **Log Redaction** | Sensitive header masking | < 2μs, 0 allocs | `internal/sanitizer/redactor.go` |
| **Full Request Cycle** | End-to-end with mock upstream | < 100μs overhead | `internal/proxy/` |

## Target Metrics & Acceptance Criteria

### Performance Tiers

| Metric | Acceptable | Good | Excellent |
|--------|------------|------|-----------|
| **Proxy overhead** (excl. upstream) | < 500μs | < 100μs | < 50μs |
| **Allocations/request** | < 100 | < 50 | < 20 |
| **Bytes/request** | < 8KB | < 4KB | < 2KB |
| **P99 latency** (under load) | < 10ms | < 5ms | < 2ms |
| **Throughput** (single core) | > 5K req/s | > 10K req/s | > 20K req/s |
| **TLS handshake** (mTLS) | < 10ms | < 5ms | < 2ms |

### Component-Specific Targets

| Component | Time Target | Alloc Target |
|-----------|-------------|--------------|
| Context Parsing | < 1μs | 0 allocs |
| Context Hashing | < 5μs | < 3 allocs |
| Allow-List (per pattern) | < 10μs | 0 allocs |
| Header Injection | < 1μs | 0 allocs |
| Response Sanitization | < 1μs | 0 allocs |
| Log Redaction | < 2μs | 0 allocs |
| Full middleware chain | < 50μs | < 30 allocs |

## Implementation Hints

### Go Benchmark Tooling

| Tool | Purpose | Usage |
|------|---------|-------|
| `testing.B` | Benchmark framework | `func BenchmarkXxx(b *testing.B)` |
| `go test -bench=.` | Run all benchmarks | `go test -bench=. ./...` |
| `go test -benchmem` | Report memory allocations | `go test -bench=. -benchmem` |
| `go test -benchtime=5s` | Longer benchmark runs | More stable results |
| `go test -count=10` | Multiple runs | For statistical analysis |
| `go test -cpuprofile=cpu.prof` | CPU profiling | Analyze with pprof |
| `go test -memprofile=mem.prof` | Memory profiling | Analyze with pprof |
| `benchstat` | Compare benchmark results | `benchstat old.txt new.txt` |
| `go tool pprof` | Analyze profiles | Interactive analysis |

### Installing Additional Tools

```bash
# benchstat for comparing benchmark runs
go install golang.org/x/perf/cmd/benchstat@latest
```

### Benchmark Categories

#### A. Micro-benchmarks (Unit Level)

Individual functions in isolation. Goal: identify hot spots, optimize critical paths.

```
internal/context/parser_bench_test.go
internal/cache/hash_bench_test.go
internal/router/allowlist_bench_test.go  (or glob_bench_test.go)
internal/sanitizer/redactor_bench_test.go
```

#### B. Component Benchmarks (Integration Level)

Middleware chains, handler pipelines. Goal: measure overhead of each layer.

```
internal/proxy/middleware_bench_test.go
internal/telemetry/metrics_bench_test.go
```

#### C. End-to-End Benchmarks (System Level)

Full request with mock upstream. Goal: total proxy overhead measurement.

```
internal/proxy/integration_bench_test.go
test/benchmark/e2e_bench_test.go
```

#### D. TLS Benchmarks

Connection and handshake performance.

```
internal/proxy/tls_bench_test.go
pkg/crypto/certs_bench_test.go
```

## Benchmark Implementation Patterns

### Basic Benchmark

```go
func BenchmarkContextParsing(b *testing.B) {
    // Setup - not counted in benchmark time
    req := httptest.NewRequest("POST", "/proxy", nil)
    req.Header.Set("X-Connect-Target-URL", "https://api.vendor.com/v1")
    req.Header.Set("X-Connect-Vendor-ID", "microsoft")
    req.Header.Set("X-Connect-Marketplace-ID", "US")
    req.Header.Set("X-Connect-Product-ID", "product-123")
    req.Header.Set("X-Connect-Subscription-ID", "sub-456")
    
    b.ReportAllocs() // Report memory allocations
    b.ResetTimer()   // Start timing now
    
    for i := 0; i < b.N; i++ {
        ctx, err := context.Parse(req)
        if err != nil {
            b.Fatal(err)
        }
        // Prevent compiler optimization
        _ = ctx
    }
}
```

### Parallel Benchmark

```go
func BenchmarkFullRequestCycle(b *testing.B) {
    // Setup mock upstream (instant response)
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(200)
        w.Write([]byte(`{"status":"ok"}`))
    }))
    defer upstream.Close()
    
    proxy := setupTestProxy(upstream.URL)
    
    b.ReportAllocs()
    b.ResetTimer()
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            req := httptest.NewRequest("POST", "/proxy", nil)
            req.Header.Set("X-Connect-Target-URL", upstream.URL+"/api")
            req.Header.Set("X-Connect-Vendor-ID", "test-vendor")
            
            rec := httptest.NewRecorder()
            proxy.ServeHTTP(rec, req)
            
            if rec.Code != 200 {
                b.Errorf("unexpected status: %d", rec.Code)
            }
        }
    })
}
```

### Sub-benchmarks

```go
func BenchmarkAllowList(b *testing.B) {
    validator := NewAllowListValidator(testAllowList)
    
    cases := []struct {
        name string
        url  string
    }{
        {"exact_match", "https://api.vendor.com/v1/customers"},
        {"glob_single", "https://api.vendor.com/v1/customers/123"},
        {"glob_recursive", "https://api.vendor.com/v1/customers/123/orders/456"},
        {"no_match", "https://evil.com/hack"},
    }
    
    for _, tc := range cases {
        b.Run(tc.name, func(b *testing.B) {
            b.ReportAllocs()
            for i := 0; i < b.N; i++ {
                _ = validator.Validate(tc.url)
            }
        })
    }
}
```

### Benchmark with Setup/Teardown

```go
func BenchmarkWithCleanup(b *testing.B) {
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        // Per-iteration setup (not counted in time)
        b.StopTimer()
        resource := expensiveSetup()
        b.StartTimer()
        
        // The actual operation to benchmark
        result := operationUnderTest(resource)
        _ = result
        
        // Cleanup (not counted)
        b.StopTimer()
        resource.Close()
        b.StartTimer()
    }
}
```

### TLS Benchmark

```go
func BenchmarkTLSHandshake(b *testing.B) {
    // Setup server with mTLS
    server := httptest.NewUnstartedServer(handler)
    server.TLS = tlsConfig
    server.StartTLS()
    defer server.Close()

    // Create client with certificates
    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: clientTLSConfig,
            // Disable keep-alives to force new handshakes
            DisableKeepAlives: true,
        },
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        resp, err := client.Get(server.URL)
        if err != nil {
            b.Fatal(err)
        }
        resp.Body.Close()
    }
}

func BenchmarkCertificateValidation(b *testing.B) {
    certPool := x509.NewCertPool()
    certPool.AppendCertsFromPEM(caCert)
    
    cert, _ := x509.ParseCertificate(clientCertDER)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        opts := x509.VerifyOptions{
            Roots:         certPool,
            Intermediates: intermediatePool,
            KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
        }
        _, err := cert.Verify(opts)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## Regression Detection

### Workflow

```bash
# 1. Baseline (before changes)
go test -bench=. -benchmem -count=10 ./... > baseline.txt

# 2. Make changes...

# 3. After changes
go test -bench=. -benchmem -count=10 ./... > current.txt

# 4. Compare
benchstat baseline.txt current.txt
```

### Example Output

```
name                    old time/op    new time/op    delta
ContextParsing-8          892ns ± 2%     756ns ± 1%  -15.25%  (p=0.000 n=10+10)
AllowListValidate-8      1.23µs ± 3%    1.45µs ± 2%  +17.89%  (p=0.000 n=10+10)  # REGRESSION!
FullRequestCycle-8       89.2µs ± 4%    87.1µs ± 3%   -2.35%  (p=0.012 n=10+10)

name                    old alloc/op   new alloc/op   delta
ContextParsing-8           0.00B          0.00B         ~
AllowListValidate-8        0.00B         48.0B ± 0%     +Inf%  # REGRESSION!
FullRequestCycle-8        3.2kB ± 0%     3.1kB ± 0%   -3.13%
```

### Regression Threshold

**Alert/Fail if:**
- Any metric degrades by > 10%
- Any allocation count increases
- New allocations introduced in zero-alloc functions

## Makefile Targets

```makefile
.PHONY: bench bench-save bench-compare

# Run all benchmarks
bench:
	go test -bench=. -benchmem -count=5 ./... | tee benchmark-current.txt

# Save current as baseline
bench-save:
	go test -bench=. -benchmem -count=10 ./... > benchmark-baseline.txt

# Compare current with baseline
bench-compare: bench
	@if [ -f benchmark-baseline.txt ]; then \
		benchstat benchmark-baseline.txt benchmark-current.txt; \
	else \
		echo "No baseline found. Run 'make bench-save' first."; \
	fi
```

## CI Integration

### GitHub Actions Workflow

```yaml
# .github/workflows/benchmark.yml
name: Benchmark
on: [pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Run benchmarks
        run: go test -bench=. -benchmem -count=5 ./... | tee benchmark.txt
      
      - name: Compare with baseline
        run: |
          # Fetch baseline from main branch
          git fetch origin main
          git checkout origin/main -- benchmark-baseline.txt 2>/dev/null || echo "No baseline"
          
          if [ -f benchmark-baseline.txt ]; then
            go install golang.org/x/perf/cmd/benchstat@latest
            benchstat benchmark-baseline.txt benchmark.txt
          fi
```

## Files to Create/Modify

### Benchmark Files (Create)

- [ ] `internal/context/parser_bench_test.go` - Context parsing benchmarks
- [ ] `internal/cache/hash_bench_test.go` - Hash function benchmarks
- [ ] `internal/router/allowlist_bench_test.go` - Glob matching benchmarks
- [ ] `internal/proxy/middleware_bench_test.go` - Middleware chain benchmarks
- [ ] `internal/proxy/tls_bench_test.go` - TLS handshake benchmarks
- [ ] `internal/sanitizer/redactor_bench_test.go` - Log redaction benchmarks
- [ ] `test/benchmark/e2e_bench_test.go` - End-to-end benchmarks

### Infrastructure (Create/Modify)

- [ ] `Makefile` - Add bench targets
- [ ] `.github/workflows/benchmark.yml` - CI workflow
- [ ] `benchmark-baseline.txt` - Committed baseline (after initial run)

## File Organization

```
chaperone/
├── internal/
│   ├── context/
│   │   ├── parser.go
│   │   ├── parser_test.go
│   │   └── parser_bench_test.go      # Micro-benchmark
│   ├── cache/
│   │   ├── hash.go
│   │   ├── hash_test.go
│   │   └── hash_bench_test.go        # Micro-benchmark
│   ├── router/
│   │   ├── allowlist.go
│   │   └── allowlist_bench_test.go   # Micro-benchmark
│   ├── proxy/
│   │   ├── middleware_bench_test.go  # Component benchmark
│   │   └── tls_bench_test.go         # TLS benchmark
│   └── sanitizer/
│       └── redactor_bench_test.go    # Micro-benchmark
├── test/
│   └── benchmark/
│       └── e2e_bench_test.go         # End-to-end benchmark
├── benchmark-baseline.txt            # Committed baseline
└── Makefile                          # bench, bench-save, bench-compare
```

## Testing Strategy

- **Validation tests:**
  - Verify benchmark functions don't panic
  - Verify benchmarks produce consistent results (low variance)
- **CI verification:**
  - Benchmarks complete within timeout (60s)
  - No regressions vs baseline

## Memory Efficiency Metrics

| Metric | Why It Matters | Target | How to Measure |
|--------|----------------|--------|----------------|
| **Allocations/request** | GC pressure = latency spikes | < 50 allocs/op | `-benchmem` flag |
| **Bytes/request** | Memory footprint per op | < 4KB/op | `-benchmem` flag |
| **Heap growth under load** | Memory leaks | Stable (no growth) | `runtime.MemStats` |
| **Goroutine count** | Resource exhaustion | Bounded (pool) | `runtime.NumGoroutine()` |

### Memory Profiling Commands

```bash
# Generate memory profile
go test -bench=BenchmarkFullRequest -memprofile=mem.prof

# Analyze allocations
go tool pprof -alloc_space mem.prof

# Analyze in-use memory
go tool pprof -inuse_space mem.prof

# Top allocators
go tool pprof -top mem.prof
```

## Concurrency Performance

| Scenario | Purpose | Target | Implementation |
|----------|---------|--------|----------------|
| **Parallel requests** | Throughput under contention | Linear scaling to GOMAXPROCS | `b.RunParallel()` |
| **Connection pool efficiency** | Upstream connection reuse | Minimal new connections | Monitor `Transport.IdleConn` |
| **Lock contention** | Mutex bottlenecks | < 1% time in locks | Block profiling |
| **Channel throughput** | Async operations | No blocking | Goroutine profiling |

## Gotchas

- Always use `b.ReportAllocs()` to track memory
- Use `b.ResetTimer()` after expensive setup
- Prevent compiler optimization with `_ = result`
- Run multiple times (`-count=10`) for statistical significance
- TLS benchmarks need `DisableKeepAlives: true` to measure handshake
