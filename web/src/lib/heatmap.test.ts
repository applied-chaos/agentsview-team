import { describe, expect, it } from 'vitest';
import { cellSize, heatLevel, heatmap } from './heatmap';

describe('heatLevel', () => {
  it.each([
    [0, 100, 0],
    [2, 100, 0],
    [3, 100, 1],
    [20, 100, 1],
    [21, 100, 2],
    [50, 100, 3],
    [80, 100, 4],
    [81, 100, 5],
    [100, 100, 5],
    [5, 0, 0],
  ])('%i of %i is level %i', (v, max, level) => {
    expect(heatLevel(v, max)).toBe(level);
  });
});

describe('heatmap', () => {
  const days = (from: string, n: number) =>
    Array.from({ length: n }, (_, i) => ({
      date: new Date(Date.parse(`${from}T00:00:00Z`) + i * 86400000).toISOString().slice(0, 10),
      tokens: i,
    }));

  it('pads Monday-first: 2026-10-16 is a Friday', () => {
    const h = heatmap(days('2026-10-16', 30));
    expect(h.blanks).toBe(4);
    expect(h.rows).toBe(5);
    expect(h.cells[0]).toMatchObject({ date: '2026-10-16', level: 0 });
    expect(h.cells[29]?.level).toBe(5);
  });

  it('starts flush on a Monday and handles empty input', () => {
    expect(heatmap(days('2026-11-02', 7)).blanks).toBe(0);
    expect(heatmap(days('2026-11-02', 7)).rows).toBe(1);
    expect(heatmap([])).toEqual({ blanks: 0, rows: 0, cells: [] });
  });
});

describe('cellSize', () => {
  it('fits header, gaps and rows inside the height', () => {
    const c = cellSize(180, 5);
    expect(14 + 3 + c * 5 + 4 * 3).toBeLessThanOrEqual(180);
    expect(14 + 3 + (c + 1) * 5 + 4 * 3).toBeGreaterThan(180);
  });
  it('never goes below 1px', () => {
    expect(cellSize(0, 5)).toBe(1);
  });
});
