import { ref, watch, toValue, onUnmounted } from 'vue';

export function easeOutCubic(t) {
	return 1 - Math.pow(1 - t, 3);
}

export function useAnimatedValue(source, duration = 400) {
	const display = ref(toValue(source) ?? 0);
	let frameId = null;
	let startTime = 0;
	let startVal = 0;
	let endVal = 0;

	function tick(timestamp) {
		if (startTime === 0) startTime = timestamp;
		const progress = Math.min((timestamp - startTime) / duration, 1);
		display.value = startVal + (endVal - startVal) * easeOutCubic(progress);

		if (progress < 1) {
			frameId = requestAnimationFrame(tick);
		} else {
			frameId = null;
		}
	}

	watch(
		() => toValue(source),
		(newVal) => {
			if (newVal == null) {
				if (frameId != null) cancelAnimationFrame(frameId);
				frameId = null;
				display.value = 0;
				return;
			}
			if (frameId != null) cancelAnimationFrame(frameId);
			startVal = display.value;
			endVal = newVal;
			startTime = 0;
			frameId = requestAnimationFrame(tick);
		},
	);

	onUnmounted(() => {
		if (frameId != null) cancelAnimationFrame(frameId);
	});

	return display;
}
