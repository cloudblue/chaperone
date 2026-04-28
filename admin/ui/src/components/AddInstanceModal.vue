<template>
	<div :class="$style.overlay" @click.self="$emit('close')">
		<div
			ref="modalRef"
			:class="$style.modal"
			role="dialog"
			aria-labelledby="modal-title"
			aria-modal="true"
			tabindex="-1"
		>
			<h2 id="modal-title" :class="$style.title">
				{{ editing ? 'Edit Instance' : 'Add Instance' }}
			</h2>

			<form :class="$style.form" @submit.prevent="handleSubmit">
				<BaseInput
					v-model="name"
					label="Name"
					placeholder="e.g. proxy-prod-01"
					:error="errors.name"
					data-testid="instance-name"
				/>
				<BaseInput
					v-model="address"
					label="Address"
					placeholder="host:port (e.g. 10.0.0.1:9090)"
					:error="errors.address"
					data-testid="instance-address"
				/>

				<div
					v-if="testResult"
					:class="[
						$style.testResult,
						testResult.ok ? $style.testOk : $style.testFail,
					]"
					data-testid="test-result"
				>
					<span v-if="testResult.ok">
						Connected successfully — version {{ testResult.version }}
					</span>
					<span v-else>{{ testResult.error }}</span>
				</div>

				<div :class="$style.actions">
					<BaseButton
						type="button"
						variant="secondary"
						:disabled="testing || !address.trim()"
						data-testid="test-connection"
						@click="handleTest"
					>
						{{ testing ? 'Testing...' : 'Test Connection' }}
					</BaseButton>
					<div :class="$style.spacer" />
					<BaseButton type="button" variant="secondary" @click="$emit('close')">
						Cancel
					</BaseButton>
					<BaseButton
						type="submit"
						variant="primary"
						:disabled="saving"
						data-testid="save-instance"
					>
						{{
							saving ? 'Saving...' : editing ? 'Save Changes' : 'Add Instance'
						}}
					</BaseButton>
				</div>
			</form>
		</div>
	</div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue';
import BaseInput from './BaseInput.vue';
import BaseButton from './BaseButton.vue';
import { useInstanceStore } from '../stores/instances.js';
import { useInstanceForm } from '../composables/useInstanceForm.js';
import { useFocusTrap } from '../composables/useFocusTrap.js';

const props = defineProps({
	instance: { type: Object, default: null },
});

const emit = defineEmits(['close', 'saved']);
const store = useInstanceStore();
const modalRef = ref(null);
const {
	editing,
	name,
	address,
	errors,
	saving,
	testing,
	testResult,
	handleTest,
	handleSubmit: submit,
} = useInstanceForm(store, props.instance);

useFocusTrap(modalRef);

function onKeydown(e) {
	if (e.key === 'Escape') {
		e.preventDefault();
		emit('close');
	}
}

async function handleSubmit() {
	const ok = await submit();
	if (ok) {
		emit('saved');
		emit('close');
	}
}

onMounted(() => {
	document.addEventListener('keydown', onKeydown);
	modalRef.value?.focus();
});

onUnmounted(() => {
	document.removeEventListener('keydown', onKeydown);
});
</script>

<style module>
.overlay {
	position: fixed;
	inset: 0;
	background-color: rgba(0, 0, 0, 0.4);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 100;
}

.modal {
	background-color: var(--color-bg-surface);
	border-radius: var(--radius-xl);
	box-shadow: var(--shadow-modal);
	padding: var(--space-6);
	width: 100%;
	max-width: 480px;
}

.title {
	font-size: var(--font-size-lg);
	font-weight: var(--font-weight-bold);
	margin: 0 0 var(--space-5);
}

.form {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.testResult {
	padding: var(--space-3);
	border-radius: var(--radius-md);
	font-size: var(--font-size-sm);
}

.testOk {
	background-color: var(--color-success-bg);
	color: var(--color-success);
	border: 1px solid var(--color-success-border);
}

.testFail {
	background-color: var(--color-error-bg);
	color: var(--color-error);
	border: 1px solid var(--color-error-border);
}

.actions {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	padding-top: var(--space-3);
}

.spacer {
	flex: 1;
}

@media (max-width: 768px) {
	.modal {
		margin: var(--space-4);
		max-width: calc(100vw - var(--space-8));
	}

	.actions {
		flex-wrap: wrap;
	}
}
</style>
