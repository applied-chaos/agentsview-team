export const DEFAULT_ACCENT = '#3b5bdb';

export const PALETTE: Record<string, string> = {
  '#3b5bdb': 'Blue',
  '#4f46e5': 'Indigo',
  '#7c3aed': 'Violet',
  '#db2777': 'Pink',
  '#c2410c': 'Orange',
  '#15803d': 'Green',
  '#0f766e': 'Teal',
  '#3f3f46': 'Graphite',
};

function remember(k: string, v: string) {
  try {
    localStorage.setItem(k, v);
  } catch {
    /* storage blocked: choice lasts until reload */
  }
}

export const currentTheme = (): 'light' | 'dark' =>
  document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';

export function setTheme(t: 'light' | 'dark') {
  document.documentElement.dataset.theme = t;
  remember('avt-theme', t);
}

export const currentAccent = (): string =>
  document.documentElement.style.getPropertyValue('--accent').trim() || DEFAULT_ACCENT;

export function setAccent(v: string) {
  const on = onAccent(v);
  document.documentElement.style.setProperty('--accent', v);
  document.documentElement.style.setProperty('--on-accent', on);
  remember('avt-accent', v);
  remember('avt-on-accent', on);
}

function rgbOf(c: string): [number, number, number] | null {
  const x = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(c.trim());
  if (x?.[1]) {
    const d = x[1].length === 3 ? [...x[1]].map((p) => p + p).join('') : x[1];
    return [0, 2, 4].map((i) => parseInt(d.slice(i, i + 2), 16)) as [number, number, number];
  }
  const m = /^hsl\(\s*([\d.]+)(?:deg)?[\s,]+([\d.]+)%[\s,]+([\d.]+)%\s*\)$/i.exec(c.trim());
  if (!m) return null;
  const h = +(m[1] ?? 0);
  const s = +(m[2] ?? 0) / 100;
  const l = +(m[3] ?? 0) / 100;
  const a = s * Math.min(l, 1 - l);
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    return Math.round(255 * (l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))));
  };
  return [f(0), f(8), f(4)];
}

const luminance = ([r, g, b]: [number, number, number]) => {
  const lin = (v: number) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
};

/** #fff or #000, whichever has higher WCAG contrast on the accent (hex or hsl()); white if unparseable. */
export function onAccent(accent: string): '#fff' | '#000' {
  const rgb = rgbOf(accent);
  if (!rgb) return '#fff';
  const L = luminance(rgb);
  return 1.05 / (L + 0.05) >= (L + 0.05) / 0.05 ? '#fff' : '#000';
}

export const hsl = (h: number, s: number, l: number) => `hsl(${h} ${s}% ${l}%)`;
