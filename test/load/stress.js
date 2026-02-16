// test/load/stress.js
// Stress test - gradually increase load until failure to find system limits

import http from 'k6/http';
import { check } from 'k6';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';
import { config, tlsAuth, stressThresholds, getHeaders, errorRate, recordServerTiming } from './config.js';

export const options = {
    tlsAuth,
    insecureSkipTLSVerify: true,
    stages: [
        { duration: '2m', target: 100 },
        { duration: '2m', target: 500 },
        { duration: '2m', target: 1000 },
        { duration: '2m', target: 2000 },
        { duration: '2m', target: 3000 },
        { duration: '5m', target: 3000 },
        { duration: '2m', target: 0 },
    ],
    thresholds: stressThresholds,
    summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

// No sleep() between iterations — intentionally maximizes RPS per VU
// to find the system's breaking point under sustained pressure.
export default function () {
    const url = `${config.baseUrl}/proxy`;
    const params = {
        headers: getHeaders(),
        timeout: '60s',
    };

    const response = http.post(url, null, params);

    const success = check(response, {
        'not 5xx error': (r) => r.status < 500,
        'response received': (r) => r.status !== 0,
    });

    errorRate.add(!success);
    recordServerTiming(response);
}

export function handleSummary(data) {
    const maxVUs = data.metrics.vus_max ? data.metrics.vus_max.values.max : 'N/A';
    const p99 = data.metrics.http_req_duration && data.metrics.http_req_duration.values['p(99)'];
    console.log('\n=== STRESS TEST RESULTS ===');
    console.log(`Max VUs tested: ${maxVUs}`);
    console.log(`Total requests: ${data.metrics.http_reqs.values.count}`);
    console.log(`Error rate: ${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%`);
    console.log(`P99 latency: ${p99 != null ? p99.toFixed(2) + 'ms' : 'N/A'}`);

    return {
        'stdout': textSummary(data, { indent: ' ', enableColors: true }),
        'test/load/results/stress-results.json': JSON.stringify(data),
    };
}
