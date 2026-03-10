<template>
	<div :class="$style.page">
		<div :class="$style.header">
			<h1 :class="$style.title">Fleet Dashboard</h1>
			<div v-if="store.instances.length > 0" :class="$style.headerActions">
				<div :class="$style.viewToggle" role="radiogroup" aria-label="View mode">
					<button
						:class="[$style.toggleBtn, viewMode === 'card' && $style.toggleActive]"
						:aria-pressed="viewMode === 'card'"
						title="Card view"
						@click="viewMode = 'card'"
					>
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
							<rect x="1" y="1" width="6" height="6" rx="1" />
							<rect x="9" y="1" width="6" height="6" rx="1" />
							<rect x="1" y="9" width="6" height="6" rx="1" />
							<rect x="9" y="9" width="6" height="6" rx="1" />
						</svg>
					</button>
					<button
						:class="[$style.toggleBtn, viewMode === 'table' && $style.toggleActive]"
						:aria-pressed="viewMode === 'table'"
						title="Table view"
						@click="viewMode = 'table'"
					>
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
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

		<!-- Staleness banner -->
		<div
			v-if="staleInstances.length > 0"
			:class="$style.staleBanner"
			role="alert"
		>
			<svg
				width="16" height="16" viewBox="0 0 24 24" fill="none"
				stroke="currentColor" stroke-width="2"
				stroke-linecap="round" stroke-linejoin="round"
				aria-hidden="true"
			>
				<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
				<line x1="12" y1="9" x2="12" y2="13" />
				<line x1="12" y1="17" x2="12.01" y2="17" />
			</svg>
			{{ staleInstances.length === 1 ? '1 instance has' : `${staleInstances.length} instances have` }}
			stale data — last seen over 2 minutes ago
		</div>

		<div :class="$style.content">
			<!-- First-run welcome screen -->
			<div v-if="!store.loading && store.instances.length === 0" :class="$style.welcome">
				<div :class="$style.welcomeIcon" aria-hidden="true">
					<svg
						width="32" height="32" viewBox="0 0 24 24" fill="none"
						stroke="currentColor" stroke-width="1.5"
						stroke-linecap="round" stroke-linejoin="round"
					>
						<rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
						<rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
						<line x1="6" y1="6" x2="6.01" y2="6" />
						<line x1="6" y1="18" x2="6.01" y2="18" />
					</svg>
				</div>
				<h2 :class="$style.welcomeTitle">Welcome to Chaperone Admin</h2>
				<p :class="$style.welcomeDescription">
					This portal gives you operational visibility into your Chaperone proxy fleet &mdash;
					health status, live metrics, per-vendor traffic breakdown, and more. All from a single dashboard.
				</p>
				<div :class="$style.welcomeSteps">
					<div :class="$style.step">
						<span :class="$style.stepNumber">1</span>
						<div>
							<span :class="$style.stepTitle">Register a proxy instance</span>
							<span :class="$style.stepDetail">Enter the admin address (host:port) of a running Chaperone proxy</span>
						</div>
					</div>
					<div :class="$style.step">
						<span :class="$style.stepNumber">2</span>
						<div>
							<span :class="$style.stepTitle">Test the connection</span>
							<span :class="$style.stepDetail">Verify the portal can reach the proxy's admin port before saving</span>
						</div>
					</div>
					<div :class="$style.step">
						<span :class="$style.stepNumber">3</span>
						<div>
							<span :class="$style.stepTitle">Monitor your fleet</span>
							<span :class="$style.stepDetail">Health, version, request rates, and latency updated every 10 seconds</span>
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
					@edit="handleEdit"
					@delete="handleDelete"
				/>
			</div>

			<!-- Table view -->
			<BaseCard v-else>
				<InstanceTable
					:instances="store.instances"
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
			@confirm="confirmDelete"
			@cancel="deletingInstance = null"
		/>
	</div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import BaseCard from "../components/BaseCard.vue";
import BaseButton from "../components/BaseButton.vue";
import InstanceCard from "../components/InstanceCard.vue";
import InstanceTable from "../components/InstanceTable.vue";
import AddInstanceModal from "../components/AddInstanceModal.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import { useInstanceStore } from "../stores/instances.js";
import { isInstanceStale } from "../utils/instance.js";

const store = useInstanceStore();
const showModal = ref(false);
const editingInstance = ref(null);
const deletingInstance = ref(null);
const viewMode = ref("card");
let pollInterval = null;

const staleInstances = computed(() => {
	return store.instances.filter((inst) => isInstanceStale(inst));
});

onMounted(() => {
	store.fetchInstances();
	pollInterval = setInterval(() => store.fetchInstances(), 10000);
});

onUnmounted(() => {
	if (pollInterval) clearInterval(pollInterval);
});

function handleEdit(instance) {
	editingInstance.value = instance;
	showModal.value = true;
}

function handleDelete(instance) {
	deletingInstance.value = instance;
}

async function confirmDelete() {
	const instance = deletingInstance.value;
	deletingInstance.value = null;
	try {
		await store.deleteInstance(instance.id);
	} catch {
		// TODO: surface error in a toast/banner
	}
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
	transition: background-color 0.15s, color 0.15s;
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

.staleBanner {
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
</style>
