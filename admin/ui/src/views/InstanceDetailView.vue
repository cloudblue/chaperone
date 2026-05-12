<template>
	<div :class="$style.page">
		<!-- Breadcrumb -->
		<nav :class="$style.breadcrumb" aria-label="Breadcrumb">
			<RouterLink
				to="/"
				:class="$style.breadcrumbLink"
				data-testid="breadcrumb-fleet"
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
					<path d="M10 3 5 8l5 5" />
				</svg>
				Fleet</RouterLink
			>
		</nav>

		<!-- Header -->
		<div v-if="instance" :class="$style.header">
			<div :class="$style.headerContent">
				<h1 ref="headingRef" :class="$style.title" tabindex="-1">
					{{ instance.name }}
				</h1>
				<div :class="$style.meta">
					<div :class="$style.metaItem">
						<span :class="$style.metaLabel">Status</span>
						<StatusIndicator
							:status="instance.status"
							:label="getStatusLabel(instance.status)"
						/>
					</div>
					<div :class="$style.metaItem">
						<span :class="$style.metaLabel">Address</span>
						<span :class="[$style.metaValue, $style.metaMono]">{{
							instance.address
						}}</span>
					</div>
					<div :class="$style.metaItem">
						<span :class="$style.metaLabel">Version</span>
						<span :class="$style.metaValue">{{ instance.version || '—' }}</span>
					</div>
					<div :class="$style.metaItem">
						<span :class="$style.metaLabel">Last seen</span>
						<span :class="$style.metaValue">{{
							formatTime(instance.last_seen_at) || '—'
						}}</span>
					</div>
				</div>
			</div>
			<div :class="$style.actions">
				<BaseButton size="sm" variant="secondary" @click="handleEdit(instance)">
					Edit
				</BaseButton>
				<InstanceActionMenu
					:label="instance.name"
					@remove="handleDelete(instance)"
				/>
			</div>
		</div>

		<!-- Tabs -->
		<div v-if="instance" :class="$style.tabs" role="tablist">
			<button
				v-for="tab in tabs"
				:id="`tab-${tab.id}`"
				:key="tab.id"
				:class="[$style.tab, activeTab === tab.id && $style.tabActive]"
				:aria-selected="activeTab === tab.id"
				:tabindex="activeTab === tab.id ? 0 : -1"
				role="tab"
				:aria-controls="`tabpanel-${tab.id}`"
				:data-testid="`tab-${tab.id}`"
				@click="activeTab = tab.id"
				@keydown.right.prevent="nextTab"
				@keydown.left.prevent="prevTab"
			>
				{{ tab.label }}
			</button>
		</div>

		<!-- Tab content -->
		<div
			v-if="instance && metrics"
			:id="`tabpanel-${activeTab}`"
			role="tabpanel"
			:aria-labelledby="`tab-${activeTab}`"
		>
			<OverviewTab v-if="activeTab === 'overview'" :metrics="metrics" />
			<TrafficTab v-else :metrics="metrics" />
		</div>

		<AddInstanceModal
			v-if="showModal"
			:instance="editingInstance"
			@close="closeModal"
			@saved="handleSaved"
		/>

		<ConfirmDialog
			v-if="deletingInstance"
			title="Remove instance"
			:description="`This will stop monitoring &quot;${deletingInstance.name}&quot; and remove it from the registry. Metrics history for this instance will be lost.`"
			confirm-label="Remove"
			:destructive="true"
			@confirm="onConfirmDelete"
			@cancel="cancelDelete"
		/>

		<!-- Collecting data state -->
		<div
			v-else-if="instance && !metrics && !metricsStore.instanceError"
			:class="$style.collecting"
		>
			<BaseEmptyState
				title="Collecting data"
				description="Metrics will appear here once the portal has gathered enough data points. This usually takes about 30 seconds after the instance is registered."
			/>
		</div>

		<!-- Error state -->
		<div v-else-if="metricsStore.instanceError" :class="$style.error">
			<BaseEmptyState
				title="Cannot load metrics"
				:description="metricsStore.instanceError.message"
			/>
		</div>

		<!-- Loading state -->
		<div v-else-if="!store.initialized" :class="$style.loadingContainer">
			<LoadingSpinner size="lg" label="Loading instance..." />
		</div>

		<!-- Instance not found -->
		<div v-else-if="!instance" :class="$style.error">
			<BaseEmptyState
				title="Instance not found"
				description="This instance may have been removed. Return to the fleet dashboard to see active instances."
			/>
		</div>
	</div>
</template>

<script setup>
import { ref, computed, watch, onUnmounted, nextTick } from 'vue';
import { useRoute, useRouter, RouterLink } from 'vue-router';
import BaseButton from '../components/BaseButton.vue';
import StatusIndicator from '../components/StatusIndicator.vue';
import InstanceActionMenu from '../components/InstanceActionMenu.vue';
import AddInstanceModal from '../components/AddInstanceModal.vue';
import ConfirmDialog from '../components/ConfirmDialog.vue';
import BaseEmptyState from '../components/BaseEmptyState.vue';
import LoadingSpinner from '../components/LoadingSpinner.vue';
import OverviewTab from '../components/OverviewTab.vue';
import TrafficTab from '../components/TrafficTab.vue';
import { useInstanceStore } from '../stores/instances.js';
import { useMetricsStore } from '../stores/metrics.js';
import { getStatusLabel, formatTime } from '../utils/instance.js';
import { usePolling } from '../composables/usePolling.js';
import { useConfirmDialog } from '../composables/useConfirmDialog.js';

const router = useRouter();
const route = useRoute();
const store = useInstanceStore();
const metricsStore = useMetricsStore();
const tabs = [
	{ id: 'overview', label: 'Overview' },
	{ id: 'traffic', label: 'Traffic' },
];

const activeTab = ref('overview');
const headingRef = ref(null);
const showModal = ref(false);
const editingInstance = ref(null);

const instanceId = computed(() => Number(route.params.id));

const instance = computed(() =>
	store.instances.find((i) => i.id === instanceId.value),
);

const metrics = computed(() => metricsStore.instance);

const {
	pending: deletingInstance,
	requestConfirm: handleDelete,
	confirm: confirmDelete,
	cancel: cancelDelete,
} = useConfirmDialog();

function activateTab(tabId) {
	activeTab.value = tabId;
	document.getElementById(`tab-${tabId}`)?.focus();
}

function nextTab() {
	const i = tabs.findIndex((t) => t.id === activeTab.value);
	activateTab(tabs[(i + 1) % tabs.length].id);
}

function prevTab() {
	const i = tabs.findIndex((t) => t.id === activeTab.value);
	activateTab(tabs[(i - 1 + tabs.length) % tabs.length].id);
}

usePolling(() => store.fetchInstances(), 10000);
usePolling(() => metricsStore.fetchInstanceMetrics(instanceId.value), 10000);

// Focus heading when instance data becomes available after navigation
watch(instance, (inst, prevInst) => {
	if (inst && !prevInst) {
		nextTick(() => headingRef.value?.focus());
	}
});

watch(instanceId, () => {
	metricsStore.clearInstance();
	activeTab.value = 'overview';
});

function handleEdit(inst) {
	editingInstance.value = inst;
	showModal.value = true;
}

function handleSaved() {
	store.fetchInstances();
}

function closeModal() {
	showModal.value = false;
	editingInstance.value = null;
}

function onConfirmDelete() {
	confirmDelete(async (inst) => {
		await store.deleteInstance(inst.id);
		router.push({ name: 'dashboard' });
	});
}

onUnmounted(() => metricsStore.clearInstance());
</script>

<style module>
.page {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.breadcrumb {
	display: flex;
	align-items: center;
	gap: 6px;
	font-size: var(--font-size-sm);
}

.breadcrumbLink {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	color: var(--color-text-secondary);
	text-decoration: none;
}

.breadcrumbLink:hover {
	color: var(--color-accent);
}

.header {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
	gap: var(--space-4);
}

.headerContent {
	display: flex;
	flex-direction: column;
	gap: var(--space-3);
	min-width: 0;
}

.title {
	font-size: var(--font-size-xl);
	font-weight: var(--font-weight-bold);
	letter-spacing: -0.02em;
	margin: 0;
	outline: none;
}

.meta {
	display: flex;
	flex-wrap: wrap;
	align-items: flex-start;
	gap: var(--space-8);
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

.metaMono {
	font-family: var(--font-family-mono);
	font-size: var(--font-size-xs);
	color: var(--color-text-secondary);
}

.actions {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	flex-shrink: 0;
}

.tabs {
	display: flex;
	gap: 0;
	border-bottom: 1px solid var(--color-border);
}

.tab {
	padding: var(--space-2) var(--space-4);
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
	color: var(--color-text-secondary);
	background: none;
	border: none;
	border-bottom: 2px solid transparent;
	cursor: pointer;
	transition:
		color 0.15s,
		border-color 0.15s;
	margin-bottom: -1px;
}

.tab:hover {
	color: var(--color-text-primary);
}

.tabActive {
	color: var(--color-accent);
	border-bottom-color: var(--color-accent);
}

.collecting,
.error {
	padding: var(--space-8) 0;
}

.loadingContainer {
	display: flex;
	align-items: center;
	justify-content: center;
	padding: var(--space-8) 0;
}

@media (max-width: 768px) {
	.header {
		flex-direction: column;
	}

	.actions {
		align-self: flex-start;
	}
}
</style>
