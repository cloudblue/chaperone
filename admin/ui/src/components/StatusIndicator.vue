<template>
	<span
		:class="[$style.indicator, $style[status]]"
		:aria-label="computedAriaLabel"
		role="status"
	>
		<span :class="$style.dot" aria-hidden="true" />
		<span v-if="label" :class="$style.label">{{ label }}</span>
	</span>
</template>

<script setup>
import { computed } from "vue";

const props = defineProps({
	status: {
		type: String,
		default: "unknown",
		validator: (v) => ["healthy", "unreachable", "unknown", "stale"].includes(v),
	},
	label: {
		type: String,
		default: "",
	},
});

const statusLabels = {
	healthy: "Healthy",
	unreachable: "Unreachable",
	unknown: "Unknown",
	stale: "Stale",
};

const computedAriaLabel = computed(() => {
	return props.label || statusLabels[props.status];
});
</script>

<style module>
.indicator {
	display: inline-flex;
	align-items: center;
	gap: var(--space-2);
}

.dot {
	width: 8px;
	height: 8px;
	border-radius: 50%;
	flex-shrink: 0;
}

.label {
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-medium);
}

/* Status variants */
.healthy .dot {
	background-color: var(--color-success);
	box-shadow: 0 0 0 3px var(--color-success-bg);
}

.healthy .label {
	color: var(--color-success);
}

.unreachable .dot {
	background-color: var(--color-error);
	box-shadow: 0 0 0 3px var(--color-error-bg);
}

.unreachable .label {
	color: var(--color-error);
}

.unknown .dot {
	background-color: var(--color-text-tertiary);
	box-shadow: 0 0 0 3px var(--color-border-light);
}

.unknown .label {
	color: var(--color-text-tertiary);
}

.stale .dot {
	background-color: var(--color-warning);
	box-shadow: 0 0 0 3px var(--color-warning-bg);
}

.stale .label {
	color: var(--color-warning);
}
</style>
