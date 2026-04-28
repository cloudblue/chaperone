import { test as base, expect } from '@playwright/test';
import { TEST_USER, TEST_PASSWORD } from './constants.js';

/**
 * Custom fixtures for E2E tests.
 * Provides an authenticated API context with CSRF handling for seeding data.
 */
export const test = base.extend({
  /**
   * An authenticated API request context with CSRF support.
   * Use for seeding instances via the REST API in beforeAll hooks.
   */
  authedAPI: async ({ playwright }, use) => {
    const ctx = await playwright.request.newContext({
      baseURL: 'http://127.0.0.1:8080',
    });

    // Login to get session + CSRF cookies
    const loginRes = await ctx.post('/api/login', {
      data: { username: TEST_USER, password: TEST_PASSWORD },
    });
    expect(loginRes.ok()).toBeTruthy();

    // Extract CSRF token from cookies
    const cookies = await ctx.storageState();
    const csrfCookie = cookies.cookies.find((c) => c.name === 'csrf_token');
    if (!csrfCookie) throw new Error('expected csrf_token cookie after login');
    const csrfToken = csrfCookie.value;

    // Wrap context to auto-include CSRF header on writes
    const originalPost = ctx.post.bind(ctx);
    const originalPut = ctx.put.bind(ctx);
    const originalDelete = ctx.delete.bind(ctx);

    ctx.post = (url, options = {}) =>
      originalPost(url, {
        ...options,
        headers: { ...options.headers, 'X-CSRF-Token': csrfToken },
      });
    ctx.put = (url, options = {}) =>
      originalPut(url, {
        ...options,
        headers: { ...options.headers, 'X-CSRF-Token': csrfToken },
      });
    ctx.delete = (url, options = {}) =>
      originalDelete(url, {
        ...options,
        headers: { ...options.headers, 'X-CSRF-Token': csrfToken },
      });

    await use(ctx);
    await ctx.dispose();
  },
});

export { expect };
