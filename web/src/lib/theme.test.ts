import { beforeEach, describe, expect, it } from 'vitest';
import {
  currentAccent,
  currentTheme,
  DEFAULT_ACCENT,
  hsl,
  onAccent,
  PALETTE,
  setAccent,
  setTheme,
} from './theme';

beforeEach(() => {
  document.documentElement.removeAttribute('data-theme');
  document.documentElement.style.removeProperty('--accent');
  document.documentElement.style.removeProperty('--on-accent');
  localStorage.clear();
});

describe('theme', () => {
  it('defaults to the blue accent and light theme', () => {
    expect(currentAccent()).toBe(DEFAULT_ACCENT);
    expect(currentTheme()).toBe('light');
  });

  it('applies and remembers the chosen theme and accent', () => {
    setTheme('dark');
    setAccent(hsl(300, 50, 40));
    expect(currentTheme()).toBe('dark');
    expect(currentAccent()).toBe('hsl(300 50% 40%)');
    expect(localStorage.getItem('avt-theme')).toBe('dark');
    expect(localStorage.getItem('avt-accent')).toBe('hsl(300 50% 40%)');
  });
});

describe('onAccent', () => {
  it.each([
    ['#ffff00', '#000'],
    ['#fde047', '#000'],
    ['hsl(60 100% 50%)', '#000'],
    ['#000', '#fff'],
    ['#3b5bdb', '#fff'],
    ['hsl(226 69% 55%)', '#fff'],
    ['hsl(0 0% 100%)', '#000'],
    ['nonsense', '#fff'],
  ])('%s -> %s', (c, want) => {
    expect(onAccent(c)).toBe(want);
  });

  it('keeps white on every palette colour', () => {
    for (const c of Object.keys(PALETTE)) expect(onAccent(c)).toBe('#fff');
  });

  it('persists with the accent', () => {
    setAccent('#ffff00');
    expect(document.documentElement.style.getPropertyValue('--on-accent')).toBe('#000');
    expect(localStorage.getItem('avt-on-accent')).toBe('#000');
  });
});
