import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get, post, put, del, ApiError } from './api.js';

describe('api client', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	function mockFetch(status, body, { json = true } = {}) {
		const res = {
			ok: status >= 200 && status < 300,
			status,
			json: json
				? vi.fn().mockResolvedValue(body)
				: vi.fn().mockRejectedValue(new Error('not json')),
		};
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(res);
		return res;
	}

	describe('get', () => {
		it('returns parsed JSON on success', async () => {
			mockFetch(200, [{ id: 1 }]);
			const result = await get('/api/instances');
			expect(result).toEqual([{ id: 1 }]);
			expect(globalThis.fetch).toHaveBeenCalledWith('/api/instances', {
				headers: { 'Content-Type': 'application/json' },
			});
		});

		it('throws ApiError with server message on failure', async () => {
			mockFetch(404, {
				error: {
					code: 'INSTANCE_NOT_FOUND',
					message: 'No instance with ID 42',
				},
			});
			const err = await get('/api/instances/42').catch((e) => e);
			expect(err).toBeInstanceOf(ApiError);
			expect(err.message).toBe('No instance with ID 42');
			expect(err.status).toBe(404);
			expect(err.code).toBe('INSTANCE_NOT_FOUND');
		});

		it('throws ApiError with generic message when response is not JSON', async () => {
			mockFetch(500, null, { json: false });
			const err = await get('/api/instances').catch((e) => e);
			expect(err).toBeInstanceOf(ApiError);
			expect(err.message).toBe('Request failed (500)');
			expect(err.status).toBe(500);
		});
	});

	describe('post', () => {
		it('sends JSON body and returns parsed response', async () => {
			mockFetch(201, { id: 1, name: 'proxy-1' });
			const result = await post('/api/instances', {
				name: 'proxy-1',
				address: '10.0.0.1:9090',
			});
			expect(result).toEqual({ id: 1, name: 'proxy-1' });

			const [url, opts] = globalThis.fetch.mock.calls[0];
			expect(url).toBe('/api/instances');
			expect(opts.method).toBe('POST');
			expect(JSON.parse(opts.body)).toEqual({
				name: 'proxy-1',
				address: '10.0.0.1:9090',
			});
		});

		it('throws ApiError with server message on conflict', async () => {
			mockFetch(409, {
				error: {
					code: 'DUPLICATE_ADDRESS',
					message: 'Address already registered',
				},
			});
			const err = await post('/api/instances', {
				name: 'x',
				address: '1:2',
			}).catch((e) => e);
			expect(err).toBeInstanceOf(ApiError);
			expect(err.message).toBe('Address already registered');
			expect(err.code).toBe('DUPLICATE_ADDRESS');
		});
	});

	describe('put', () => {
		it('sends JSON body with PUT method', async () => {
			mockFetch(200, { id: 1, name: 'updated' });
			const result = await put('/api/instances/1', { name: 'updated' });
			expect(result).toEqual({ id: 1, name: 'updated' });

			const [, opts] = globalThis.fetch.mock.calls[0];
			expect(opts.method).toBe('PUT');
		});
	});

	describe('del', () => {
		it('returns null for 204 No Content', async () => {
			const res = {
				ok: true,
				status: 204,
				json: vi.fn(),
			};
			vi.spyOn(globalThis, 'fetch').mockResolvedValue(res);
			const result = await del('/api/instances/1');
			expect(result).toBeNull();
			expect(res.json).not.toHaveBeenCalled();
		});

		it('sends DELETE method', async () => {
			const res = { ok: true, status: 204, json: vi.fn() };
			vi.spyOn(globalThis, 'fetch').mockResolvedValue(res);
			await del('/api/instances/1');

			const [url, opts] = globalThis.fetch.mock.calls[0];
			expect(url).toBe('/api/instances/1');
			expect(opts.method).toBe('DELETE');
		});
	});
});
