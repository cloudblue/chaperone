import { test as setup, expect } from '@playwright/test';
import path from 'node:path';
import { TEST_USER, TEST_PASSWORD } from './helpers/constants.js';

const authFile = path.join(import.meta.dirname, '.auth', 'user.json');

setup('authenticate', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('login-username').fill(TEST_USER);
  await page.getByTestId('login-password').fill(TEST_PASSWORD);
  await page.getByTestId('login-submit').click();

  // Wait until redirected to dashboard
  await expect(page.getByTestId('dashboard-title')).toBeVisible();

  await page.context().storageState({ path: authFile });
});
