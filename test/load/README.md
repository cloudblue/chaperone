# Chaperone Load Testing

This directory contains k6-based load testing scripts for the Chaperone egress proxy.

## Prerequisites

### Install k6

```bash
# macOS
brew install k6

# Linux (Debian/Ubuntu)
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
    --keyserver hkp://keyserver.ubuntu.com:80 \
    --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
    | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install k6

# From source (Go)
go install go.k6.io/k6@latest

# Docker
docker run --rm -i grafana/k6 run - <script.js
```

### Start Chaperone

```bash
make gencerts
make run
```

### TLS Note

When testing against localhost with self-signed certificates, you may need to
skip TLS verification:

```bash
K6_INSECURE_SKIP_TLS_VERIFY=true make load-baseline
```

## Running Load Tests

### Quick Commands

```bash
make load-smoke      # 1 minute quick validation
make load-baseline   # 5 minutes, 50 VUs
make load-spike      # Traffic surge test
make load-stress     # ~17 minutes, find limits
make load-soak       # 4+ hours endurance
make load-mtls       # With client certificates
```

### Custom Configuration

```bash
k6 run -e PROXY_URL=https://staging:8443 test/load/baseline.js
k6 run -e TARGET_URL=https://api.vendor.com/v1/status test/load/baseline.js
k6 run --vus 100 --duration 10m test/load/baseline.js
k6 run --out json=results.json test/load/baseline.js
```

## Test Scenarios

| Scenario | Duration | Max VUs | Purpose |
|----------|----------|---------|---------|
| baseline | ~6 min | 50 | Establish performance baseline |
| spike | ~5 min | 1000 | Test resilience to traffic surges |
| stress | ~17 min | 3000 | Find system breaking point |
| soak | 4+ hours | 200 | Detect memory leaks |
| mtls | ~7 min | 100 | Validate TLS performance |

## Performance Targets (SLOs)

| Metric | Baseline | Spike | Stress | Soak |
|--------|----------|-------|--------|------|
| P50 latency | < 20ms | < 50ms | < 100ms | < 30ms |
| P95 latency | < 50ms | < 200ms | < 500ms | < 75ms |
| P99 latency | < 100ms | < 500ms | < 1s | < 200ms |
| Error rate | < 0.1% | < 1% | < 5% | < 0.1% |

## Monitoring During Tests

```bash
# Watch Prometheus metrics
curl -s localhost:9090/metrics | grep chaperone_requests_total
curl -s localhost:9090/metrics | grep chaperone_request_duration

# Watch active connections
curl -s localhost:9090/metrics | grep chaperone_active_connections
```

## File Organization

```
test/load/
├── README.md        # This file
├── config.js        # Shared configuration and thresholds
├── baseline.js      # Baseline scenario (steady traffic)
├── spike.js         # Spike test (sudden surge)
├── stress.js        # Stress test (find breaking point)
├── soak.js          # Soak test (endurance, 4+ hours)
├── mtls.js          # mTLS scenario with client certs
├── certs/           # Certificates (generated at runtime)
│   ├── client.crt
│   ├── client.key
│   └── ca.crt
└── results/         # Test results (gitignored)
    └── *-results.json
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROXY_URL` | `https://localhost:8443` | Chaperone proxy URL |
| `TARGET_URL` | `http://localhost:9999/api` | Target URL for proxied requests |
| `VENDOR_ID` | `load-test-vendor` | Vendor ID for X-Connect-Vendor-ID header |

## Interpreting Results

### Success Indicators
- All thresholds pass (green checkmarks)
- P99 latency under target
- Error rate below threshold
- No timeout errors

### Warning Signs
- Increasing latency over time (potential memory leak)
- Sudden error spikes (capacity limit reached)
- Connection timeouts (resource exhaustion)
- 5xx errors (upstream failures)

### Failure Recovery
If tests fail:
1. Check Chaperone logs for errors
2. Monitor `/metrics` for resource usage
3. Review connection pool saturation
4. Verify target service availability
