<template>
	<div :class="[$style.spinner, $style[size]]" role="status">
		<svg
			:class="$style.icon"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2.5"
			stroke-linecap="round"
		>
			<path d="M12 2a10 10 0 0 1 10 10" />
		</svg>
		<span :class="$style.srOnly">{{ label }}</span>
	</div>
</template>

<script setup>
defineProps({
	size: {
		type: String,
		default: 'md',
		validator: (v) => ['sm', 'md', 'lg'].includes(v),
	},
	label: {
		type: String,
		default: 'Loading...',
	},
});
</script>

<style module>
.spinner {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	color: var(--color-text-tertiary);
}

.icon {
	animation: spin 0.8s linear infinite;
}

.sm .icon {
	width: 16px;
	height: 16px;
}

.md .icon {
	width: 24px;
	height: 24px;
}

.lg .icon {
	width: 32px;
	height: 32px;
}

.srOnly {
	position: absolute;
	width: 1px;
	height: 1px;
	padding: 0;
	margin: -1px;
	overflow: hidden;
	clip: rect(0, 0, 0, 0);
	white-space: nowrap;
	border: 0;
}

@keyframes spin {
	to {
		transform: rotate(360deg);
	}
}
</style>
