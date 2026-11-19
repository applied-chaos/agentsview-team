export const STEPS = 5;
export const GAP = 3;
export const HEADER = 14;

/** 0 = no activity (under 3% of the peak), 1..STEPS otherwise. */
export function heatLevel(v: number, max: number): number {
  if (max <= 0 || v / max < 0.03) return 0;
  return Math.min(STEPS, Math.ceil((v / max) * STEPS));
}

/** Monday-first month calendar: blanks before the first day, one cell per day. */
export function heatmap(daily: { date: string; tokens: number }[]) {
  const first = daily[0];
  if (!first) return { blanks: 0, rows: 0, cells: [] };
  const max = Math.max(...daily.map((d) => d.tokens));
  const blanks = (new Date(`${first.date}T00:00:00Z`).getUTCDay() + 6) % 7;
  return {
    blanks,
    rows: Math.ceil((blanks + daily.length) / 7),
    cells: daily.map((d) => ({ date: d.date, tokens: d.tokens, level: heatLevel(d.tokens, max) })),
  };
}

/** Square cell size so header row, gaps and `rows` rows fill `height`. */
export const cellSize = (height: number, rows: number) =>
  Math.max(1, Math.floor((height - HEADER - GAP - (rows - 1) * GAP) / rows));
