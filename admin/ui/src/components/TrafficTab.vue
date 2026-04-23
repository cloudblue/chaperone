<template>
	<div :class="$style.tab">
		<VendorTable
			v-if="hasVendors"
			:vendors="sortedVendors"
			:selected="selectedVendors"
			:color-map="colorMap"
			@update:selected="selectedVendors = $event"
		/>

		<div
			v-if="hasVendorSeries && selectedVendors.size > 0"
			:class="$style.charts"
		>
			<div :class="$style.chartSection">
				<h3 id="rps-chart-title" :class="$style.chartTitle">
					Requests Per Second
				</h3>
				<div role="img" aria-labelledby="rps-chart-title">
					<VChart
						ref="rpsChartRef"
						:option="rpsChartOption"
						autoresize
						:class="$style.chart"
						@datazoom="(e) => syncZoom('rps', e)"
					/>
				</div>
			</div>
			<div :class="$style.chartSection">
				<h3 id="latency-vendor-chart-title" :class="$style.chartTitle">
					Latency
				</h3>
				<div role="img" aria-labelledby="latency-vendor-chart-title">
					<VChart
						ref="latencyChartRef"
						:option="latencyChartOption"
						autoresize
						:class="$style.chart"
						@datazoom="(e) => syncZoom('latency', e)"
					/>
				</div>
			</div>
			<div :class="$style.chartSection">
				<h3 id="error-chart-title" :class="$style.chartTitle">Error Rate</h3>
				<div role="img" aria-labelledby="error-chart-title">
					<VChart
						ref="errorChartRef"
						:option="errorChartOption"
						autoresize
						:class="$style.chart"
						@datazoom="(e) => syncZoom('error', e)"
					/>
				</div>
			</div>
		</div>

		<div
			v-else-if="hasVendors && !hasVendorSeries"
			:class="$style.chartPlaceholder"
		>
			Collecting time series data...
		</div>

		<div v-if="!hasVendors" :class="$style.chartPlaceholder">
			No vendor traffic data available yet.
		</div>
	</div>
</template>

<script setup>
import { ref, computed, watch, shallowRef } from 'vue';
import VendorTable from './VendorTable.vue';
import { VChart } from '../utils/chart-setup.js';
import { escapeHtml } from '../utils/html.js';
import {
	formatRps,
	formatLatency,
	formatErrorRate,
	assignVendorColors,
	LATENCY_COLORS,
} from '../utils/metrics.js';

const props = defineProps({
	metrics: { type: Object, required: true },
});

const selectedVendors = ref(new Set());

const sortedVendors = computed(() =>
	[...(props.metrics.vendors || [])].sort((a, b) => b.rps - a.rps),
);

const hasVendors = computed(
	() => props.metrics.vendors && props.metrics.vendors.length > 0,
);

const hasVendorSeries = computed(() => {
	const vs = props.metrics.vendor_series;
	return vs && Object.keys(vs).length > 0;
});

const vendorIds = computed(() => sortedVendors.value.map((v) => v.vendor_id));

const colorMap = computed(() => assignVendorColors(vendorIds.value));

// Auto-select top 3 vendors by RPS when vendor list changes
watch(
	vendorIds,
	(ids) => {
		const pruned = new Set(
			[...selectedVendors.value].filter((id) => ids.includes(id)),
		);
		if (pruned.size === 0 && ids.length > 0) {
			selectedVendors.value = new Set(ids.slice(0, 3));
		} else if (pruned.size !== selectedVendors.value.size) {
			selectedVendors.value = pruned;
		}
	},
	{ immediate: true },
);

const selected = computed(() => [...selectedVendors.value]);
const singleVendor = computed(() => selected.value.length === 1);

// Shared zoom state for time-aligned charts.
const rpsChartRef = shallowRef(null);
const latencyChartRef = shallowRef(null);
const errorChartRef = shallowRef(null);
const zoomStart = ref(0);
const zoomEnd = ref(100);

let syncing = false;

function syncZoom(source, event) {
	if (syncing) return;
	syncing = true;

	const batch = event.batch?.[0] ?? event;
	const start = batch.start ?? zoomStart.value;
	const end = batch.end ?? zoomEnd.value;
	zoomStart.value = start;
	zoomEnd.value = end;

	const refs = {
		rps: rpsChartRef,
		latency: latencyChartRef,
		error: errorChartRef,
	};
	for (const [key, chartRef] of Object.entries(refs)) {
		if (key === source || !chartRef.value) continue;
		chartRef.value.dispatchAction({
			type: 'dataZoom',
			start,
			end,
		});
	}
	syncing = false;
}

function buildBaseOption() {
	return {
		grid: { top: 16, right: 16, bottom: 56, left: 56 },
		xAxis: {
			type: 'time',
			axisLabel: { fontSize: 11 },
			splitLine: { show: false },
		},
		tooltip: { trigger: 'axis' },
		dataZoom: [
			{
				type: 'slider',
				xAxisIndex: 0,
				start: zoomStart.value,
				end: zoomEnd.value,
				height: 20,
				bottom: 4,
			},
			{
				type: 'inside',
				xAxisIndex: 0,
				start: zoomStart.value,
				end: zoomEnd.value,
			},
		],
		animationDuration: 300,
	};
}

const rpsChartOption = computed(() => {
	const vs = props.metrics.vendor_series || {};
	const base = buildBaseOption();
	return {
		...base,
		yAxis: {
			type: 'value',
			name: 'req/s',
			nameTextStyle: { fontSize: 11 },
			axisLabel: { fontSize: 11 },
			splitLine: { lineStyle: { color: '#f0f0f0' } },
		},
		tooltip: {
			trigger: 'axis',
			formatter(params) {
				const time = new Date(params[0].value[0]).toLocaleTimeString();
				const lines = params.map(
					(p) =>
						`<span style="color:${p.color}">\u25CF</span> ${escapeHtml(p.seriesName)}: ${formatRps(p.value[1])}`,
				);
				return `${time}<br/>${lines.join('<br/>')}`;
			},
		},
		series: selected.value
			.filter((id) => vs[id])
			.map((id) => ({
				name: id,
				type: 'line',
				data: vs[id].map((p) => [p.t * 1000, p.rps]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 2, color: colorMap.value[id] },
				itemStyle: { color: colorMap.value[id] },
			})),
	};
});

const latencyChartOption = computed(() => {
	const vs = props.metrics.vendor_series || {};
	const base = buildBaseOption();
	let series;

	if (singleVendor.value) {
		const id = selected.value[0];
		const points = vs[id] || [];
		series = [
			{
				name: `${id} P99`,
				type: 'line',
				data: points.map((p) => [p.t * 1000, p.p99]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 2, color: LATENCY_COLORS.p99 },
				itemStyle: { color: LATENCY_COLORS.p99 },
			},
			{
				name: `${id} P95`,
				type: 'line',
				data: points.map((p) => [p.t * 1000, p.p95]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 1.5, color: LATENCY_COLORS.p95 },
				itemStyle: { color: LATENCY_COLORS.p95 },
			},
			{
				name: `${id} P50`,
				type: 'line',
				data: points.map((p) => [p.t * 1000, p.p50]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 1.5, color: LATENCY_COLORS.p50 },
				itemStyle: { color: LATENCY_COLORS.p50 },
			},
		];
	} else {
		series = selected.value
			.filter((id) => vs[id])
			.map((id) => ({
				name: `${id} P99`,
				type: 'line',
				data: vs[id].map((p) => [p.t * 1000, p.p99]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 2, color: colorMap.value[id] },
				itemStyle: { color: colorMap.value[id] },
			}));
	}

	return {
		...base,
		yAxis: {
			type: 'value',
			name: 'ms',
			nameTextStyle: { fontSize: 11 },
			axisLabel: { fontSize: 11 },
			splitLine: { lineStyle: { color: '#f0f0f0' } },
		},
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
		series,
	};
});

const errorChartOption = computed(() => {
	const vs = props.metrics.vendor_series || {};
	const base = buildBaseOption();
	return {
		...base,
		yAxis: {
			type: 'value',
			name: '%',
			nameTextStyle: { fontSize: 11 },
			axisLabel: {
				fontSize: 11,
				formatter: (v) => `${(v * 100).toFixed(1)}`,
			},
			splitLine: { lineStyle: { color: '#f0f0f0' } },
		},
		tooltip: {
			trigger: 'axis',
			formatter(params) {
				const time = new Date(params[0].value[0]).toLocaleTimeString();
				const lines = params.map(
					(p) =>
						`<span style="color:${p.color}">\u25CF</span> ${escapeHtml(p.seriesName)}: ${formatErrorRate(p.value[1])}`,
				);
				return `${time}<br/>${lines.join('<br/>')}`;
			},
		},
		series: selected.value
			.filter((id) => vs[id])
			.map((id) => ({
				name: id,
				type: 'line',
				data: vs[id].map((p) => [p.t * 1000, p.err]),
				smooth: true,
				showSymbol: false,
				lineStyle: { width: 2, color: colorMap.value[id] },
				itemStyle: { color: colorMap.value[id] },
			})),
	};
});
</script>

<style module>
.tab {
	display: flex;
	flex-direction: column;
	gap: var(--space-5);
}

.charts {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
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
	margin: 0 0 var(--space-2) 0;
}

.chart {
	width: 100%;
	height: 220px;
}

.chartPlaceholder {
	display: flex;
	align-items: center;
	justify-content: center;
	height: 200px;
	color: var(--color-text-tertiary);
	font-size: var(--font-size-sm);
	background: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
}
</style>
