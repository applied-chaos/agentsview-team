import { describe, expect, it } from 'vitest';
import { describeRule } from './rule';

describe('describeRule', () => {
  it.each([
    [{ domains: ['halcyon.example'], group: '' }, 'halcyon.example accounts'],
    [{ domains: ['a.example', 'b.example'], group: '' }, 'a.example or b.example accounts'],
    [{ domains: [], group: '' }, ''],
    [{ domains: [], group: 'staff' }, 'Users in the staff group'],
    [
      { domains: ['halcyon.example'], group: 'staff' },
      'halcyon.example accounts in the staff group',
    ],
  ])('%j', (rule, want) => {
    expect(describeRule(rule)).toBe(want);
  });
});
