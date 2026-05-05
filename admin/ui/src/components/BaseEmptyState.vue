<template>
	<div :class="$style.empty">
		<div :class="$style.icon" aria-hidden="true">
			<slot name="icon" />
		</div>
		<h2 :class="$style.title">{{ title }}</h2>
		<p :class="$style.description">{{ description }}</p>
	</div>
</template>

<script setup>
defineProps({
	title: {
		type: String,
		required: true,
	},
	description: {
		type: String,
		required: true,
	},
});
</script>

<style module>
.empty {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: var(--space-3);
	padding: var(--space-8) var(--space-4);
	text-align: center;
}

.icon {
	width: 56px;
	height: 56px;
	display: flex;
	align-items: center;
	justify-content: center;
	background-color: var(--color-border-light);
	border-radius: 14px;
	color: var(--color-text-tertiary);
}

.title {
	font-size: var(--font-size-md);
	font-weight: var(--font-weight-bold);
}

.description {
	font-size: var(--font-size-sm);
	color: var(--color-text-secondary);
	max-width: 320px;
}
</style>
