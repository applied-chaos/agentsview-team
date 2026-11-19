const DAY = 86_400_000;
const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

/** en-GB date/time text; ICU renders September as "Sept", the mockups use "Sep". */
export const dateText = (d: Date | string, o: Intl.DateTimeFormatOptions) =>
  new Date(d).toLocaleString('en-GB', o).replace('Sept', 'Sep');

/** "21 Nov" */
export const shortDate = (iso: string) => dateText(iso, { day: 'numeric', month: 'short' });

const GROUPED = new Intl.NumberFormat('en-GB');

/** Integer count with en-GB grouping: 4,321. */
export const num = (n: number) => GROUPED.format(n);

/** Three significant figures, zeros trimmed: 2.31B, 612M, 98M, 1.5k. */
export function tokenLabel(n: number): string {
  if (n < 1e3) return num(n);
  for (const [size, suffix] of [
    [1e3, 'k'],
    [1e6, 'M'],
  ] as const) {
    const v = +(n / size).toPrecision(3);
    if (v < 1000) return `${v}${suffix}`;
  }
  return `${+(n / 1e9).toPrecision(3)}B`;
}

/**
 * 'host': "just now", "2 min ago", "3 h ago", "4 days ago", "never".
 * 'admin': sentence case with calendar days: "Just now", "Today", "Yesterday", "10 Nov", "Never".
 */
export function ago(
  iso: string | null,
  now: number | Date = Date.now(),
  style: 'host' | 'admin' = 'host',
): string {
  const nowMs = +now;
  const admin = style === 'admin';
  if (!iso) return admin ? 'Never' : 'never';
  const t = new Date(iso);
  const mins = Math.floor((nowMs - t.getTime()) / 60_000);
  if (admin) {
    if (mins < 1) return 'Just now';
    if (mins < 60) return `${mins} min ago`;
    if (mins < 180) return `${Math.floor(mins / 60)} h ago`;
    const days = Math.round((startOfDay(new Date(nowMs)) - startOfDay(t)) / DAY);
    if (days < 1) return 'Today';
    if (days === 1) return 'Yesterday';
    const elapsed = Math.round((nowMs - t.getTime()) / DAY);
    return elapsed <= 3 ? `${elapsed} days ago` : shortDate(iso);
  }
  const m = Math.max(0, mins);
  if (m < 1) return 'just now';
  if (m < 60) return `${m} min ago`;
  const hours = Math.floor(m / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.floor(hours / 24);
  return `${days} ${days === 1 ? 'day' : 'days'} ago`;
}

/** 0..n-1, for skeleton placeholders. */
export const placeholders = (n: number): number[] => Array.from({ length: n }, (_, i) => i);
