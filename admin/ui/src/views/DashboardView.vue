<template>
	<div :class="$style.page">
		<div :class="$style.header">
			<h1 :class="$style.title" data-testid="dashboard-title">
				Fleet Dashboard
			</h1>
			<div v-if="store.instances.length > 0" :class="$style.headerActions">
				<div
					:class="$style.viewToggle"
					role="radiogroup"
					aria-label="View mode"
				>
					<button
						:class="[
							$style.toggleBtn,
							viewMode === 'card' && $style.toggleActive,
						]"
						:aria-pressed="viewMode === 'card'"
						title="Card view"
						data-testid="view-toggle-card"
						@click="viewMode = 'card'"
					>
						<svg
							width="16"
							height="16"
							viewBox="0 0 16 16"
							fill="none"
							stroke="currentColor"
							stroke-width="1.5"
						>
							<rect x="1" y="1" width="6" height="6" rx="1" />
							<rect x="9" y="1" width="6" height="6" rx="1" />
							<rect x="1" y="9" width="6" height="6" rx="1" />
							<rect x="9" y="9" width="6" height="6" rx="1" />
						</svg>
					</button>
					<button
						:class="[
							$style.toggleBtn,
							viewMode === 'table' && $style.toggleActive,
						]"
						:aria-pressed="viewMode === 'table'"
						title="Table view"
						data-testid="view-toggle-table"
						@click="viewMode = 'table'"
					>
						<svg
							width="16"
							height="16"
							viewBox="0 0 16 16"
							fill="none"
							stroke="currentColor"
							stroke-width="1.5"
						>
							<line x1="1" y1="3" x2="15" y2="3" />
							<line x1="1" y1="8" x2="15" y2="8" />
							<line x1="1" y1="13" x2="15" y2="13" />
						</svg>
					</button>
				</div>
				<BaseButton data-testid="add-instance-btn" @click="showModal = true">
					Add Instance
				</BaseButton>
			</div>
		</div>

		<!-- Unreachable instances banner -->
		<div
			v-if="unreachableCount > 0"
			:class="
				unreachableCount === store.instances.length
					? $style.errorBanner
					: $style.warningBanner
			"
			role="alert"
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
				<circle cx="12" cy="12" r="10" />
				<line x1="12" y1="8" x2="12" y2="12" />
				<line x1="12" y1="16" x2="12.01" y2="16" />
			</svg>
			<template v-if="unreachableCount === store.instances.length">
				All {{ unreachableCount }} instances are unreachable &mdash; metrics
				shown are from the last successful poll
			</template>
			<template v-else>
				{{ unreachableCount }} of {{ store.instances.length }}
				{{ unreachableCount === 1 ? 'instance is' : 'instances are' }}
				unreachable
			</template>
		</div>

		<!-- Fleet metrics error -->
		<div
			v-if="metricsStore.fleetError && store.instances.length > 0"
			:class="$style.errorBanner"
			role="alert"
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
				<circle cx="12" cy="12" r="10" />
				<line x1="12" y1="8" x2="12" y2="12" />
				<line x1="12" y1="16" x2="12.01" y2="16" />
			</svg>
			Failed to load fleet metrics &mdash; data may be stale
		</div>

		<!-- Fleet KPI panel -->
		<FleetKpiPanel
			v-if="metricsStore.fleet && store.instances.length > 0"
			:metrics="metricsStore.fleet"
			:total-instances="store.instances.length"
		/>

		<div :class="$style.content">
			<!-- Loading state -->
			<div v-if="!store.initialized" :class="$style.loadingContainer">
				<LoadingSpinner size="lg" label="Loading fleet data..." />
			</div>

			<!-- First-run welcome screen -->
			<div
				v-else-if="store.instances.length === 0"
				:class="$style.welcome"
				data-testid="welcome-screen"
			>
				<div :class="$style.welcomeIcon" aria-hidden="true">
					<svg
						width="32"
						height="32"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.5"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
						<rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
						<line x1="6" y1="6" x2="6.01" y2="6" />
						<line x1="6" y1="18" x2="6.01" y2="18" />
					</svg>
				</div>
				<h2 :class="$style.welcomeTitle">Welcome to Chaperone Admin</h2>
				<p :class="$style.welcomeDescription">
					This portal gives you operational visibility into your Chaperone proxy
					fleet &mdash; health status, live metrics, per-vendor traffic
					breakdown, and more. All from a single dashboard.
				</p>
				<div :class="$style.welcomeSteps">
					<div :class="$style.step">
						<span :class="$style.stepNumber">1</span>
						<div>
							<span :class="$style.stepTitle">Register a proxy instance</span>
							<span :class="$style.stepDetail"
								>Enter the admin address (host:port) of a running Chaperone
								proxy</span
							>
						</div>
					</div>
					<div :class="$style.step">
						<span :class="$style.stepNumber">2</span>
						<div>
							<span :class="$style.stepTitle">Test the connection</span>
							<span :class="$style.stepDetail"
								>Verify the portal can reach the proxy's admin port before
								saving</span
							>
						</div>
					</div>
					<div :class="$style.step">
						<span :class="$style.stepNumber">3</span>
						<div>
							<span :class="$style.stepTitle">Monitor your fleet</span>
							<span :class="$style.stepDetail"
								>Health, version, request rates, and latency updated every 10
								seconds</span
							>
						</div>
					</div>
				</div>
				<BaseButton data-testid="add-first-instance" @click="showModal = true">
					Add Your First Instance
				</BaseButton>
			</div>

			<!-- Card view -->
			<div v-else-if="viewMode === 'card'" :class="$style.grid">
				<InstanceCard
					v-for="inst in store.instances"
					:key="inst.id"
					:instance="inst"
					@click="handleInstanceClick(inst)"
					@edit="handleEdit"
					@delete="handleDelete"
				/>
			</div>

			<!-- Table view -->
			<BaseCard v-else>
				<InstanceTable
					:instances="store.instances"
					data-testid="instance-table"
					@click="handleInstanceClick"
					@edit="handleEdit"
					@delete="handleDelete"
				/>
			</BaseCard>
		</div>

		<!-- Add/Edit modal -->
		<AddInstanceModal
			v-if="showModal"
			:instance="editingInstance"
			@close="closeModal"
			@saved="handleSaved"
		/>

		<!-- Remove confirmation dialog -->
		<ConfirmDialog
			v-if="deletingInstance"
			title="Remove instance"
			:description="`This will stop monitoring &quot;${deletingInstance.name}&quot; and remove it from the registry. Metrics history for this instance will be lost.`"
			confirm-label="Remove"
			:destructive="true"
			@confirm="onConfirmDelete"
			@cancel="cancelDelete"
		/>
	</div>
</template>

<script setup>
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import BaseCard from '../components/BaseCard.vue';
import BaseButton from '../components/BaseButton.vue';
import InstanceCard from '../components/InstanceCard.vue';
import InstanceTable from '../components/InstanceTable.vue';
import AddInstanceModal from '../components/AddInstanceModal.vue';
import ConfirmDialog from '../components/ConfirmDialog.vue';
import FleetKpiPanel from '../components/FleetKpiPanel.vue';
import LoadingSpinner from '../components/LoadingSpinner.vue';
import { useInstanceStore } from '../stores/instances.js';
import { useMetricsStore } from '../stores/metrics.js';
import { usePolling } from '../composables/usePolling.js';
import { useConfirmDialog } from '../composables/useConfirmDialog.js';

const router = useRouter();
const store = useInstanceStore();
const metricsStore = useMetricsStore();
const showModal = ref(false);
const editingInstance = ref(null);
const viewMode = ref('card');

const unreachableCount = computed(
	() => store.instances.filter((i) => i.status === 'unreachable').length,
);

usePolling(() => store.fetchInstances(), 10000);
usePolling(() => metricsStore.fetchFleetMetrics(), 10000);

function handleInstanceClick(instance) {
	router.push({ name: 'instance-detail', params: { id: instance.id } });
}

const {
	pending: deletingInstance,
	requestConfirm: handleDelete,
	confirm: confirmDelete,
	cancel: cancelDelete,
} = useConfirmDialog();

function handleEdit(instance) {
	editingInstance.value = instance;
	showModal.value = true;
}

function onConfirmDelete() {
	confirmDelete((inst) => store.deleteInstance(inst.id));
}

function handleSaved() {
	store.fetchInstances();
}

function closeModal() {
	showModal.value = false;
	editingInstance.value = null;
}
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

.headerActions {
	display: flex;
	align-items: center;
	gap: var(--space-3);
}

.viewToggle {
	display: flex;
	border: 1px solid var(--color-border);
	border-radius: var(--radius-lg);
	overflow: hidden;
}

.toggleBtn {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 36px;
	height: 36px;
	background: var(--color-bg-surface);
	border: none;
	color: var(--color-text-tertiary);
	cursor: pointer;
	transition:
		background-color 0.15s,
		color 0.15s;
}

.toggleBtn:hover {
	background-color: var(--color-bg-primary);
	color: var(--color-text-primary);
}

.toggleActive {
	background-color: var(--color-accent-light);
	color: var(--color-accent);
}

.toggleActive:hover {
	background-color: var(--color-accent-light);
	color: var(--color-accent);
}

.toggleBtn:focus-visible {
	outline: 2px solid var(--color-accent);
	outline-offset: -2px;
	z-index: 1;
}

.warningBanner {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	padding: var(--space-3) var(--space-4);
	background-color: var(--color-warning-bg);
	color: var(--color-warning);
	border: 1px solid var(--color-warning-border);
	border-radius: var(--radius-lg);
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
}

.content {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.welcome {
	display: flex;
	flex-direction: column;
	align-items: center;
	text-align: center;
	padding: var(--space-8) var(--space-4);
	gap: var(--space-4);
	background-color: var(--color-bg-surface);
	border: 1px solid var(--color-border);
	border-radius: var(--radius-xl);
	box-shadow: var(--shadow-sm);
}

.welcomeIcon {
	width: 64px;
	height: 64px;
	display: flex;
	align-items: center;
	justify-content: center;
	background-color: var(--color-accent-light);
	border-radius: 16px;
	color: var(--color-accent);
}

.welcomeTitle {
	font-size: var(--font-size-xl);
	font-weight: var(--font-weight-bold);
	letter-spacing: -0.02em;
	margin: 0;
}

.welcomeDescription {
	font-size: var(--font-size-sm);
	color: var(--color-text-secondary);
	line-height: var(--line-height-relaxed);
	max-width: 480px;
	margin: 0;
}

.welcomeSteps {
	display: flex;
	flex-direction: column;
	gap: var(--space-3);
	text-align: left;
	width: 100%;
	max-width: 400px;
	padding: var(--space-4) 0;
}

.step {
	display: flex;
	align-items: flex-start;
	gap: var(--space-3);
}

.stepNumber {
	width: 24px;
	height: 24px;
	display: flex;
	align-items: center;
	justify-content: center;
	border-radius: 50%;
	background-color: var(--color-bg-primary);
	border: 1px solid var(--color-border);
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-semibold);
	color: var(--color-text-secondary);
	flex-shrink: 0;
	margin-top: 1px;
}

.stepTitle {
	display: block;
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
	color: var(--color-text-primary);
}

.stepDetail {
	display: block;
	font-size: var(--font-size-xs);
	color: var(--color-text-tertiary);
	line-height: var(--line-height-relaxed);
}

.grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
	gap: var(--space-4);
}

.errorBanner {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	padding: var(--space-3) var(--space-4);
	background-color: var(--color-error-bg);
	color: var(--color-error);
	border: 1px solid var(--color-error-border);
	border-radius: var(--radius-lg);
	font-size: var(--font-size-sm);
	font-weight: var(--font-weight-medium);
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
		align-items: flex-start;
		gap: var(--space-3);
	}

	.grid {
		grid-template-columns: 1fr;
	}
}
</style>
