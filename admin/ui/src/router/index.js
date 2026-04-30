import { createRouter, createWebHistory } from 'vue-router';
import { useAuthStore } from '../stores/auth.js';
import DashboardView from '../views/DashboardView.vue';
import AuditLogView from '../views/AuditLogView.vue';
import InstanceDetailView from '../views/InstanceDetailView.vue';
import LoginView from '../views/LoginView.vue';
import SettingsView from '../views/SettingsView.vue';

const routes = [
	{
		path: '/login',
		name: 'login',
		component: LoginView,
		meta: { public: true },
	},
	{
		path: '/',
		name: 'dashboard',
		component: DashboardView,
	},
	{
		path: '/instances/:id',
		name: 'instance-detail',
		component: InstanceDetailView,
	},
	{
		path: '/audit-log',
		name: 'audit-log',
		component: AuditLogView,
	},
	{
		path: '/settings',
		name: 'settings',
		component: SettingsView,
	},
	{
		path: '/:pathMatch(.*)*',
		name: 'not-found',
		redirect: '/',
	},
];

const router = createRouter({
	history: createWebHistory(),
	routes,
});

router.beforeEach(async (to) => {
	const auth = useAuthStore();

	if (!auth.ready) await auth.checkSession();

	if (!to.meta.public && !auth.isAuthenticated) {
		return { name: 'login', query: { redirect: to.fullPath } };
	}

	if (to.name === 'login' && auth.isAuthenticated) {
		return { name: 'dashboard' };
	}
});

export default router;
