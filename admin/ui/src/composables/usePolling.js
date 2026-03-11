import { onMounted, onUnmounted } from "vue";

export function usePolling(fn, intervalMs = 10000) {
	let id = null;

	function start() {
		stop();
		fn();
		id = setInterval(fn, intervalMs);
	}

	function stop() {
		if (id !== null) {
			clearInterval(id);
			id = null;
		}
	}

	onMounted(start);
	onUnmounted(stop);

	return { start, stop };
}
