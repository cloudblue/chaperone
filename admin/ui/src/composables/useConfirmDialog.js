import { ref } from 'vue';

export function useConfirmDialog() {
	const pending = ref(null);

	function requestConfirm(item) {
		pending.value = item;
	}

	async function confirm(action) {
		const item = pending.value;
		pending.value = null;
		if (item && action) await action(item);
	}

	function cancel() {
		pending.value = null;
	}

	return { pending, requestConfirm, confirm, cancel };
}
