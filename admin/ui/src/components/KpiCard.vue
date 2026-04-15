<template>
	<div :class="$style.card" v-bind="$attrs">
		<span :class="$style.label">{{ label }}</span>
		<div :class="$style.valueRow">
			<span :class="$style.value">{{ value }}</span>
			<span
				v-if="trend !== null"
				:class="[$style.trend, $style[trendClass]]"
				:aria-label="trendAriaLabel"
			>
				<svg
					v-if="trend !== 'flat'"
					width="12"
					height="12"
					viewBox="0 0 12 12"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path v-if="trend === 'up'" d="M6 9V3M3 5l3-2 3 2" />
					<path v-else d="M6 3v6M3 7l3 2 3-2" />
				</svg>
				<span v-else aria-hidden="true">&mdash;</span>
			</span>
		</div>
		<span v-if="subtitle" :class="$style.subtitle">{{ subtitle }}</span>
	</div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
	label: { type: String, required: true },
	value: { type: String, required: true },
	subtitle: { type: String, default: '' },
	trend: { type: String, default: null }, // 'up' | 'down' | 'flat' | null
	trendSentiment: { type: String, default: 'neutral' }, // 'positive' | 'negative' | 'neutral'
});

const trendClass = computed(() => {
	if (props.trend === 'flat' || props.trend === null) return 'trendNeutral';
	if (props.trendSentiment === 'positive') {
		return props.trend === 'up' ? 'trendGood' : 'trendBad';
	}
	if (props.trendSentiment === 'negative') {
		return props.trend === 'up' ? 'trendBad' : 'trendGood';
	}
	return 'trendNeutral';
});

const trendAriaLabel = computed(() => {
	if (props.trend === 'flat') return 'No change';
	if (props.trend === 'up') return 'Trending up';
	if (props.trend === 'down') return 'Trending down';
	return '';
});
</script>

<style module>
.card {
	display: flex;
	flex-direction: column;
	gap: 2px;
	padding: var(--space-4);
	background: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
}

.label {
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-medium);
	color: var(--color-text-tertiary);
	text-transform: uppercase;
	letter-spacing: 0.04em;
}

.valueRow {
	display: flex;
	align-items: center;
	gap: var(--space-2);
}

.value {
	font-size: var(--font-size-2xl);
	font-weight: var(--font-weight-semibold);
	color: var(--color-text-primary);
	letter-spacing: -0.02em;
	line-height: var(--line-height-tight);
}

.trend {
	display: flex;
	align-items: center;
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-medium);
}

.trendGood {
	color: var(--color-success);
}

.trendBad {
	color: var(--color-error);
}

.trendNeutral {
	color: var(--color-text-tertiary);
}

.subtitle {
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
	margin-top: 2px;
}
</style>
