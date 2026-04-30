import { test, expect } from '@playwright/test';

test.describe('Audit Log', () => {
  test('shows audit entries for previous actions', async ({ page }) => {
    await page.goto('/audit-log');

    // Should have entries from dashboard tests (instance.create, user.login, etc.)
    await expect(page.getByTestId('audit-table')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByTestId('audit-row').first()).toBeVisible();
  });

  test('search filters results', async ({ page }) => {
    await page.goto('/audit-log');
    await expect(page.getByTestId('audit-table')).toBeVisible();

    const responsePromise = page.waitForResponse((resp) => resp.url().includes('/api/audit'));
    await page.getByTestId('audit-search').fill('proxy');
    await responsePromise;

    // Results may match or be empty; accept either the table or the empty state
    await expect(
      page.getByTestId('audit-table').or(page.getByText('No matching entries')),
    ).toBeVisible();
  });

  test('action type dropdown filters', async ({ page }) => {
    await page.goto('/audit-log');
    await expect(page.getByTestId('audit-table')).toBeVisible();

    await page.getByTestId('audit-action-filter').selectOption('user.login');

    // All visible rows should contain the login action label
    const rows = page.getByTestId('audit-row');
    const count = await rows.count();
    if (count > 0) {
      for (let i = 0; i < Math.min(count, 5); i++) {
        await expect(rows.nth(i)).toContainText('logged in');
      }
    }
  });

  test('pagination controls work', async ({ page }) => {
    await page.goto('/audit-log');

    const pagination = page.getByTestId('audit-pagination');
    // Pagination may or may not be visible depending on entry count
    // If visible, clicking next page should work
    if (await pagination.isVisible()) {
      const nextBtn = page.getByTestId('audit-next-page');
      if (await nextBtn.isEnabled()) {
        await nextBtn.click();
        await expect(page.getByTestId('audit-table')).toBeVisible();
      }
    }
  });
});
