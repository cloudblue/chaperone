<template>
	<div ref="root" :class="$style.menu">
		<BaseButton
			:class="$style.trigger"
			size="sm"
			variant="ghost"
			type="button"
			:aria-label="`More actions for ${label}`"
			aria-haspopup="menu"
			:aria-expanded="open ? 'true' : 'false'"
			@click.stop="toggleMenu"
		>
			<svg
				width="16"
				height="16"
				viewBox="0 0 16 16"
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<circle cx="3" cy="8" r="1" fill="currentColor" stroke="none" />
				<circle cx="8" cy="8" r="1" fill="currentColor" stroke="none" />
				<circle cx="13" cy="8" r="1" fill="currentColor" stroke="none" />
			</svg>
		</BaseButton>
		<div v-if="open" :class="$style.panel" role="menu">
			<button
				:class="$style.item"
				type="button"
				role="menuitem"
				@click.stop="handleRemove"
			>
				Remove
			</button>
		</div>
	</div>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue';
import BaseButton from './BaseButton.vue';

defineProps({
	label: { type: String, required: true },
});

const emit = defineEmits(['remove']);

const open = ref(false);
const root = ref(null);

function closeMenu() {
	open.value = false;
}

function toggleMenu() {
	open.value = !open.value;
}

function handleRemove() {
	closeMenu();
	emit('remove');
}

function handleDocumentClick(event) {
	if (!open.value || root.value?.contains(event.target)) {
		return;
	}

	closeMenu();
}

function handleDocumentKeydown(event) {
	if (event.key === 'Escape') {
		closeMenu();
	}
}

watch(open, (isOpen) => {
	if (typeof document === 'undefined') {
		return;
	}

	const method = isOpen ? 'addEventListener' : 'removeEventListener';
	document[method]('click', handleDocumentClick);
	document[method]('keydown', handleDocumentKeydown);
});

onBeforeUnmount(() => {
	if (typeof document === 'undefined') {
		return;
	}

	document.removeEventListener('click', handleDocumentClick);
	document.removeEventListener('keydown', handleDocumentKeydown);
});
</script>

<style module>
.menu {
	position: relative;
	display: inline-flex;
	align-items: center;
}

.trigger {
	width: 28px;
	height: 28px;
	padding: 0;
	border-color: var(--color-border);
	color: var(--color-text-secondary);
	background-color: var(--color-bg-surface);
	border-radius: var(--radius-md);
	flex-shrink: 0;
}

.trigger:hover:not(:disabled),
.trigger[aria-expanded='true'] {
	background-color: var(--color-bg-primary);
	color: var(--color-text-primary);
	border-color: var(--color-border);
}

.panel {
	position: absolute;
	top: calc(100% + var(--space-2));
	right: 0;
	min-width: 132px;
	padding: var(--space-1);
	background-color: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	box-shadow: var(--shadow-md);
	z-index: 10;
	display: flex;
	flex-direction: column;
	gap: 2px;
}

.item {
	width: 100%;
	padding: var(--space-2) var(--space-3);
	border: none;
	border-radius: var(--radius-md);
	background: transparent;
	color: var(--color-error);
	text-align: left;
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
	cursor: pointer;
}

.item:hover,
.item:focus-visible {
	background-color: var(--color-error-bg);
	outline: none;
}
</style>
