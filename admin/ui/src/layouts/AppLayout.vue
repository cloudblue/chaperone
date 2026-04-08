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
							{
								[$style.active]:
									route.name === 'dashboard' ||
									route.name === 'instance-detail',
							},
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
					<RouterLink
						to="/settings"
						:class="[
							$style.navItem,
							{ [$style.active]: route.name === 'settings' },
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
							<circle cx="12" cy="12" r="3" />
							<path
								d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
							/>
						</svg>
						Settings
					</RouterLink>
				</div>
			</nav>
			<div :class="$style.userSection">
				<div :class="$style.userInfo">
					<svg
						:class="$style.userIcon"
						width="18"
						height="18"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
						<circle cx="12" cy="7" r="4" />
					</svg>
					<span :class="$style.username">{{ auth.user?.username }}</span>
				</div>
				<button
					:class="$style.logoutButton"
					aria-label="Sign out"
					@click="handleLogout"
				>
					<svg
						width="18"
						height="18"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
						<polyline points="16 17 21 12 16 7" />
						<line x1="21" y1="12" x2="9" y2="12" />
					</svg>
				</button>
			</div>
		</aside>
		<main :class="$style.main">
			<slot />
		</main>
	</div>
</template>

<script setup>
import { RouterLink, useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth.js';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

async function handleLogout() {
	try {
		await auth.logout();
	} catch {
		// Clear local state even if the server request fails
		auth.user = null;
	}
	router.push({ name: 'login' });
}
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
	flex: 1;
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

.userSection {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: var(--space-4) var(--space-5);
	border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.userInfo {
	display: flex;
	align-items: center;
	gap: var(--space-3);
	min-width: 0;
}

.userIcon {
	flex-shrink: 0;
	color: var(--color-text-sidebar);
}

.username {
	font-size: var(--font-size-sm);
	color: var(--color-text-sidebar);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.logoutButton {
	display: flex;
	align-items: center;
	justify-content: center;
	padding: var(--space-2);
	border: none;
	border-radius: var(--radius-md);
	background: transparent;
	color: var(--color-text-sidebar);
	transition:
		background-color 0.15s,
		color 0.15s;
}

.logoutButton:hover {
	background-color: var(--color-bg-sidebar-hover);
	color: var(--color-text-sidebar-active);
}
</style>
