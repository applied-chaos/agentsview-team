import type { BrowserContext, Page } from '@playwright/test';

export const ISSUER = 'http://fakeoidc:9000';
export type Theme = 'light' | 'dark';

export async function setIdentity(
  context: BrowserContext,
  email: string,
  name: string,
  groups: string[] = [],
) {
  await context.addCookies([
    {
      name: 'fake_identity',
      value: encodeURIComponent(`${email}|${name}|${groups.join(',')}`),
      url: ISSUER,
    },
  ]);
}

export async function useTheme(page: Page, theme: Theme) {
  await page.addInitScript((t) => {
    localStorage.setItem('avt-theme', t);
    localStorage.setItem('theme', t); // agentsview's own key, for the Instance page
  }, theme);
}

export async function signIn(page: Page) {
  await page.goto('/sign-in');
  await page.getByRole('link', { name: /Continue with/ }).click();
}
