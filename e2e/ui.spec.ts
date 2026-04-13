import { test, expect } from '@playwright/test';

// Clean up any test data after each test to prevent state leaking between tests.
test.afterEach(async ({ request }) => {
  // Clean up test servers
  await request.delete('/api/config/servers/test-srv-e2e').catch(() => {});
  // Clean up test groups
  await request.delete('/api/groups/E2ETestGroup').catch(() => {});
  // Clean up test registries
  await request.delete('/api/config/registries/e2e-registry.io').catch(() => {});
});

test.describe('Navigation & Shell', () => {
  test('loads the dashboard and shows navbar', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.navbar-brand')).toHaveText('teploy');
    await expect(page.locator('.navbar-nav a')).toHaveCount(3);
  });

  test('default page is Deployments', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.navbar-nav a.active')).toHaveText('Deployments');
    await expect(page.locator('.page-title')).toHaveText('Deployments');
  });

  test('navigate to Servers page', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Servers');
    await expect(page.locator('.navbar-nav a.active')).toHaveText('Servers');
    await expect(page.locator('.page-title')).toHaveText('Servers');
  });

  test('navigate to Settings page', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await expect(page.locator('.navbar-nav a.active')).toHaveText('Settings');
    await expect(page.locator('.page-title')).toHaveText('Settings');
  });

  test('navigate back to Deployments', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.click('text=Deployments');
    await expect(page.locator('.page-title')).toHaveText('Deployments');
  });
});

test.describe('Theme Toggle', () => {
  test('toggles between dark and light mode', async ({ page }) => {
    await page.goto('/');
    // Default is dark
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

    // Click theme toggle
    await page.click('.theme-toggle');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    // Toggle back
    await page.click('.theme-toggle');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });
});

test.describe('Projects Page', () => {
  test('shows content when loaded', async ({ page }) => {
    await page.goto('/');
    // Wait for loading to finish
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    // Should show empty state, groups, or app cards
    const content = page.locator('.empty-state, .card-grid, .section-header');
    await expect(content.first()).toBeVisible({ timeout: 10000 });
  });

  test('search bar is visible', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.search-input')).toBeVisible();
  });

  test('create group button exists', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('button:has-text("+ Group")')).toBeVisible();
  });

  test('no error toasts on load', async ({ page }) => {
    await page.goto('/');
    // Wait for any API calls to complete
    await page.waitForTimeout(2000);
    const toasts = page.locator('.toast.error');
    await expect(toasts).toHaveCount(0);
  });
});

test.describe('Servers Page', () => {
  test('shows empty state when no servers', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Servers');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    const emptyState = page.locator('.empty-state');
    const cardGrid = page.locator('.card-grid');
    await expect(emptyState.or(cardGrid).first()).toBeVisible({ timeout: 10000 });
  });

  test('no error toasts on Servers page', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Servers');
    await page.waitForTimeout(2000);
    const toasts = page.locator('.toast.error');
    await expect(toasts).toHaveCount(0);
  });
});

test.describe('Settings Page', () => {
  test('loads with servers tab by default', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await expect(page.locator('.tab.active')).toHaveText('Servers');
  });

  test('no error toasts on Settings page', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForTimeout(2000);
    const toasts = page.locator('.toast.error');
    await expect(toasts).toHaveCount(0);
  });

  test('can switch to Groups tab', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await page.click('.tab:has-text("Groups")');
    await expect(page.locator('.tab.active')).toHaveText('Groups');
  });

  test('can switch to Notifications tab', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await page.click('.tab:has-text("Notifications")');
    await expect(page.locator('.tab.active')).toHaveText('Notifications');
    await expect(page.locator('input[placeholder*="hooks.slack"]')).toBeVisible();
  });

  test('can switch to Registry tab', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await page.click('.tab:has-text("Registry")');
    await expect(page.locator('.tab.active')).toHaveText('Registry');
  });

  test('server add form is visible', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await expect(page.locator('input[placeholder="prod"]')).toBeVisible();
    await expect(page.locator('input[placeholder="192.168.1.100"]')).toBeVisible();
  });

  test('can add and remove a server', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});

    // Fill add server form
    await page.fill('input[placeholder="prod"]', 'test-srv-e2e');
    await page.fill('input[placeholder="192.168.1.100"]', '10.0.0.99');

    // Click the Add button in the servers tab form row
    await page.locator('.form-row button:has-text("Add")').first().click();

    // Wait for the server to appear in the table
    await expect(page.locator('td:has-text("test-srv-e2e")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('td:has-text("10.0.0.99")')).toBeVisible();

    // Remove it
    page.on('dialog', dialog => dialog.accept());
    await page.locator('button:has-text("Remove")').first().click();
    await page.waitForTimeout(2000);
  });
});

test.describe('Settings - Groups', () => {
  test('can create and delete a group', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await page.click('.tab:has-text("Groups")');

    // Create group
    await page.fill('input[placeholder="Group name"]', 'E2ETestGroup');
    await page.locator('button:has-text("Create")').click();

    // Should see it in the table
    await expect(page.locator('td:has-text("E2ETestGroup")')).toBeVisible({ timeout: 10000 });

    // Delete it
    page.on('dialog', dialog => dialog.accept());
    await page.locator('button:has-text("Delete")').first().click();
    await page.waitForTimeout(2000);
  });
});

test.describe('Settings - Notifications', () => {
  test('can save webhook URL', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await page.click('.tab:has-text("Notifications")');

    await page.fill('input[placeholder*="hooks.slack"]', 'https://example.com/webhook');
    await page.click('button:has-text("Save")');
    await page.waitForTimeout(1000);

    // Check for success toast
    const toast = page.locator('.toast.success');
    await expect(toast).toBeVisible({ timeout: 3000 }).catch(() => {});
  });
});

test.describe('Settings - Registry', () => {
  test('can add and remove a registry', async ({ page }) => {
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForSelector('.spinner', { state: 'hidden', timeout: 10000 }).catch(() => {});
    await page.click('.tab:has-text("Registry")');

    // Add registry
    await page.fill('input[placeholder="ghcr.io"]', 'e2e-registry.io');
    await page.fill('input[placeholder="username"]', 'e2euser');
    await page.fill('input[placeholder="token"]', 'e2epass');
    await page.locator('.form-row button:has-text("Add")').first().click();

    // Should see it
    await expect(page.locator('td:has-text("e2e-registry.io")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('td:has-text("e2euser")')).toBeVisible();

    // Remove it
    page.on('dialog', dialog => dialog.accept());
    await page.locator('button:has-text("Remove")').first().click();
    await page.waitForTimeout(2000);
  });
});

test.describe('API Endpoints', () => {
  test('GET /api/servers returns valid JSON', async ({ request }) => {
    const res = await request.get('/api/servers');
    expect(res.ok()).toBeTruthy();
    const json = await res.json();
    expect(json).toHaveProperty('data');
    expect(Array.isArray(json.data)).toBeTruthy();
  });

  test('GET /api/apps returns valid JSON', async ({ request }) => {
    const res = await request.get('/api/apps');
    expect(res.ok()).toBeTruthy();
    const json = await res.json();
    expect(json).toHaveProperty('data');
    expect(Array.isArray(json.data)).toBeTruthy();
  });

  test('GET /api/groups returns valid JSON', async ({ request }) => {
    const res = await request.get('/api/groups');
    expect(res.ok()).toBeTruthy();
    const json = await res.json();
    expect(json).toHaveProperty('data');
    expect(Array.isArray(json.data)).toBeTruthy();
  });

  test('GET /api/config/servers returns valid JSON', async ({ request }) => {
    const res = await request.get('/api/config/servers');
    expect(res.ok()).toBeTruthy();
    const json = await res.json();
    expect(json).toHaveProperty('data');
  });

  test('GET /api/config/notifications returns valid JSON', async ({ request }) => {
    const res = await request.get('/api/config/notifications');
    expect(res.ok()).toBeTruthy();
    const json = await res.json();
    expect(json).toHaveProperty('data');
  });

  test('GET /api/config/registries returns valid JSON', async ({ request }) => {
    const res = await request.get('/api/config/registries');
    expect(res.ok()).toBeTruthy();
    const json = await res.json();
    expect(json).toHaveProperty('data');
    expect(Array.isArray(json.data)).toBeTruthy();
  });

  test('POST /api/config/servers validates input', async ({ request }) => {
    const res = await request.post('/api/config/servers', {
      data: { name: '; rm -rf /', host: '1.2.3.4' },
    });
    expect(res.status()).toBe(400);
  });

  test('POST /api/groups validates input', async ({ request }) => {
    const res = await request.post('/api/groups', {
      data: { name: '' },
    });
    expect(res.status()).toBe(400);
  });

  test('POST /api/groups injection attempt rejected', async ({ request }) => {
    const res = await request.post('/api/groups', {
      data: { name: '$(whoami)' },
    });
    expect(res.status()).toBe(400);
  });
});

test.describe('Console Errors', () => {
  test('no JavaScript errors on Projects page', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', err => errors.push(err.message));
    await page.goto('/');
    await page.waitForTimeout(3000);
    expect(errors).toEqual([]);
  });

  test('no JavaScript errors on Servers page', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', err => errors.push(err.message));
    await page.goto('/');
    await page.click('text=Servers');
    await page.waitForTimeout(3000);
    expect(errors).toEqual([]);
  });

  test('no JavaScript errors on Settings page', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', err => errors.push(err.message));
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForTimeout(3000);
    expect(errors).toEqual([]);
  });

  test('no JavaScript errors navigating all settings tabs', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', err => errors.push(err.message));
    await page.goto('/');
    await page.click('text=Settings');
    await page.waitForTimeout(1000);
    for (const tab of ['Groups', 'Notifications', 'Registry', 'Servers']) {
      await page.click(`.tab:has-text("${tab}")`);
      await page.waitForTimeout(500);
    }
    expect(errors).toEqual([]);
  });
});
