import { describe, it, expect, vi } from 'vitest';
import { withSetup } from '../utils/test-utils.js';
import { useConfirmDialog } from './useConfirmDialog.js';

describe('useConfirmDialog', () => {
	it('starts with null pending', () => {
		const { result } = withSetup(() => useConfirmDialog());
		expect(result.pending.value).toBeNull();
	});

	it('requestConfirm sets pending to the item', () => {
		const { result } = withSetup(() => useConfirmDialog());
		const item = { id: 1, name: 'proxy-1' };
		result.requestConfirm(item);
		expect(result.pending.value).toStrictEqual(item);
	});

	it('confirm calls action with the pending item and clears it', async () => {
		const { result } = withSetup(() => useConfirmDialog());
		const item = { id: 1 };
		const action = vi.fn();
		result.requestConfirm(item);
		await result.confirm(action);
		expect(action).toHaveBeenCalledWith(item);
		expect(result.pending.value).toBeNull();
	});

	it('confirm clears pending even without an action', async () => {
		const { result } = withSetup(() => useConfirmDialog());
		result.requestConfirm({ id: 1 });
		await result.confirm();
		expect(result.pending.value).toBeNull();
	});

	it('cancel clears pending without calling any action', () => {
		const { result } = withSetup(() => useConfirmDialog());
		result.requestConfirm({ id: 1 });
		result.cancel();
		expect(result.pending.value).toBeNull();
	});
});
