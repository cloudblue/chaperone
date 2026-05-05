import { test, expect } from '../helpers/fixtures.js';

// Tests in this suite are intentionally ordered and sequentially dependent.
// Each test builds on state created by earlier tests (add → verify health →
// seed more → toggle view → edit → delete → navigate). This mirrors the
// real CRUD flow. Requires: fullyParallel: false, workers: 1.
test.describe('Fleet Dashboard', () => {
  test('shows welcome screen when no instances registered', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('welcome-screen')).toBeVisible();
    await expect(page.getByTestId('add-first-instance')).toBeVisible();
  });

  test('add instance via modal', async ({ page }) => {
    await page.goto('/');

    // Open add modal (welcome screen button or header button)
    const addBtn = page.getByTestId('add-first-instance').or(
      page.getByTestId('add-instance-btn'),
    );
    await addBtn.first().click();

    // Fill form
    await page.getByTestId('instance-name').fill('proxy-us-east-1');
    await page.getByTestId('instance-address').fill('127.0.0.1:19091');

    // Test connection
    await page.getByTestId('test-connection').click();
    await expect(page.getByTestId('test-result')).toContainText('Connected successfully');

    // Save
    await page.getByTestId('save-instance').click();

    // Card should appear
    await expect(page.getByTestId('instance-card')).toBeVisible();
  });

  test('instance becomes healthy after polling', async ({ page }) => {
    await page.goto('/');

    // Wait for status to show healthy (after scrape cycle)
    await expect(
      page.getByTestId('instance-card').getByTestId('status-healthy'),
    ).toBeVisible({ timeout: 15_000 });
  });

  test('add multiple instances shows KPI panel', async ({ page, authedAPI }) => {
    // Seed second and third instances via API
    await authedAPI.post('/api/instances', {
      data: { name: 'proxy-eu-west-1', address: '127.0.0.1:19092' },
    });
    await authedAPI.post('/api/instances', {
      data: { name: 'proxy-ap-south-1', address: '127.0.0.1:19093' },
    });

    await page.goto('/');
    await expect(page.getByTestId('kpi-panel')).toBeVisible({ timeout: 15_000 });
  });

  test('view toggle switches between card and table', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('instance-card').first()).toBeVisible();

    // Switch to table
    await page.getByTestId('view-toggle-table').click();
    await expect(page.getByTestId('instance-table')).toBeVisible();

    // Switch back to cards
    await page.getByTestId('view-toggle-card').click();
    await expect(page.getByTestId('instance-card').first()).toBeVisible();
  });

  test('edit instance', async ({ page }) => {
    await page.goto('/');

    // Find a specific card by name and click its edit button
    const card = page.getByTestId('instance-card').filter({ hasText: 'proxy-us-east-1' });
    await card.getByTestId('instance-edit').click();

    // Modal should be pre-filled
    await expect(page.getByTestId('instance-name')).toHaveValue('proxy-us-east-1');

    // Change name
    await page.getByTestId('instance-name').fill('proxy-renamed');
    await page.getByTestId('save-instance').click();

    // Updated name should appear
    await expect(page.getByText('proxy-renamed')).toBeVisible();
  });

  test('delete instance with confirmation', async ({ page }) => {
    await page.goto('/');

    const cardCount = await page.getByTestId('instance-card').count();

    // Click remove on last card
    await page.getByTestId('instance-card').last().getByTestId('instance-delete').click();

    // Confirm dialog
    await expect(page.getByTestId('confirm-ok')).toBeVisible();
    await page.getByTestId('confirm-ok').click();

    // One fewer card
    await expect(page.getByTestId('instance-card')).toHaveCount(cardCount - 1);
  });

  test('click instance navigates to detail', async ({ page }) => {
    await page.goto('/');
    await page.getByTestId('instance-card').first().click();
    await expect(page).toHaveURL(/\/instances\/\d+/);
  });
});
