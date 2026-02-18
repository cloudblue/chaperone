// test/load/soak.js
// Soak test - long-running endurance test (4+ hours)
// WARNING: This test runs for 4+ hours

import http from 'k6/http';
import { check, sleep } from 'k6';
import { textSummary } from './lib/k6-summary.js';
import { config, tlsAuth, soakThresholds, getHeaders, errorRate, recordServerTiming } from './config.js';

export const options = {
    tlsAuth,
    insecureSkipTLSVerify: true,
    stages: [
        { duration: '5m', target: 200 },
        { duration: '4h', target: 200 },
        { duration: '5m', target: 0 },
    ],
    thresholds: soakThresholds,
    summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
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

    sleep(1);
}

export function handleSummary(data) {
    const duration = data.state.testRunDurationMs / 1000 / 60;
    console.log(`\n=== SOAK TEST COMPLETE ===`);
    console.log(`Duration: ${duration.toFixed(1)} minutes`);
    console.log(`Total requests: ${data.metrics.http_reqs.values.count}`);
    console.log(`Error rate: ${(data.metrics.http_req_failed.values.rate * 100).toFixed(4)}%`);
    const p99 = data.metrics.http_req_duration && data.metrics.http_req_duration.values['p(99)'];
    console.log(`P99 latency: ${p99 != null ? p99.toFixed(2) + 'ms' : 'N/A'}`);

    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        'test/load/results/soak-results.json': JSON.stringify(data),
    };
}
