<template>
	<AppLayout v-if="auth.isAuthenticated">
		<RouterView />
	</AppLayout>
	<RouterView v-else />
</template>

<script setup>
import { RouterView } from 'vue-router';
import { useAuthStore } from './stores/auth.js';
import AppLayout from './layouts/AppLayout.vue';

const auth = useAuthStore();
</script>
