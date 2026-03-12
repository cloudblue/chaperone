import { ref, reactive } from 'vue';
import { validateInstanceForm } from '../utils/validation.js';

export function useInstanceForm(store, instance = null) {
	const editing = !!instance;
	const name = ref(instance?.name || '');
	const address = ref(instance?.address || '');
	const errors = reactive({ name: '', address: '' });
	const saving = ref(false);
	const testing = ref(false);
	const testResult = ref(null);

	function validate() {
		const result = validateInstanceForm(name.value, address.value);
		errors.name = result.name;
		errors.address = result.address;
		return !result.name && !result.address;
	}

	async function handleTest() {
		testResult.value = null;
		testing.value = true;
		try {
			testResult.value = await store.testConnection(address.value.trim());
		} catch {
			testResult.value = { ok: false, error: 'Failed to test connection' };
		} finally {
			testing.value = false;
		}
	}

	async function handleSubmit() {
		if (!validate()) return false;
		saving.value = true;
		try {
			if (editing) {
				await store.updateInstance(
					instance.id,
					name.value.trim(),
					address.value.trim(),
				);
			} else {
				await store.createInstance(name.value.trim(), address.value.trim());
			}
			return true;
		} catch (e) {
			errors.address = e.message;
			return false;
		} finally {
			saving.value = false;
		}
	}

	return {
		editing,
		name,
		address,
		errors,
		saving,
		testing,
		testResult,
		validate,
		handleTest,
		handleSubmit,
	};
}
