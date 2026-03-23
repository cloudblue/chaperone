import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useMetricsStore } from './metrics.js';

vi.mock('../utils/api.js', () => ({
	get: vi.fn(),
}));

import * as api from '../utils/api.js';

beforeEach(() => {
	setActivePinia(createPinia());
	vi.clearAllMocks();
});

describe('useMetricsStore', () => {
	describe('fetchFleetMetrics', () => {
		it('stores fleet metrics on success', async () => {
			const data = { total_rps: 100, fleet_error_rate: 0.02 };
			api.get.mockResolvedValue(data);

			const store = useMetricsStore();
			await store.fetchFleetMetrics();

			expect(api.get).toHaveBeenCalledWith('/api/metrics/fleet');
			expect(store.fleet).toEqual(data);
			expect(store.fleetError).toBe(null);
		});

		it('stores error on failure', async () => {
			const err = new Error('network');
			api.get.mockRejectedValue(err);

			const store = useMetricsStore();
			await store.fetchFleetMetrics();

			expect(store.fleetError).toBe(err);
		});
	});

	describe('fetchInstanceMetrics', () => {
		it('stores instance metrics on success', async () => {
			const data = { instance_id: 1, rps: 50 };
			api.get.mockResolvedValue(data);

			const store = useMetricsStore();
			await store.fetchInstanceMetrics(1);

			expect(api.get).toHaveBeenCalledWith('/api/metrics/1');
			expect(store.instance).toEqual(data);
			expect(store.instanceError).toBe(null);
		});

		it('clears instance on 404', async () => {
			const err = new Error('not found');
			err.status = 404;
			api.get.mockRejectedValue(err);

			const store = useMetricsStore();
			store.instance = { old: true };
			await store.fetchInstanceMetrics(99);

			expect(store.instance).toBe(null);
			expect(store.instanceError).toBe(err);
		});

		it('preserves instance data on non-404 error', async () => {
			const err = new Error('server error');
			err.status = 500;
			api.get.mockRejectedValue(err);

			const store = useMetricsStore();
			const existing = { instance_id: 1 };
			store.instance = existing;
			await store.fetchInstanceMetrics(1);

			expect(store.instance).toStrictEqual(existing);
			expect(store.instanceError).toBe(err);
		});
	});

	describe('clearInstance', () => {
		it('resets instance state', () => {
			const store = useMetricsStore();
			store.instance = { rps: 100 };
			store.instanceError = new Error('old');

			store.clearInstance();

			expect(store.instance).toBe(null);
			expect(store.instanceError).toBe(null);
		});
	});
});
