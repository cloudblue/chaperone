// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// test/load/config.js
// Shared k6 configuration for Chaperone load tests
// Matches SLOs from Design Spec Section 9.3.B

import { Rate, Trend } from 'k6/metrics';

// Custom metrics for Chaperone-specific measurements
export const errorRate = new Rate('errors');
export const proxyOverhead = new Trend('proxy_overhead_ms');

// mTLS client certificates (required by Chaperone server)
// Generated via: make gencerts && make gencerts-load
export const tlsAuth = [
    {
        cert: open('./certs/client.crt'),
        key: open('./certs/client.key'),
    },
];

// Environment configuration with defaults
export const config = {
    baseUrl: __ENV.PROXY_URL || 'https://localhost:8443',
    targetUrl: __ENV.TARGET_URL || 'http://localhost:9999/api',
    vendorId: __ENV.VENDOR_ID || 'load-test-vendor',
};

// Thresholds matching Chaperone SLOs
export const baselineThresholds = {
    http_req_duration: ['p(50)<20', 'p(95)<50', 'p(99)<100'],
    http_req_failed: ['rate<0.001'],
    errors: ['rate<0.001'],
    proxy_overhead_ms: ['p(99)<50'],  // Design Spec 9.3.B SLO
};

export const spikeThresholds = {
    http_req_duration: ['p(50)<50', 'p(95)<200', 'p(99)<500'],
    http_req_failed: ['rate<0.01'],
    errors: ['rate<0.01'],
};

export const stressThresholds = {
    http_req_duration: ['p(50)<100', 'p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.05'],
    errors: ['rate<0.05'],
};

// Relaxed thresholds for smoke tests — no ramp-up phase means cold-start
// requests have outsized impact on percentiles in a short 1-minute window.
export const smokeThresholds = {
    http_req_duration: ['p(50)<50', 'p(95)<150', 'p(99)<300'],
    http_req_failed: ['rate<0.01'],
    errors: ['rate<0.01'],
};

// Relaxed thresholds for soak tests — accounts for GC pauses, memory
// pressure, and occasional jitter across 4+ hour runs.
export const soakThresholds = {
    http_req_duration: ['p(50)<30', 'p(95)<75', 'p(99)<200'],
    http_req_failed: ['rate<0.001'],
    errors: ['rate<0.001'],
    proxy_overhead_ms: ['p(99)<75'],
};

// Shared request headers
export function getHeaders(overrides = {}) {
    return {
        'X-Connect-Target-URL': config.targetUrl,
        'X-Connect-Vendor-ID': config.vendorId,
        'X-Connect-Marketplace-ID': 'load-test',
        'Connect-Request-ID': `load-${Date.now()}-${Math.random().toString(36).substring(2, 11)}`,
        ...overrides,
    };
}

// Extract Server-Timing header for proxy overhead measurement
export function recordServerTiming(response) {
    const serverTiming = response.headers['Server-Timing'];
    if (serverTiming) {
        const match = serverTiming.match(/overhead;dur=([\d.]+)/);
        if (match) {
            proxyOverhead.add(parseFloat(match[1]));
        }
    }
}
