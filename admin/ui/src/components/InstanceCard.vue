<template>
	<BaseCard :class="$style.card" @click="$emit('click')">
		<div :class="$style.header">
			<div :class="$style.titleRow">
				<h3 :class="$style.name">{{ instance.name }}</h3>
				<StatusIndicator
					:status="isStale ? 'stale' : instance.status"
					:label="statusLabel"
				/>
			</div>
			<div :class="$style.address">{{ instance.address }}</div>
		</div>
		<div :class="$style.meta">
			<div v-if="instance.version" :class="$style.metaItem">
				<span :class="$style.metaLabel">Version</span>
				<span :class="$style.metaValue">{{ instance.version }}</span>
			</div>
			<div v-if="instance.last_seen_at" :class="$style.metaItem">
				<span :class="$style.metaLabel">Last seen</span>
				<span :class="[$style.metaValue, isStale && $style.staleValue]">{{
					formatTime(instance.last_seen_at)
				}}</span>
			</div>
		</div>
		<div :class="$style.actions">
			<BaseButton
				size="sm"
				variant="secondary"
				@click.stop="$emit('edit', instance)"
			>
				Edit
			</BaseButton>
			<BaseButton
				size="sm"
				variant="ghost"
				@click.stop="$emit('delete', instance)"
			>
				Remove
			</BaseButton>
		</div>
	</BaseCard>
</template>

<script setup>
import { computed } from 'vue';
import BaseCard from './BaseCard.vue';
import BaseButton from './BaseButton.vue';
import StatusIndicator from './StatusIndicator.vue';
import {
	isInstanceStale,
	formatTime,
	getStatusLabel,
} from '../utils/instance.js';

const props = defineProps({
	instance: { type: Object, required: true },
});

defineEmits(['click', 'edit', 'delete']);

const isStale = computed(() => isInstanceStale(props.instance));
const statusLabel = computed(() =>
	getStatusLabel(props.instance.status, isStale.value),
);
</script>

<style module>
.card {
	cursor: pointer;
	transition: box-shadow 0.15s;
}

.card:hover {
	box-shadow: var(--shadow-md);
}

.header {
	margin-bottom: var(--space-3);
}

.titleRow {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: var(--space-3);
}

.name {
	font-size: var(--font-size-md);
	font-weight: var(--font-weight-semibold);
	margin: 0;
}

.address {
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
	font-family: var(--font-family-mono);
	margin-top: var(--space-1);
}

.meta {
	display: flex;
	gap: var(--space-5);
	margin-bottom: var(--space-3);
}

.metaItem {
	display: flex;
	flex-direction: column;
	gap: 2px;
}

.metaLabel {
	font-size: var(--font-size-xs);
	color: var(--color-text-tertiary);
	text-transform: uppercase;
	letter-spacing: 0.04em;
}

.metaValue {
	font-size: var(--font-size-sm);
	color: var(--color-text-primary);
}

.staleValue {
	color: var(--color-warning);
}

.actions {
	display: flex;
	gap: var(--space-2);
	padding-top: var(--space-3);
	border-top: 1px solid var(--color-border-light);
}
</style>
