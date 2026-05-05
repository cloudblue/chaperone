<template>
	<div :class="$style.page">
		<h1 :class="$style.heading">Settings</h1>

		<section :class="$style.section">
			<h2 :class="$style.sectionTitle">Change Password</h2>
			<p :class="$style.sectionDesc">
				Password must be between 12 and 72 characters.
			</p>

			<form :class="$style.form" @submit.prevent="handleSubmit">
				<div
					v-if="successMessage"
					:class="$style.success"
					role="status"
					data-testid="settings-success"
				>
					{{ successMessage }}
				</div>
				<div
					v-if="serverError"
					:class="$style.alert"
					role="alert"
					data-testid="settings-error"
				>
					{{ serverError }}
				</div>
				<BaseInput
					v-model="currentPassword"
					label="Current password"
					type="password"
					autocomplete="current-password"
					:error="errors.currentPassword"
					:disabled="loading"
					data-testid="settings-current-password"
				/>
				<BaseInput
					v-model="newPassword"
					label="New password"
					type="password"
					autocomplete="new-password"
					:error="errors.newPassword"
					:disabled="loading"
					data-testid="settings-new-password"
				/>
				<BaseInput
					v-model="confirmPassword"
					label="Confirm new password"
					type="password"
					autocomplete="new-password"
					:error="errors.confirmPassword"
					:disabled="loading"
					data-testid="settings-confirm-password"
				/>
				<div :class="$style.actions">
					<BaseButton
						type="submit"
						:disabled="loading"
						data-testid="settings-submit"
					>
						{{ loading ? 'Changing...' : 'Change password' }}
					</BaseButton>
				</div>
			</form>
		</section>
	</div>
</template>

<script setup>
import { ref } from 'vue';
import { useAuthStore } from '../stores/auth.js';
import { validatePasswordChange } from '../utils/validation.js';
import BaseInput from '../components/BaseInput.vue';
import BaseButton from '../components/BaseButton.vue';

const auth = useAuthStore();

const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const errors = ref({});
const serverError = ref('');
const successMessage = ref('');
const loading = ref(false);

function clearErrors() {
	errors.value = {};
	serverError.value = '';
	successMessage.value = '';
}

async function handleSubmit() {
	clearErrors();

	const validation = validatePasswordChange(
		currentPassword.value,
		newPassword.value,
		confirmPassword.value,
	);
	if (Object.keys(validation).length > 0) {
		errors.value = validation;
		return;
	}

	loading.value = true;
	try {
		await auth.changePassword(currentPassword.value, newPassword.value);
		currentPassword.value = '';
		newPassword.value = '';
		confirmPassword.value = '';
		successMessage.value = 'Password changed successfully.';
	} catch (err) {
		if (err.status === 403) {
			serverError.value = 'Current password is incorrect.';
		} else if (err.status === 400) {
			serverError.value = err.message;
		} else {
			serverError.value = 'Something went wrong. Please try again.';
		}
	} finally {
		loading.value = false;
	}
}
</script>

<style module>
.page {
	max-width: 480px;
}

.heading {
	font-size: var(--font-size-xl);
	font-weight: var(--font-weight-bold);
	color: var(--color-text-primary);
	margin-bottom: var(--space-6);
}

.section {
	background-color: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	padding: var(--space-6);
}

.sectionTitle {
	font-size: var(--font-size-md);
	font-weight: var(--font-weight-semibold);
	color: var(--color-text-primary);
	margin-bottom: var(--space-1);
}

.sectionDesc {
	font-size: var(--font-size-sm);
	color: var(--color-text-secondary);
	margin-bottom: var(--space-5);
}

.form {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.actions {
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

.success {
	padding: var(--space-3);
	border-radius: var(--radius-md);
	background-color: var(--color-success-bg);
	border: 1px solid var(--color-success-border);
	color: var(--color-success);
	font-size: var(--font-size-sm);
}
</style>
