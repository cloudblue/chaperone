class ApiError extends Error {
	constructor(message, status, code) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
	}
}

async function request(path, options = {}) {
	const res = await fetch(path, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options.headers,
		},
	});

	if (!res.ok) {
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
