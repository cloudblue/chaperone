import { test, expect } from '@playwright/test';
import { TEST_USER, TEST_PASSWORD } from '../helpers/constants.js';

test.describe('Authentication', () => {
  test('login with valid credentials redirects to dashboard', async ({ page }) => {
    await page.goto('/login');
    await page.getByTestId('login-username').fill(TEST_USER);
    await page.getByTestId('login-password').fill(TEST_PASSWORD);
    await page.getByTestId('login-submit').click();

    await expect(page.getByTestId('dashboard-title')).toBeVisible();
    await expect(page.getByTestId('sidebar-username')).toHaveText(TEST_USER);
  });

  test('login with invalid credentials shows error', async ({ page }) => {
    await page.goto('/login');
    await page.getByTestId('login-username').fill(TEST_USER);
    await page.getByTestId('login-password').fill('wrongpassword1');
    await page.getByTestId('login-submit').click();

    await expect(page.getByTestId('login-error')).toBeVisible();
    await expect(page.getByTestId('login-error')).toContainText('Invalid username or password');
    await expect(page).toHaveURL(/\/login/);
  });

  test('unauthenticated user is redirected to login', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveURL(/\/login/);
  });

  test('redirect back after login', async ({ page }) => {
    await page.goto('/audit-log');
    await expect(page).toHaveURL(/\/login\?redirect=/);

    await page.getByTestId('login-username').fill(TEST_USER);
    await page.getByTestId('login-password').fill(TEST_PASSWORD);
    await page.getByTestId('login-submit').click();

    await expect(page).toHaveURL(/\/audit-log/);
  });

  test('logout redirects to login', async ({ page }) => {
    // First login
    await page.goto('/login');
    await page.getByTestId('login-username').fill(TEST_USER);
    await page.getByTestId('login-password').fill(TEST_PASSWORD);
    await page.getByTestId('login-submit').click();
    await expect(page.getByTestId('dashboard-title')).toBeVisible();

    // Then logout
    await page.getByTestId('sidebar-logout').click();
    await expect(page).toHaveURL(/\/login/);

    // Session is invalidated — going back to / should redirect to login
    await page.goto('/');
    await expect(page).toHaveURL(/\/login/);
  });
});
