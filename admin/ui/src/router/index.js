import { createRouter, createWebHistory } from 'vue-router';
import DashboardView from '../views/DashboardView.vue';
import AuditLogView from '../views/AuditLogView.vue';

const routes = [
	{
		path: '/',
		name: 'dashboard',
		component: DashboardView,
	},
	{
		path: '/audit-log',
		name: 'audit-log',
		component: AuditLogView,
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

export default router;
