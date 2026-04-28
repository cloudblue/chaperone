import { test, expect } from '@playwright/test';

test('authenticated user sees fleet dashboard', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('dashboard-title')).toBeVisible();
  await expect(page.getByTestId('dashboard-title')).toHaveText('Fleet Dashboard');
});
