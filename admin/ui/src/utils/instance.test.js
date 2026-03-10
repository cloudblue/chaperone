import { describe, it, expect, vi, afterEach } from "vitest";
import { STALE_THRESHOLD_MS, isInstanceStale, formatTime } from "./instance.js";

describe("STALE_THRESHOLD_MS", () => {
	it("is 2 minutes in milliseconds", () => {
		expect(STALE_THRESHOLD_MS).toBe(120_000);
	});
});

describe("isInstanceStale", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("returns false for non-healthy instance", () => {
		expect(isInstanceStale({ status: "unreachable", last_seen_at: "2020-01-01T00:00:00Z" })).toBe(false);
		expect(isInstanceStale({ status: "unknown", last_seen_at: "2020-01-01T00:00:00Z" })).toBe(false);
	});

	it("returns false when last_seen_at is null", () => {
		expect(isInstanceStale({ status: "healthy", last_seen_at: null })).toBe(false);
	});

	it("returns false when last seen recently", () => {
		const recent = new Date(Date.now() - 30_000).toISOString(); // 30s ago
		expect(isInstanceStale({ status: "healthy", last_seen_at: recent })).toBe(false);
	});

	it("returns true when last seen over 2 minutes ago", () => {
		const old = new Date(Date.now() - STALE_THRESHOLD_MS - 1000).toISOString();
		expect(isInstanceStale({ status: "healthy", last_seen_at: old })).toBe(true);
	});

	it("returns false at exactly the threshold boundary", () => {
		vi.spyOn(Date, "now").mockReturnValue(1000000);
		const atBoundary = new Date(1000000 - STALE_THRESHOLD_MS).toISOString();
		expect(isInstanceStale({ status: "healthy", last_seen_at: atBoundary })).toBe(false);
	});
});

describe("formatTime", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("returns empty string for null/undefined", () => {
		expect(formatTime(null)).toBe("");
		expect(formatTime(undefined)).toBe("");
	});

	it('returns "just now" for timestamps under 60 seconds ago', () => {
		const ts = new Date(Date.now() - 5000).toISOString();
		expect(formatTime(ts)).toBe("just now");
	});

	it("returns minutes ago for timestamps under 1 hour", () => {
		const ts = new Date(Date.now() - 5 * 60 * 1000).toISOString();
		expect(formatTime(ts)).toBe("5m ago");
	});

	it("returns hours ago for timestamps under 24 hours", () => {
		const ts = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
		expect(formatTime(ts)).toBe("3h ago");
	});

	it("returns locale date string for timestamps over 24 hours", () => {
		const d = new Date(Date.now() - 48 * 3600 * 1000);
		expect(formatTime(d.toISOString())).toBe(d.toLocaleDateString());
	});

	it("floors minutes correctly", () => {
		// 90 seconds = 1.5 minutes → should show "1m ago"
		const ts = new Date(Date.now() - 90_000).toISOString();
		expect(formatTime(ts)).toBe("1m ago");
	});

	it("floors hours correctly", () => {
		// 5400 seconds = 1.5 hours → should show "1h ago"
		const ts = new Date(Date.now() - 5400 * 1000).toISOString();
		expect(formatTime(ts)).toBe("1h ago");
	});
});
