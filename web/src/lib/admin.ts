import { array, boolean, nullable, number, object, oneOf, string, type Infer } from './parse';
import { InstallToken } from './hosts';
import { dateText } from './format';

export const AdminInstanceInfo = object({
  state: oneOf('starting', 'running', 'failed'),
  url: string,
  space: object({ total: number, adminInstance: number, spool: number }),
  users: array(
    object({
      id: string,
      name: string,
      sessions: number,
      tokens: number,
      cost: number,
      hosts: number,
      storageBytes: number,
      lastActive: nullable(string),
    }),
  ),
});
export type AdminInstanceInfo = Infer<typeof AdminInstanceInfo>;

const adminUserShape = {
  /** Null only for an email with a pending Admin grant and no User yet. */
  id: nullable(string),
  name: string,
  email: string,
  role: oneOf('admin', 'user'),
  pendingAdmin: boolean,
  status: oneOf('active', 'disabled', 'instance_failed'),
  hosts: number,
  storageBytes: number,
  lastActive: nullable(string),
};
export const AdminUser = object(adminUserShape);
export type AdminUser = Infer<typeof AdminUser>;
export const AdminUsers = object({ users: array(AdminUser) });

export const AdminUserDetail = object({
  ...adminUserShape,
  id: string,
  joined: string,
  instance: nullable(object({ status: oneOf('running', 'starting', 'failed'), version: string })),
  hostList: array(
    object({ id: string, name: string, os: string, online: boolean, lastSeen: nullable(string) }),
  ),
  installTokens: array(InstallToken),
  usage: object({
    sessions: number,
    tokens: number,
    costUsd: number,
    daily: array(number),
  }),
});
export type AdminUserDetail = Infer<typeof AdminUserDetail>;

export const AuditPage = object({
  entries: array(
    object({
      id: number,
      at: string,
      actor: string,
      action: string,
      target: string,
      details: string,
    }),
  ),
  total: number,
  page: number,
  pageSize: number,
});
export type AuditPage = Infer<typeof AuditPage>;

export function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${i > 0 && n < 10 ? n.toFixed(1) : Math.round(n)} ${units[i]}`;
}

export const formatDateTime = (iso: string) =>
  dateText(iso, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });

export const formatFullDateTime = (iso: string) =>
  dateText(iso, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });

export const formatJoined = (iso: string) =>
  new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' });

/** Last-30-days activity grid: leading blanks to align Monday-first weeks, then one cell per day. */
export function calendar(daily: number[], now: Date) {
  const max = Math.max(0, ...daily);
  const first = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 29);
  return {
    pad: Array.from({ length: (first.getDay() + 6) % 7 }, (_, i) => i),
    cells: Array.from({ length: 30 }, (_, i) => {
      const d = new Date(first.getFullYear(), first.getMonth(), first.getDate() + i);
      const t = daily[i] ?? 0;
      const day = dateText(d, {
        weekday: 'short',
        day: 'numeric',
        month: 'short',
      });
      return {
        level: t ? Math.min(4, Math.ceil((t / max) * 4)) : 0,
        title: `${day}: ${t ? `${t}M tokens` : 'no activity'}`,
        today: i === 29,
      };
    }),
  };
}
