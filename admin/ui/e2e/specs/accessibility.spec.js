import AxeBuilder from '@axe-core/playwright';
import { test, expect } from '../helpers/fixtures.js';

const axeTags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

test.describe('Accessibility — authenticated pages', () => {
  test('dashboard', async ({ page }) => {
    await page.goto('/');
    // Accept whatever state the dashboard is in (welcome screen or instances)
    await expect(
      page.getByTestId('dashboard-title'),
    ).toBeVisible({ timeout: 10_000 });

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });

  test('dashboard — table view', async ({ page, authedAPI }) => {
    // Ensure at least one instance exists for table view
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    if (instances.length === 0) {
      await authedAPI.post('/api/instances', {
        data: { name: 'a11y-proxy-1', address: '127.0.0.1:19091' },
      });
    }

    await page.goto('/');
    await expect(page.getByTestId('instance-card').first()).toBeVisible({ timeout: 15_000 });

    await page.getByTestId('view-toggle-table').click();
    await expect(page.getByTestId('instance-table')).toBeVisible();

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });

  test('instance detail — overview tab', async ({ page, authedAPI }) => {
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    const inst = instances[0];

    await page.goto(`/instances/${inst.id}`);
    await expect(page.getByTestId('overview-tab')).toBeVisible({ timeout: 15_000 });

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });

  test('instance detail — traffic tab', async ({ page, authedAPI }) => {
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    const inst = instances[0];

    await page.goto(`/instances/${inst.id}`);
    await page.getByTestId('tab-traffic').click();
    await expect(page.getByTestId('traffic-tab')).toBeVisible({ timeout: 15_000 });

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });

  test('audit log', async ({ page }) => {
    await page.goto('/audit-log');
    await expect(page.getByTestId('audit-table')).toBeVisible({ timeout: 10_000 });

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });

  test('settings page', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByTestId('settings-submit')).toBeVisible();

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });

  test('add instance modal', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('dashboard-title')).toBeVisible({ timeout: 10_000 });

    const addBtn = page.getByTestId('add-instance-btn').or(
      page.getByTestId('add-first-instance'),
    );
    await addBtn.first().click();
    await expect(page.getByTestId('instance-name')).toBeVisible();

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });
});

test.describe('Accessibility — login page', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('login page', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByTestId('login-submit')).toBeVisible();

    const results = await new AxeBuilder({ page })
      .withTags(axeTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });
});
