import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { withSetup } from "../utils/test-utils.js";
import { usePolling } from "./usePolling.js";

describe("usePolling", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("calls fn immediately on mount", () => {
		const fn = vi.fn();
		withSetup(() => usePolling(fn, 5000));
		expect(fn).toHaveBeenCalledTimes(1);
	});

	it("calls fn repeatedly at the configured interval", () => {
		const fn = vi.fn();
		withSetup(() => usePolling(fn, 5000));
		expect(fn).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(5000);
		expect(fn).toHaveBeenCalledTimes(2);
		vi.advanceTimersByTime(5000);
		expect(fn).toHaveBeenCalledTimes(3);
	});

	it("stops polling when stop is called", () => {
		const fn = vi.fn();
		const { result } = withSetup(() => usePolling(fn, 5000));
		result.stop();
		vi.advanceTimersByTime(15000);
		// Only the initial call on mount
		expect(fn).toHaveBeenCalledTimes(1);
	});

	it("cleans up interval when app unmounts", () => {
		const fn = vi.fn();
		const { app } = withSetup(() => usePolling(fn, 5000));
		app.unmount();
		vi.advanceTimersByTime(15000);
		expect(fn).toHaveBeenCalledTimes(1);
	});

	it("uses 10s default interval", () => {
		const fn = vi.fn();
		withSetup(() => usePolling(fn));
		vi.advanceTimersByTime(10000);
		expect(fn).toHaveBeenCalledTimes(2);
		vi.advanceTimersByTime(5000);
		expect(fn).toHaveBeenCalledTimes(2);
	});
});
