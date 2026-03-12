import { defineStore } from 'pinia';
import { ref } from 'vue';
import * as api from '../utils/api.js';

export const useInstanceStore = defineStore('instances', () => {
	const instances = ref([]);
	const loading = ref(false);

	async function fetchInstances() {
		loading.value = true;
		try {
			instances.value = await api.get('/api/instances');
		} finally {
			loading.value = false;
		}
	}

	async function createInstance(name, address) {
		const inst = await api.post('/api/instances', { name, address });
		await fetchInstances();
		return inst;
	}

	async function updateInstance(id, name, address) {
		const inst = await api.put(`/api/instances/${id}`, { name, address });
		await fetchInstances();
		return inst;
	}

	async function deleteInstance(id) {
		await api.del(`/api/instances/${id}`);
		await fetchInstances();
	}

	async function testConnection(address) {
		return api.post('/api/instances/test', { address });
	}

	return {
		instances,
		loading,
		fetchInstances,
		createInstance,
		updateInstance,
		deleteInstance,
		testConnection,
	};
});
