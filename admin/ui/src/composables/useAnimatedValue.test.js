import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref, nextTick } from 'vue';
import { withSetup } from '../utils/test-utils.js';
import { easeOutCubic, useAnimatedValue } from './useAnimatedValue.js';

describe('easeOutCubic', () => {
	it('returns 0 at start', () => {
		expect(easeOutCubic(0)).toBe(0);
	});

	it('returns 1 at end', () => {
		expect(easeOutCubic(1)).toBe(1);
	});

	it('decelerates — first half covers more than 50%', () => {
		expect(easeOutCubic(0.5)).toBeGreaterThan(0.5);
	});

	it('is monotonically increasing', () => {
		let prev = 0;
		for (let t = 0.1; t <= 1; t += 0.1) {
			const val = easeOutCubic(t);
			expect(val).toBeGreaterThan(prev);
			prev = val;
		}
	});
});

describe('useAnimatedValue', () => {
	let rafCallbacks;
	let rafId;

	beforeEach(() => {
		rafCallbacks = [];
		rafId = 0;
		vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
			rafCallbacks.push(cb);
			return ++rafId;
		});
		vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {});
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	function flushFrames(timestamp) {
		const pending = [...rafCallbacks];
		rafCallbacks = [];
		pending.forEach((cb) => cb(timestamp));
	}

	it('starts with the initial source value', () => {
		const source = ref(100);
		const { result } = withSetup(() => useAnimatedValue(source));
		expect(result.value).toBe(100);
	});

	it('starts at 0 for null source', () => {
		const source = ref(null);
		const { result } = withSetup(() => useAnimatedValue(source));
		expect(result.value).toBe(0);
	});

	it('schedules animation when source changes', async () => {
		const source = ref(100);
		withSetup(() => useAnimatedValue(source));

		source.value = 200;
		await nextTick();

		expect(window.requestAnimationFrame).toHaveBeenCalled();
	});

	it('reaches target value after full duration', async () => {
		const duration = 400;
		const source = ref(100);
		const { result } = withSetup(() => useAnimatedValue(source, duration));

		source.value = 200;
		await nextTick();

		// First frame sets startTime
		flushFrames(1000);
		// Frame past duration
		flushFrames(1000 + duration + 1);

		expect(result.value).toBe(200);
	});

	it('shows intermediate value mid-animation', async () => {
		const duration = 400;
		const source = ref(0);
		const { result } = withSetup(() => useAnimatedValue(source, duration));

		source.value = 100;
		await nextTick();

		// First frame at t=0
		flushFrames(1000);
		// Frame at t=200 (50% duration)
		flushFrames(1200);

		expect(result.value).toBeGreaterThan(0);
		expect(result.value).toBeLessThan(100);
	});

	it('resets to 0 when source becomes null', async () => {
		const source = ref(100);
		const { result } = withSetup(() => useAnimatedValue(source));

		source.value = null;
		await nextTick();

		expect(result.value).toBe(0);
	});

	it('interrupts running animation on new value', async () => {
		const duration = 400;
		const source = ref(0);
		const { result } = withSetup(() => useAnimatedValue(source, duration));

		source.value = 100;
		await nextTick();
		flushFrames(1000);
		flushFrames(1100); // partial animation

		const midValue = result.value;
		expect(midValue).toBeGreaterThan(0);
		expect(midValue).toBeLessThan(100);

		// Change target mid-animation
		source.value = 50;
		await nextTick();

		expect(window.cancelAnimationFrame).toHaveBeenCalled();

		// Complete the new animation
		flushFrames(2000);
		flushFrames(2000 + duration + 1);

		expect(result.value).toBe(50);
	});
});
