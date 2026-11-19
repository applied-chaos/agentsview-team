// Set theme and accent before first paint to avoid a flash.
try {
  const root = document.documentElement;
  const theme =
    localStorage.getItem('avt-theme') ||
    (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  root.dataset.theme = theme;
  const accent = localStorage.getItem('avt-accent');
  if (accent) root.style.setProperty('--accent', accent);
  const on = localStorage.getItem('avt-on-accent');
  if (accent && on) root.style.setProperty('--on-accent', on);
} catch {
  /* storage blocked: defaults apply */
}
