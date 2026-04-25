import { test, expect } from '@playwright/test';
import { PW_CHANGE_USER, TEST_PASSWORD } from '../helpers/constants.js';

// Use a separate browser context (no saved state) for the password-change user
test.use({ storageState: { cookies: [], origins: [] } });

test.describe('Settings — Password Change', () => {
  test.beforeEach(async ({ page }) => {
    // Login as the dedicated password-change test user
    await page.goto('/login');
    await page.getByTestId('login-username').fill(PW_CHANGE_USER);
    await page.getByTestId('login-password').fill(TEST_PASSWORD);
    await page.getByTestId('login-submit').click();
    await expect(page.getByTestId('dashboard-title')).toBeVisible();
  });

  test('change password successfully', async ({ page }) => {
    await page.goto('/settings');

    await page.getByTestId('settings-current-password').fill(TEST_PASSWORD);
    await page.getByTestId('settings-new-password').fill('newpassword1234');
    await page.getByTestId('settings-confirm-password').fill('newpassword1234');
    await page.getByTestId('settings-submit').click();

    await expect(page.getByTestId('settings-success')).toBeVisible();
    await expect(page.getByTestId('settings-success')).toContainText('Password changed');

    // Change it back so tests remain idempotent
    await page.getByTestId('settings-current-password').fill('newpassword1234');
    await page.getByTestId('settings-new-password').fill(TEST_PASSWORD);
    await page.getByTestId('settings-confirm-password').fill(TEST_PASSWORD);
    await page.getByTestId('settings-submit').click();
    await expect(page.getByTestId('settings-success')).toBeVisible();
  });

  test('wrong current password shows error', async ({ page }) => {
    await page.goto('/settings');

    await page.getByTestId('settings-current-password').fill('wrongpassword1');
    await page.getByTestId('settings-new-password').fill('newpassword1234');
    await page.getByTestId('settings-confirm-password').fill('newpassword1234');
    await page.getByTestId('settings-submit').click();

    // Backend returns 403 (not 401) for wrong current password, so the
    // global 401 interceptor does NOT trigger — user stays on settings page.
    await expect(page.getByTestId('settings-error')).toBeVisible();
    await expect(page.getByTestId('settings-error')).toContainText('Current password is incorrect');
  });

  test('password too short shows validation error', async ({ page }) => {
    await page.goto('/settings');

    await page.getByTestId('settings-current-password').fill(TEST_PASSWORD);
    await page.getByTestId('settings-new-password').fill('short');
    await page.getByTestId('settings-confirm-password').fill('short');
    await page.getByTestId('settings-submit').click();

    // Should show client-side validation error (not server error)
    await expect(page.getByTestId('settings-new-password-error')).toBeVisible();
  });
});
