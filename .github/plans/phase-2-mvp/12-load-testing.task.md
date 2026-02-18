# Task: Load Testing

**Status:** [x] Completed
**Priority:** P1
**Estimated Effort:** M

## Objective

Implement k6-based load testing infrastructure to validate system behavior under realistic production load (concurrent users, sustained traffic, spikes, endurance).

## Design Spec Reference

- **Primary:** Section 9.3.B - Load Testing (k6)
- **Related:** Section 8.1 - Resilience & Reliability
- **Related:** Section 8.3 - Observability & Telemetry

## Dependencies

- [x] `07-telemetry-metrics.task.md` - Prometheus metrics to monitor during load tests
- [x] `11-benchmark-testing.task.md` - Establishes baseline performance expectations

## Acceptance Criteria

- [x] k6 installed and documented
- [x] `test/load/` directory structure created
- [x] Baseline scenario script implemented
- [x] Spike test scenario implemented
- [x] Stress test scenario implemented
- [x] Soak test scenario implemented (documented as manual)
- [x] mTLS scenario with test certificates
- [x] All tests use shared mTLS client certs (chaperone always requires mTLS)
- [x] Thresholds match Chaperone SLOs
- [x] Makefile targets: `load-test`, `load-baseline`, `load-spike`, `load-stress`, `load-smoke`
- [x] Target echo server auto-started by Makefile targets
- [x] README documents how to run tests
- [x] CI workflow runs smoke test on PRs
- [x] Tests pass: `go test ./...`
- [x] Lint passes: `make lint`

## Load Testing vs Benchmark Testing

| Aspect | Benchmark Testing | Load Testing |
|--------|-------------------|--------------|
| **Scope** | Individual functions, components | Full system under load |
| **Tool** | `go test -bench` | k6, hey, wrk |
| **Metric** | ns/op, allocs/op | RPS, P99 latency, error rate |
| **Environment** | Developer machine | Staging/production-like |
| **Duration** | Seconds | Minutes to hours |
| **Concurrency** | `b.RunParallel()` | 100s-1000s virtual users |

## k6 - The Standard Load Testing Tool

### What is k6?

[k6](https://k6.io/) is an open-source load testing tool developed by Grafana Labs:

- **Written in Go** - Fast, single binary, no runtime dependencies
- **JavaScript scripts** - Developer-friendly test authoring
- **Built-in metrics** - Response time, throughput, error rates
- **Thresholds** - Pass/fail criteria for CI/CD
- **Cloud integration** - Optional Grafana Cloud for distributed testing

### Why k6 over alternatives?

| Tool | Pros | Cons | Verdict |
|------|------|------|---------|
| **k6** | Modern, fast, JS scripts, built-in metrics | Learning curve for JS | ✅ Recommended |
| **hey** | Simple, Go-based | Basic features only | Good for quick tests |
| **wrk** | Very fast | Lua scripting, limited metrics | Good for raw throughput |
| **JMeter** | Feature-rich GUI | Heavy, XML config, slow | ❌ Outdated for APIs |
| **Locust** | Python scripts | Slower than Go tools | Good if Python preferred |
| **Vegeta** | Go library | Limited scenarios | Good for programmatic use |

### Installing k6

```bash
# macOS
brew install k6

# Linux (Debian/Ubuntu)
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# Docker
docker run --rm -i grafana/k6 run - <script.js

# From source (Go)
go install go.k6.io/k6@latest
```

## Target Metrics & Acceptance Criteria

### Performance Targets

| Metric | Baseline | Spike | Stress |
|--------|----------|-------|--------|
| **P50 latency** | < 20ms | < 50ms | < 100ms |
| **P95 latency** | < 50ms | < 200ms | < 500ms |
| **P99 latency** | < 100ms | < 500ms | < 1s |
| **Error rate** | < 0.1% | < 1% | < 5% |
| **Throughput** | > 1000 RPS | Sustain | Find limit |

### Chaperone-Specific Criteria

| Criterion | Target | Measurement |
|-----------|--------|-------------|
| **Proxy overhead** | < 5ms added latency | `Server-Timing` header |
| **Connection reuse** | > 95% | Monitor new connections |
| **Memory stability** | No growth over 4h | Soak test + metrics |
| **Graceful degradation** | No crashes under spike | Spike test |
| **Recovery time** | < 30s after spike | Monitor after spike |

## Load Testing Scenarios for Chaperone

### Scenario A: Baseline Load

Steady traffic to establish baseline metrics.

```javascript
// test/load/baseline.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const proxyOverhead = new Trend('proxy_overhead_ms');

export const options = {
    stages: [
        { duration: '30s', target: 50 },    // Warm up
        { duration: '5m', target: 50 },     // Steady state
        { duration: '30s', target: 0 },     // Cool down
    ],
    thresholds: {
        http_req_duration: ['p(99)<100'],   // 99% under 100ms
        http_req_failed: ['rate<0.001'],    // Error rate < 0.1%
        errors: ['rate<0.001'],
    },
};

const BASE_URL = __ENV.PROXY_URL || 'https://localhost:8443';

export default function () {
    const res = http.post(`${BASE_URL}/proxy`, null, {
        headers: {
            'X-Connect-Target-URL': 'https://httpbin.org/status/200',
            'X-Connect-Vendor-ID': 'load-test-vendor',
        },
    });
    
    const success = check(res, {
        'status is 200': (r) => r.status === 200,
    });
    
    errorRate.add(!success);
    
    // Extract Server-Timing if available
    const serverTiming = res.headers['Server-Timing'];
    if (serverTiming) {
        const match = serverTiming.match(/overhead;dur=([\d.]+)/);
        if (match) {
            proxyOverhead.add(parseFloat(match[1]));
        }
    }
    
    sleep(0.5);
}
```

### Scenario B: Spike Test

Sudden traffic surge to test resilience.

```javascript
// test/load/spike.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '1m', target: 100 },    // Normal load
        { duration: '10s', target: 1000 },  // SPIKE! 10x traffic
        { duration: '1m', target: 1000 },   // Sustain spike
        { duration: '10s', target: 100 },   // Back to normal
        { duration: '2m', target: 100 },    // Recovery observation
        { duration: '30s', target: 0 },
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'],   // Relaxed during spike
        http_req_failed: ['rate<0.05'],     // Allow 5% errors during spike
    },
};

export default function () {
    const res = http.post('https://localhost:8443/proxy', null, {
        headers: {
            'X-Connect-Target-URL': 'https://httpbin.org/delay/0',
            'X-Connect-Vendor-ID': 'spike-test',
        },
        timeout: '10s',
    });
    
    check(res, {
        'status is 2xx': (r) => r.status >= 200 && r.status < 300,
    });
    
    sleep(0.1);
}
```

### Scenario C: Stress Test

Find breaking point by increasing load until failure.

```javascript
// test/load/stress.js
import http from 'k6/http';
import { check } from 'k6';

export const options = {
    stages: [
        { duration: '2m', target: 100 },
        { duration: '2m', target: 500 },
        { duration: '2m', target: 1000 },
        { duration: '2m', target: 2000 },
        { duration: '2m', target: 3000 },   // Keep increasing
        { duration: '5m', target: 3000 },   // Hold at max
        { duration: '2m', target: 0 },
    ],
    // No thresholds - we want to find the breaking point
};

export default function () {
    const res = http.post('https://localhost:8443/proxy', null, {
        headers: {
            'X-Connect-Target-URL': 'https://httpbin.org/status/200',
            'X-Connect-Vendor-ID': 'stress-test',
        },
        timeout: '30s',
    });
    
    check(res, {
        'not 5xx': (r) => r.status < 500,
    });
}
```

### Scenario D: Soak Test (Endurance)

Long-running test to detect memory leaks, connection exhaustion.

```javascript
// test/load/soak.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '5m', target: 200 },    // Ramp up
        { duration: '4h', target: 200 },    // 4 hours steady
        { duration: '5m', target: 0 },      // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(99)<100'],
        http_req_failed: ['rate<0.001'],
    },
};

export default function () {
    const res = http.post('https://localhost:8443/proxy', null, {
        headers: {
            'X-Connect-Target-URL': 'https://httpbin.org/get',
            'X-Connect-Vendor-ID': 'soak-test',
        },
    });
    
    check(res, {
        'status is 200': (r) => r.status === 200,
    });
    
    sleep(1);
}
```

### Scenario E: mTLS Load Test

Test with client certificates (Chaperone's main use case).

```javascript
// test/load/mtls.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    tlsAuth: [
        {
            cert: open('./certs/client.crt'),
            key: open('./certs/client.key'),
        },
    ],
    stages: [
        { duration: '1m', target: 100 },
        { duration: '5m', target: 100 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        http_req_duration: ['p(99)<200'],
        http_req_failed: ['rate<0.001'],
    },
};

export default function () {
    const res = http.post('https://localhost:8443/proxy', null, {
        headers: {
            'X-Connect-Target-URL': 'https://api.vendor.com/v1/data',
            'X-Connect-Vendor-ID': 'mtls-test-vendor',
        },
    });
    
    check(res, {
        'status is 200': (r) => r.status === 200,
        'mTLS succeeded': (r) => r.status !== 403,
    });
    
    sleep(0.5);
}
```

## k6 Script Structure

### Key Concepts

| Concept | Description |
|---------|-------------|
| **VU (Virtual User)** | Simulated concurrent user |
| **Iteration** | One execution of `default` function |
| **Stage** | Ramp pattern (duration + target VUs) |
| **Threshold** | Pass/fail criteria |
| **Check** | Assertion (doesn't fail test) |
| **Sleep** | Think time between requests |

## Running k6 Tests

### Basic Execution

```bash
# Run a test
k6 run test/load/baseline.js

# With environment variables
k6 run -e PROXY_URL=https://staging.proxy.local:8443 test/load/baseline.js

# Override VUs and duration
k6 run --vus 50 --duration 2m test/load/baseline.js

# Output to JSON for analysis
k6 run --out json=results.json test/load/baseline.js

# Output to CSV
k6 run --out csv=results.csv test/load/baseline.js
```

### Output Formats

```bash
# Console summary (default)
k6 run script.js

# JSON file
k6 run --out json=results.json script.js

# InfluxDB (for Grafana dashboards)
k6 run --out influxdb=http://localhost:8086/k6 script.js

# Prometheus remote write
k6 run --out experimental-prometheus-rw script.js

# Multiple outputs
k6 run --out json=results.json --out influxdb=http://localhost:8086/k6 script.js
```

## Makefile Integration

```makefile
.PHONY: load-test load-baseline load-spike load-stress load-soak

# Default load test (baseline)
load-test: load-baseline

# Baseline load test
load-baseline:
	k6 run test/load/baseline.js

# Spike test
load-spike:
	k6 run test/load/spike.js

# Stress test (find breaking point)
load-stress:
	k6 run test/load/stress.js

# Soak test (long-running, run manually)
load-soak:
	@echo "WARNING: Soak test runs for 4+ hours"
	@read -p "Continue? [y/N] " confirm && [ "$$confirm" = "y" ]
	k6 run test/load/soak.js

# Run with mTLS
load-mtls:
	k6 run test/load/mtls.js

# Quick smoke test (1 minute, low VUs)
load-smoke:
	k6 run --vus 10 --duration 1m test/load/baseline.js
```

## CI/CD Integration

### GitHub Actions Workflow

```yaml
# .github/workflows/load-test.yml
name: Load Test

on:
  schedule:
    - cron: '0 2 * * *'  # Nightly at 2 AM
  workflow_dispatch:      # Manual trigger
    inputs:
      scenario:
        description: 'Test scenario'
        required: true
        default: 'baseline'
        type: choice
        options:
          - baseline
          - spike
          - stress

jobs:
  load-test:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup k6
        uses: grafana/setup-k6-action@v1
      
      - name: Start Chaperone
        run: |
          docker compose up -d chaperone
          sleep 10  # Wait for startup
      
      - name: Run load test
        run: |
          k6 run \
            -e PROXY_URL=http://localhost:8443 \
            --out json=results.json \
            test/load/${{ inputs.scenario || 'baseline' }}.js
      
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: k6-results
          path: results.json
      
      - name: Check thresholds
        if: failure()
        run: |
          echo "Load test failed! Check results artifact."
          exit 1
```

## Monitoring During Load Tests

### Metrics to Watch

| Metric | Source | Alert Threshold |
|--------|--------|-----------------|
| **CPU usage** | Host/container | > 80% sustained |
| **Memory usage** | Host/container | > 70% or growing |
| **Goroutine count** | `/debug/pprof/goroutine` | Growing unbounded |
| **Open connections** | `netstat` / metrics | > connection pool size |
| **GC pause time** | Go runtime metrics | > 10ms |
| **Error rate** | k6 / Prometheus | > threshold |

### Prometheus Queries

```promql
# Request rate
rate(chaperone_requests_total[1m])

# Error rate
rate(chaperone_requests_total{status=~"5.."}[1m]) / rate(chaperone_requests_total[1m])

# P99 latency
histogram_quantile(0.99, rate(chaperone_request_duration_seconds_bucket[1m]))

# Goroutine count
go_goroutines{job="chaperone"}

# Memory usage
go_memstats_alloc_bytes{job="chaperone"}
```

### Grafana Dashboard

Create a load test dashboard with:

1. **Request Rate** - `rate(chaperone_requests_total[1m])`
2. **Latency Percentiles** - P50, P95, P99
3. **Error Rate** - By status code
4. **Resource Usage** - CPU, Memory, Goroutines
5. **Connection Pool** - Active/idle connections

## Troubleshooting Load Test Issues

### Common Problems

| Issue | Symptom | Solution |
|-------|---------|----------|
| **Connection refused** | Errors spike early | Increase connection pool, check max open files |
| **Timeout errors** | Slow requests fail | Increase timeout, check upstream |
| **Memory growth** | OOM during soak | Profile with pprof, check for leaks |
| **CPU saturation** | 100% CPU, latency degrades | Scale horizontally, optimize hot paths |
| **TLS errors** | Handshake failures | Check cert validity, connection reuse |

### Debugging Commands

```bash
# Check open connections
netstat -an | grep 8443 | wc -l

# Check file descriptors
lsof -p $(pgrep chaperone) | wc -l

# Real-time CPU/memory
htop -p $(pgrep chaperone)

# Goroutine dump
curl http://localhost:9090/debug/pprof/goroutine?debug=2

# Heap profile during load
curl -o heap.prof http://localhost:9090/debug/pprof/heap
go tool pprof heap.prof
```

## Alternative Tools (Quick Reference)

### hey (Simple HTTP load generator)

```bash
# Install
go install github.com/rakyll/hey@latest

# Basic usage
hey -n 10000 -c 100 https://localhost:8443/proxy

# With headers
hey -n 10000 -c 100 \
  -H "X-Connect-Target-URL: https://httpbin.org/get" \
  -H "X-Connect-Vendor-ID: test" \
  https://localhost:8443/proxy
```

### wrk (High-performance HTTP benchmark)

```bash
# Install
brew install wrk  # macOS

# Basic usage
wrk -t12 -c400 -d30s https://localhost:8443/proxy

# With Lua script for headers
wrk -t12 -c400 -d30s -s headers.lua https://localhost:8443/proxy
```

### Vegeta (Go load testing library)

```bash
# Install
go install github.com/tsenart/vegeta@latest

# Basic usage
echo "POST https://localhost:8443/proxy" | \
  vegeta attack -duration=30s -rate=100 | \
  vegeta report
```

## Files to Create/Modify

### k6 Scripts (Create)

- [x] `test/load/README.md` - How to run load tests
- [x] `test/load/config.js` - Shared configuration, thresholds, mTLS certs
- [x] `test/load/baseline.js` - Normal traffic baseline
- [x] `test/load/spike.js` - Traffic spike scenario
- [x] `test/load/stress.js` - Find breaking point
- [x] `test/load/soak.js` - Long-running endurance
- [x] `test/load/mtls.js` - mTLS client cert test

### Target Server (Create)

- [x] `test/load/targetserver/main.go` - Minimal Go echo server for load testing

### Test Certificates (Runtime)

- [x] `test/load/certs/` - Copied from `certs/` by `make gencerts-load` (gitignored)

### Infrastructure (Create/Modify)

- [x] `Makefile` - Add load-* targets, target server, gencerts-load
- [x] `.github/workflows/load-test.yml` - Smoke test on PRs
- [x] `.gitignore` - Ignore `test/load/results/`

## File Organization

```
chaperone/
├── test/
│   └── load/
│       ├── README.md           # How to run load tests
│       ├── baseline.js         # Normal traffic baseline
│       ├── spike.js            # Traffic spike scenario
│       ├── stress.js           # Find breaking point
│       ├── soak.js             # Long-running endurance
│       ├── mtls.js             # mTLS client cert test
│       └── certs/              # Test certificates for mTLS
│           ├── client.crt
│           └── client.key
├── deployments/
│   └── docker-compose.load-test.yml  # Containerized test setup
└── Makefile                    # load-* targets
```

## Testing Strategy

- **Validation:**
  - k6 scripts parse without errors
  - Thresholds are reasonable
  - mTLS certificates work
- **Manual verification:**
  - Run smoke test locally
  - Verify output format

## Summary Checklist

When implementing the load testing task:

- [ ] Install k6 and add to CI dependencies
- [ ] Create `test/load/` directory structure
- [ ] Implement baseline scenario script
- [ ] Implement spike test scenario
- [ ] Implement stress test scenario
- [ ] Implement soak test scenario (document as manual)
- [ ] Add mTLS scenario with test certificates
- [ ] Define thresholds matching Chaperone SLOs
- [ ] Add Makefile targets (`make load-test`, etc.)
- [ ] Document how to run tests in README
- [ ] (Optional) Set up Grafana dashboard for visualization
- [ ] (Optional) Add nightly CI job for regression detection
