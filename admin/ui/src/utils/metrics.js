export const VENDOR_COLORS = [
	'#3b82f6', // blue
	'#059669', // emerald
	'#8b5cf6', // purple
	'#f97316', // orange
	'#06b6d4', // cyan
	'#ec4899', // pink
	'#f59e0b', // amber
	'#64748b', // slate
];

export const LATENCY_COLORS = {
	p50: '#3b82f6', // blue
	p95: '#f59e0b', // amber
	p99: '#dc2626', // red
};

export function formatRps(value) {
	if (value == null) return '-';
	if (value >= 10000) return `${(value / 1000).toFixed(1)}k`;
	if (value >= 1000) return `${(value / 1000).toFixed(2)}k`;
	if (value >= 100) return Math.round(value).toString();
	if (value >= 10) return value.toFixed(1);
	return value.toFixed(2);
}

export function formatLatency(ms) {
	if (ms == null) return '-';
	if (ms >= 10000) return `${(ms / 1000).toFixed(1)}s`;
	if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
	if (ms >= 100) return `${Math.round(ms)}ms`;
	if (ms >= 10) return `${ms.toFixed(1)}ms`;
	return `${ms.toFixed(2)}ms`;
}

export function formatErrorRate(rate) {
	if (rate == null) return '-';
	const pct = rate * 100;
	if (pct >= 10) return `${pct.toFixed(1)}%`;
	if (pct >= 1) return `${pct.toFixed(2)}%`;
	if (pct >= 0.01) return `${pct.toFixed(3)}%`;
	if (pct === 0) return '0%';
	return `${pct.toFixed(3)}%`;
}

export function formatCount(value) {
	if (value == null) return '-';
	return Math.round(value).toLocaleString();
}

export function trendDirection(value) {
	if (value == null) return null;
	if (value > 0) return 'up';
	if (value < 0) return 'down';
	return 'flat';
}

export function assignVendorColors(vendorIds) {
	const map = {};
	vendorIds.forEach((id, i) => {
		map[id] = VENDOR_COLORS[i % VENDOR_COLORS.length];
	});
	return map;
}
