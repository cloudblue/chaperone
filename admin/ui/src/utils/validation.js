export function validateInstanceForm(name, address) {
	return {
		name: name.trim() ? '' : 'Name is required',
		address: address.trim() ? '' : 'Address is required',
	};
}
