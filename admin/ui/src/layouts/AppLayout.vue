<template>
	<div :class="$style.layout">
		<aside :class="$style.sidebar">
			<div :class="$style.logo">
				<span :class="$style.logoText">Chaperone</span>
			</div>
			<nav :class="$style.nav" aria-label="Main navigation">
				<div :class="$style.navSection">
					<span :class="$style.navLabel">Monitoring</span>
					<RouterLink
						to="/"
						:class="[
							$style.navItem,
							{ [$style.active]: route.name === 'dashboard' },
						]"
					>
						<svg
							:class="$style.navIcon"
							width="18"
							height="18"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<rect x="3" y="3" width="7" height="7" />
							<rect x="14" y="3" width="7" height="7" />
							<rect x="3" y="14" width="7" height="7" />
							<rect x="14" y="14" width="7" height="7" />
						</svg>
						Fleet Dashboard
					</RouterLink>
				</div>
				<div :class="$style.navSection">
					<span :class="$style.navLabel">Administration</span>
					<RouterLink
						to="/audit-log"
						:class="[
							$style.navItem,
							{ [$style.active]: route.name === 'audit-log' },
						]"
					>
						<svg
							:class="$style.navIcon"
							width="18"
							height="18"
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
						Audit Log
					</RouterLink>
				</div>
			</nav>
		</aside>
		<main :class="$style.main">
			<slot />
		</main>
	</div>
</template>

<script setup>
import { RouterLink, useRoute } from "vue-router";

const route = useRoute();
</script>

<style module>
.layout {
	display: flex;
	min-height: 100vh;
}

.sidebar {
	width: var(--sidebar-width);
	background-color: var(--color-bg-sidebar);
	display: flex;
	flex-direction: column;
	flex-shrink: 0;
}

.logo {
	padding: var(--space-5) var(--space-5) var(--space-6);
}

.logoText {
	font-size: var(--font-size-md);
	font-weight: var(--font-weight-bold);
	color: var(--color-text-sidebar-active);
	letter-spacing: -0.01em;
}

.nav {
	display: flex;
	flex-direction: column;
	gap: var(--space-5);
	padding: 0 var(--space-3);
}

.navSection {
	display: flex;
	flex-direction: column;
	gap: var(--space-1);
}

.navLabel {
	font-size: var(--font-size-xs);
	font-weight: var(--font-weight-semibold);
	color: var(--color-text-sidebar);
	text-transform: uppercase;
	letter-spacing: 0.05em;
	padding: 0 var(--space-3) var(--space-2);
}

.navItem {
	display: flex;
	align-items: center;
	gap: var(--space-3);
	padding: var(--space-2) var(--space-3);
	border-radius: var(--radius-md);
	font-size: var(--font-size-base);
	font-weight: var(--font-weight-normal);
	color: var(--color-text-sidebar);
	text-decoration: none;
	transition:
		background-color 0.15s,
		color 0.15s;
}

.navItem:hover {
	background-color: var(--color-bg-sidebar-hover);
	color: var(--color-text-sidebar-active);
	text-decoration: none;
}

.navItem.active {
	background-color: var(--color-bg-sidebar-active);
	color: var(--color-text-sidebar-active);
}

.navIcon {
	flex-shrink: 0;
	opacity: 0.7;
}

.active .navIcon {
	opacity: 1;
}

.main {
	flex: 1;
	padding: var(--space-6);
	overflow-y: auto;
}
</style>
