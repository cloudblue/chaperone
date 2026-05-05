import { ref, computed } from 'vue';
import { defineStore } from 'pinia';
import * as api from '../utils/api.js';

export const useAuthStore = defineStore('auth', () => {
	const user = ref(null);
	const ready = ref(false);
	const isAuthenticated = computed(() => user.value !== null);

	async function checkSession() {
		try {
			const data = await api.get('/api/me');
			user.value = data.user;
		} catch {
			user.value = null;
		} finally {
			ready.value = true;
		}
	}

	async function login(username, password) {
		const data = await api.post('/api/login', { username, password });
		user.value = data.user;
	}

	async function logout() {
		await api.post('/api/logout');
		user.value = null;
	}

	async function changePassword(currentPassword, newPassword) {
		await api.put('/api/user/password', {
			current_password: currentPassword,
			new_password: newPassword,
		});
	}

	return {
		user,
		ready,
		isAuthenticated,
		checkSession,
		login,
		logout,
		changePassword,
	};
});
