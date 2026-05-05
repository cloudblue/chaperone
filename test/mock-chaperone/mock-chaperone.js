#!/usr/bin/env node
/**
 * Copyright 2026 CloudBlue LLC
 * SPDX-License-Identifier: Apache-2.0
 */


'use strict';

const http = require('node:http');

// ─── Configuration ──────────────────────────────────────────────────────────
// Edit this section to customize the mock fleet.
// Alternatively, pass CLI args: node mock-chaperone.js [count] [startPort]

const CONFIG = {
  instances: [
    { port: 19091, name: 'proxy-us-east-1', profile: 'healthy' },
    { port: 19092, name: 'proxy-eu-west-1', profile: 'healthy' },
    { port: 19093, name: 'proxy-ap-south-1', profile: 'degraded' },
  ],
  vendors: ['acme-corp', 'globex-inc', 'initech-llc'],
  version: '0.8.2',
  tickIntervalMs: 1000,
};

// ─── Profile Definitions ────────────────────────────────────────────────────
// Each profile defines traffic characteristics per tick (1 second).

const PROFILES = {
  healthy: {
    rpsRange: [20, 60],
    errorRate: 0.02,
    latencyMean: 0.15,
    latencyStddev: 0.08,
    activeConnRange: [5, 25],
    panicRate: 0.0001,
    available: true,
  },
  degraded: {
    rpsRange: [10, 30],
    errorRate: 0.15,
    latencyMean: 0.6,
    latencyStddev: 0.3,
    activeConnRange: [15, 50],
    panicRate: 0.005,
    available: true,
  },
  flapping: {
    rpsRange: [20, 60],
    errorRate: 0.02,
    latencyMean: 0.15,
    latencyStddev: 0.08,
    activeConnRange: [5, 25],
    panicRate: 0.0001,
    available: true,
    flapIntervalMs: 30000,
  },
};

// ─── Histogram Buckets ──────────────────────────────────────────────────────
// Matches chaperone's APILatencyBuckets exactly (internal/telemetry/metrics.go).

const BUCKETS = [0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.4, 0.5, 0.75, 1, 2, 5, 10];

// ─── Helpers ────────────────────────────────────────────────────────────────

function randInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function gaussianRandom(mean, stddev) {
  const u1 = Math.random();
  const u2 = Math.random();
  const z = Math.sqrt(-2 * Math.log(u1)) * Math.cos(2 * Math.PI * u2);
  return mean + z * stddev;
}

function weightedSplit(total, weights) {
  const result = {};
  let remaining = total;
  const entries = Object.entries(weights);
  for (let i = 0; i < entries.length - 1; i++) {
    const [key, weight] = entries[i];
    const count = Math.round(total * weight);
    result[key] = count;
    remaining -= count;
  }
  result[entries[entries.length - 1][0]] = Math.max(0, remaining);
  return result;
}

function addHistogramObservation(histogram, value) {
  histogram.sum += value;
  histogram.count++;
  for (let i = 0; i < BUCKETS.length; i++) {
    if (value <= BUCKETS[i]) {
      histogram.buckets[i]++;
      return;
    }
  }
  // Falls into +Inf only (counted in .count but not in any named bucket)
}

// ─── Instance State ─────────────────────────────────────────────────────────

class InstanceState {
  constructor(instanceConfig, vendors) {
    this.instanceConfig = instanceConfig;
    this.profile = PROFILES[instanceConfig.profile];
    this.startTime = new Date().toISOString();
    this.vendors = vendors;
    this.available = this.profile.available;

    // Counters: { vendor: { '2xx': { GET: n, ... }, '4xx': {...}, '5xx': {...} } }
    this.requests = {};
    // Histograms: { vendor: { buckets: [...], sum: n, count: n } }
    this.requestDuration = {};
    this.upstreamDuration = {};
    this.activeConnections = 0;
    this.panicsTotal = 0;

    // V0.5 — cache stats
    this.cacheHits = 0;
    this.cacheMisses = 0;
    this.cacheEvictions = 0;

    for (const vendor of vendors) {
      this.requests[vendor] = {};
      for (const cls of ['2xx', '3xx', '4xx', '5xx']) {
        this.requests[vendor][cls] = { GET: 0, POST: 0, PUT: 0, DELETE: 0 };
      }
      this.requestDuration[vendor] = {
        buckets: new Array(BUCKETS.length).fill(0), sum: 0, count: 0,
      };
      this.upstreamDuration[vendor] = {
        buckets: new Array(BUCKETS.length).fill(0), sum: 0, count: 0,
      };
    }

    // Flapping: toggle availability on interval
    if (instanceConfig.profile === 'flapping') {
      setInterval(() => { this.available = !this.available; },
        this.profile.flapIntervalMs || 30000);
    }
  }

  tick() {
    if (!this.available) return;

    const p = this.profile;
    const rps = randInt(p.rpsRange[0], p.rpsRange[1]);

    for (const vendor of this.vendors) {
      const vendorRps = Math.max(1, Math.round(rps / this.vendors.length + randInt(-3, 3)));
      const methods = weightedSplit(vendorRps, { GET: 0.6, POST: 0.2, PUT: 0.15, DELETE: 0.05 });

      for (const [method, count] of Object.entries(methods)) {
        const errors = Math.round(count * p.errorRate);
        const clientErrors = Math.round(errors * 0.7);
        const serverErrors = errors - clientErrors;

        this.requests[vendor]['2xx'][method] += count - errors;
        this.requests[vendor]['4xx'][method] += clientErrors;
        this.requests[vendor]['5xx'][method] += serverErrors;

        for (let i = 0; i < count; i++) {
          const latency = Math.max(0.001, gaussianRandom(p.latencyMean, p.latencyStddev));
          addHistogramObservation(this.requestDuration[vendor], latency);
          addHistogramObservation(this.upstreamDuration[vendor], latency * (0.7 + Math.random() * 0.2));
        }
      }

      // Cache: ~80% hit rate for healthy, ~50% for degraded
      const hitRate = p.errorRate < 0.1 ? 0.8 : 0.5;
      const cacheOps = Math.round(vendorRps * 0.6);
      this.cacheHits += Math.round(cacheOps * hitRate);
      this.cacheMisses += cacheOps - Math.round(cacheOps * hitRate);
      if (Math.random() < 0.01) this.cacheEvictions += randInt(1, 5);
    }

    this.activeConnections = randInt(p.activeConnRange[0], p.activeConnRange[1]);
    if (Math.random() < p.panicRate) this.panicsTotal++;
  }

  toPrometheus() {
    const lines = [];

    // chaperone_requests_total
    lines.push('# HELP chaperone_requests_total Total number of requests processed');
    lines.push('# TYPE chaperone_requests_total counter');
    for (const vendor of this.vendors) {
      for (const [cls, methods] of Object.entries(this.requests[vendor])) {
        for (const [method, count] of Object.entries(methods)) {
          if (count > 0) {
            lines.push(`chaperone_requests_total{vendor_id="${vendor}",status_class="${cls}",method="${method}"} ${count}`);
          }
        }
      }
    }

    // chaperone_request_duration_seconds
    lines.push('');
    lines.push('# HELP chaperone_request_duration_seconds Total request duration including plugin and upstream');
    lines.push('# TYPE chaperone_request_duration_seconds histogram');
    for (const vendor of this.vendors) {
      const h = this.requestDuration[vendor];
      let cumulative = 0;
      for (let i = 0; i < BUCKETS.length; i++) {
        cumulative += h.buckets[i];
        lines.push(`chaperone_request_duration_seconds_bucket{vendor_id="${vendor}",le="${BUCKETS[i]}"} ${cumulative}`);
      }
      lines.push(`chaperone_request_duration_seconds_bucket{vendor_id="${vendor}",le="+Inf"} ${h.count}`);
      lines.push(`chaperone_request_duration_seconds_sum{vendor_id="${vendor}"} ${h.sum.toFixed(6)}`);
      lines.push(`chaperone_request_duration_seconds_count{vendor_id="${vendor}"} ${h.count}`);
    }

    // chaperone_upstream_duration_seconds
    lines.push('');
    lines.push('# HELP chaperone_upstream_duration_seconds Time spent waiting for upstream response');
    lines.push('# TYPE chaperone_upstream_duration_seconds histogram');
    for (const vendor of this.vendors) {
      const h = this.upstreamDuration[vendor];
      let cumulative = 0;
      for (let i = 0; i < BUCKETS.length; i++) {
        cumulative += h.buckets[i];
        lines.push(`chaperone_upstream_duration_seconds_bucket{vendor_id="${vendor}",le="${BUCKETS[i]}"} ${cumulative}`);
      }
      lines.push(`chaperone_upstream_duration_seconds_bucket{vendor_id="${vendor}",le="+Inf"} ${h.count}`);
      lines.push(`chaperone_upstream_duration_seconds_sum{vendor_id="${vendor}"} ${h.sum.toFixed(6)}`);
      lines.push(`chaperone_upstream_duration_seconds_count{vendor_id="${vendor}"} ${h.count}`);
    }

    // chaperone_active_connections
    lines.push('');
    lines.push('# HELP chaperone_active_connections Number of active connections');
    lines.push('# TYPE chaperone_active_connections gauge');
    lines.push(`chaperone_active_connections ${this.activeConnections}`);

    // chaperone_panics_total
    lines.push('');
    lines.push('# HELP chaperone_panics_total Total number of recovered panics');
    lines.push('# TYPE chaperone_panics_total counter');
    lines.push(`chaperone_panics_total ${this.panicsTotal}`);

    return lines.join('\n') + '\n';
  }

  getCacheStats() {
    const total = this.cacheHits + this.cacheMisses;
    return {
      entries: Math.min(Math.round(total * 0.1), 10000),
      hits: this.cacheHits,
      misses: this.cacheMisses,
      hit_ratio: total > 0 ? +(this.cacheHits / total).toFixed(4) : 0,
      evictions: this.cacheEvictions,
    };
  }

  getTlsStatus() {
    const now = new Date();
    const notBefore = new Date(now);
    notBefore.setMonth(notBefore.getMonth() - 6);
    const notAfter = new Date(now);
    notAfter.setMonth(notAfter.getMonth() + 3);
    const daysUntilExpiry = Math.round((notAfter - now) / (1000 * 60 * 60 * 24));

    return {
      issuer: 'CN=Chaperone Internal CA,O=CloudBlue,C=US',
      subject: `CN=${this.instanceConfig.name}.chaperone.local`,
      not_before: notBefore.toISOString(),
      not_after: notAfter.toISOString(),
      days_until_expiry: daysUntilExpiry,
      serial: 'AB:CD:EF:01:23:45:67:89',
    };
  }

  getConfig() {
    return {
      server: {
        addr: `:8443`,
        admin_addr: `:${this.instanceConfig.port}`,
      },
      upstream: {
        allowed_hosts: ['*.vendor-api.com', 'api.acme-corp.com', 'api.globex-inc.com'],
        timeout: '30s',
      },
      tls: {
        cert_file: '[REDACTED]',
        key_file: '[REDACTED]',
        ca_file: '[REDACTED]',
        min_version: 'TLS1.3',
      },
      cache: { ttl: '5m', max_entries: 10000 },
      headers: { prefix: 'X-Connect' },
      observability: { metrics_enabled: true, profiling_enabled: false },
    };
  }

  testAllowlist(testUrl) {
    const allowedPatterns = ['*.vendor-api.com', 'api.acme-corp.com', 'api.globex-inc.com'];
    let hostname = '';
    try {
      hostname = new URL(testUrl).hostname;
    } catch {
      return { allowed: false, matched_host: '', matched_pattern: '', explanation: 'Invalid URL' };
    }
    for (const pattern of allowedPatterns) {
      if (pattern.startsWith('*.')) {
        const suffix = pattern.slice(1); // ".vendor-api.com"
        if (hostname.endsWith(suffix) || hostname === pattern.slice(2)) {
          return { allowed: true, matched_host: hostname, matched_pattern: pattern, explanation: `Wildcard match: ${hostname} matches ${pattern}` };
        }
      } else if (hostname === pattern) {
        return { allowed: true, matched_host: hostname, matched_pattern: pattern, explanation: `Exact match: ${hostname}` };
      }
    }
    return { allowed: false, matched_host: hostname, matched_pattern: '', explanation: `No matching allow-list entry for ${hostname}` };
  }
}

// ─── HTTP Server ────────────────────────────────────────────────────────────

function createServer(instanceConfig, state) {
  return http.createServer((req, res) => {
    if (!state.available) {
      res.destroy();
      return;
    }

    const url = new URL(req.url, `http://localhost:${instanceConfig.port}`);

    if (req.method === 'GET' && url.pathname === '/_ops/health') {
      json(res, 200, { status: 'alive' });
    } else if (req.method === 'GET' && url.pathname === '/_ops/version') {
      json(res, 200, { version: CONFIG.version, start_time: state.startTime });
    } else if (req.method === 'GET' && url.pathname === '/metrics') {
      res.writeHead(200, { 'Content-Type': 'text/plain; version=0.0.4; charset=utf-8' });
      res.end(state.toPrometheus());
    } else if (req.method === 'GET' && url.pathname === '/_ops/config') {
      json(res, 200, state.getConfig());
    } else if (req.method === 'GET' && url.pathname === '/_ops/cache/stats') {
      json(res, 200, state.getCacheStats());
    } else if (req.method === 'GET' && url.pathname === '/_ops/tls/status') {
      json(res, 200, state.getTlsStatus());
    } else if (req.method === 'GET' && url.pathname === '/_ops/allowlist/test') {
      json(res, 200, state.testAllowlist(url.searchParams.get('url') || ''));
    } else {
      json(res, 404, { error: 'not found' });
    }
  });
}

function json(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(body));
}

// ─── CLI ────────────────────────────────────────────────────────────────────

function parseArgs() {
  const args = process.argv.slice(2);

  if (args.includes('--help') || args.includes('-h')) {
    console.log(`Usage: node mock-chaperone.js [options]

Options:
  [count]                 Number of instances (default: uses CONFIG)
  [count] [startPort]     Number of instances + starting port (default: 19091)
  --help, -h              Show this help

Examples:
  node mock-chaperone.js                  # 3 instances from CONFIG
  node mock-chaperone.js 5                # 5 healthy instances on 19091-19095
  node mock-chaperone.js 5 19100          # 5 healthy instances on 19100-19104

Edit CONFIG in the script for full customization (profiles, vendors, etc.).`);
    process.exit(0);
  }

  if (args.length >= 1 && !isNaN(args[0])) {
    const count = parseInt(args[0], 10);
    const startPort = args.length >= 2 && !isNaN(args[1]) ? parseInt(args[1], 10) : 19091;
    const names = ['us-east-1', 'eu-west-1', 'ap-south-1', 'us-west-2', 'eu-central-1',
      'ap-northeast-1', 'sa-east-1', 'af-south-1', 'me-south-1', 'ca-central-1'];
    CONFIG.instances = [];
    for (let i = 0; i < count; i++) {
      const region = names[i % names.length];
      CONFIG.instances.push({
        port: startPort + i,
        name: `proxy-${region}`,
        profile: i === count - 1 && count > 1 ? 'degraded' : 'healthy',
      });
    }
  }
}

// ─── Main ───────────────────────────────────────────────────────────────────

function main() {
  parseArgs();

  for (const inst of CONFIG.instances) {
    const state = new InstanceState(inst, CONFIG.vendors);
    const server = createServer(inst, state);
    server.listen(inst.port, () => {
      console.log(`  [${inst.name}] :${inst.port} (${inst.profile})`);
    });

    // Evolve metrics every tick
    setInterval(() => state.tick(), CONFIG.tickIntervalMs);
  }

  console.log(`\nMock Chaperone fleet — ${CONFIG.instances.length} instances`);
  console.log(`Vendors: ${CONFIG.vendors.join(', ')}`);
  console.log(`Metrics tick: every ${CONFIG.tickIntervalMs}ms\n`);

  console.log('Endpoints per instance:');
  console.log('  GET /_ops/health         Health check');
  console.log('  GET /_ops/version        Version + start_time');
  console.log('  GET /metrics             Prometheus metrics');
  console.log('  GET /_ops/config         Running config (V0.5)');
  console.log('  GET /_ops/cache/stats    Cache statistics (V0.5)');
  console.log('  GET /_ops/tls/status     TLS cert info (V0.5)');
  console.log('  GET /_ops/allowlist/test?url=...  URL test (V0.5)');
  console.log('\nPress Ctrl+C to stop.\n');
}

main();
