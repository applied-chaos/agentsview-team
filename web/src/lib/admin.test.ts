import { describe, expect, it } from 'vitest';
import { formatBytes } from './admin';

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [96 * 1024 ** 2, '96 MB'],
    [1.2 * 1024 ** 3, '1.2 GB'],
    [684 * 1024 ** 2, '684 MB'],
  ])('%s -> %s', (n, want) => {
    expect(formatBytes(n)).toBe(want);
  });
});
