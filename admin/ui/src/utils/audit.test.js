import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
	getActionLabel,
	getActionOptions,
	formatAuditTimestamp,
	buildAuditQueryString,
	totalPages,
	startOfLocalDay,
	endOfLocalDay,
} from './audit.js';

describe('getActionLabel', () => {
	it('returns human-readable label for known actions', () => {
		expect(getActionLabel('instance.create')).toBe('Instance created');
		expect(getActionLabel('user.login')).toBe('User logged in');
		expect(getActionLabel('user.password_change')).toBe('Password changed');
	});

	it('returns raw action string for unknown actions', () => {
		expect(getActionLabel('some.unknown')).toBe('some.unknown');
	});
});

describe('getActionOptions', () => {
	it('returns array with "All actions" as first option', () => {
		const options = getActionOptions();
		expect(options[0]).toEqual({ value: '', label: 'All actions' });
		expect(options.length).toBeGreaterThan(1);
	});

	it('includes all known action types', () => {
		const options = getActionOptions();
		const values = options.map((o) => o.value);
		expect(values).toContain('instance.create');
		expect(values).toContain('instance.delete');
		expect(values).toContain('user.login');
		expect(values).toContain('user.logout');
	});
});

describe('formatAuditTimestamp', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-03-09T12:00:00Z'));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('returns empty string for falsy input', () => {
		expect(formatAuditTimestamp(null)).toBe('');
		expect(formatAuditTimestamp('')).toBe('');
		expect(formatAuditTimestamp(undefined)).toBe('');
	});

	it('returns empty string for invalid date', () => {
		expect(formatAuditTimestamp('not-a-date')).toBe('');
	});

	it('returns "just now" for timestamps less than 60s ago', () => {
		expect(formatAuditTimestamp('2026-03-09T11:59:30Z')).toBe('just now');
	});

	it('returns minutes ago for timestamps less than 1h ago', () => {
		expect(formatAuditTimestamp('2026-03-09T11:45:00Z')).toBe('15m ago');
	});

	it('returns "Today" with time for older timestamps today', () => {
		const result = formatAuditTimestamp('2026-03-09T08:30:00Z');
		expect(result).toMatch(/^Today /);
	});

	it('returns "Yesterday" with time for timestamps from yesterday', () => {
		const result = formatAuditTimestamp('2026-03-08T14:00:00Z');
		expect(result).toMatch(/^Yesterday /);
	});

	it('returns formatted date for older timestamps', () => {
		const result = formatAuditTimestamp('2026-03-01T10:00:00Z');
		expect(result).toBeTruthy();
		expect(result).not.toMatch(/^Today/);
		expect(result).not.toMatch(/^Yesterday/);
	});
});

describe('buildAuditQueryString', () => {
	it('returns empty string when no filters are set', () => {
		expect(buildAuditQueryString({})).toBe('');
	});

	it('includes search query', () => {
		expect(buildAuditQueryString({ q: 'test' })).toBe('?q=test');
	});

	it('includes action filter', () => {
		expect(buildAuditQueryString({ action: 'user.login' })).toBe(
			'?action=user.login',
		);
	});

	it('converts date-only from/to into RFC 3339 UTC via local timezone', () => {
		const qs = buildAuditQueryString({
			from: '2026-03-01',
			to: '2026-03-09',
		});
		expect(qs).toContain('from=');
		expect(qs).toContain('to=');
		// The serialized values should be valid ISO timestamps, not date-only
		const params = new URLSearchParams(qs.slice(1));
		expect(params.get('from')).toMatch(
			/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d{3}Z$/,
		);
		expect(params.get('to')).toMatch(
			/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d{3}Z$/,
		);
		// The UTC timestamps must represent local midnight and local end-of-day
		expect(new Date(params.get('from')).getTime()).toBe(
			new Date('2026-03-01T00:00:00').getTime(),
		);
		expect(new Date(params.get('to')).getTime()).toBe(
			new Date('2026-03-09T23:59:59').getTime(),
		);
	});

	it('includes page only when > 1', () => {
		expect(buildAuditQueryString({ page: 1 })).toBe('');
		expect(buildAuditQueryString({ page: 3 })).toBe('?page=3');
	});

	it('includes per_page only when not default', () => {
		expect(buildAuditQueryString({ perPage: 20 })).toBe('');
		expect(buildAuditQueryString({ perPage: 50 })).toBe('?per_page=50');
	});

	it('combines multiple filters', () => {
		const qs = buildAuditQueryString({
			q: 'proxy',
			action: 'instance.create',
			page: 2,
		});
		expect(qs).toContain('q=proxy');
		expect(qs).toContain('action=instance.create');
		expect(qs).toContain('page=2');
	});
});

describe('totalPages', () => {
	it('returns 1 for zero or negative total', () => {
		expect(totalPages(0, 20)).toBe(1);
		expect(totalPages(-5, 20)).toBe(1);
	});

	it('returns 1 for zero or negative perPage', () => {
		expect(totalPages(100, 0)).toBe(1);
		expect(totalPages(100, -1)).toBe(1);
	});

	it('computes correct page count', () => {
		expect(totalPages(20, 20)).toBe(1);
		expect(totalPages(21, 20)).toBe(2);
		expect(totalPages(100, 20)).toBe(5);
		expect(totalPages(1, 20)).toBe(1);
	});
});

describe('startOfLocalDay', () => {
	it('returns empty string for falsy input', () => {
		expect(startOfLocalDay('')).toBe('');
		expect(startOfLocalDay(null)).toBe('');
		expect(startOfLocalDay(undefined)).toBe('');
	});

	it('returns an ISO string based on local midnight', () => {
		const result = startOfLocalDay('2026-03-09');
		expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d{3}Z$/);
		// Parse back and verify it represents local midnight
		const d = new Date(result);
		const local = new Date('2026-03-09T00:00:00');
		expect(d.getTime()).toBe(local.getTime());
	});
});

describe('endOfLocalDay', () => {
	it('returns empty string for falsy input', () => {
		expect(endOfLocalDay('')).toBe('');
		expect(endOfLocalDay(null)).toBe('');
		expect(endOfLocalDay(undefined)).toBe('');
	});

	it('returns an ISO string based on local end of day', () => {
		const result = endOfLocalDay('2026-03-09');
		expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d{3}Z$/);
		const d = new Date(result);
		const local = new Date('2026-03-09T23:59:59');
		expect(d.getTime()).toBe(local.getTime());
	});
});
