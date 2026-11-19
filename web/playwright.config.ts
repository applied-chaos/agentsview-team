import { defineConfig } from '@playwright/test';

// Runs in compose (web/Dockerfile.ui), sharing platform-prod's network namespace: the Platform is http://localhost:8080, the issuer http://fakeoidc:9000.
export default defineConfig({
  testDir: 'e2e',
  outputDir: 'test-results',
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: process.env.UI_URL ?? 'http://localhost:8080',
  },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1440, height: 900 } } },
    { name: 'mobile', use: { viewport: { width: 390, height: 844 }, hasTouch: true } },
  ],
});
