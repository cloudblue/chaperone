import { ref, computed, watch } from 'vue';
import { buildAuditQueryString, totalPages } from '../utils/audit.js';

export function useAuditLog(api) {
	const items = ref([]);
	const total = ref(0);
	const loading = ref(false);
	const error = ref(null);

	const filters = ref({
		q: '',
		action: '',
		from: '',
		to: '',
		page: 1,
		perPage: 20,
	});

	let fetchId = 0;

	async function fetch() {
		const id = ++fetchId;
		loading.value = true;
		error.value = null;
		try {
			const qs = buildAuditQueryString(filters.value);
			const data = await api.get(`/api/audit${qs}`);
			if (id !== fetchId) return;
			items.value = data.items;
			total.value = data.total;
		} catch (err) {
			if (id !== fetchId) return;
			error.value = err.message || 'Failed to load audit log';
			items.value = [];
			total.value = 0;
		} finally {
			if (id === fetchId) loading.value = false;
		}
	}

	function setFilter(key, value) {
		filters.value = { ...filters.value, [key]: value, page: 1 };
	}

	function setPage(page) {
		const max = totalPages(total.value, filters.value.perPage);
		filters.value = {
			...filters.value,
			page: Math.max(1, Math.min(page, max)),
		};
	}

	function nextPage() {
		setPage(filters.value.page + 1);
	}

	function prevPage() {
		setPage(filters.value.page - 1);
	}

	const pageCount = computed(() =>
		totalPages(total.value, filters.value.perPage),
	);

	// Refetch when filters change.
	watch(filters, () => fetch(), { deep: true });

	return {
		items,
		total,
		loading,
		error,
		filters,
		fetch,
		setFilter,
		setPage,
		nextPage,
		prevPage,
		pageCount,
	};
}
