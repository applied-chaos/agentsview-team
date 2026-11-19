import { expect, test, type Page } from '@playwright/test';
import { stubAddHost, stubHostDetail, stubHosts, stubHostsPage } from './host-fixtures';
import { ADMIN_INSTANCE, AUDIT, NOW, PRIYA, TEAM, USERS } from './fixtures';
import { setIdentity, signIn, useTheme, type Theme } from './helpers';

const THEMES: Theme[] = ['light', 'dark'];

async function shoot(page: Page, size: string, stem: string, theme: Theme) {
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: `e2e/screens/${stem}-${theme}-${size}.png` });
}

for (const theme of THEMES) {
  test.describe(theme, () => {
    test.beforeEach(async ({ page }) => useTheme(page, theme));

    test('sign-in', async ({ page }, info) => {
      await page.goto('/sign-in');
      await expect(page.getByText('Access is limited to')).toBeVisible();
      await shoot(page, info.project.name, 'sign-in', theme);
    });

    test('not-permitted', async ({ page, context }, info) => {
      await setIdentity(context, 'jess.okafor@gmail.com', 'Jess Okafor');
      await signIn(page);
      await expect(page.getByText('You signed in as')).toBeVisible();
      await shoot(page, info.project.name, 'not-permitted', theme);
    });

    test.describe('signed in', () => {
      test.beforeEach(async ({ page, context }) => {
        await setIdentity(context, 'jess@halcyon.example', 'Jess Okafor');
        await signIn(page);
        await expect(page.getByRole('heading', { name: 'Add your first Host' })).toBeVisible();
      });

      test('overview-empty', async ({ page }, info) => {
        await shoot(page, info.project.name, 'overview-empty', theme);
      });

      test('overview', async ({ page }, info) => {
        await stubHosts(page);
        await page.goto('/');
        await expect(page.getByRole('heading', { name: 'Your month' })).toBeVisible();
        await expect(page.locator('.cal i.today')).toBeVisible();
        await shoot(page, info.project.name, 'overview', theme);
      });

      test('team', async ({ page }, info) => {
        await page.route('**/api/team', (route) => route.fulfill({ json: TEAM }));
        await page.goto('/team');
        await expect(page.getByRole('heading', { name: 'Top agents' })).toBeVisible();
        await shoot(page, info.project.name, 'team', theme);
      });

      test('hosts', async ({ page }, info) => {
        await stubHostsPage(page);
        await page.goto('/hosts');
        await expect(page.getByRole('heading', { name: 'Hosts', exact: true })).toBeVisible();
        await expect(page.getByText('codespace-api-7f2').first()).toBeVisible();
        await expect(page.getByText('CI runners').first()).toBeVisible();
        await expect(page.getByText('old-laptop').first()).toBeVisible();
        await shoot(page, info.project.name, 'hosts', theme);
      });

      test('hosts-loading', async ({ page }, info) => {
        await stubHostsPage(page);
        let release = () => {};
        const held = new Promise<void>((r) => (release = r));
        await page.route('**/api/hosts', async (route) => {
          await held;
          await route.fallback();
        });
        await page.goto('/hosts');
        await expect(page.getByRole('heading', { name: 'Hosts', exact: true })).toBeVisible();
        await expect(page.getByLabel('Loading')).toBeVisible();
        await shoot(page, info.project.name, 'hosts-loading', theme);
        release();
        await expect(page.getByText('codespace-api-7f2').first()).toBeVisible();
        await expect(page.getByLabel('Loading')).toHaveCount(0);
      });

      test('hosts-dlg-token', async ({ page }, info) => {
        await stubHostsPage(page);
        await page.goto('/hosts');
        await page.getByRole('button', { name: 'New Install Token' }).click();
        await expect(page.getByRole('dialog', { name: 'New Install Token' })).toBeVisible();
        await shoot(page, info.project.name, 'hosts-dlg-token', theme);
      });

      test('hosts-dlg-created', async ({ page }, info) => {
        await stubHostsPage(page);
        await page.goto('/hosts');
        await page.getByRole('button', { name: 'New Install Token' }).click();
        await page.getByRole('button', { name: 'Create Install Token' }).click();
        await expect(page.getByRole('dialog', { name: 'Install Token created' })).toBeVisible();
        await shoot(page, info.project.name, 'hosts-dlg-created', theme);
      });

      test('host', async ({ page }, info) => {
        await stubHostDetail(page);
        await page.goto('/hosts/h4');
        await expect(page.getByRole('heading', { name: 'devbox-win' })).toBeVisible();
        await expect(page.locator('.cal i.today')).toBeVisible();
        await shoot(page, info.project.name, 'host', theme);
      });

      test('host-reinstall', async ({ page }, info) => {
        await stubHostDetail(page);
        await page.goto('/hosts/h4');
        const h = page.getByRole('heading', { name: 'Reinstall or update' });
        await expect(h).toBeVisible();
        if (info.project.name === 'desktop') {
          await page.setViewportSize({ width: 1440, height: 1500 });
          await shoot(page, info.project.name, 'host-full', theme);
        } else {
          await h.evaluate((el) => el.scrollIntoView({ block: 'start' }));
          await shoot(page, info.project.name, 'host-re-h', theme);
        }
      });

      test('host-dlg-revoke', async ({ page }, info) => {
        await stubHostDetail(page);
        await page.goto('/hosts/h4');
        await page.getByRole('button', { name: 'Revoke Host' }).click();
        await expect(page.getByRole('dialog', { name: 'Revoke devbox-win?' })).toBeVisible();
        await page.evaluate(() => scrollTo(0, 0));
        await shoot(page, info.project.name, 'host-dlg-revoke', theme);
      });

      test('host-dlg-delete', async ({ page }, info) => {
        await stubHostDetail(page);
        await page.goto('/hosts/h4');
        await page.getByRole('button', { name: 'Delete Host' }).click();
        await expect(page.getByRole('dialog', { name: 'Delete devbox-win?' })).toBeVisible();
        await page.evaluate(() => scrollTo(0, 0));
        await shoot(page, info.project.name, 'host-dlg-delete', theme);
      });

      test('add-host', async ({ page }, info) => {
        await stubAddHost(page, false);
        await page.goto('/hosts/new');
        await expect(page.locator('.code code')).toContainText('AVT_TOKEN=avt_tk_9fK2mQx7LwR4tVb8');
        await shoot(page, info.project.name, 'add-host', theme);
      });

      test('add-host-connected', async ({ page }, info) => {
        await stubAddHost(page, true);
        await page.goto('/hosts/new');
        await expect(page.getByText('build-02 connected')).toBeVisible();
        await page.getByText('build-02 connected').scrollIntoViewIfNeeded();
        await shoot(page, info.project.name, 'add-host-connected', theme);
      });

      test('overview-settings', async ({ page }, info) => {
        await stubHosts(page);
        await page.goto('/#settings');
        await page.reload();
        await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
        await shoot(page, info.project.name, 'overview-settings', theme);
      });

      test('instance', async ({ page }, info) => {
        await page.goto('/i/me/');
        await expect(page.locator('#av-platbar .back')).toHaveText('Back to AgentsView Teams');
        await expect(page.locator('#app')).not.toBeEmpty();
        await page.waitForLoadState('networkidle');
        await shoot(page, info.project.name, 'instance', theme);
      });

      test('instance-failed', async ({ page }, info) => {
        // The Instance is healthy; serve the SPA shell in its place and report it failed.
        await page.route('**/i/*/', async (route) => {
          if (route.request().resourceType() !== 'document') return route.continue();
          await route.fulfill({ response: await page.request.get('/sign-in') });
        });
        await page.route('**/api/instance', (route) =>
          route.fulfill({ json: { state: 'failed', url: '/i/x/', version: '0.47.0', error: '' } }),
        );
        await page.goto('/i/me/');
        await expect(
          page.getByRole('heading', { name: /Your Instance didn.t start/ }),
        ).toBeVisible();
        await shoot(page, info.project.name, 'instance-failed', theme);
      });

      test.describe('Instance View and Impersonation', () => {
        let priyaId: string;
        test.beforeEach(async ({ page, browser }, info) => {
          const ctx = await browser.newContext({ baseURL: info.project.use.baseURL });
          const other = await ctx.newPage();
          await setIdentity(ctx, 'priya@halcyon.example', 'Priya Nair');
          await signIn(other);
          await expect(other).toHaveURL('/');
          await ctx.close();
          const res = await page.request.get('/api/admin/users');
          const users = (await res.json()).users as { id: string; email: string }[];
          priyaId = users.find((u) => u.email === 'priya@halcyon.example')?.id ?? '';
          expect(priyaId).not.toBe('');
        });

        const ready = async (page: Page, banner: string) => {
          await expect(page.locator(`#av-platbar .banner.${banner}`)).toBeVisible({
            timeout: 60_000,
          });
          await expect(page.locator('#app')).not.toBeEmpty();
          await page.waitForLoadState('networkidle');
        };

        test('instance-banner-view', async ({ page }, info) => {
          await page.goto(`/i/${priyaId}/`);
          await ready(page, 'view');
          await shoot(page, info.project.name, 'instance-banner-view', theme);
        });

        test('instance-banner-imp', async ({ page }, info) => {
          expect((await page.request.post(`/auth/impersonate/${priyaId}`)).status()).toBe(204);
          await page.goto('/i/me/');
          await ready(page, 'imp');
          await shoot(page, info.project.name, 'instance-banner-imp', theme);
          await page.request.post('/auth/impersonate/stop');
        });
      });

      test('overview-menu-open', async ({ page }, info) => {
        await stubHosts(page);
        await page.goto('/');
        await expect(page.getByRole('heading', { name: 'Your month' })).toBeVisible();
        const burger = page.getByRole('button', { name: 'Menu' });
        if (await burger.isVisible()) await burger.click();
        await shoot(page, info.project.name, 'overview-menu-open', theme);
      });

      test.describe('admin', () => {
        test.use({ timezoneId: 'Europe/London' });
        test.beforeEach(async ({ page }) => {
          await page.clock.setFixedTime(NOW);
          const stub = (url: string, body: unknown) =>
            page.route(url, (route) => route.fulfill({ json: body }));
          await stub('**/api/admin/users', { users: USERS });
          await stub('**/api/admin/users/*', PRIYA);
          await stub('**/api/admin/audit*', AUDIT);
          await stub('**/api/admin/instance', ADMIN_INSTANCE);
        });

        test('admin-users', async ({ page }, info) => {
          await page.goto('/admin/users');
          await expect(page.getByText('Hana Sato')).toBeVisible();
          await shoot(page, info.project.name, 'admin-users', theme);
        });

        test('admin-users-dlg-grant', async ({ page }, info) => {
          await page.goto('/admin/users#dlg-grant');
          await expect(page.getByRole('dialog', { name: 'Grant Admin' })).toBeVisible();
          await shoot(page, info.project.name, 'admin-users-dlg-grant', theme);
        });

        test('admin-user', async ({ page }, info) => {
          await page.goto('/admin/users/priya-nair');
          await expect(page.getByRole('heading', { name: /Priya Nair/ })).toBeVisible();
          await shoot(page, info.project.name, 'admin-user', theme);
        });

        test('admin-user-dlg-disable', async ({ page }, info) => {
          await page.goto('/admin/users/priya-nair#dlg-disable');
          await expect(page.getByRole('dialog', { name: 'Disable Priya Nair?' })).toBeVisible();
          await shoot(page, info.project.name, 'admin-user-dlg-disable', theme);
        });

        test('admin-user-dlg-delete', async ({ page }, info) => {
          await page.goto('/admin/users/priya-nair#dlg-delete');
          await expect(page.getByRole('dialog', { name: 'Delete Priya Nair?' })).toBeVisible();
          await shoot(page, info.project.name, 'admin-user-dlg-delete', theme);
        });

        test('admin-instance', async ({ page }, info) => {
          await page.goto('/admin/instance');
          await expect(page.getByText('Rafael Costa')).toBeVisible();
          await shoot(page, info.project.name, 'admin-instance', theme);
        });

        test('audit', async ({ page }, info) => {
          await page.goto('/admin/audit');
          await expect(page.getByText('Showing 12 of 1,284')).toBeVisible();
          await shoot(page, info.project.name, 'audit', theme);
        });
      });
    });
  });
}
