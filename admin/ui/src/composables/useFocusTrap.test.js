import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { ref } from 'vue';
import { withSetup } from '../utils/test-utils.js';
import { useFocusTrap } from './useFocusTrap.js';

describe('useFocusTrap', () => {
	let container;
	let btn1;
	let btn2;
	let btn3;
	let app;

	beforeEach(() => {
		container = document.createElement('div');
		btn1 = document.createElement('button');
		btn1.textContent = 'First';
		btn2 = document.createElement('button');
		btn2.textContent = 'Second';
		btn3 = document.createElement('button');
		btn3.textContent = 'Third';
		container.append(btn1, btn2, btn3);
		document.body.appendChild(container);
	});

	afterEach(() => {
		app?.unmount();
		container.remove();
	});

	it('wraps focus from last to first on Tab', () => {
		const containerRef = ref(container);
		({ app } = withSetup(() => useFocusTrap(containerRef)));

		btn3.focus();
		const event = new KeyboardEvent('keydown', {
			key: 'Tab',
			bubbles: true,
		});
		let prevented = false;
		event.preventDefault = () => {
			prevented = true;
		};
		document.dispatchEvent(event);

		expect(prevented).toBe(true);
	});

	it('wraps focus from first to last on Shift+Tab', () => {
		const containerRef = ref(container);
		({ app } = withSetup(() => useFocusTrap(containerRef)));

		btn1.focus();
		const event = new KeyboardEvent('keydown', {
			key: 'Tab',
			shiftKey: true,
			bubbles: true,
		});
		let prevented = false;
		event.preventDefault = () => {
			prevented = true;
		};
		document.dispatchEvent(event);

		expect(prevented).toBe(true);
	});

	it('does not trap non-Tab keys', () => {
		const containerRef = ref(container);
		({ app } = withSetup(() => useFocusTrap(containerRef)));

		btn1.focus();
		const event = new KeyboardEvent('keydown', {
			key: 'Escape',
			bubbles: true,
		});
		let prevented = false;
		event.preventDefault = () => {
			prevented = true;
		};
		document.dispatchEvent(event);

		expect(prevented).toBe(false);
	});

	it('does not interfere when focus is in the middle', () => {
		const containerRef = ref(container);
		({ app } = withSetup(() => useFocusTrap(containerRef)));

		btn2.focus();
		const event = new KeyboardEvent('keydown', {
			key: 'Tab',
			bubbles: true,
		});
		let prevented = false;
		event.preventDefault = () => {
			prevented = true;
		};
		document.dispatchEvent(event);

		expect(prevented).toBe(false);
	});
});
