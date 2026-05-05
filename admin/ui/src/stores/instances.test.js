import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useInstanceStore } from './instances.js';

vi.mock('../utils/api.js', () => ({
	get: vi.fn(),
	post: vi.fn(),
	put: vi.fn(),
	del: vi.fn(),
}));

import * as api from '../utils/api.js';

describe('useInstanceStore', () => {
	let store;

	beforeEach(() => {
		vi.restoreAllMocks();
		setActivePinia(createPinia());
		store = useInstanceStore();
	});

	describe('fetchInstances', () => {
		it('populates instances from API', async () => {
			const data = [{ id: 1, name: 'proxy-1' }];
			api.get.mockResolvedValue(data);
			await store.fetchInstances();
			expect(api.get).toHaveBeenCalledWith('/api/instances');
			expect(store.instances).toEqual(data);
		});

		it('sets initialized after first fetch', async () => {
			expect(store.initialized).toBe(false);
			api.get.mockResolvedValue([]);
			await store.fetchInstances();
			expect(store.initialized).toBe(true);
		});

		it('sets initialized even on error', async () => {
			api.get.mockRejectedValue(new Error('network error'));
			await store.fetchInstances().catch(() => {});
			expect(store.initialized).toBe(true);
		});
	});

	describe('createInstance', () => {
		it('calls api.post and refreshes instances', async () => {
			api.post.mockResolvedValue({
				id: 1,
				name: 'proxy-1',
				address: '10.0.0.1:9090',
			});
			api.get.mockResolvedValue([{ id: 1 }]);
			const result = await store.createInstance('proxy-1', '10.0.0.1:9090');
			expect(api.post).toHaveBeenCalledWith('/api/instances', {
				name: 'proxy-1',
				address: '10.0.0.1:9090',
			});
			expect(result).toEqual({
				id: 1,
				name: 'proxy-1',
				address: '10.0.0.1:9090',
			});
			expect(api.get).toHaveBeenCalledWith('/api/instances');
		});

		it('throws on API error', async () => {
			api.post.mockRejectedValue(new Error('Address already registered'));
			await expect(store.createInstance('x', '1:2')).rejects.toThrow(
				'Address already registered',
			);
		});
	});

	describe('updateInstance', () => {
		it('calls api.put and refreshes instances', async () => {
			api.put.mockResolvedValue({ id: 1, name: 'updated' });
			api.get.mockResolvedValue([{ id: 1 }]);
			const result = await store.updateInstance(1, 'updated', '10.0.0.1:9090');
			expect(api.put).toHaveBeenCalledWith('/api/instances/1', {
				name: 'updated',
				address: '10.0.0.1:9090',
			});
			expect(result).toEqual({ id: 1, name: 'updated' });
		});
	});

	describe('deleteInstance', () => {
		it('calls api.del and refreshes instances', async () => {
			api.del.mockResolvedValue(null);
			api.get.mockResolvedValue([]);
			await store.deleteInstance(1);
			expect(api.del).toHaveBeenCalledWith('/api/instances/1');
			expect(api.get).toHaveBeenCalledWith('/api/instances');
		});
	});

	describe('testConnection', () => {
		it('calls api.post and returns result', async () => {
			api.post.mockResolvedValue({ ok: true, version: '1.0.0' });
			const result = await store.testConnection('10.0.0.1:9090');
			expect(api.post).toHaveBeenCalledWith('/api/instances/test', {
				address: '10.0.0.1:9090',
			});
			expect(result).toEqual({ ok: true, version: '1.0.0' });
		});
	});
});
