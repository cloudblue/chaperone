import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get, post, put, del, getCsrfToken, ApiError } from './api.js';

describe('api client', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		Object.defineProperty(document, 'cookie', {
			writable: true,
			value: '',
		});
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

	describe('getCsrfToken', () => {
		it('returns empty string when no cookie', () => {
			document.cookie = '';
			expect(getCsrfToken()).toBe('');
		});

		it('extracts csrf_token from cookies', () => {
			document.cookie = 'session=abc123; csrf_token=my-token-value';
			expect(getCsrfToken()).toBe('my-token-value');
		});

		it('decodes URL-encoded token', () => {
			document.cookie = 'csrf_token=token%20with%20spaces';
			expect(getCsrfToken()).toBe('token with spaces');
		});
	});

	describe('get', () => {
		it('returns parsed JSON on success', async () => {
			mockFetch(200, [{ id: 1 }]);
			const result = await get('/api/instances');
			expect(result).toEqual([{ id: 1 }]);
			expect(globalThis.fetch).toHaveBeenCalledWith('/api/instances', {
				headers: { 'Content-Type': 'application/json' },
			});
		});

		it('does not send CSRF token on GET', async () => {
			document.cookie = 'csrf_token=my-token';
			mockFetch(200, {});
			await get('/api/me');
			const [, opts] = globalThis.fetch.mock.calls[0];
			expect(opts.headers['X-CSRF-Token']).toBeUndefined();
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

		it('redirects to login on 401 when session is established', async () => {
			mockFetch(401, {
				error: { code: 'UNAUTHORIZED', message: 'No valid session' },
			});
			const mockStore = { user: { id: 1 }, ready: true };
			vi.doMock('../stores/auth.js', () => ({
				useAuthStore: () => mockStore,
			}));
			delete window.location;
			window.location = { href: '/' };
			const err = await get('/api/me').catch((e) => e);
			expect(err).toBeInstanceOf(ApiError);
			expect(err.status).toBe(401);
			expect(mockStore.user).toBeNull();
			expect(window.location.href).toBe('/login');
			vi.doUnmock('../stores/auth.js');
		});

		it('does not redirect on 401 during initial session check', async () => {
			mockFetch(401, {
				error: { code: 'UNAUTHORIZED', message: 'No valid session' },
			});
			const mockStore = { user: null, ready: false };
			vi.doMock('../stores/auth.js', () => ({
				useAuthStore: () => mockStore,
			}));
			delete window.location;
			window.location = { href: '/' };
			const err = await get('/api/me').catch((e) => e);
			expect(err).toBeInstanceOf(ApiError);
			expect(err.status).toBe(401);
			expect(window.location.href).toBe('/');
			vi.doUnmock('../stores/auth.js');
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

		it('includes CSRF token on POST', async () => {
			document.cookie = 'csrf_token=my-csrf-token';
			mockFetch(200, {});
			await post('/api/logout');
			const [, opts] = globalThis.fetch.mock.calls[0];
			expect(opts.headers['X-CSRF-Token']).toBe('my-csrf-token');
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

		it('includes CSRF token on PUT', async () => {
			document.cookie = 'csrf_token=put-token';
			mockFetch(204, null);
			await put('/api/user/password', {
				current_password: 'a',
				new_password: 'b',
			});
			const [, opts] = globalThis.fetch.mock.calls[0];
			expect(opts.headers['X-CSRF-Token']).toBe('put-token');
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

		it('sends DELETE method with CSRF token', async () => {
			document.cookie = 'csrf_token=del-token';
			const res = { ok: true, status: 204, json: vi.fn() };
			vi.spyOn(globalThis, 'fetch').mockResolvedValue(res);
			await del('/api/instances/1');

			const [url, opts] = globalThis.fetch.mock.calls[0];
			expect(url).toBe('/api/instances/1');
			expect(opts.method).toBe('DELETE');
			expect(opts.headers['X-CSRF-Token']).toBe('del-token');
		});
	});
});
