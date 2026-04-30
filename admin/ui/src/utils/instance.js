export function formatTime(ts) {
	if (!ts) return '';
	const d = new Date(ts);
	const secs = Math.floor((Date.now() - d.getTime()) / 1000);
	if (secs < 60) return 'just now';
	if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
	if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
	return d.toLocaleDateString();
}

const STATUS_LABELS = {
	healthy: 'Healthy',
	unreachable: 'Unreachable',
	unknown: 'Unknown',
};

export function getStatusLabel(status) {
	return STATUS_LABELS[status] || STATUS_LABELS.unknown;
}
