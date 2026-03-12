import { describe, it, expect, vi, beforeEach } from 'vitest';
import { withSetup } from '../utils/test-utils.js';
import { useInstanceForm } from './useInstanceForm.js';

function mockStore(overrides = {}) {
	return {
		testConnection: vi.fn().mockResolvedValue({ ok: true, version: '1.0.0' }),
		createInstance: vi.fn().mockResolvedValue({ id: 1 }),
		updateInstance: vi.fn().mockResolvedValue({ id: 1 }),
		...overrides,
	};
}

describe('useInstanceForm', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	describe('initialization', () => {
		it('starts empty for a new instance', () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			expect(result.editing).toBe(false);
			expect(result.name.value).toBe('');
			expect(result.address.value).toBe('');
			expect(result.errors.name).toBe('');
			expect(result.errors.address).toBe('');
		});

		it('populates from existing instance for editing', () => {
			const store = mockStore();
			const { result } = withSetup(() =>
				useInstanceForm(store, {
					id: 5,
					name: 'proxy-1',
					address: '10.0.0.1:9090',
				}),
			);
			expect(result.editing).toBe(true);
			expect(result.name.value).toBe('proxy-1');
			expect(result.address.value).toBe('10.0.0.1:9090');
		});
	});

	describe('validate', () => {
		it('returns true and clears errors for valid inputs', () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			result.name.value = 'proxy-1';
			result.address.value = '10.0.0.1:9090';
			expect(result.validate()).toBe(true);
			expect(result.errors.name).toBe('');
			expect(result.errors.address).toBe('');
		});

		it('returns false and sets errors for empty inputs', () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			expect(result.validate()).toBe(false);
			expect(result.errors.name).toBe('Name is required');
			expect(result.errors.address).toBe('Address is required');
		});
	});

	describe('handleTest', () => {
		it('sets testResult on successful connection', async () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			result.address.value = '10.0.0.1:9090';
			await result.handleTest();
			expect(store.testConnection).toHaveBeenCalledWith('10.0.0.1:9090');
			expect(result.testResult.value).toEqual({ ok: true, version: '1.0.0' });
		});

		it('sets error testResult on network failure', async () => {
			const store = mockStore({
				testConnection: vi.fn().mockRejectedValue(new Error('network')),
			});
			const { result } = withSetup(() => useInstanceForm(store));
			result.address.value = '10.0.0.1:9090';
			await result.handleTest();
			expect(result.testResult.value).toEqual({
				ok: false,
				error: 'Failed to test connection',
			});
		});

		it('manages testing flag during the request', async () => {
			let resolve;
			const store = mockStore({
				testConnection: vi
					.fn()
					.mockReturnValue(new Promise((r) => (resolve = r))),
			});
			const { result } = withSetup(() => useInstanceForm(store));
			result.address.value = '10.0.0.1:9090';
			const p = result.handleTest();
			expect(result.testing.value).toBe(true);
			resolve({ ok: true, version: '1.0.0' });
			await p;
			expect(result.testing.value).toBe(false);
		});

		it('clears previous testResult before starting', async () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			result.testResult.value = { ok: true, version: 'old' };
			result.address.value = '10.0.0.1:9090';
			const p = result.handleTest();
			// testResult is null immediately after calling handleTest
			expect(result.testResult.value).toBeNull();
			await p;
		});
	});

	describe('handleSubmit', () => {
		it('returns false without calling store when validation fails', async () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			const ok = await result.handleSubmit();
			expect(ok).toBe(false);
			expect(store.createInstance).not.toHaveBeenCalled();
		});

		it('calls createInstance for new instance and returns true', async () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			result.name.value = 'proxy-1';
			result.address.value = '10.0.0.1:9090';
			const ok = await result.handleSubmit();
			expect(ok).toBe(true);
			expect(store.createInstance).toHaveBeenCalledWith(
				'proxy-1',
				'10.0.0.1:9090',
			);
		});

		it('calls updateInstance for existing instance and returns true', async () => {
			const store = mockStore();
			const { result } = withSetup(() =>
				useInstanceForm(store, {
					id: 5,
					name: 'proxy-1',
					address: '10.0.0.1:9090',
				}),
			);
			result.name.value = 'proxy-updated';
			const ok = await result.handleSubmit();
			expect(ok).toBe(true);
			expect(store.updateInstance).toHaveBeenCalledWith(
				5,
				'proxy-updated',
				'10.0.0.1:9090',
			);
		});

		it('sets address error on API failure and returns false', async () => {
			const store = mockStore({
				createInstance: vi
					.fn()
					.mockRejectedValue(new Error('Address already registered')),
			});
			const { result } = withSetup(() => useInstanceForm(store));
			result.name.value = 'proxy-1';
			result.address.value = '10.0.0.1:9090';
			const ok = await result.handleSubmit();
			expect(ok).toBe(false);
			expect(result.errors.address).toBe('Address already registered');
		});

		it('manages saving flag during the request', async () => {
			let resolve;
			const store = mockStore({
				createInstance: vi
					.fn()
					.mockReturnValue(new Promise((r) => (resolve = r))),
			});
			const { result } = withSetup(() => useInstanceForm(store));
			result.name.value = 'proxy-1';
			result.address.value = '10.0.0.1:9090';
			const p = result.handleSubmit();
			expect(result.saving.value).toBe(true);
			resolve({ id: 1 });
			await p;
			expect(result.saving.value).toBe(false);
		});

		it('trims name and address before sending', async () => {
			const store = mockStore();
			const { result } = withSetup(() => useInstanceForm(store));
			result.name.value = '  proxy-1  ';
			result.address.value = '  10.0.0.1:9090  ';
			await result.handleSubmit();
			expect(store.createInstance).toHaveBeenCalledWith(
				'proxy-1',
				'10.0.0.1:9090',
			);
		});
	});
});
