// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// test/load/baseline.js
// Baseline load test - steady traffic to establish performance baseline
// Target: P50<20ms, P95<50ms, P99<100ms, Error rate<0.1%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { textSummary } from './lib/k6-summary.js';
import { config, tlsAuth, baselineThresholds, smokeThresholds, getHeaders, errorRate, recordServerTiming } from './config.js';

const isSmoke = __ENV.K6_SCENARIO === 'smoke';

export const options = {
    tlsAuth,
    insecureSkipTLSVerify: true,
    // When invoked as smoke (--vus/--duration override stages), use relaxed
    // thresholds that account for cold-start requests without a ramp-up phase.
    stages: isSmoke ? undefined : [
        { duration: '30s', target: 50 },    // Warm up to 50 VUs
        { duration: '5m', target: 50 },     // Steady state at 50 VUs
        { duration: '30s', target: 0 },     // Cool down
    ],
    thresholds: isSmoke ? smokeThresholds : baselineThresholds,
};

export default function () {
    const url = `${config.baseUrl}/proxy`;
    const params = {
        headers: getHeaders(),
        timeout: '10s',
    };

    const response = http.post(url, null, params);

    const success = check(response, {
        'status is 200': (r) => r.status === 200,
    });

    errorRate.add(!success);
    recordServerTiming(response);

    sleep(0.5);
}

export function handleSummary(data) {
    const scenario = __ENV.K6_SCENARIO || 'baseline';
    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        [`test/load/results/${scenario}-results.json`]: JSON.stringify(data),
    };
}
