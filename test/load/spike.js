// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// test/load/spike.js
// Spike test - sudden 10x traffic surge to test resilience and recovery
// Target: P95<500ms during spike, Error rate<1%, Recovery<30s

import http from 'k6/http';
import { check, sleep } from 'k6';
import { textSummary } from './lib/k6-summary.js';
import { config, tlsAuth, spikeThresholds, getHeaders, errorRate, recordServerTiming } from './config.js';

export const options = {
    tlsAuth,
    insecureSkipTLSVerify: true,
    stages: [
        { duration: '1m', target: 100 },    // Normal load baseline
        { duration: '10s', target: 1000 },  // SPIKE! 10x traffic in 10s
        { duration: '1m', target: 1000 },   // Sustain spike
        { duration: '10s', target: 100 },   // Return to normal
        { duration: '2m', target: 100 },    // Recovery observation
        { duration: '30s', target: 0 },     // Cool down
    ],
    thresholds: spikeThresholds,
};

export default function () {
    const url = `${config.baseUrl}/proxy`;
    const params = {
        headers: getHeaders(),
        timeout: '30s',
    };

    const response = http.post(url, null, params);

    const success = check(response, {
        'status is 2xx': (r) => r.status >= 200 && r.status < 300,
        'no server errors': (r) => r.status < 500,
    });

    errorRate.add(!success);
    recordServerTiming(response);

    sleep(0.1);
}

export function handleSummary(data) {
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        'test/load/results/spike-results.json': JSON.stringify(data),
    };
}
