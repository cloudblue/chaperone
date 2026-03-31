<template>
	<div :class="$style.tab">
		<!-- KPI cards -->
		<div :class="$style.kpiGrid">
			<KpiCard
				label="RPS"
				:value="formatRps(rps)"
				:trend="trendDirection(metrics.rps_trend)"
				trend-sentiment="positive"
			/>
			<KpiCard
				label="P99 Latency"
				:value="formatLatency(p99)"
				:subtitle="latencySubtitle"
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
				:value="formatCount(panicCount)"
				trend-sentiment="negative"
			/>
		</div>

		<!-- Latency distribution chart -->
		<div :class="$style.chartSection">
			<h3 :class="$style.chartTitle">Latency Over Time</h3>
			<div :class="$style.chartContainer">
				<VChart
					v-if="hasSeries"
					:option="latencyChartOption"
					autoresize
					:class="$style.chart"
				/>
				<div v-else :class="$style.chartPlaceholder">
					Collecting data points...
				</div>
			</div>
		</div>
	</div>
</template>

<script setup>
import { computed } from 'vue';
import KpiCard from './KpiCard.vue';
import { VChart } from '../utils/chart-setup.js';
import { escapeHtml } from '../utils/html.js';
import { useAnimatedValue } from '../composables/useAnimatedValue.js';
import {
	formatRps,
	formatLatency,
	formatErrorRate,
	formatCount,
	trendDirection,
	LATENCY_COLORS,
} from '../utils/metrics.js';

const props = defineProps({
	metrics: { type: Object, required: true },
});

const rps = useAnimatedValue(computed(() => props.metrics.rps));
const p99 = useAnimatedValue(computed(() => props.metrics.p99_ms));
const errorRate = useAnimatedValue(computed(() => props.metrics.error_rate));
const connections = useAnimatedValue(
	computed(() => props.metrics.active_connections),
);
const panicCount = useAnimatedValue(computed(() => props.metrics.panics_total));
const latencySubtitle = computed(() => {
	return `p50 ${formatLatency(props.metrics.p50_ms)} · p95 ${formatLatency(props.metrics.p95_ms)}`;
});

const hasSeries = computed(
	() => props.metrics.series && props.metrics.series.length >= 2,
);

const latencyChartOption = computed(() => {
	const series = props.metrics.series || [];
	return {
		grid: { top: 40, right: 16, bottom: 40, left: 56 },
		tooltip: {
			trigger: 'axis',
			formatter(params) {
				const time = new Date(params[0].value[0]).toLocaleTimeString();
				const lines = params.map(
					(p) =>
						`<span style="color:${p.color}">\u25CF</span> ${escapeHtml(p.seriesName)}: ${formatLatency(p.value[1])}`,
				);
				return `${time}<br/>${lines.join('<br/>')}`;
			},
		},
		legend: {
			data: ['P99', 'P95', 'P50'],
			bottom: 0,
			textStyle: { fontSize: 11 },
		},
		xAxis: {
			type: 'time',
			axisLabel: { fontSize: 11 },
			splitLine: { show: false },
		},
		yAxis: {
			type: 'value',
			name: 'ms',
			nameTextStyle: { fontSize: 11 },
			axisLabel: { fontSize: 11 },
			splitLine: { lineStyle: { color: '#f0f0f0' } },
		},
		series: [
			{
				name: 'P99',
				type: 'line',
				data: series.map((p) => [p.t * 1000, p.p99]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 2, color: LATENCY_COLORS.p99 },
				itemStyle: { color: LATENCY_COLORS.p99 },
			},
			{
				name: 'P95',
				type: 'line',
				data: series.map((p) => [p.t * 1000, p.p95]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 1.5, color: LATENCY_COLORS.p95 },
				itemStyle: { color: LATENCY_COLORS.p95 },
			},
			{
				name: 'P50',
				type: 'line',
				data: series.map((p) => [p.t * 1000, p.p50]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 1.5, color: LATENCY_COLORS.p50 },
				itemStyle: { color: LATENCY_COLORS.p50 },
			},
		],
		animationDuration: 300,
	};
});
</script>

<style module>
.tab {
	display: flex;
	flex-direction: column;
	gap: var(--space-5);
}

.kpiGrid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
	gap: var(--space-3);
}

.chartSection {
	background: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	padding: var(--space-4);
}

.chartTitle {
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-semibold);
	color: var(--color-text-primary);
	margin: 0 0 var(--space-3) 0;
}

.chartContainer {
	position: relative;
}

.chart {
	width: 100%;
	height: 300px;
}

.chartPlaceholder {
	display: flex;
	align-items: center;
	justify-content: center;
	height: 300px;
	color: var(--color-text-tertiary);
	font-size: var(--font-size-sm);
}
</style>
