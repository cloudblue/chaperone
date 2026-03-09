<template>
	<button
		:class="[$style.button, $style[variant], $style[size]]"
		:disabled="disabled"
		v-bind="$attrs"
	>
		<slot />
	</button>
</template>

<script setup>
defineProps({
	variant: {
		type: String,
		default: "primary",
		validator: (v) => ["primary", "secondary", "danger", "ghost"].includes(v),
	},
	size: {
		type: String,
		default: "md",
		validator: (v) => ["sm", "md"].includes(v),
	},
	disabled: {
		type: Boolean,
		default: false,
	},
});
</script>

<style module>
.button {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	gap: var(--space-2);
	border: 1px solid transparent;
	border-radius: var(--radius-lg);
	font-weight: var(--font-weight-semibold);
	font-size: var(--font-size-sm);
	line-height: var(--line-height-tight);
	transition:
		background-color 0.15s,
		border-color 0.15s,
		box-shadow 0.15s;
}

.button:disabled {
	opacity: 0.5;
	cursor: not-allowed;
}

/* Sizes */
.md {
	padding: 0.5rem 1rem;
}

.sm {
	padding: 0.3125rem 0.625rem;
	font-size: var(--font-size-xs);
}

/* Variants */
.primary {
	background-color: var(--color-accent);
	color: var(--color-text-inverse);
}

.primary:hover:not(:disabled) {
	background-color: var(--color-accent-hover);
}

.secondary {
	background-color: var(--color-bg-surface);
	color: var(--color-text-primary);
	border-color: var(--color-border);
}

.secondary:hover:not(:disabled) {
	background-color: var(--color-bg-primary);
}

.danger {
	background-color: var(--color-error);
	color: var(--color-text-inverse);
}

.danger:hover:not(:disabled) {
	background-color: var(--color-error-hover);
}

.ghost {
	background-color: transparent;
	color: var(--color-text-tertiary);
	border-color: transparent;
}

.ghost:hover:not(:disabled) {
	background-color: var(--color-error-bg);
	color: var(--color-error);
}
</style>
