import type { Page } from '@playwright/test';

export const NOW = new Date('2026-11-14T15:45:00Z');
const ago = (mins: number) => new Date(NOW.getTime() - mins * 60000).toISOString();

const host = (
  id: string,
  name: string,
  os: string,
  status: string,
  mins: number,
  version: string,
  outdated = false,
  ephemeral = false,
  revokedAt: string | null = null,
) => ({
  id,
  name,
  os,
  installMode: 'systemd',
  status,
  lastSeenAt: ago(mins),
  lastUploadAt: ago(mins),
  revokedAt,
  createdAt: '2026-09-03T09:00:00Z',
  version,
  outdated,
  ephemeral,
});

export const HOSTS = [
  { ...host('h1', 'jess-mbp', 'macOS 15.1', 'online', 2, '0.47.0'), installMode: 'launchd' },
  host('h2', 'build-01', 'Ubuntu 24.04', 'online', 11, '0.47.0'),
  host('h3', 'ci-runner-03', 'Debian 12', 'online', 4, '0.47.0', false, true),
  {
    ...host('h4', 'devbox-win', 'Windows 11', 'offline', 120, '0.45.2', true),
    installMode: 'task',
  },
  host('h5', 'codespace-api-7f2', 'Ubuntu 22.04', 'offline', 60 * 24 * 3, '0.47.0', false, true),
];

const HOSTS_PAGE = [
  ...HOSTS,
  host(
    'h6',
    'old-laptop',
    'macOS 14.6',
    'revoked',
    60 * 24 * 12,
    '0.44.0',
    false,
    false,
    '2026-11-02T10:00:00Z',
  ),
];

const TOKENS = [
  tokenRow('k1', 'CI runners', '2026-11-01T09:00:00Z', null, true, true, 3, 2 * 24 * 60),
  tokenRow('k2', 'Build servers', '2026-11-14T09:00:00Z', '2026-11-21T09:00:00Z', false, false, 0),
  tokenRow('k3', 'New laptop', '2026-11-14T15:30:00Z', '2026-11-14T16:30:00Z', false, false, 0),
];

function tokenRow(
  id: string,
  description: string,
  createdAt: string,
  expiresAt: string | null,
  reusable: boolean,
  ephemeral: boolean,
  hostsCreated: number,
  usedMins: number | null = null,
) {
  return {
    id,
    description,
    reusable,
    ephemeral,
    createdAt,
    expiresAt,
    lastUsedAt: usedMins === null ? null : ago(usedMins),
    hostsCreated,
  };
}

// Same pseudo-random month as the mockup's calendar (mock.js `usage(seed)`), in tokens.
function daily(seed = 1) {
  const end = Date.UTC(2026, 10, 14);
  return Array.from({ length: 30 }, (_, i) => {
    const d = new Date(end + (i - 29) * 86400000);
    const we = d.getUTCDay() % 6 === 0;
    const h = Math.sin((i + 1) * 12.9898 * seed + 78.233) * 43758.5453;
    const r = h - Math.floor(h);
    const t = r < (we ? 0.55 : 0.1) ? 0 : +((we ? 1 : 2.5) + r * (we ? 3 : 9.5)).toFixed(1);
    return { date: d.toISOString().slice(0, 10), tokens: Math.round(t * 1e6) };
  });
}

export const OVERVIEW = {
  hosts: HOSTS,
  pinnedVersion: '0.47.0',
  storageBytes: 1.2e9,
  usage: {
    from: '2026-10-16',
    to: '2026-11-14',
    sessions: 312,
    tokens: 188_000_000,
    costUsd: 612,
    activeDays: 23,
    daily: daily(),
  },
  usageStale: false,
};

export async function stubHosts(page: Page) {
  await page.clock.setFixedTime(NOW);
  await page.route('**/api/overview', (route) => route.fulfill({ json: OVERVIEW }));
  await page.route('**/api/hosts', (route) =>
    route.fulfill({ json: { hosts: HOSTS, pinnedVersion: '0.47.0' } }),
  );
}

export async function stubHostDetail(page: Page, id = 'h4') {
  await stubHosts(page);
  const h = HOSTS.find((x) => x.id === id)!;
  await page.route(`**/api/hosts/${id}`, (route) =>
    route.fulfill({
      json: {
        host: h,
        pinnedVersion: '0.47.0',
        uninstallCommand: 'irm https://teams.halcyon.example/install/windows-uninstall.ps1 | iex',
        updateCommand: 'irm https://teams.halcyon.example/install/windows.ps1 | iex',
        usage: { ...OVERVIEW.usage, sessions: 41, tokens: 52_000_000, daily: daily(4) },
        usageStale: false,
      },
    }),
  );
}

export async function stubAddHost(page: Page, connected: boolean) {
  await page.clock.setFixedTime(NOW);
  await page.route('**/api/hosts/tokens', (route) =>
    route.fulfill({
      status: 201,
      json: {
        id: 't1',
        token: 'avt_tk_9fK2mQx7LwR4tVb8',
        baseUrl: 'https://teams.halcyon.example',
      },
    }),
  );
  await page.route('**/api/hosts/tokens/t1', (route) =>
    route.fulfill({
      json: connected
        ? {
            state: 'connected',
            host: host('h6', 'build-02', 'Ubuntu 24.04', 'online', 0, '0.47.0'),
          }
        : { state: 'waiting' },
    }),
  );
}

export async function stubHostsPage(page: Page) {
  await page.clock.setFixedTime(NOW);
  await page.route('**/api/hosts', (route) =>
    route.fulfill({ json: { hosts: HOSTS_PAGE, pinnedVersion: '0.47.0' } }),
  );
  await page.route('**/api/hosts/tokens', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 201,
          json: { id: 'k4', token: 'avt_tk_9fK2mQx7LwR4tVb8Nc3HdYe1ZsA6JpU0' },
        })
      : route.fulfill({ json: { tokens: TOKENS } }),
  );
}
