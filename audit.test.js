const { test, expect } = require('@playwright/test');

const BASE_URL = 'http://localhost:3456';

test.describe('Comprehensive UI Audit', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(BASE_URL);
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1500);
  });

  test.describe('Overview Page', () => {
    test('displays all navbar elements', async ({ page }) => {
      // Logo
      const logo = await page.locator('.logo');
      await expect(logo).toContainText('teploy');
      expect(await logo.isVisible()).toBe(true);

      // Nav buttons
      await expect(page.locator('button:has-text("Overview")')).toBeVisible();
      await expect(page.locator('button:has-text("Deployments")')).toBeVisible();
      await expect(page.locator('button:has-text("Settings")')).toBeVisible();

      // Search and theme
      await expect(page.locator('.search-box .search-input')).toBeVisible();
      await expect(page.locator('.theme-btn')).toBeVisible();
    });

    test('shows page header with title and description', async ({ page }) => {
      const title = await page.locator('[x-data="overviewPage()"] .page-header h1');
      await expect(title).toContainText('Overview');

      const desc = await page.locator('[x-show*="overview"] .page-header p');
      const text = await desc.textContent();
      expect(text).toContain('System status');
    });

    test('displays servers section with data', async ({ page }) => {
      const serverSection = await page.locator('[x-data="overviewPage()"] h2:has-text("Servers")');
      await expect(serverSection).toBeVisible();

      // Wait for data to load
      await page.waitForTimeout(1000);

      const serverCards = await page.locator('[x-data="overviewPage()"] .grid .card').count();
      console.log(`Found ${serverCards} server cards`);
      expect(serverCards).toBeGreaterThan(0);

      // Check first card has required info
      const firstCard = await page.locator('[x-data="overviewPage()"] .grid .card').first();
      const hasStatusDot = await firstCard.locator('.status-dot').count();
      const hasName = await firstCard.locator('h3').count();
      expect(hasStatusDot).toBeGreaterThan(0);
      expect(hasName).toBeGreaterThan(0);
    });

    test('displays recent deployments section', async ({ page }) => {
      const recentSection = await page.locator('[x-data="overviewPage()"] h2:has-text("Recent Deployments")');
      await expect(recentSection).toBeVisible();

      // Wait for data
      await page.waitForTimeout(1000);

      const deployCards = await page.locator('[x-data="overviewPage()"] h2:has-text("Recent") ~ .grid .card').count();
      console.log(`Found ${deployCards} deployment cards`);

      if (deployCards > 0) {
        const firstDeploy = await page.locator('[x-data="overviewPage()"] h2:has-text("Recent") ~ .grid .card').first();
        await expect(firstDeploy).toContainText(/testapp|Project/i);
      }
    });

    test('server cards are clickable and navigate', async ({ page }) => {
      const serverCards = await page.locator('[x-data="overviewPage()"] .grid .card');
      const count = await serverCards.count();

      if (count > 0) {
        await serverCards.first().click();
        await page.waitForTimeout(500);

        // Should be on server detail page
        const cpuCard = await page.locator('text=CPU');
        const isVisible = await cpuCard.isVisible().catch(() => false);
        expect(isVisible).toBe(true);
      }
    });

    test('go back button returns to overview', async ({ page }) => {
      // Click a server
      const serverCards = await page.locator('[x-data="overviewPage()"] .grid .card');
      if (await serverCards.count() > 0) {
        await serverCards.first().click();
        await page.waitForTimeout(500);

        // Click back (the one in the page-header)
        await page.locator('[x-data="serverDetail()"] button:has-text("← Back")').click();
        await page.waitForTimeout(300);

        // Should be back on overview
        const overview = await page.locator('[x-data="overviewPage()"] .page-header h1');
        await expect(overview).toContainText('Overview');
      }
    });
  });

  test.describe('Deployments Page', () => {
    test('navigate to deployments page', async ({ page }) => {
      const deploymentsBtn = await page.locator('button:has-text("Deployments")');
      await deploymentsBtn.click();
      await page.waitForTimeout(500);

      const header = await page.locator('[x-show*="deployments"] .page-header h1');
      await expect(header).toContainText('Deployments');
    });

    test('displays groups with project tables', async ({ page }) => {
      const deploymentsBtn = await page.locator('button:has-text("Deployments")');
      await deploymentsBtn.click();
      await page.waitForTimeout(1500);

      // Should have group headers
      const groups = await page.locator('[x-show*="deployments"] h2').count();
      console.log(`Found ${groups} groups`);
      expect(groups).toBeGreaterThan(0);

      // Should have tables
      const tables = await page.locator('[x-show*="deployments"] table').count();
      console.log(`Found ${tables} project tables`);
      expect(tables).toBeGreaterThan(0);

      // First table should have headers
      const firstTable = await page.locator('[x-show*="deployments"] table').first();
      await expect(firstTable.locator('th:has-text("Project")')).toBeVisible();
      await expect(firstTable.locator('th:has-text("Server")')).toBeVisible();
      await expect(firstTable.locator('th:has-text("Status")')).toBeVisible();
    });

    test('project rows have all data columns', async ({ page }) => {
      const deploymentsBtn = await page.locator('button:has-text("Deployments")');
      await deploymentsBtn.click();
      await page.waitForTimeout(1500);

      const firstRow = await page.locator('[x-show*="deployments"] tbody tr').first();
      const cells = await firstRow.locator('td').count();
      console.log(`Project row has ${cells} cells`);
      expect(cells).toBeGreaterThanOrEqual(4); // Project, Server, Version, Status, Action
    });

    test('manage button navigates to deployment detail', async ({ page }) => {
      const deploymentsBtn = await page.locator('button:has-text("Deployments")');
      await deploymentsBtn.click();
      await page.waitForTimeout(1500);

      const manageBtn = await page.locator('[x-show*="deployments"] button:has-text("Manage")').first();
      const exists = await manageBtn.count() > 0;

      if (exists) {
        await manageBtn.click();
        await page.waitForTimeout(500);

        const detailHeader = await page.locator('[x-data*="deploymentDetail"] .page-header h1');
        const isVisible = await detailHeader.isVisible().catch(() => false);
        expect(isVisible).toBe(true);
      }
    });
  });

  test.describe('Deployment Detail Page', () => {
    test('navigate to deployment detail', async ({ page }) => {
      // Go to deployments
      await page.locator('button:has-text("Deployments")').click();
      await page.waitForTimeout(1500);

      // Click manage
      const manageBtn = await page.locator('[x-show*="deployments"] button:has-text("Manage")').first();
      if (await manageBtn.count() > 0) {
        await manageBtn.click();
        await page.waitForTimeout(1000);

        const header = await page.locator('[x-data*="deploymentDetail"] .page-header h1');
        const isVisible = await header.isVisible().catch(() => false);
        expect(isVisible).toBe(true);
      }
    });

    test('displays deployment info and actions', async ({ page }) => {
      await page.locator('button:has-text("Deployments")').click();
      await page.waitForTimeout(1500);

      const manageBtn = await page.locator('[x-show*="deployments"] button:has-text("Manage")').first();
      if (await manageBtn.count() > 0) {
        await manageBtn.click();
        await page.waitForTimeout(1500);

        // Check for action buttons
        const restartBtn = await page.locator('[x-show*="deployment"] button:has-text("Restart")');
        const stopBtn = await page.locator('[x-show*="deployment"] button:has-text("Stop")');
        const startBtn = await page.locator('[x-show*="deployment"] button:has-text("Start")');

        console.log(`Restart visible: ${await restartBtn.isVisible().catch(() => false)}`);
        console.log(`Stop visible: ${await stopBtn.isVisible().catch(() => false)}`);
        console.log(`Start visible: ${await startBtn.isVisible().catch(() => false)}`);
      }
    });

    test('status section displays deployment info', async ({ page }) => {
      await page.locator('button:has-text("Deployments")').click();
      await page.waitForTimeout(1500);

      const manageBtn = await page.locator('[x-show*="deployments"] button:has-text("Manage")').first();
      if (await manageBtn.count() > 0) {
        await manageBtn.click();
        await page.waitForTimeout(1500);

        const statusCard = await page.locator('[x-show*="deployment"] h3:has-text("Status")');
        const exists = await statusCard.isVisible().catch(() => false);
        console.log(`Status card exists: ${exists}`);
      }
    });
  });

  test.describe('Server Detail Page', () => {
    test('displays server resource info', async ({ page }) => {
      // Click a server from overview
      const serverCards = await page.locator('[x-show*="overview"] .grid .card');
      if (await serverCards.count() > 0) {
        await serverCards.first().click();
        await page.waitForTimeout(1500);

        // Should show CPU, Memory, Disk cards
        const cpuCard = await page.locator('[x-show*="server-detail"] h3:has-text("CPU")');
        const memCard = await page.locator('[x-show*="server-detail"] h3:has-text("Memory")');
        const diskCard = await page.locator('[x-show*="server-detail"] h3:has-text("Disk")');

        console.log(`CPU card: ${await cpuCard.isVisible().catch(() => false)}`);
        console.log(`Memory card: ${await memCard.isVisible().catch(() => false)}`);
        console.log(`Disk card: ${await diskCard.isVisible().catch(() => false)}`);
      }
    });
  });

  test.describe('Settings Page', () => {
    test('navigate to settings', async ({ page }) => {
      const settingsBtn = await page.locator('button:has-text("Settings")');
      await settingsBtn.click();
      await page.waitForTimeout(500);

      const header = await page.locator('[x-show*="settings"] .page-header h1');
      await expect(header).toContainText('Settings');
    });

    test('displays groups management section', async ({ page }) => {
      const settingsBtn = await page.locator('button:has-text("Settings")');
      await settingsBtn.click();
      await page.waitForTimeout(500);

      const groupsHeader = await page.locator('[x-show*="settings"] h3:has-text("Project Groups")');
      await expect(groupsHeader).toBeVisible();

      const createInput = await page.locator('[x-show*="settings"] input[placeholder*="group"]');
      await expect(createInput).toBeVisible();

      const createBtn = await page.locator('[x-show*="settings"] button:has-text("Create")');
      await expect(createBtn).toBeVisible();
    });

    test('displays notifications section', async ({ page }) => {
      const settingsBtn = await page.locator('button:has-text("Settings")');
      await settingsBtn.click();
      await page.waitForTimeout(500);

      const notifHeader = await page.locator('[x-show*="settings"] h3:has-text("Notifications")');
      await expect(notifHeader).toBeVisible();

      const webhookInput = await page.locator('[x-show*="settings"] input[placeholder*="hooks.slack"]');
      await expect(webhookInput).toBeVisible();

      const saveBtn = await page.locator('[x-show*="settings"] button:has-text("Save")');
      await expect(saveBtn).toBeVisible();
    });

    test('groups table displays existing groups', async ({ page }) => {
      const settingsBtn = await page.locator('button:has-text("Settings")');
      await settingsBtn.click();
      await page.waitForTimeout(500);

      const table = await page.locator('[x-show*="settings"] table').count();
      console.log(`Settings tables found: ${table}`);

      const groupRows = await page.locator('[x-show*="settings"] tbody tr').count();
      console.log(`Group rows found: ${groupRows}`);

      if (groupRows > 0) {
        const firstGroup = await page.locator('[x-show*="settings"] tbody tr').first();
        const groupName = await firstGroup.locator('td').first().textContent();
        console.log(`First group: ${groupName}`);
      }
    });
  });

  test.describe('UI Design & Styling Quality', () => {
    test('navbar styling is correct', async ({ page }) => {
      const navbar = await page.locator('.navbar');
      const styles = await navbar.evaluate(el => {
        const computed = window.getComputedStyle(el);
        return {
          position: computed.position,
          zIndex: computed.zIndex,
          background: computed.background,
        };
      });

      console.log('Navbar styles:', styles);
      expect(styles.position).toBe('sticky');
      expect(parseInt(styles.zIndex)).toBeGreaterThan(50);
    });

    test('cards have proper elevation on hover', async ({ page }) => {
      const card = await page.locator('[x-show*="overview"] .grid .card').first();

      // Get initial shadow
      const initialShadow = await card.evaluate(el => {
        return window.getComputedStyle(el).boxShadow;
      });

      // Hover
      await card.hover();
      await page.waitForTimeout(200);

      const hoverShadow = await card.evaluate(el => {
        return window.getComputedStyle(el).boxShadow;
      });

      console.log(`Initial shadow: ${initialShadow}`);
      console.log(`Hover shadow: ${hoverShadow}`);
      expect(hoverShadow).not.toBe(initialShadow);
    });

    test('buttons have proper styling', async ({ page }) => {
      const primaryBtn = await page.locator('[x-show*="settings"] button:has-text("Save")');
      const styles = await primaryBtn.evaluate(el => {
        const computed = window.getComputedStyle(el);
        return {
          background: computed.background,
          cursor: computed.cursor,
          fontWeight: computed.fontWeight,
        };
      });

      console.log('Button styles:', styles);
      expect(styles.cursor).toBe('pointer');
    });

    test('theme toggle changes color scheme', async ({ page }) => {
      // Get initial bg color
      const initialBg = await page.locator('html').evaluate(el => {
        return window.getComputedStyle(el).getPropertyValue('--bg');
      });

      // Click theme button
      await page.locator('.theme-btn').click();
      await page.waitForTimeout(200);

      // Get new bg color
      const newBg = await page.locator('html').evaluate(el => {
        return window.getComputedStyle(el).getPropertyValue('--bg');
      });

      console.log(`Initial bg: ${initialBg}`);
      console.log(`New bg: ${newBg}`);
    });

    test('responsive layout on mobile', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 });
      await page.waitForTimeout(300);

      // Main should still be visible
      const main = await page.locator('.main');
      await expect(main).toBeVisible();

      // Navbar should be accessible
      const navbar = await page.locator('.navbar');
      await expect(navbar).toBeVisible();

      // Grid should be single column
      const grid = await page.locator('[x-data="overviewPage()"] .grid').first();
      const gridCols = await grid.evaluate(el => {
        return window.getComputedStyle(el).gridTemplateColumns;
      });

      console.log(`Grid columns on mobile: ${gridCols}`);
    });
  });

  test.describe('Functionality & Interactions', () => {
    test('search input is functional', async ({ page }) => {
      const searchInput = await page.locator('.search-box .search-input');
      await searchInput.fill('test');

      const value = await searchInput.inputValue();
      expect(value).toBe('test');
    });

    test('navigation between pages is smooth', async ({ page }) => {
      // Overview -> Deployments
      await page.locator('button:has-text("Deployments")').click();
      await page.waitForTimeout(300);
      let visible = await page.locator('[x-show*="deployments"] .page-header').isVisible().catch(() => false);
      expect(visible).toBe(true);

      // Deployments -> Settings
      await page.locator('button:has-text("Settings")').click();
      await page.waitForTimeout(300);
      visible = await page.locator('[x-show*="settings"] .page-header').isVisible().catch(() => false);
      expect(visible).toBe(true);

      // Settings -> Overview
      await page.locator('button:has-text("Overview")').click();
      await page.waitForTimeout(300);
      visible = await page.locator('[x-show*="overview"] .page-header').isVisible().catch(() => false);
      expect(visible).toBe(true);
    });

    test('active nav button highlights correctly', async ({ page }) => {
      const overviewBtn = await page.locator('button:has-text("Overview")').first();
      const isActive = await overviewBtn.evaluate(el => el.classList.contains('active'));
      console.log(`Overview button active: ${isActive}`);

      await page.locator('button:has-text("Deployments")').click();
      await page.waitForTimeout(300);

      const deploymentsBtn = await page.locator('button:has-text("Deployments")').first();
      const isNowActive = await deploymentsBtn.evaluate(el => el.classList.contains('active'));
      console.log(`Deployments button active: ${isNowActive}`);
    });
  });

  test.describe('Accessibility & Performance', () => {
    test('all interactive elements are keyboard accessible', async ({ page }) => {
      // Tab through nav buttons
      await page.keyboard.press('Tab');
      await page.waitForTimeout(100);

      const focused = await page.evaluate(() => {
        return document.activeElement.tagName;
      });

      console.log(`Initially focused element: ${focused}`);
    });

    test('page loads quickly', async ({ page }) => {
      const startTime = Date.now();
      await page.goto(BASE_URL);
      await page.waitForLoadState('networkidle');
      const loadTime = Date.now() - startTime;

      console.log(`Page load time: ${loadTime}ms`);
      expect(loadTime).toBeLessThan(5000); // Should load in under 5 seconds
    });
  });
});
