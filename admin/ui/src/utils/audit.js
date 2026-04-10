// Action identifiers must match the backend constants in admin/api/audit_actions.go.
const ACTION_LABELS = {
	'instance.create': 'Instance created',
	'instance.update': 'Instance updated',
	'instance.delete': 'Instance deleted',
	'user.login': 'User logged in',
	'user.logout': 'User logged out',
	'user.password_change': 'Password changed',
};

const ACTION_OPTIONS = [
	{ value: '', label: 'All actions' },
	{ value: 'instance.create', label: 'Instance created' },
	{ value: 'instance.update', label: 'Instance updated' },
	{ value: 'instance.delete', label: 'Instance deleted' },
	{ value: 'user.login', label: 'User logged in' },
	{ value: 'user.logout', label: 'User logged out' },
	{ value: 'user.password_change', label: 'Password changed' },
];

export function getActionLabel(action) {
	return ACTION_LABELS[action] || action;
}

export function getActionOptions() {
	return ACTION_OPTIONS;
}

export function formatAuditTimestamp(isoString) {
	if (!isoString) return '';
	const d = new Date(isoString);
	if (isNaN(d.getTime())) return '';

	const now = new Date();
	const diffMs = now - d;
	const diffSecs = Math.floor(diffMs / 1000);

	if (diffSecs < 60) return 'just now';
	if (diffSecs < 3600) return `${Math.floor(diffSecs / 60)}m ago`;

	const isToday =
		d.getDate() === now.getDate() &&
		d.getMonth() === now.getMonth() &&
		d.getFullYear() === now.getFullYear();

	const time = d.toLocaleTimeString(undefined, {
		hour: '2-digit',
		minute: '2-digit',
	});

	if (isToday) return `Today ${time}`;

	const yesterday = new Date(now);
	yesterday.setDate(yesterday.getDate() - 1);
	const isYesterday =
		d.getDate() === yesterday.getDate() &&
		d.getMonth() === yesterday.getMonth() &&
		d.getFullYear() === yesterday.getFullYear();

	if (isYesterday) return `Yesterday ${time}`;

	return d.toLocaleDateString(undefined, {
		month: 'short',
		day: 'numeric',
		year: d.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
		hour: '2-digit',
		minute: '2-digit',
	});
}

// Converts a YYYY-MM-DD date string to an RFC 3339 UTC timestamp
// representing the start of that day in the user's local timezone.
export function startOfLocalDay(dateStr) {
	if (!dateStr) return '';
	return new Date(dateStr + 'T00:00:00').toISOString();
}

// Converts a YYYY-MM-DD date string to an RFC 3339 UTC timestamp
// representing the end of that day in the user's local timezone.
export function endOfLocalDay(dateStr) {
	if (!dateStr) return '';
	return new Date(dateStr + 'T23:59:59').toISOString();
}

export function buildAuditQueryString(filters) {
	const params = new URLSearchParams();

	if (filters.q) params.set('q', filters.q);
	if (filters.action) params.set('action', filters.action);
	if (filters.from) params.set('from', startOfLocalDay(filters.from));
	if (filters.to) params.set('to', endOfLocalDay(filters.to));
	if (filters.page && filters.page > 1)
		params.set('page', String(filters.page));
	if (filters.perPage && filters.perPage !== 20)
		params.set('per_page', String(filters.perPage));

	const qs = params.toString();
	return qs ? `?${qs}` : '';
}

export function totalPages(total, perPage) {
	if (total <= 0 || perPage <= 0) return 1;
	return Math.ceil(total / perPage);
}
