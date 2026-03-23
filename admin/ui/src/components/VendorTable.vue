<template>
	<div :class="$style.wrapper">
		<table :class="$style.table">
			<thead>
				<tr>
					<th :class="[$style.th, $style.checkCol]">
						<input
							type="checkbox"
							:checked="allSelected"
							:indeterminate="someSelected && !allSelected"
							aria-label="Select all vendors"
							@change="toggleAll"
						/>
					</th>
					<th :class="$style.th">Vendor</th>
					<th :class="[$style.th, $style.numCol]">RPS</th>
					<th :class="[$style.th, $style.numCol]">P50</th>
					<th :class="[$style.th, $style.numCol]">P95</th>
					<th :class="[$style.th, $style.numCol]">P99</th>
					<th :class="[$style.th, $style.numCol]">Error %</th>
				</tr>
			</thead>
			<tbody>
				<tr
					v-for="v in vendors"
					:key="v.vendor_id"
					:class="$style.row"
					@click="toggleVendor(v.vendor_id)"
				>
					<td :class="[$style.td, $style.checkCol]">
						<input
							type="checkbox"
							:checked="selected.has(v.vendor_id)"
							:aria-label="`Select ${v.vendor_id}`"
							@click.stop
							@change="toggleVendor(v.vendor_id)"
						/>
					</td>
					<td :class="$style.td">
						<span :class="$style.vendorName">
							<span
								:class="$style.colorDot"
								:style="{ backgroundColor: colorMap[v.vendor_id] }"
								aria-hidden="true"
							/>
							{{ v.vendor_id }}
						</span>
					</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatRps(v.rps) }}
					</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatLatency(v.p50_ms) }}
					</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatLatency(v.p95_ms) }}
					</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatLatency(v.p99_ms) }}
					</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatErrorRate(v.error_rate) }}
					</td>
				</tr>
			</tbody>
			<tfoot v-if="vendors.length > 1">
				<tr :class="$style.totalsRow">
					<td :class="[$style.td, $style.checkCol]" />
					<td :class="$style.td">
						<span :class="$style.totalsLabel">Total</span>
					</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatRps(totals.rps) }}
					</td>
					<td :class="[$style.td, $style.numCol]">&mdash;</td>
					<td :class="[$style.td, $style.numCol]">&mdash;</td>
					<td :class="[$style.td, $style.numCol]">&mdash;</td>
					<td :class="[$style.td, $style.numCol]">
						{{ formatErrorRate(totals.errorRate) }}
					</td>
				</tr>
			</tfoot>
		</table>
	</div>
</template>

<script setup>
import { computed } from 'vue';
import { formatRps, formatLatency, formatErrorRate } from '../utils/metrics.js';

const props = defineProps({
	vendors: { type: Array, required: true },
	selected: { type: Set, required: true },
	colorMap: { type: Object, required: true },
});

const emit = defineEmits(['update:selected']);

const allSelected = computed(
	() =>
		props.vendors.length > 0 &&
		props.vendors.every((v) => props.selected.has(v.vendor_id)),
);

const someSelected = computed(() =>
	props.vendors.some((v) => props.selected.has(v.vendor_id)),
);

const totals = computed(() => {
	const totalRps = props.vendors.reduce((sum, v) => sum + v.rps, 0);
	const totalErrors = props.vendors.reduce(
		(sum, v) => sum + v.rps * v.error_rate,
		0,
	);
	return {
		rps: totalRps,
		errorRate: totalRps > 0 ? totalErrors / totalRps : 0,
	};
});

function toggleVendor(id) {
	const next = new Set(props.selected);
	if (next.has(id)) {
		next.delete(id);
	} else {
		next.add(id);
	}
	emit('update:selected', next);
}

function toggleAll() {
	if (allSelected.value) {
		emit('update:selected', new Set());
	} else {
		emit('update:selected', new Set(props.vendors.map((v) => v.vendor_id)));
	}
}
</script>

<style module>
.wrapper {
	overflow-x: auto;
	background: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
}

.table {
	width: 100%;
	border-collapse: collapse;
	font-size: var(--font-size-sm);
}

.th {
	text-align: left;
	padding: var(--space-2) var(--space-3);
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-semibold);
	color: var(--color-text-tertiary);
	text-transform: uppercase;
	letter-spacing: 0.04em;
	border-bottom: 1px solid var(--color-border);
}

.row {
	cursor: pointer;
}

.row:hover {
	background-color: var(--color-bg-primary);
}

.td {
	padding: var(--space-2) var(--space-3);
	border-bottom: 1px solid var(--color-border-light);
	color: var(--color-text-primary);
	vertical-align: middle;
}

.checkCol {
	width: 36px;
	text-align: center;
}

.numCol {
	text-align: right;
	font-family: var(--font-family-mono);
	font-size: var(--font-size-xs);
}

.vendorName {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	font-weight: var(--font-weight-medium);
}

.colorDot {
	display: inline-block;
	width: 10px;
	height: 10px;
	border-radius: 50%;
	flex-shrink: 0;
}

.totalsRow {
	font-weight: var(--font-weight-semibold);
}

.totalsRow .td {
	border-bottom: none;
	border-top: 1px solid var(--color-border);
}

.totalsLabel {
	color: var(--color-text-secondary);
}
</style>
