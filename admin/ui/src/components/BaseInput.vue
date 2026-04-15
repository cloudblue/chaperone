<template>
	<div :class="$style.wrapper">
		<label v-if="label" :class="$style.label" :for="inputId">{{ label }}</label>
		<input
			:id="inputId"
			:class="[$style.input, { [$style.hasError]: error }]"
			:type="type"
			:value="modelValue"
			:placeholder="placeholder"
			:disabled="disabled"
			:aria-invalid="error ? 'true' : undefined"
			:aria-describedby="error ? errorId : undefined"
			v-bind="$attrs"
			@input="$emit('update:modelValue', $event.target.value)"
		/>
		<p
			v-if="error"
			:id="errorId"
			:class="$style.errorText"
			:data-testid="
				$attrs['data-testid'] ? `${$attrs['data-testid']}-error` : undefined
			"
		>
			{{ error }}
		</p>
	</div>
</template>

<script setup>
import { computed, useId } from 'vue';

defineOptions({ inheritAttrs: false });

const generatedId = useId();

const props = defineProps({
	modelValue: {
		type: String,
		default: '',
	},
	label: {
		type: String,
		default: '',
	},
	type: {
		type: String,
		default: 'text',
	},
	placeholder: {
		type: String,
		default: '',
	},
	error: {
		type: String,
		default: '',
	},
	disabled: {
		type: Boolean,
		default: false,
	},
	id: {
		type: String,
		default: null,
	},
});

defineEmits(['update:modelValue']);

const inputId = computed(() => props.id ?? generatedId);
const errorId = computed(() => `${inputId.value}-error`);
</script>

<style module>
.wrapper {
	display: flex;
	flex-direction: column;
	gap: var(--space-1);
}

.label {
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
	color: var(--color-text-primary);
}

.input {
	padding: 0.625rem 0.875rem;
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	font-family: inherit;
	font-size: var(--font-size-base);
	color: var(--color-text-primary);
	background-color: var(--color-bg-surface);
	transition:
		border-color 0.15s,
		box-shadow 0.15s;
}

.input::placeholder {
	color: var(--color-text-tertiary);
}

.input:focus {
	outline: none;
	border-color: var(--color-accent);
	box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.input:focus-visible {
	box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.35);
}

.input:disabled {
	opacity: 0.5;
	cursor: not-allowed;
}

.input.hasError {
	border-color: var(--color-error);
	box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.1);
}

.errorText {
	font-size: var(--font-size-xs);
	color: var(--color-error);
}
</style>
