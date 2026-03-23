import { defineStore } from 'pinia';
import { ref } from 'vue';
import * as api from '../utils/api.js';

export const useMetricsStore = defineStore('metrics', () => {
	const fleet = ref(null);
	const instance = ref(null);
	const fleetError = ref(null);
	const instanceError = ref(null);

	async function fetchFleetMetrics() {
		try {
			fleet.value = await api.get('/api/metrics/fleet');
			fleetError.value = null;
		} catch (err) {
			fleetError.value = err;
		}
	}

	async function fetchInstanceMetrics(id) {
		try {
			instance.value = await api.get(`/api/metrics/${id}`);
			instanceError.value = null;
		} catch (err) {
			if (err.status === 404) {
				instance.value = null;
			}
			instanceError.value = err;
		}
	}

	function clearInstance() {
		instance.value = null;
		instanceError.value = null;
	}

	return {
		fleet,
		instance,
		fleetError,
		instanceError,
		fetchFleetMetrics,
		fetchInstanceMetrics,
		clearInstance,
	};
});
