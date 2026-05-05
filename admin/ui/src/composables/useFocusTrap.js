import { onMounted, onUnmounted } from 'vue';

const FOCUSABLE =
	'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';

export function useFocusTrap(containerRef) {
	const previouslyFocused = document.activeElement;

	function handleKeydown(e) {
		if (e.key !== 'Tab') return;

		const container = containerRef.value?.$el ?? containerRef.value;
		if (!container) return;

		const focusable = [...container.querySelectorAll(FOCUSABLE)];
		if (focusable.length === 0) return;

		const first = focusable[0];
		const last = focusable[focusable.length - 1];

		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	onMounted(() => {
		document.addEventListener('keydown', handleKeydown);
	});

	onUnmounted(() => {
		document.removeEventListener('keydown', handleKeydown);
		if (previouslyFocused && document.contains(previouslyFocused)) {
			previouslyFocused.focus();
		}
	});
}
