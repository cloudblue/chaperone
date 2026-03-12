import { createApp } from 'vue';

export function withSetup(composable) {
	let result;
	const app = createApp({
		setup() {
			result = composable();
			return () => {};
		},
	});
	app.mount(document.createElement('div'));
	return { result, app };
}
