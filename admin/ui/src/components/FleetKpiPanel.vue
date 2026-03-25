<template>
	<div :class="$style.panel">
		<div :class="$style.grid">
			<KpiCard
				label="Total RPS"
				:value="formatRps(rps)"
				:trend="trendDirection(metrics.rps_trend)"
				trend-sentiment="positive"
			/>
			<KpiCard
				label="Error Rate"
				:value="formatErrorRate(errorRate)"
				:trend="trendDirection(metrics.error_rate_trend)"
				trend-sentiment="negative"
			/>
			<KpiCard label="Active Connections" :value="formatCount(connections)" />
			<KpiCard
				label="Panics"
				:value="formatCount(panics)"
				trend-sentiment="negative"
			/>
		</div>
		<span v-if="scopeNote" :class="$style.scope">{{ scopeNote }}</span>
	</div>
</template>

<script setup>
import { computed } from 'vue';
import KpiCard from './KpiCard.vue';
import { useAnimatedValue } from '../composables/useAnimatedValue.js';
import {
	formatRps,
	formatErrorRate,
	formatCount,
	trendDirection,
} from '../utils/metrics.js';

const props = defineProps({
	metrics: { type: Object, required: true },
	totalInstances: { type: Number, required: true },
});

const rps = useAnimatedValue(computed(() => props.metrics.total_rps));
const errorRate = useAnimatedValue(
	computed(() => props.metrics.fleet_error_rate),
);
const connections = useAnimatedValue(
	computed(() => props.metrics.total_active_connections),
);
const panics = useAnimatedValue(computed(() => props.metrics.total_panics));

const scopeNote = computed(() => {
	const reporting = props.metrics.instances?.length ?? 0;
	if (reporting < props.totalInstances && props.totalInstances > 0) {
		return `${reporting} of ${props.totalInstances} instances reporting`;
	}
	return '';
});
</script>

<style module>
.panel {
	display: flex;
	flex-direction: column;
	gap: var(--space-2);
}

.grid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	gap: var(--space-3);
}

.scope {
	font-size: var(--font-size-xs);
	color: var(--color-text-tertiary);
}
</style>
