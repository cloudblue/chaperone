export function validateInstanceForm(name, address) {
	return {
		name: name.trim() ? '' : 'Name is required',
		address: address.trim() ? '' : 'Address is required',
	};
}

const MIN_PASSWORD_LENGTH = 12;
const MAX_PASSWORD_LENGTH = 72;

export function validatePasswordChange(
	currentPassword,
	newPassword,
	confirmPassword,
) {
	const errors = {};
	if (!currentPassword) errors.currentPassword = 'Current password is required';
	if (!newPassword) {
		errors.newPassword = 'New password is required';
	} else if (newPassword.length < MIN_PASSWORD_LENGTH) {
		errors.newPassword = `Password must be at least ${MIN_PASSWORD_LENGTH} characters`;
	} else if (newPassword.length > MAX_PASSWORD_LENGTH) {
		errors.newPassword = `Password must be at most ${MAX_PASSWORD_LENGTH} characters`;
	}
	if (!confirmPassword) {
		errors.confirmPassword = 'Please confirm your new password';
	} else if (newPassword && confirmPassword !== newPassword) {
		errors.confirmPassword = 'Passwords do not match';
	}
	return errors;
}
