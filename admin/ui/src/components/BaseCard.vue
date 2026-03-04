<template>
	<div :class="$style.card">
		<div v-if="$slots.header" :class="$style.header">
			<slot name="header" />
		</div>
		<div :class="$style.body">
			<slot />
		</div>
	</div>
</template>

<script setup></script>

<style module>
.card {
	background-color: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-xl);
	box-shadow: var(--shadow-sm);
}

.header {
	padding: var(--space-3) var(--space-5);
	border-bottom: 1px solid var(--color-border);
	font-weight: var(--font-weight-semibold);
	font-size: var(--font-size-sm);
}

.body {
	padding: var(--space-5);
}
</style>
