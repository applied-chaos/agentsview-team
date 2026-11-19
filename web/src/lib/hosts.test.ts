import { describe, expect, it } from 'vitest';
import { expiry } from './hosts';

const now = Date.parse('2026-11-14T15:45:00Z');
const before = (mins: number) => new Date(now - mins * 60000).toISOString();

describe('expiry', () => {
  it('reads a missing expiry as Never', () => {
    expect(expiry(null, now)).toBe('Never');
    expect(expiry(before(-90), now)).toBe('in 2 hours');
  });
});
