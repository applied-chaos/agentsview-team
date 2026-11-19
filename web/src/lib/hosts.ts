import { dateText, shortDate } from './format';
import {
  array,
  boolean,
  nullable,
  number,
  object,
  oneOf,
  optional,
  string,
  type Infer,
} from './parse';

export const Host = object({
  id: string,
  name: string,
  os: string,
  installMode: oneOf('systemd', 'profile', 'launchd', 'task'),
  status: oneOf('online', 'offline', 'revoked'),
  lastSeenAt: nullable(string),
  lastUploadAt: nullable(string),
  revokedAt: nullable(string),
  createdAt: string,
  version: string,
  outdated: boolean,
  ephemeral: boolean,
});
export type Host = Infer<typeof Host>;

export const INSTALL_MODE: Record<Host['installMode'], { label: string; purge: boolean }> = {
  systemd: { label: 'System service', purge: true },
  profile: { label: 'Shell profile', purge: true },
  launchd: { label: 'Launch agent', purge: false },
  task: { label: 'Scheduled task', purge: false },
};

export const Usage = object({
  from: string,
  to: string,
  sessions: number,
  tokens: number,
  costUsd: number,
  activeDays: number,
  daily: array(object({ date: string, tokens: number })),
});
export type Usage = Infer<typeof Usage>;

export const Overview = object({
  hosts: array(Host),
  pinnedVersion: string,
  /** Null while the Platform cannot read it. */
  storageBytes: nullable(number),
  usage: nullable(Usage),
  usageStale: boolean,
});
export type Overview = Infer<typeof Overview>;

export const HostsList = object({ hosts: array(Host), pinnedVersion: string });
export type HostsList = Infer<typeof HostsList>;

export const HostDetail = object({
  host: Host,
  pinnedVersion: string,
  uninstallCommand: string,
  updateCommand: string,
  usage: nullable(Usage),
  usageStale: boolean,
});
export type HostDetail = Infer<typeof HostDetail>;

export const InstallToken = object({
  id: string,
  description: string,
  reusable: boolean,
  ephemeral: boolean,
  createdAt: string,
  expiresAt: nullable(string),
  lastUsedAt: nullable(string),
  hostsCreated: number,
});
export type InstallToken = Infer<typeof InstallToken>;

export const InstallTokens = object({ tokens: array(InstallToken) });

export const NewToken = object({ id: string, token: string, baseUrl: string });
export type NewToken = Infer<typeof NewToken>;

export const CreatedToken = object({ token: string });

export const TokenState = object({
  state: oneOf('waiting', 'connected', 'expired'),
  host: optional(Host),
});

/** "Never", "in 1 hour" within a day, else "21 Nov" */
export function expiry(iso: string | null, now = Date.now()): string {
  if (iso === null) return 'Never';
  const hours = Math.max(1, Math.ceil((Date.parse(iso) - now) / 3600000));
  if (hours > 24) return shortDate(iso);
  return `in ${hours} ${hours === 1 ? 'hour' : 'hours'}`;
}

/** "3 of unlimited" for Reusable tokens, "0 of 1" for single-use */
export const uses = (t: InstallToken) => `${t.hostsCreated} of ${t.reusable ? 'unlimited' : 1}`;

/** "3 Sep 2026" */
export const day = (iso: string) =>
  dateText(iso, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  });
