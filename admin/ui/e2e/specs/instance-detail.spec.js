import { test, expect } from '../helpers/fixtures.js';

test.describe('Instance Detail', () => {
  // Seed instances before each test (idempotent — skips if already registered).
  test.beforeEach(async ({ authedAPI }) => {
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    if (!instances.find((i) => i.address === '127.0.0.1:19091')) {
      await authedAPI.post('/api/instances', {
        data: { name: 'proxy-us-east-1', address: '127.0.0.1:19091' },
      });
    }
    if (!instances.find((i) => i.address === '127.0.0.1:19092')) {
      await authedAPI.post('/api/instances', {
        data: { name: 'proxy-eu-west-1', address: '127.0.0.1:19092' },
      });
    }
    if (!instances.find((i) => i.address === '127.0.0.1:19093')) {
      await authedAPI.post('/api/instances', {
        data: { name: 'proxy-ap-south-1', address: '127.0.0.1:19093' },
      });
    }
  });

  test('overview tab shows metrics after polling', async ({ page, authedAPI }) => {
    // Get first instance ID
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    const inst = instances[0];

    await page.goto(`/instances/${inst.id}`);

    // Wait for metrics to appear (scraper polls every 3s, may need multiple cycles)
    await expect(page.getByTestId('overview-tab')).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId('kpi-rps')).toBeVisible({ timeout: 15_000 });
  });

  test('traffic tab shows vendor breakdown', async ({ page, authedAPI }) => {
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    const inst = instances[0];

    await page.goto(`/instances/${inst.id}`);

    // Switch to traffic tab
    await page.getByTestId('tab-traffic').click();
    await expect(page.getByTestId('traffic-tab')).toBeVisible();

    // Vendor names should appear
    await expect(page.getByText('acme-corp')).toBeVisible({ timeout: 15_000 });
  });

  test('tab keyboard navigation', async ({ page, authedAPI }) => {
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    const inst = instances[0];

    await page.goto(`/instances/${inst.id}`);

    // Focus overview tab and press arrow right
    await page.getByTestId('tab-overview').focus();
    await page.keyboard.press('ArrowRight');

    // Traffic tab should now be active
    await expect(page.getByTestId('tab-traffic')).toHaveAttribute('aria-selected', 'true');
  });

  test('breadcrumb Fleet link navigates back', async ({ page, authedAPI }) => {
    const res = await authedAPI.get('/api/instances');
    const instances = await res.json();
    const inst = instances[0];

    await page.goto(`/instances/${inst.id}`);
    await page.getByTestId('breadcrumb-fleet').click();
    await expect(page.getByTestId('dashboard-title')).toBeVisible();
  });

  test('non-existent instance shows not found', async ({ page }) => {
    await page.goto('/instances/99999');
    // May show "Instance not found" or "Cannot load metrics" depending on race
    await expect(
      page.getByText('Instance not found').or(page.getByText('Cannot load metrics')),
    ).toBeVisible();
  });
});
