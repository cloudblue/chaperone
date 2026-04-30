<template>
	<div :class="$style.page">
		<!-- Breadcrumb -->
		<nav :class="$style.breadcrumb" aria-label="Breadcrumb">
			<RouterLink
				to="/"
				:class="$style.breadcrumbLink"
				data-testid="breadcrumb-fleet"
				>Fleet</RouterLink
			>
			<span :class="$style.breadcrumbSep" aria-hidden="true">/</span>
			<span :class="$style.breadcrumbCurrent">{{
				instance?.name ?? 'Loading...'
			}}</span>
		</nav>

		<!-- Header -->
		<div v-if="instance" :class="$style.header">
			<div>
				<h1 ref="headingRef" :class="$style.title" tabindex="-1">
					{{ instance.name }}
				</h1>
				<div :class="$style.meta">
					<StatusIndicator
						:status="instance.status"
						:label="getStatusLabel(instance.status)"
						size="sm"
					/>
					<span :class="$style.address">{{ instance.address }}</span>
					<span v-if="instance.version" :class="$style.version"
						>v{{ instance.version }}</span
					>
				</div>
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
import { useRoute, RouterLink } from 'vue-router';
import StatusIndicator from '../components/StatusIndicator.vue';
import BaseEmptyState from '../components/BaseEmptyState.vue';
import LoadingSpinner from '../components/LoadingSpinner.vue';
import OverviewTab from '../components/OverviewTab.vue';
import TrafficTab from '../components/TrafficTab.vue';
import { useInstanceStore } from '../stores/instances.js';
import { useMetricsStore } from '../stores/metrics.js';
import { getStatusLabel } from '../utils/instance.js';
import { usePolling } from '../composables/usePolling.js';

const route = useRoute();
const store = useInstanceStore();
const metricsStore = useMetricsStore();
const tabs = [
	{ id: 'overview', label: 'Overview' },
	{ id: 'traffic', label: 'Traffic' },
];

const activeTab = ref('overview');
const headingRef = ref(null);

const instanceId = computed(() => Number(route.params.id));

const instance = computed(() =>
	store.instances.find((i) => i.id === instanceId.value),
);

const metrics = computed(() => metricsStore.instance);

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

onUnmounted(() => metricsStore.clearInstance());
</script>

<style module>
.page {
	display: flex;
	flex-direction: column;
	gap: var(--space-5);
}

.breadcrumb {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	font-size: var(--font-size-sm);
}

.breadcrumbLink {
	color: var(--color-text-secondary);
	text-decoration: none;
}

.breadcrumbLink:hover {
	color: var(--color-accent);
}

.breadcrumbSep {
	color: var(--color-text-tertiary);
}

.breadcrumbCurrent {
	color: var(--color-text-primary);
	font-weight: var(--font-weight-medium);
}

.header {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
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
	align-items: center;
	gap: var(--space-3);
	margin-top: var(--space-2);
}

.address {
	font-size: var(--font-size-xs);
	font-family: var(--font-family-mono);
	color: var(--color-text-secondary);
}

.version {
	font-size: var(--font-size-xs);
	color: var(--color-text-tertiary);
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
</style>
