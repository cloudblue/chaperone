import { describe, it, expect } from 'vitest';
import { validateInstanceForm, validatePasswordChange } from './validation.js';

describe('validateInstanceForm', () => {
	it('returns no errors for valid inputs', () => {
		const errors = validateInstanceForm('proxy-1', '10.0.0.1:9090');
		expect(errors.name).toBe('');
		expect(errors.address).toBe('');
	});

	it('returns name error when name is empty', () => {
		const errors = validateInstanceForm('', '10.0.0.1:9090');
		expect(errors.name).toBe('Name is required');
		expect(errors.address).toBe('');
	});

	it('returns address error when address is empty', () => {
		const errors = validateInstanceForm('proxy-1', '');
		expect(errors.address).toBe('Address is required');
		expect(errors.name).toBe('');
	});

	it('returns both errors when both are empty', () => {
		const errors = validateInstanceForm('', '');
		expect(errors.name).toBe('Name is required');
		expect(errors.address).toBe('Address is required');
	});

	it('treats whitespace-only as empty', () => {
		const errors = validateInstanceForm('   ', '  \t ');
		expect(errors.name).toBe('Name is required');
		expect(errors.address).toBe('Address is required');
	});

	it('accepts values with leading/trailing whitespace', () => {
		const errors = validateInstanceForm('  proxy-1  ', '  10.0.0.1:9090  ');
		expect(errors.name).toBe('');
		expect(errors.address).toBe('');
	});
});

describe('validatePasswordChange', () => {
	it('requires all fields', () => {
		const errors = validatePasswordChange('', '', '');
		expect(errors.currentPassword).toBe('Current password is required');
		expect(errors.newPassword).toBe('New password is required');
		expect(errors.confirmPassword).toBe('Please confirm your new password');
	});

	it('rejects passwords shorter than 12 characters', () => {
		const errors = validatePasswordChange('currentpass1', 'short', 'short');
		expect(errors.newPassword).toBe('Password must be at least 12 characters');
	});

	it('rejects passwords longer than 72 characters', () => {
		const long = 'a'.repeat(73);
		const errors = validatePasswordChange('currentpass1', long, long);
		expect(errors.newPassword).toBe('Password must be at most 72 characters');
	});

	it('rejects mismatched passwords', () => {
		const errors = validatePasswordChange(
			'currentpass1',
			'validpassword1',
			'differentpass1',
		);
		expect(errors.confirmPassword).toBe('Passwords do not match');
	});

	it('returns empty object for valid input', () => {
		const errors = validatePasswordChange(
			'currentpass1',
			'newpassword12',
			'newpassword12',
		);
		expect(Object.keys(errors)).toHaveLength(0);
	});

	it('accepts exactly 12 character password', () => {
		const errors = validatePasswordChange(
			'currentpass1',
			'exactly12chr',
			'exactly12chr',
		);
		expect(Object.keys(errors)).toHaveLength(0);
	});

	it('accepts exactly 72 character password', () => {
		const pw = 'a'.repeat(72);
		const errors = validatePasswordChange('currentpass1', pw, pw);
		expect(Object.keys(errors)).toHaveLength(0);
	});
});
