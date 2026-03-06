import { defineStore } from "pinia";
import { ref } from "vue";

export const useInstanceStore = defineStore("instances", () => {
	const instances = ref([]);
	const loading = ref(false);

	async function fetchInstances() {
		loading.value = true;
		try {
			const res = await fetch("/api/instances");
			if (!res.ok) throw new Error("Failed to fetch instances");
			instances.value = await res.json();
		} finally {
			loading.value = false;
		}
	}

	async function createInstance(name, address) {
		const res = await fetch("/api/instances", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ name, address }),
		});
		if (!res.ok) {
			const data = await res.json();
			throw new Error(data.error?.message || "Failed to create instance");
		}
		const inst = await res.json();
		await fetchInstances();
		return inst;
	}

	async function updateInstance(id, name, address) {
		const res = await fetch(`/api/instances/${id}`, {
			method: "PUT",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ name, address }),
		});
		if (!res.ok) {
			const data = await res.json();
			throw new Error(data.error?.message || "Failed to update instance");
		}
		const inst = await res.json();
		await fetchInstances();
		return inst;
	}

	async function deleteInstance(id) {
		const res = await fetch(`/api/instances/${id}`, { method: "DELETE" });
		if (!res.ok) throw new Error("Failed to delete instance");
		await fetchInstances();
	}

	async function testConnection(address) {
		const res = await fetch("/api/instances/test", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ address }),
		});
		return await res.json();
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
