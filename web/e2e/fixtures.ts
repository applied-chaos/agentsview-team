// Admin API payloads for the screenshot tests: the mockup's fictional org. Hosts, storage,
// Instance state and usage have no backing data until later tickets, so these are stubbed.
const GB = 1024 ** 3;
const MB = 1024 ** 2;

export const NOW = new Date('2026-11-14T15:45:00Z');

interface User {
  id: string | null;
  name: string;
  email: string;
  role: 'admin' | 'user';
  pendingAdmin: boolean;
  status: 'active' | 'disabled' | 'instance_failed';
  hosts: number;
  storageBytes: number;
  lastActive: string | null;
}

const user = (
  name: string,
  role: User['role'],
  hosts: number,
  storageBytes: number,
  lastActive: string | null,
  extra: Partial<User> = {},
): User => ({
  id: name.toLowerCase().replace(/\W/g, '-'),
  name,
  email: `${(name.split(' ')[0] ?? '').toLowerCase().replace('á', 'a')}@halcyon.example`,
  role,
  pendingAdmin: false,
  status: 'active',
  hosts,
  storageBytes,
  lastActive,
  ...extra,
});

export const USERS: User[] = [
  user('Jess Okafor', 'admin', 6, 2.4 * GB, '2026-11-14T09:00:00Z'),
  user('Priya Nair', 'user', 3, 1.2 * GB, '2026-11-14T15:33:00Z'),
  user('Sam Lee', 'user', 2, 684 * MB, '2026-11-14T14:45:00Z'),
  user('Tomás Ruiz', 'user', 4, 1.8 * GB, '2026-11-13T10:00:00Z'),
  user('Mei Chen', 'admin', 3, 920 * MB, '2026-11-14T11:00:00Z'),
  user('Ade Bello', 'user', 1, 212 * MB, '2026-11-11T12:00:00Z'),
  user('Lena Fischer', 'user', 2, 540 * MB, '2026-11-11T00:00:00Z'),
  user('Owen Price', 'user', 0, 0, null, { id: null, pendingAdmin: true }),
  user('Rafael Costa', 'user', 2, 1.1 * GB, '2026-10-28T12:00:00Z', { status: 'disabled' }),
  user('Hana Sato', 'user', 1, 96 * MB, '2026-11-09T12:00:00Z', { status: 'instance_failed' }),
];

// Same generator as docs/mockups/mock.js usage(7).
function daily(seed: number): number[] {
  const end = new Date(2026, 10, 14);
  return [...Array(30)].map((_, i) => {
    const d = new Date(end);
    d.setDate(end.getDate() - 29 + i);
    const we = d.getDay() % 6 === 0;
    const h = Math.sin((i + 1) * 12.9898 * seed + 78.233) * 43758.5453;
    const r = h - Math.floor(h);
    return r < (we ? 0.55 : 0.1) ? 0 : +((we ? 1 : 2.5) + r * (we ? 3 : 9.5)).toFixed(1);
  });
}

export const PRIYA = {
  ...USERS[1],
  joined: '2026-03-03T09:00:00Z',
  lastActive: '2026-11-14T15:30:00Z',
  instance: { status: 'running', version: '0.47.0' },
  hostList: [
    {
      id: 'h1',
      name: 'priya-mbp',
      os: 'macOS 15.1',
      online: true,
      lastSeen: '2026-11-14T15:43:00Z',
    },
    {
      id: 'h2',
      name: 'priya-dev',
      os: 'Ubuntu 24.04',
      online: true,
      lastSeen: '2026-11-14T15:05:00Z',
    },
    {
      id: 'h3',
      name: 'priya-win',
      os: 'Windows 11',
      online: false,
      lastSeen: '2026-11-12T12:00:00Z',
    },
  ],
  installTokens: [
    {
      id: 't1',
      description: 'Work machines',
      reusable: true,
      ephemeral: false,
      createdAt: '2026-11-01T09:00:00Z',
      expiresAt: null,
      lastUsedAt: '2026-11-12T15:45:00Z',
      hostsCreated: 3,
    },
    {
      id: 't2',
      description: 'New laptop',
      reusable: false,
      ephemeral: false,
      createdAt: '2026-11-14T09:00:00Z',
      expiresAt: '2026-11-21T09:00:00Z',
      lastUsedAt: null,
      hostsCreated: 0,
    },
  ],
  usage: { sessions: 184, tokens: 96e6, costUsd: 318, daily: daily(7) },
};

const row = (
  id: number,
  at: string,
  actor: string,
  action: string,
  target: string,
  details: string,
) => ({ id, at: `2026-${at}Z`, actor, action, target, details });

const usage = (
  name: string,
  sessions: number,
  tokensM: number,
  cost: number,
  hosts: number,
  lastActive: string,
  storageBytes: number,
) => ({
  id: name.toLowerCase().replace(/\W/g, '-'),
  name,
  sessions,
  tokens: tokensM * 1e6,
  cost,
  hosts,
  lastActive,
  storageBytes,
});

// Stubbed: the mockup's per-User usage table (real figures need a live Admin Instance).
export const ADMIN_INSTANCE = {
  id: 'admin',
  state: 'running',
  url: '/i/admin/',
  space: { total: 12.1e9, adminInstance: 3.1e9, spool: 120e6 },
  users: [
    usage('Jess Okafor', 412, 261, 842, 6, '2026-11-14T09:00:00Z', 2.4e9),
    usage('Mei Chen', 356, 214, 701, 3, '2026-11-14T10:00:00Z', 920e6),
    usage('Priya Nair', 184, 96, 318, 3, '2026-11-14T15:33:00Z', 1.2e9),
    usage('Tomás Ruiz', 171, 88, 290, 4, '2026-11-13T16:00:00Z', 1.8e9),
    usage('Sam Lee', 120, 52, 171, 2, '2026-11-14T14:45:00Z', 684e6),
    usage('Lena Fischer', 64, 30, 98, 2, '2026-11-11T00:00:00Z', 540e6),
    usage('Ade Bello', 41, 18, 57, 1, '2026-11-11T12:00:00Z', 212e6),
    usage('Hana Sato', 9, 3, 9, 1, '2026-11-09T12:00:00Z', 96e6),
    usage('Rafael Costa', 0, 0, 0, 2, '2026-10-28T12:00:00Z', 1.1e9),
  ],
};

export const AUDIT = {
  total: 1284,
  page: 1,
  pageSize: 25,
  entries: [
    row(12, '11-14T15:42', 'Jess Okafor', 'Impersonation started', 'Priya Nair', 'Support request'),
    row(11, '11-14T15:31', 'Jess Okafor', 'Impersonation ended', 'Priya Nair', 'Lasted 11 min'),
    row(10, '11-14T14:07', 'Mei Chen', 'Instance View opened', 'Tomás Ruiz', 'Read only'),
    row(
      9,
      '11-14T11:20',
      'Jess Okafor',
      'Admin granted',
      'owen@halcyon.example',
      'Pending until first sign-in',
    ),
    row(8, '11-13T17:45', 'Mei Chen', 'User disabled', 'Rafael Costa', 'Left the company'),
    row(7, '11-13T16:02', 'Jess Okafor', 'Host revoked', 'priya-win', 'Lost laptop'),
    row(6, '11-13T09:18', 'Mei Chen', 'Install Token created', 'Lena Fischer', 'Expires in 24 h'),
    row(5, '11-12T15:55', 'Jess Okafor', 'Impersonation ended', 'Sam Lee', 'Lasted 4 min'),
    row(4, '11-12T15:51', 'Jess Okafor', 'Impersonation started', 'Sam Lee', 'Sync not appearing'),
    row(
      3,
      '11-11T10:30',
      'Mei Chen',
      'User deleted',
      'Dana Whitlock',
      'Instance and 38 sessions removed',
    ),
    row(2, '11-10T13:12', 'Jess Okafor', 'Admin role removed', 'Ade Bello', 'Granted 2 Nov'),
    row(1, '11-10T09:40', 'Mei Chen', 'Instance View opened', 'Hana Sato', 'Read only'),
  ],
};

// Team page figures from the mockup; the daily series reproduces mock.js's usage(3, 9).
function teamDaily() {
  const end = Date.UTC(2026, 10, 14);
  return Array.from({ length: 30 }, (_, i) => {
    const d = new Date(end + (i - 29) * 86400000);
    const we = d.getUTCDay() % 6 === 0;
    const h = Math.sin((i + 1) * 12.9898 * 3 + 78.233) * 43758.5453;
    const r = h - Math.floor(h);
    const t = r < (we ? 0.55 : 0.1) ? 0 : +(((we ? 1 : 2.5) + r * (we ? 3 : 9.5)) * 9).toFixed(1);
    return {
      date: d.toISOString().slice(0, 10),
      tokens: Math.round(t * 1e6),
      sessions: Math.round(t * 4),
      cost: +(t * 1.4).toFixed(2),
      activeUsers: t ? 8 : 0,
      activeHosts: t ? 14 : 0,
    };
  });
}

const ranked = (name: string, sessions: number, tokens: number, share: number) => ({
  name,
  sessions,
  tokens,
  share,
});

export const TEAM = {
  from: '2026-10-16',
  to: '2026-11-14',
  org: {
    sessions: 4812,
    tokensIn: 2.31e9,
    tokensOut: 61e6,
    costUsd: 9240,
    activeUsers: 38,
    activeHosts: 112,
    daily: teamDaily(),
    models: [
      ranked('claude-sonnet-5-5', 1926, 1.05e9, 44),
      ranked('claude-opus-5-5', 1204, 612e6, 26),
      ranked('gpt-5.4', 842, 371e6, 16),
      ranked('claude-haiku-5-5', 536, 214e6, 9),
      ranked('gemini-3-pro', 304, 98e6, 5),
    ],
    agents: [
      ranked('Claude Code', 2788, 1.38e9, 58),
      ranked('Codex', 1011, 502e6, 21),
      ranked('Cursor', 622, 281e6, 12),
      ranked('Gemini CLI', 264, 98e6, 6),
      ranked('Aider', 127, 46e6, 3),
    ],
  },
  you: {
    sessions: 312,
    tokensIn: 151e6,
    tokensOut: 37e6,
    costUsd: 612,
    activeUsers: 1,
    activeHosts: 5,
    daily: [],
    models: [],
    agents: [],
  },
};
