<template>
	<div :class="$style.page">
		<div :class="$style.card">
			<div :class="$style.header">
				<h1 :class="$style.title">Chaperone</h1>
				<p :class="$style.subtitle">Sign in to the admin portal</p>
			</div>
			<form :class="$style.form" @submit.prevent="handleSubmit">
				<div
					v-if="error"
					:class="$style.alert"
					role="alert"
					data-testid="login-error"
				>
					{{ error }}
				</div>
				<BaseInput
					v-model="username"
					label="Username"
					placeholder="Enter your username"
					autocomplete="username"
					:disabled="loading"
					data-testid="login-username"
				/>
				<BaseInput
					v-model="password"
					label="Password"
					type="password"
					placeholder="Enter your password"
					autocomplete="current-password"
					:disabled="loading"
					data-testid="login-password"
				/>
				<BaseButton
					type="submit"
					:disabled="loading || !username || !password"
					data-testid="login-submit"
				>
					{{ loading ? 'Signing in...' : 'Sign in' }}
				</BaseButton>
			</form>
		</div>
		<div :class="$style.branding" aria-label="Chaperone, a CloudBlue project">
			<span>Chaperone &mdash; a</span>
			<img :class="$style.brandingLogo" :src="cloudBlueLogo" alt="CloudBlue" />
			<span>project</span>
		</div>
	</div>
</template>

<script setup>
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth.js';
import BaseInput from '../components/BaseInput.vue';
import BaseButton from '../components/BaseButton.vue';
import cloudBlueLogo from '../assets/cloudblue-logo-white.png';

const router = useRouter();
const auth = useAuthStore();

const username = ref('');
const password = ref('');
const error = ref('');
const loading = ref(false);

async function handleSubmit() {
	error.value = '';
	loading.value = true;
	try {
		await auth.login(username.value, password.value);
		router.replace(router.currentRoute.value.query.redirect || '/');
	} catch (err) {
		if (err.status === 429) {
			error.value = 'Too many failed attempts. Please try again later.';
		} else if (err.status === 401) {
			error.value = 'Invalid username or password.';
		} else {
			error.value = 'Something went wrong. Please try again.';
		}
		password.value = '';
	} finally {
		loading.value = false;
	}
}
</script>

<style module>
.page {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	min-height: 100vh;
	background-color: var(--color-bg-primary);
	padding: var(--space-8) var(--space-4);
	gap: var(--space-6);
}

.card {
	width: 100%;
	max-width: 380px;
	background-color: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-xl);
	padding: var(--space-8);
	box-shadow: var(--shadow-lg);
}

.header {
	text-align: center;
	margin-bottom: var(--space-6);
}

.title {
	font-size: var(--font-size-xl);
	font-weight: var(--font-weight-bold);
	color: var(--color-text-primary);
	margin-bottom: var(--space-1);
}

.subtitle {
	font-size: var(--font-size-base);
	color: var(--color-text-secondary);
}

.form {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.form button {
	width: 100%;
	margin-top: var(--space-2);
}

.alert {
	padding: var(--space-3);
	border-radius: var(--radius-md);
	background-color: var(--color-error-bg);
	border: 1px solid var(--color-error-border);
	color: var(--color-error);
	font-size: var(--font-size-sm);
}

.branding {
	display: inline-flex;
	align-items: baseline;
	gap: var(--space-3);
	margin: 0;
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
	text-align: center;
	letter-spacing: -0.02em;
}

.brandingLogo {
	height: 1.3rem;
	width: auto;
	filter: brightness(0) saturate(100%);
}

@media (max-width: 520px) {
	.branding {
		flex-wrap: wrap;
		justify-content: center;
	}
}
</style>
