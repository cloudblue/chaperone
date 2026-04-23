<template>
	<div :class="$style.page">
		<div :class="$style.header">
			<h1 :class="$style.title">Audit Log</h1>
		</div>

		<!-- Filters bar -->
		<div
			:class="$style.filters"
			role="search"
			aria-label="Filter audit entries"
		>
			<div :class="$style.searchWrapper">
				<svg
					:class="$style.searchIcon"
					width="16"
					height="16"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<circle cx="11" cy="11" r="8" />
					<line x1="21" y1="21" x2="16.65" y2="16.65" />
				</svg>
				<input
					:class="$style.searchInput"
					type="search"
					placeholder="Search audit log..."
					:value="audit.filters.value.q"
					aria-label="Search audit log"
					@input="handleSearch($event.target.value)"
				/>
			</div>
			<select
				:class="$style.select"
				:value="audit.filters.value.action"
				aria-label="Filter by action type"
				@change="audit.setFilter('action', $event.target.value)"
			>
				<option
					v-for="opt in actionOptions"
					:key="opt.value"
					:value="opt.value"
				>
					{{ opt.label }}
				</option>
			</select>
			<label :class="$style.dateLabel">
				<span :class="$style.dateLabelText">From</span>
				<input
					:class="$style.dateInput"
					type="date"
					:value="audit.filters.value.from"
					@change="handleFromDate($event.target.value)"
				/>
			</label>
			<label :class="$style.dateLabel">
				<span :class="$style.dateLabelText">To</span>
				<input
					:class="$style.dateInput"
					type="date"
					:value="audit.filters.value.to"
					@change="handleToDate($event.target.value)"
				/>
			</label>
		</div>

		<div :class="$style.content">
			<!-- Loading state -->
			<div
				v-if="audit.loading.value && audit.items.value.length === 0"
				:class="$style.loadingContainer"
			>
				<LoadingSpinner size="lg" label="Loading audit log..." />
			</div>

			<!-- Error state -->
			<div
				v-else-if="audit.error.value"
				:class="$style.errorBanner"
				role="alert"
			>
				Failed to load audit log: {{ audit.error.value }}
			</div>

			<!-- Empty state -->
			<BaseCard
				v-else-if="
					!audit.loading.value &&
					audit.items.value.length === 0 &&
					!hasActiveFilters
				"
			>
				<BaseEmptyState
					title="No activity yet"
					description="Portal actions will be logged here once you start managing instances."
				>
					<template #icon>
						<svg
							width="24"
							height="24"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<path
								d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"
							/>
							<polyline points="14 2 14 8 20 8" />
							<line x1="16" y1="13" x2="8" y2="13" />
							<line x1="16" y1="17" x2="8" y2="17" />
						</svg>
					</template>
				</BaseEmptyState>
			</BaseCard>

			<!-- No results for active filters -->
			<BaseCard
				v-else-if="
					!audit.loading.value &&
					audit.items.value.length === 0 &&
					hasActiveFilters
				"
			>
				<BaseEmptyState
					title="No matching entries"
					description="Try adjusting your filters or search query."
				>
					<template #icon>
						<svg
							width="24"
							height="24"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<circle cx="11" cy="11" r="8" />
							<line x1="21" y1="21" x2="16.65" y2="16.65" />
						</svg>
					</template>
				</BaseEmptyState>
			</BaseCard>

			<!-- Audit table -->
			<BaseCard v-else>
				<div :class="$style.tableWrapper">
					<table :class="$style.table" aria-label="Audit log entries">
						<thead>
							<tr>
								<th :class="$style.th">Time</th>
								<th :class="$style.th">User</th>
								<th :class="$style.th">Action</th>
								<th :class="$style.th">Detail</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-for="entry in audit.items.value"
								:key="entry.id"
								:class="$style.row"
							>
								<td :class="[$style.td, $style.timeCell]">
									<time
										:datetime="entry.created_at"
										:title="fullTimestamp(entry.created_at)"
									>
										{{ formatAuditTimestamp(entry.created_at) }}
									</time>
								</td>
								<td :class="$style.td">{{ entry.user }}</td>
								<td :class="$style.td">
									<span
										:class="[
											$style.actionBadge,
											$style[actionCategory(entry.action)],
										]"
									>
										{{ getActionLabel(entry.action) }}
									</span>
								</td>
								<td :class="[$style.td, $style.detailCell]">
									{{ entry.detail }}
								</td>
							</tr>
						</tbody>
					</table>
				</div>

				<!-- Pagination -->
				<div
					v-if="audit.total.value > audit.filters.value.perPage"
					:class="$style.pagination"
					role="navigation"
					aria-label="Audit log pagination"
				>
					<span :class="$style.pageInfo">
						{{ paginationLabel }}
					</span>
					<div :class="$style.pageButtons">
						<button
							:class="$style.pageBtn"
							:disabled="audit.filters.value.page <= 1"
							aria-label="Previous page"
							@click="audit.prevPage()"
						>
							<svg
								width="16"
								height="16"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
							>
								<polyline points="15 18 9 12 15 6" />
							</svg>
						</button>
						<span :class="$style.pageLabel">
							{{ audit.filters.value.page }} of {{ audit.pageCount.value }}
						</span>
						<button
							:class="$style.pageBtn"
							:disabled="audit.filters.value.page >= audit.pageCount.value"
							aria-label="Next page"
							@click="audit.nextPage()"
						>
							<svg
								width="16"
								height="16"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
							>
								<polyline points="9 18 15 12 9 6" />
							</svg>
						</button>
					</div>
				</div>
			</BaseCard>
		</div>
	</div>
</template>

<script setup>
import { computed, onMounted, onUnmounted } from 'vue';
import BaseCard from '../components/BaseCard.vue';
import BaseEmptyState from '../components/BaseEmptyState.vue';
import LoadingSpinner from '../components/LoadingSpinner.vue';
import * as api from '../utils/api.js';
import { useAuditLog } from '../composables/useAuditLog.js';
import {
	getActionLabel,
	getActionOptions,
	formatAuditTimestamp,
} from '../utils/audit.js';

const actionOptions = getActionOptions();
const audit = useAuditLog(api);

let searchTimeout = null;
function handleSearch(value) {
	clearTimeout(searchTimeout);
	searchTimeout = setTimeout(() => audit.setFilter('q', value), 300);
}
onUnmounted(() => clearTimeout(searchTimeout));

function handleFromDate(value) {
	audit.setFilter('from', value);
}

function handleToDate(value) {
	audit.setFilter('to', value);
}

const hasActiveFilters = computed(() => {
	const f = audit.filters.value;
	return !!(f.q || f.action || f.from || f.to);
});

function actionCategory(action) {
	if (action.startsWith('instance.')) return 'actionInstance';
	if (action.startsWith('user.')) return 'actionUser';
	return 'actionOther';
}

function fullTimestamp(isoString) {
	if (!isoString) return '';
	return new Date(isoString).toLocaleString();
}

const paginationLabel = computed(() => {
	const { page, perPage } = audit.filters.value;
	const start = (page - 1) * perPage + 1;
	const end = Math.min(page * perPage, audit.total.value);
	return `${start}–${end} of ${audit.total.value}`;
});

onMounted(() => audit.fetch());
</script>

<style module>
.page {
	display: flex;
	flex-direction: column;
	gap: var(--space-6);
}

.header {
	display: flex;
	align-items: center;
	justify-content: space-between;
}

.title {
	font-size: var(--font-size-xl);
	font-weight: var(--font-weight-bold);
	letter-spacing: -0.02em;
}

.filters {
	display: flex;
	gap: var(--space-3);
	align-items: center;
	flex-wrap: wrap;
}

.searchWrapper {
	position: relative;
	flex: 1;
	min-width: 200px;
	max-width: 320px;
}

.searchIcon {
	position: absolute;
	left: 0.75rem;
	top: 50%;
	transform: translateY(-50%);
	color: var(--color-text-tertiary);
	pointer-events: none;
}

.searchInput {
	width: 100%;
	padding: 0.5rem 0.75rem 0.5rem 2.25rem;
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	font-family: inherit;
	font-size: var(--font-size-sm);
	color: var(--color-text-primary);
	background-color: var(--color-bg-surface);
	transition:
		border-color 0.15s,
		box-shadow 0.15s;
}

.searchInput::placeholder {
	color: var(--color-text-tertiary);
}

.searchInput:focus {
	outline: none;
	border-color: var(--color-accent);
	box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.select {
	padding: 0.5rem 2rem 0.5rem 0.75rem;
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	font-family: inherit;
	font-size: var(--font-size-sm);
	color: var(--color-text-primary);
	background-color: var(--color-bg-surface);
	appearance: none;
	background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%239ca3af' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'/%3E%3C/svg%3E");
	background-repeat: no-repeat;
	background-position: right 0.625rem center;
	cursor: pointer;
	transition:
		border-color 0.15s,
		box-shadow 0.15s;
}

.select:focus {
	outline: none;
	border-color: var(--color-accent);
	box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.dateLabel {
	display: flex;
	align-items: center;
	gap: var(--space-2);
}

.dateLabelText {
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-medium);
	color: var(--color-text-secondary);
	white-space: nowrap;
}

.dateInput {
	padding: 0.5rem 0.75rem;
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	font-family: inherit;
	font-size: var(--font-size-sm);
	color: var(--color-text-primary);
	background-color: var(--color-bg-surface);
	transition:
		border-color 0.15s,
		box-shadow 0.15s;
}

.dateInput:focus {
	outline: none;
	border-color: var(--color-accent);
	box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.content {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.errorBanner {
	padding: var(--space-3) var(--space-4);
	background-color: var(--color-error-bg);
	color: var(--color-error);
	border: 1px solid var(--color-error-border);
	border-radius: var(--radius-lg);
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
}

.tableWrapper {
	overflow-x: auto;
}

.table {
	width: 100%;
	border-collapse: collapse;
	font-size: var(--font-size-sm);
}

.th {
	text-align: left;
	padding: var(--space-3) var(--space-4);
	font-weight: var(--font-weight-semibold);
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
	text-transform: uppercase;
	letter-spacing: 0.05em;
	border-bottom: 1px solid var(--color-border);
	white-space: nowrap;
}

.row {
	transition: background-color 0.1s;
}

.row:hover {
	background-color: var(--color-bg-primary);
}

.td {
	padding: var(--space-3) var(--space-4);
	border-bottom: 1px solid var(--color-border-light);
	color: var(--color-text-primary);
	vertical-align: top;
}

.timeCell {
	white-space: nowrap;
	color: var(--color-text-secondary);
	font-variant-numeric: tabular-nums;
}

.detailCell {
	color: var(--color-text-secondary);
	max-width: 400px;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.actionBadge {
	display: inline-block;
	padding: 0.125rem 0.5rem;
	border-radius: var(--radius-md);
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-medium);
	white-space: nowrap;
}

.actionInstance {
	background-color: var(--color-accent-light);
	color: var(--color-accent);
}

.actionUser {
	background-color: var(--color-purple-light);
	color: var(--color-purple);
}

.actionOther {
	background-color: var(--color-border-light);
	color: var(--color-text-secondary);
}

.pagination {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: var(--space-3) var(--space-4);
	border-top: 1px solid var(--color-border-light);
}

.pageInfo {
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
}

.pageButtons {
	display: flex;
	align-items: center;
	gap: var(--space-2);
}

.pageBtn {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 32px;
	height: 32px;
	border: 1px solid var(--color-border);
	border-radius: var(--radius-md);
	background-color: var(--color-bg-surface);
	color: var(--color-text-primary);
	cursor: pointer;
	transition:
		background-color 0.15s,
		border-color 0.15s;
}

.pageBtn:hover:not(:disabled) {
	background-color: var(--color-bg-primary);
}

.pageBtn:disabled {
	opacity: 0.4;
	cursor: not-allowed;
}

.pageLabel {
	font-size: var(--font-size-sm);
	color: var(--color-text-secondary);
	min-width: 60px;
	text-align: center;
}

.loadingContainer {
	display: flex;
	align-items: center;
	justify-content: center;
	padding: var(--space-8) 0;
}

@media (max-width: 768px) {
	.filters {
		flex-direction: column;
	}

	.searchWrapper {
		max-width: none;
	}

	.select,
	.dateInput {
		width: 100%;
	}

	.pagination {
		flex-direction: column;
		gap: var(--space-2);
	}
}
</style>
