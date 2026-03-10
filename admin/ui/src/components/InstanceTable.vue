<template>
	<div :class="$style.wrapper">
		<table :class="$style.table">
			<thead>
				<tr>
					<th :class="$style.th">Status</th>
					<th :class="$style.th">Name</th>
					<th :class="$style.th">Address</th>
					<th :class="$style.th">Version</th>
					<th :class="$style.th">Last Seen</th>
					<th :class="[$style.th, $style.actionsCol]">Actions</th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="inst in instances" :key="inst.id" :class="$style.row">
					<td :class="$style.td">
						<StatusIndicator :status="isInstanceStale(inst) ? 'stale' : inst.status" />
					</td>
					<td :class="[$style.td, $style.name]">{{ inst.name }}</td>
					<td :class="[$style.td, $style.mono]">{{ inst.address }}</td>
					<td :class="$style.td">{{ inst.version || "—" }}</td>
					<td :class="$style.td">{{ formatTime(inst.last_seen_at) || "—" }}</td>
					<td :class="[$style.td, $style.actionsCol]">
						<BaseButton size="sm" variant="secondary" @click="$emit('edit', inst)">
							Edit
						</BaseButton>
						<BaseButton size="sm" variant="ghost" @click="$emit('delete', inst)">
							Remove
						</BaseButton>
					</td>
				</tr>
			</tbody>
		</table>
	</div>
</template>

<script setup>
import StatusIndicator from "./StatusIndicator.vue";
import BaseButton from "./BaseButton.vue";
import { isInstanceStale, formatTime } from "../utils/instance.js";

defineProps({
	instances: { type: Array, required: true },
});

defineEmits(["edit", "delete"]);
</script>

<style module>
.wrapper {
	overflow-x: auto;
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

.row:hover {
	background-color: var(--color-bg-primary);
}

.td {
	padding: var(--space-2) var(--space-3);
	border-bottom: 1px solid var(--color-border-light);
	color: var(--color-text-primary);
	vertical-align: middle;
}

.name {
	font-weight: var(--font-weight-medium);
}

.mono {
	font-family: var(--font-family-mono);
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
}

.actionsCol {
	text-align: right;
	white-space: nowrap;
}

.actionsCol :global(button) {
	margin-left: var(--space-2);
}

.actionsCol :global(button:first-child) {
	margin-left: 0;
}
</style>
