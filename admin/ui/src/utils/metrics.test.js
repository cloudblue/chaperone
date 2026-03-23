import { describe, it, expect } from 'vitest';
import {
	formatRps,
	formatLatency,
	formatErrorRate,
	formatCount,
	trendDirection,
	assignVendorColors,
	VENDOR_COLORS,
} from './metrics.js';

describe('formatRps', () => {
	it('returns dash for null/undefined', () => {
		expect(formatRps(null)).toBe('-');
		expect(formatRps(undefined)).toBe('-');
	});

	it('formats large values with k suffix', () => {
		expect(formatRps(10000)).toBe('10.0k');
		expect(formatRps(15432)).toBe('15.4k');
	});

	it('formats thousands with two decimal k', () => {
		expect(formatRps(1000)).toBe('1.00k');
		expect(formatRps(1234)).toBe('1.23k');
	});

	it('formats hundreds as integers', () => {
		expect(formatRps(100)).toBe('100');
		expect(formatRps(456)).toBe('456');
	});

	it('formats tens with one decimal', () => {
		expect(formatRps(10)).toBe('10.0');
		expect(formatRps(42.7)).toBe('42.7');
	});

	it('formats small values with two decimals', () => {
		expect(formatRps(0.5)).toBe('0.50');
		expect(formatRps(9.99)).toBe('9.99');
	});
});

describe('formatLatency', () => {
	it('returns dash for null/undefined', () => {
		expect(formatLatency(null)).toBe('-');
		expect(formatLatency(undefined)).toBe('-');
	});

	it('formats large values as seconds', () => {
		expect(formatLatency(10000)).toBe('10.0s');
		expect(formatLatency(1500)).toBe('1.50s');
	});

	it('formats hundreds as integer ms', () => {
		expect(formatLatency(245)).toBe('245ms');
	});

	it('formats tens with one decimal ms', () => {
		expect(formatLatency(42.7)).toBe('42.7ms');
	});

	it('formats small values with two decimal ms', () => {
		expect(formatLatency(3.14)).toBe('3.14ms');
	});
});

describe('formatErrorRate', () => {
	it('returns dash for null/undefined', () => {
		expect(formatErrorRate(null)).toBe('-');
		expect(formatErrorRate(undefined)).toBe('-');
	});

	it('formats zero as 0%', () => {
		expect(formatErrorRate(0)).toBe('0%');
	});

	it('formats high rates with one decimal', () => {
		expect(formatErrorRate(0.15)).toBe('15.0%');
	});

	it('formats moderate rates with two decimals', () => {
		expect(formatErrorRate(0.023)).toBe('2.30%');
	});

	it('formats small rates with three decimals', () => {
		expect(formatErrorRate(0.0005)).toBe('0.050%');
	});
});

describe('formatCount', () => {
	it('returns dash for null/undefined', () => {
		expect(formatCount(null)).toBe('-');
		expect(formatCount(undefined)).toBe('-');
	});

	it('rounds and formats with locale separators', () => {
		expect(formatCount(42)).toBe('42');
		expect(formatCount(3.7)).toBe('4');
	});
});

describe('trendDirection', () => {
	it('returns null for null/undefined', () => {
		expect(trendDirection(null)).toBe(null);
		expect(trendDirection(undefined)).toBe(null);
	});

	it('returns up for positive values', () => {
		expect(trendDirection(5.2)).toBe('up');
	});

	it('returns down for negative values', () => {
		expect(trendDirection(-3.1)).toBe('down');
	});

	it('returns flat for zero', () => {
		expect(trendDirection(0)).toBe('flat');
	});
});

describe('assignVendorColors', () => {
	it('assigns unique colors to vendors', () => {
		const map = assignVendorColors(['a', 'b', 'c']);
		expect(map.a).toBe(VENDOR_COLORS[0]);
		expect(map.b).toBe(VENDOR_COLORS[1]);
		expect(map.c).toBe(VENDOR_COLORS[2]);
	});

	it('wraps around when more vendors than colors', () => {
		const ids = Array.from(
			{ length: VENDOR_COLORS.length + 1 },
			(_, i) => `v${i}`,
		);
		const map = assignVendorColors(ids);
		expect(map[`v${VENDOR_COLORS.length}`]).toBe(VENDOR_COLORS[0]);
	});

	it('returns empty map for empty input', () => {
		expect(assignVendorColors([])).toEqual({});
	});
});
