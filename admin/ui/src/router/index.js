import { createRouter, createWebHistory } from 'vue-router';
import DashboardView from '../views/DashboardView.vue';
import AuditLogView from '../views/AuditLogView.vue';
import InstanceDetailView from '../views/InstanceDetailView.vue';

const routes = [
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
