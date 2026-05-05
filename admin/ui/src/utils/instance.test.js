import { describe, it, expect, vi, afterEach } from 'vitest';
import { formatTime, getStatusLabel } from './instance.js';

describe('formatTime', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('returns empty string for null/undefined', () => {
		expect(formatTime(null)).toBe('');
		expect(formatTime(undefined)).toBe('');
	});

	it('returns "just now" for timestamps under 60 seconds ago', () => {
		const ts = new Date(Date.now() - 5000).toISOString();
		expect(formatTime(ts)).toBe('just now');
	});

	it('returns minutes ago for timestamps under 1 hour', () => {
		const ts = new Date(Date.now() - 5 * 60 * 1000).toISOString();
		expect(formatTime(ts)).toBe('5m ago');
	});

	it('returns hours ago for timestamps under 24 hours', () => {
		const ts = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
		expect(formatTime(ts)).toBe('3h ago');
	});

	it('returns locale date string for timestamps over 24 hours', () => {
		const d = new Date(Date.now() - 48 * 3600 * 1000);
		expect(formatTime(d.toISOString())).toBe(d.toLocaleDateString());
	});

	it('floors minutes correctly', () => {
		const ts = new Date(Date.now() - 90_000).toISOString();
		expect(formatTime(ts)).toBe('1m ago');
	});

	it('floors hours correctly', () => {
		const ts = new Date(Date.now() - 5400 * 1000).toISOString();
		expect(formatTime(ts)).toBe('1h ago');
	});
});

describe('getStatusLabel', () => {
	it('returns correct label for each status', () => {
		expect(getStatusLabel('healthy')).toBe('Healthy');
		expect(getStatusLabel('unreachable')).toBe('Unreachable');
		expect(getStatusLabel('unknown')).toBe('Unknown');
	});

	it('falls back to "Unknown" for unrecognized status', () => {
		expect(getStatusLabel('bogus')).toBe('Unknown');
		expect(getStatusLabel(undefined)).toBe('Unknown');
	});
});
