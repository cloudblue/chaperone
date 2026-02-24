# Chaperone Load Testing

Run k6-based load tests against the Chaperone egress proxy to validate performance, resilience, and mTLS overhead.

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

The `make load-*` targets automatically generate certificates (`gencerts-load`) and start the target echo server (`load-target-start`). You only need to start the proxy itself in a separate terminal:

```bash
make gencerts   # Generate TLS certificates (one-time)
make run        # Start the proxy (keep this running)
```

### TLS Note

All `make load-*` targets set `K6_INSECURE_SKIP_TLS_VERIFY=true` automatically for the self-signed certificates. When running k6 directly, pass the variable yourself:

```bash
K6_INSECURE_SKIP_TLS_VERIFY=true k6 run test/load/baseline.js
```

## Running Load Tests

### Quick Commands

```bash
make load-smoke      # 1 minute quick validation
make load-baseline   # ~6 minutes (includes ramp), 50 VUs
make load-spike      # Traffic surge test
make load-stress     # ~17 minutes, find limits (see ulimit note below)
make load-soak       # 4+ hours endurance
make load-mtls       # With client certificates
```

### Custom Configuration

When running k6 directly against localhost, remember the TLS variable (see [TLS Note](#tls-note) above):

```bash
K6_INSECURE_SKIP_TLS_VERIFY=true k6 run -e PROXY_URL=https://staging:8443 test/load/baseline.js
K6_INSECURE_SKIP_TLS_VERIFY=true k6 run -e TARGET_URL=https://api.vendor.com/v1/status test/load/baseline.js
K6_INSECURE_SKIP_TLS_VERIFY=true k6 run --vus 100 --duration 10m test/load/baseline.js
K6_INSECURE_SKIP_TLS_VERIFY=true k6 run --out json=results.json test/load/baseline.js
```

## Test Scenarios

| Scenario | Duration | Max VUs | Purpose |
|----------|----------|---------|---------|
| smoke | 1 min | 10 | Quick validation (overrides baseline stages) |
| baseline | ~6 min | 50 | Establish performance baseline |
| spike | ~5 min | 1000 | Test resilience to traffic surges |
| stress | ~17 min | 3000 | Find system breaking point |
| soak | 4+ hours | 200 | Detect memory leaks |
| mtls | ~7 min | 100 | Validate TLS performance |

## Performance Targets (SLOs)

| Metric | Smoke | Baseline | Spike | Stress | Soak | mTLS |
|--------|-------|----------|-------|--------|------|------|
| P50 latency | < 50ms | < 20ms | < 50ms | < 100ms | < 30ms | < 30ms |
| P95 latency | < 150ms | < 50ms | < 200ms | < 500ms | < 75ms | < 75ms |
| P99 latency | < 300ms | < 100ms | < 500ms | < 1s | < 200ms | < 200ms |
| Error rate | < 1% | < 0.1% | < 1% | < 5% | < 0.1% | < 0.1% |

### Stress Test Prerequisites

The stress test ramps to 3000 VUs with no sleep between iterations. Before running, tune your OS to prevent client-side resource exhaustion from masking server limits. See [k6: Fine-tune OS](https://grafana.com/docs/k6/latest/set-up/fine-tune-os/) for the full guide.

```bash
# Linux
sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
sudo sysctl -w net.ipv4.tcp_timestamps=1
ulimit -n 250000

# Then run
make load-stress
```

Without this, you may see connection errors that reflect client fd/port exhaustion
rather than actual server limits.

## Monitoring During Tests

```bash
# Watch Prometheus metrics
curl -s localhost:9090/metrics | grep chaperone_requests_total
curl -s localhost:9090/metrics | grep chaperone_request_duration_seconds

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
├── targetserver/    # Minimal echo server for load testing
│   └── main.go
├── lib/             # Vendored k6 libraries
│   └── k6-summary.js
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
