import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAuthStore } from './auth.js';

vi.mock('../utils/api.js', () => ({
	get: vi.fn(),
	post: vi.fn(),
	put: vi.fn(),
}));

import * as api from '../utils/api.js';

describe('useAuthStore', () => {
	let store;

	beforeEach(() => {
		setActivePinia(createPinia());
		store = useAuthStore();
		vi.restoreAllMocks();
	});

	describe('checkSession', () => {
		it('sets user on valid session', async () => {
			api.get.mockResolvedValue({ user: { id: 1, username: 'admin' } });
			await store.checkSession();
			expect(store.user).toEqual({ id: 1, username: 'admin' });
			expect(store.ready).toBe(true);
			expect(store.isAuthenticated).toBe(true);
		});

		it('clears user on invalid session', async () => {
			api.get.mockRejectedValue(new Error('401'));
			await store.checkSession();
			expect(store.user).toBeNull();
			expect(store.ready).toBe(true);
			expect(store.isAuthenticated).toBe(false);
		});
	});

	describe('login', () => {
		it('sets user on success', async () => {
			api.post.mockResolvedValue({ user: { id: 1, username: 'admin' } });
			await store.login('admin', 'password123456');
			expect(api.post).toHaveBeenCalledWith('/api/login', {
				username: 'admin',
				password: 'password123456',
			});
			expect(store.user).toEqual({ id: 1, username: 'admin' });
		});

		it('propagates error on failure', async () => {
			const err = new Error('Invalid');
			err.status = 401;
			api.post.mockRejectedValue(err);
			await expect(store.login('admin', 'wrong')).rejects.toThrow('Invalid');
			expect(store.user).toBeNull();
		});
	});

	describe('logout', () => {
		it('clears user on success', async () => {
			store.user = { id: 1, username: 'admin' };
			api.post.mockResolvedValue(null);
			await store.logout();
			expect(api.post).toHaveBeenCalledWith('/api/logout');
			expect(store.user).toBeNull();
		});
	});

	describe('changePassword', () => {
		it('sends correct payload', async () => {
			api.put.mockResolvedValue(null);
			await store.changePassword('old-password1', 'new-password1');
			expect(api.put).toHaveBeenCalledWith('/api/user/password', {
				current_password: 'old-password1',
				new_password: 'new-password1',
			});
		});

		it('propagates error on failure', async () => {
			const err = new Error('Current password is incorrect');
			err.status = 401;
			api.put.mockRejectedValue(err);
			await expect(
				store.changePassword('wrong', 'new-password1'),
			).rejects.toThrow('Current password is incorrect');
		});
	});
});
