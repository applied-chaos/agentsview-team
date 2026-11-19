import { describe, expect, it } from 'vitest';
import { ago, dateText, num, shortDate, tokenLabel } from './format';

describe('num', () => {
  it('groups with en-GB commas', () => {
    expect(num(4321)).toBe('4,321');
    expect(num(0)).toBe('0');
    expect(num(1234567)).toBe('1,234,567');
  });
});

const hostNow = Date.parse('2026-11-14T15:45:00Z');
const before = (mins: number) => new Date(hostNow - mins * 60000).toISOString();

describe('ago (host)', () => {
  it('formats minutes, hours and days', () => {
    expect(ago(before(0), hostNow)).toBe('just now');
    expect(ago(before(2), hostNow)).toBe('2 min ago');
    expect(ago(before(125), hostNow)).toBe('2 h ago');
    expect(ago(before(60 * 24), hostNow)).toBe('1 day ago');
    expect(ago(before(60 * 24 * 3), hostNow)).toBe('3 days ago');
    expect(ago(null, hostNow)).toBe('never');
  });
});

describe('ago (admin)', () => {
  const now = new Date(2026, 10, 14, 15, 45);
  const back = (ms: number) => new Date(now.getTime() - ms).toISOString();

  it.each([
    [null, 'Never'],
    [back(12 * 60_000), '12 min ago'],
    [back(60 * 60_000), '1 h ago'],
    [back(5 * 3_600_000), 'Today'],
    [back(20 * 3_600_000), 'Yesterday'],
    [back(3 * 86_400_000), '3 days ago'],
    [back(4 * 86_400_000), '10 Nov'],
    [back(30 * 86_400_000), '15 Oct'],
  ])('%s -> %s', (iso, want) => {
    expect(ago(iso, now, 'admin')).toBe(want);
  });
});

describe('tokenLabel', () => {
  it('uses three significant figures, trimming zeros', () => {
    const cases: [number, string][] = [
      [7, '7'],
      [42_000, '42k'],
      [999_600, '1M'],
      [1_500, '1.5k'],
      [3_000_000, '3M'],
      [97_960_000, '98M'],
      [188_400_000, '188M'],
      [612_300_000, '612M'],
      [1_050_000_000, '1.05B'],
      [1_384_000_000, '1.38B'],
      [2_310_000_000, '2.31B'],
      [15_400_000_000, '15.4B'],
    ];
    for (const [n, label] of cases) expect(tokenLabel(n)).toBe(label);
  });
});

describe('shortDate', () => {
  it('drops the year', () => {
    expect(shortDate('2026-11-21T12:00:00Z')).toBe('21 Nov');
  });
});

describe('dateText', () => {
  it('uses three-letter months', () => {
    expect(shortDate('2026-09-03T12:00:00Z')).toBe('3 Sep');
    expect(
      dateText('2026-09-03T12:00:00Z', { day: 'numeric', month: 'short', year: 'numeric' }),
    ).toBe('3 Sep 2026');
  });
});
