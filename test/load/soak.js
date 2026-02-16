// test/load/soak.js
// Soak test - long-running endurance test (4+ hours)
// WARNING: This test runs for 4+ hours

import http from 'k6/http';
import { check, sleep } from 'k6';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';
import { config, soakThresholds, getHeaders, errorRate, recordServerTiming } from './config.js';

export const options = {
    stages: [
        { duration: '5m', target: 200 },
        { duration: '4h', target: 200 },
        { duration: '5m', target: 0 },
    ],
    thresholds: soakThresholds,
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
    console.log(`P99 latency: ${data.metrics.http_req_duration.values['p(99)'].toFixed(2)}ms`);

    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        'test/load/results/soak-results.json': JSON.stringify(data),
    };
}
