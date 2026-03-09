<template>
	<div :class="$style.overlay" @click.self="$emit('cancel')">
		<div :class="$style.dialog" role="alertdialog" :aria-labelledby="titleId" :aria-describedby="descId" aria-modal="true">
			<h2 :id="titleId" :class="$style.title">{{ title }}</h2>
			<p :id="descId" :class="$style.description">{{ description }}</p>
			<div :class="$style.actions">
				<BaseButton variant="secondary" data-testid="confirm-cancel" @click="$emit('cancel')">
					Cancel
				</BaseButton>
				<BaseButton
					:variant="destructive ? 'danger' : 'primary'"
					data-testid="confirm-ok"
					@click="$emit('confirm')"
				>
					{{ confirmLabel }}
				</BaseButton>
			</div>
		</div>
	</div>
</template>

<script setup>
import { onMounted, onUnmounted, useId } from "vue";
import BaseButton from "./BaseButton.vue";

defineProps({
	title: { type: String, required: true },
	description: { type: String, default: "" },
	confirmLabel: { type: String, default: "Confirm" },
	destructive: { type: Boolean, default: false },
});

const emit = defineEmits(["confirm", "cancel"]);

const titleId = `confirm-title-${useId()}`;
const descId = `confirm-desc-${useId()}`;

function onKeydown(e) {
	if (e.key === "Escape") {
		e.preventDefault();
		emit("cancel");
	}
}

onMounted(() => {
	document.addEventListener("keydown", onKeydown);
	document.querySelector('[data-testid="confirm-cancel"]')?.focus();
});

onUnmounted(() => {
	document.removeEventListener("keydown", onKeydown);
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
	z-index: 200;
}

.dialog {
	background-color: var(--color-bg-surface);
	border-radius: var(--radius-xl);
	box-shadow: var(--shadow-modal);
	padding: var(--space-6);
	width: 100%;
	max-width: 400px;
}

.title {
	font-size: var(--font-size-lg);
	font-weight: var(--font-weight-bold);
	margin: 0 0 var(--space-2);
}

.description {
	font-size: var(--font-size-sm);
	color: var(--color-text-secondary);
	line-height: var(--line-height-relaxed);
	margin: 0 0 var(--space-5);
}

.actions {
	display: flex;
	justify-content: flex-end;
	gap: var(--space-2);
}
</style>
