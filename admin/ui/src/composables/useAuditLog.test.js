import { describe, it, expect, vi, beforeEach } from 'vitest';
import { nextTick } from 'vue';
import { withSetup } from '../utils/test-utils.js';
import { useAuditLog } from './useAuditLog.js';

function makeApi(response) {
	return {
		get: vi
			.fn()
			.mockResolvedValue(response ?? { items: [], total: 0, page: 1 }),
	};
}

function makeMockData(count = 3) {
	return {
		items: Array.from({ length: count }, (_, i) => ({
			id: i + 1,
			user_id: 1,
			user: 'admin',
			action: 'instance.create',
			instance_id: i + 10,
			detail: `Created instance ${i + 1}`,
			created_at: '2026-03-09T12:00:00Z',
		})),
		total: count,
		page: 1,
	};
}

describe('useAuditLog', () => {
	let api;

	beforeEach(() => {
		api = makeApi(makeMockData());
	});

	it('initializes with empty state', () => {
		const { result } = withSetup(() => useAuditLog(api));
		expect(result.items.value).toEqual([]);
		expect(result.total.value).toBe(0);
		expect(result.loading.value).toBe(false);
		expect(result.error.value).toBeNull();
	});

	it('fetches audit entries from API', async () => {
		const { result } = withSetup(() => useAuditLog(api));
		await result.fetch();
		expect(api.get).toHaveBeenCalledWith('/api/audit');
		expect(result.items.value).toHaveLength(3);
		expect(result.total.value).toBe(3);
	});

	it('sets loading state during fetch', async () => {
		let resolvePromise;
		api.get = vi.fn(
			() =>
				new Promise((resolve) => {
					resolvePromise = resolve;
				}),
		);
		const { result } = withSetup(() => useAuditLog(api));

		const fetchPromise = result.fetch();
		expect(result.loading.value).toBe(true);

		resolvePromise({ items: [], total: 0, page: 1 });
		await fetchPromise;
		expect(result.loading.value).toBe(false);
	});

	it('sets error on API failure', async () => {
		api.get = vi.fn().mockRejectedValue(new Error('Network error'));
		const { result } = withSetup(() => useAuditLog(api));

		await result.fetch();
		expect(result.error.value).toBe('Network error');
		expect(result.items.value).toEqual([]);
	});

	it('builds query string from filters', async () => {
		const { result } = withSetup(() => useAuditLog(api));
		result.setFilter('action', 'user.login');
		await nextTick();
		// Wait for the watcher-triggered fetch
		await vi.waitFor(() => {
			expect(api.get).toHaveBeenCalledWith(
				expect.stringContaining('action=user.login'),
			);
		});
	});

	it('resets page to 1 when setting a filter', () => {
		const { result } = withSetup(() => useAuditLog(api));
		result.filters.value.page = 3;
		result.setFilter('q', 'test');
		expect(result.filters.value.page).toBe(1);
	});

	it('navigates pages with nextPage/prevPage', async () => {
		api = makeApi({ items: [], total: 60, page: 1 });
		const { result } = withSetup(() => useAuditLog(api));
		await result.fetch();

		result.nextPage();
		expect(result.filters.value.page).toBe(2);

		result.nextPage();
		expect(result.filters.value.page).toBe(3);

		result.prevPage();
		expect(result.filters.value.page).toBe(2);
	});

	it('clamps page within valid range', async () => {
		api = makeApi({ items: [], total: 40, page: 1 });
		const { result } = withSetup(() => useAuditLog(api));
		await result.fetch();

		result.prevPage();
		expect(result.filters.value.page).toBe(1);

		result.setPage(999);
		expect(result.filters.value.page).toBe(2); // total 40, perPage 20 = 2 pages
	});

	it('computes pageCount correctly', async () => {
		api = makeApi({ items: [], total: 45, page: 1 });
		const { result } = withSetup(() => useAuditLog(api));
		await result.fetch();
		expect(result.pageCount.value).toBe(3);
	});

	it('discards stale responses from overlapping fetches', async () => {
		let resolveFirst;
		let resolveSecond;
		api.get = vi
			.fn()
			.mockImplementationOnce(
				() =>
					new Promise((r) => {
						resolveFirst = r;
					}),
			)
			.mockImplementationOnce(
				() =>
					new Promise((r) => {
						resolveSecond = r;
					}),
			);
		const { result } = withSetup(() => useAuditLog(api));

		const first = result.fetch();
		const second = result.fetch();

		// Resolve second (newer) first
		resolveSecond({ items: [{ id: 2 }], total: 1, page: 1 });
		await second;

		// Resolve first (stale) after
		resolveFirst({ items: [{ id: 1 }], total: 1, page: 1 });
		await first;

		// Should keep the second (newer) result
		expect(result.items.value).toEqual([{ id: 2 }]);
	});

	it('refetches automatically when filters change', async () => {
		const { result } = withSetup(() => useAuditLog(api));
		// Initial fetch
		await result.fetch();
		api.get.mockClear();

		result.setFilter('q', 'proxy');
		await nextTick();
		await vi.waitFor(() => {
			expect(api.get).toHaveBeenCalled();
		});
	});
});
