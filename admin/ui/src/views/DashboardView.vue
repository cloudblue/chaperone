<template>
	<div :class="$style.page">
		<div :class="$style.header">
			<h1 :class="$style.title">Fleet Dashboard</h1>
		</div>
		<div :class="$style.content">
			<BaseCard>
				<BaseEmptyState
					title="No instances registered"
					description="Add your first Chaperone proxy instance to start monitoring."
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
							<rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
							<rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
							<line x1="6" y1="6" x2="6.01" y2="6" />
							<line x1="6" y1="18" x2="6.01" y2="18" />
						</svg>
					</template>
				</BaseEmptyState>
			</BaseCard>
		</div>
	</div>
</template>

<script setup>
import BaseCard from "../components/BaseCard.vue";
import BaseEmptyState from "../components/BaseEmptyState.vue";
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

.content {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}
</style>
