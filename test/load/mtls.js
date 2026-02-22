// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// test/load/mtls.js
// mTLS load test - validates TLS performance under load with client certs
// Requires: test/load/certs/client.crt and test/load/certs/client.key

import http from 'k6/http';
import { check, sleep } from 'k6';
import { textSummary } from './lib/k6-summary.js';
import { config, tlsAuth, baselineThresholds, getHeaders, errorRate, recordServerTiming } from './config.js';

export const options = {
    tlsAuth,
    insecureSkipTLSVerify: true,
    stages: [
        { duration: '1m', target: 100 },
        { duration: '5m', target: 100 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        ...baselineThresholds,
        http_req_duration: ['p(50)<30', 'p(95)<75', 'p(99)<200'],
    },
};

export default function () {
    const url = `${config.baseUrl}/proxy`;
    const params = {
        headers: getHeaders({ 'X-Connect-Vendor-ID': 'mtls-test-vendor' }),
        timeout: '10s',
    };

    const response = http.post(url, null, params);

    const success = check(response, {
        'status is 200': (r) => r.status === 200,
        'mTLS succeeded (not 403)': (r) => r.status !== 403,
        'TLS handshake ok (not 0)': (r) => r.status !== 0,
    });

    errorRate.add(!success);
    recordServerTiming(response);

    sleep(0.5);
}

export function setup() {
    console.log('Testing mTLS connection...');
    const res = http.get(`${config.baseUrl}/_ops/health`);
    if (res.status !== 200) {
        throw new Error(`mTLS setup failed: status ${res.status}`);
    }
    console.log('mTLS connection verified');
}

export function handleSummary(data) {
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        'test/load/results/mtls-results.json': JSON.stringify(data),
    };
}
