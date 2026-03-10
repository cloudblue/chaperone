export const STALE_THRESHOLD_MS = 2 * 60 * 1000; // 2 minutes

export function isInstanceStale(instance) {
	if (instance.status !== "healthy" || !instance.last_seen_at) return false;
	return Date.now() - new Date(instance.last_seen_at).getTime() > STALE_THRESHOLD_MS;
}

export function formatTime(ts) {
	if (!ts) return "";
	const d = new Date(ts);
	const secs = Math.floor((Date.now() - d.getTime()) / 1000);
	if (secs < 60) return "just now";
	if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
	if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
	return d.toLocaleDateString();
}
