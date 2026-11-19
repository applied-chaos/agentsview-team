import { expect, test, type Page } from '@playwright/test';
import { ISSUER, signIn, useTheme, type Theme } from './helpers';

const THEMES: Theme[] = ['light', 'dark'];

for (const theme of THEMES) {
  test.describe(`error pages ${theme}`, () => {
    test.beforeEach(async ({ page }) => useTheme(page, theme));

    const shoot = async (page: Page, size: string, stem: string) => {
      await page.evaluate(() => document.fonts.ready);
      await page.screenshot({ path: `e2e/screens/error-${stem}-${theme}-${size}.png` });
    };

    test('sign-in-expired', async ({ page }, info) => {
      const resp = await page.goto('/auth/callback');
      expect(resp?.status()).toBe(400);
      await expect(page.getByRole('heading', { name: 'Your sign-in expired' })).toBeVisible();
      await expect(page.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
        'href',
        '/auth/login',
      );
      await shoot(page, info.project.name, 'sign-in-expired');
    });

    test('no-verified-email', async ({ page, context }, info) => {
      await context.addCookies([
        {
          name: 'fake_identity',
          value: encodeURIComponent('jess@halcyon.example|Jess Okafor||false'),
          url: ISSUER,
        },
      ]);
      await signIn(page);
      await expect(page.getByRole('heading', { name: 'We need a verified email' })).toBeVisible();
      await shoot(page, info.project.name, 'no-verified-email');
    });

    test('not-found', async ({ page }, info) => {
      const resp = await page.goto('/i/nope/');
      expect(resp?.status()).toBe(404);
      await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
      await shoot(page, info.project.name, 'not-found');
    });
  });
}
