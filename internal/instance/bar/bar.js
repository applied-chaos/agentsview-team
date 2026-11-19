// Served from the Platform origin because agentsview's CSP allows no inline script.
(() => {
  const bar = document.getElementById('av-platbar');
  if (!bar) return;
  const get = (k) => {
    try {
      return localStorage.getItem(k);
    } catch {
      return null;
    }
  };
  const dark = () =>
    (get('avt-theme') || get('theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')) ===
    'dark';
  const accent = get('avt-accent');
  if (accent) bar.style.setProperty('--accent', accent);
  const paint = () => (bar.dataset.theme = dark() ? 'dark' : 'light');
  paint();
  // agentsview's own theme switch writes 'theme'.
  addEventListener('storage', paint);
  new MutationObserver(paint).observe(document.documentElement, { attributes: true });

  // The banner makes the bar taller; keep agentsview's own layout below it.
  const fit = () => document.documentElement.style.setProperty('--av-bar-h', bar.offsetHeight + 'px');
  fit();
  new ResizeObserver(fit).observe(bar);

  const spaCachePrefix = 'avc:';
  const dropImpersonatedCache = () => {
    try {
      Object.keys(sessionStorage)
        .filter((k) => k.startsWith(spaCachePrefix))
        .forEach((k) => sessionStorage.removeItem(k));
    } catch {
      /* storage blocked: nothing cached */
    }
  };

  bar.querySelector('[data-act=stop-imp]')?.addEventListener('click', (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    fetch('/auth/impersonate/stop', { method: 'POST' })
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status));
        dropImpersonatedCache();
        location.assign('/admin/users/' + btn.dataset.user);
      })
      .catch(() => (btn.disabled = false));
  });
})();
