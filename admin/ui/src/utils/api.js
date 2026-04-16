class ApiError extends Error {
	constructor(message, status, code) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
	}
}

export function getCsrfToken() {
	const match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]*)/);
	return match ? decodeURIComponent(match[1]) : '';
}

const writeMethods = new Set(['POST', 'PUT', 'DELETE', 'PATCH']);

async function request(path, options = {}) {
	const headers = {
		'Content-Type': 'application/json',
		...options.headers,
	};

	if (writeMethods.has(options.method)) {
		const token = getCsrfToken();
		if (token) headers['X-CSRF-Token'] = token;
	}

	const res = await fetch(path, { ...options, headers });

	if (!res.ok) {
		if (res.status === 401 && path !== '/api/login') {
			const { useAuthStore } = await import('../stores/auth.js');
			const auth = useAuthStore();
			if (auth.ready) {
				auth.user = null;
				window.location.href = '/login';
			}
		}

		let message = `Request failed (${res.status})`;
		let code;
		try {
			const data = await res.json();
			if (data.error?.message) message = data.error.message;
			if (data.error?.code) code = data.error.code;
		} catch {
			// response body not JSON — keep generic message
		}
		throw new ApiError(message, res.status, code);
	}

	if (res.status === 204) return null;
	return res.json();
}

export function get(path) {
	return request(path);
}

export function post(path, body) {
	return request(path, { method: 'POST', body: JSON.stringify(body) });
}

export function put(path, body) {
	return request(path, { method: 'PUT', body: JSON.stringify(body) });
}

export function del(path) {
	return request(path, { method: 'DELETE' });
}

export { ApiError };
