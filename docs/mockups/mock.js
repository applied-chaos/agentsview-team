(() => {
  const q = new URLSearchParams(location.search);
  const store = { get: k => { try { return localStorage.getItem(k) } catch { return null } }, set: (k, v) => { try { localStorage.setItem(k, v) } catch {} } };
  const root = document.documentElement;
  const theme = q.get('theme') || store.get('avt-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  root.dataset.theme = theme;
  const DEFAULT_ACCENT = '#3b5bdb';
  const accent = q.get('accent') || store.get('avt-accent') || DEFAULT_ACCENT;
  root.style.setProperty('--accent', accent);

  const ICONS = {
    ext: '<path d="M7 7h10v10"/><path d="M7 17 17 7"/>',
    down: '<path d="m6 9 6 6 6-6"/>',
    menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
    copy: '<rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    up: '<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>',
    plus: '<path d="M5 12h14M12 5v14"/>',
    x: '<path d="M18 6 6 18M6 6l12 12"/>',
    search: '<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
    right: '<path d="M5 12h14"/><path d="m12 5 7 7-7 7"/>',
    left: '<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>',
    eye: '<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>',
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    alert: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
    info: '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>',
    shield: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>',
    terminal: '<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>',
  };
  const icon = n => `<svg class="i" aria-hidden="true"><use href="#i-${n}"/></svg>`;
  window.icon = icon;

  const PALETTE = { '#3b5bdb': 'Blue', '#4f46e5': 'Indigo', '#7c3aed': 'Violet', '#db2777': 'Pink', '#c2410c': 'Orange', '#15803d': 'Green', '#0f766e': 'Teal', '#3f3f46': 'Graphite' };
  const ME = { name: 'Jess Okafor', email: 'jess@halcyon.example', initials: 'JO' };
  const TABS = [['overview', 'Overview', 'overview.html'], ['hosts', 'Hosts', 'hosts.html'], ['team', 'Team', 'team.html']];
  const BANNERS = {
    view: ['view', 'eye', 'Instance View: you are looking at Priya Nair’s Instance as yourself.', 'Exit Instance View'],
    imp: ['imp', 'shield', 'Impersonating Priya Nair. Everything you do is recorded in the Audit Log.', 'Stop impersonating'],
  };

  const END = new Date(2026, 10, 14);
  function usage(seed = 1, scale = 1) {
    return [...Array(30)].map((_, i) => {
      const d = new Date(END); d.setDate(END.getDate() - 29 + i);
      const we = d.getDay() % 6 === 0;
      const h = Math.sin((i + 1) * 12.9898 * seed + 78.233) * 43758.5453, r = h - Math.floor(h);
      const t = r < (we ? .55 : .1) ? 0 : +(((we ? 1 : 2.5) + r * (we ? 3 : 9.5)) * scale).toFixed(1);
      return { d, we, t };
    });
  }
  const lvl = (t, max) => !t ? 0 : Math.min(4, Math.ceil(t / max * 4));
  const fmtD = d => d.toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short' });

  const toggleTheme = () => { const t = root.dataset.theme === 'dark' ? 'light' : 'dark'; root.dataset.theme = t; store.set('avt-theme', t); syncTheme(); };
  const syncTheme = () => document.querySelectorAll('[data-act=theme]').forEach(b => b.setAttribute('aria-checked', root.dataset.theme === 'dark'));
  const setAccent = v => { root.style.setProperty('--accent', v); store.set('avt-accent', v); };

  function topbar(tab) {
    const tabs = TABS.map(([k, l, h]) => `<a href="${h}"${k === tab ? ' aria-current="page"' : ''}>${l}</a>`).join('');
    const inst = `<a class="inst" href="instance.html">My Instance ${icon('ext')}</a>`;
    const items = `<a role="menuitem" href="#settings">Settings</a><a role="menuitem" href="admin-users.html">Admin</a>
      <button role="menuitemcheckbox" data-act="theme" aria-checked="false">Dark mode <span class="tog"></span></button>
      <a role="menuitem" href="sign-in.html">Sign out</a>`;
    return `<header class="topbar${q.get('menu') === 'open' ? ' menu-open' : ''}"><div class="wrap">
      <a class="brand" href="overview.html"><span class="logo"></span>AgentsView Teams</a>
      <nav class="tabs" aria-label="Main">${tabs}${inst}</nav><span class="sp"></span>
      <div class="acct"><button class="acct-btn" aria-haspopup="menu" aria-expanded="false"><span class="av">${ME.initials}</span><span>${ME.name}</span>${icon('down')}</button>
        <div class="menu" role="menu"><div class="who">${ME.name}<span>${ME.email}</span></div>${items}</div></div>
      <button class="burger" aria-label="Menu" aria-expanded="${q.get('menu') === 'open'}">${icon('menu')}</button></div>
      <nav class="mnav" aria-label="Main">${TABS.map(([k, l, h]) => `<a href="${h}"${k === tab ? ' aria-current="page"' : ''}>${l}</a>`).join('')}${inst}
        <hr><div class="who">${ME.name} · ${ME.email}</div>${items.replace(/ role="menuitem(checkbox)?"/g, '')}</nav></header>`;
  }

  function platbar() {
    return `<div class="platbar"><div class="wrap"><a class="back" href="overview.html">${icon('left')}Back to AgentsView Teams</a></div></div>`;
  }

  function settings() {
    const cur = getComputedStyle(root).getPropertyValue('--accent').trim();
    return `<div class="dlg" id="settings" role="dialog" aria-modal="true" aria-labelledby="settings-h"><div class="box">
      <div class="dh"><h2 id="settings-h">Settings</h2><a class="x" href="#" aria-label="Close">${icon('x')}</a></div>
      <div class="body"><div><h3>Accent colour</h3><p class="mu">Used for links, buttons and activity charts.</p></div>
        <div class="pal" role="group" aria-label="Accent colour">${Object.entries(PALETTE).map(([v, n]) => `<button style="--c:${v}" data-acc="${v}" aria-label="${n}" title="${n}" aria-pressed="${v === cur}"></button>`).join('')}</div>
        <div><h3>Custom colour</h3></div>
        <div class="hsl">${['Hue', 'Saturation', 'Lightness'].map((n, i) => `<label for="hsl${i}">${n}</label><input id="hsl${i}" type="range" data-hsl="${i}" min="${[0, 0, 25][i]}" max="${[360, 100, 65][i]}" value="${[226, 69, 55][i]}"><output for="hsl${i}">${[226, 69, 55][i]}${i ? '%' : '°'}</output>`).join('')}</div>
        <div class="swatch"><i></i><span id="acc-val">${cur}</span></div></div>
      <div class="df"><button class="btn" data-acc="${DEFAULT_ACCENT}">Reset to default</button><a class="btn primary" href="#">Done</a></div></div></div>`;
  }

  function cal(el) {
    const days = usage(+el.dataset.cal || 1), max = Math.max(...days.map(x => x.t));
    const pad = (days[0].d.getDay() + 6) % 7;
    el.innerHTML = `<div class="cal-wd" aria-hidden="true">${['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map(w => `<span>${w}</span>`).join('')}</div>
      <div class="cal" role="img" aria-label="Tokens per day, last 30 days">${'<i class="pad"></i>'.repeat(pad)}${days.map((x, i) => `<i class="l${lvl(x.t, max)}${i === 29 ? ' today' : ''}" title="${fmtD(x.d)}: ${x.t ? x.t + 'M tokens' : 'no activity'}"></i>`).join('')}</div>
      <div class="legend"><span>Tokens per day</span>Less <i></i><i class="l1"></i><i class="l2"></i><i class="l3"></i><i class="l4"></i> More</div>`;
  }

  function chart(el) {
    const days = usage(+el.dataset.chart || 3, 9), max = Math.max(...days.map(x => x.t));
    const bars = `<div class="chart" role="img" aria-label="${el.dataset.label || 'Tokens per day'}, last 30 days">${days.map((x, i) => `<i${x.we ? ' class="we"' : ''} data-i="${i}" style="height:${(x.t / max * 100).toFixed(1)}%" title="${fmtD(x.d)}: ${x.t}M tokens"></i>`).join('')}</div>
      <div class="chart-x"><span>${fmtD(days[0].d)}</span><span>${fmtD(days[15].d)}</span><span>Today</span></div>`;
    if (!el.hasAttribute('data-heat')) { el.innerHTML = bars; return; }
    const blanks = (days[0].d.getDay() + 6) % 7, rows = Math.ceil((blanks + 30) / 7);
    const hl = t => !t || t / max < .03 ? 0 : Math.min(5, Math.ceil(t / max * 5));
    el.innerHTML = `<div class="heat"><div>${bars}</div><div>
      <div class="hmap" role="img" aria-label="Tokens per day calendar, last 30 days">${[...'MTWTFSS'].map(w => `<div class="h" aria-hidden="true">${w}</div>`).join('')}${'<div class="c out"></div>'.repeat(blanks)}${days.map((x, i) => `<div class="c h${hl(x.t)}" data-i="${i}" title="${fmtD(x.d)}: ${x.t}M tokens"></div>`).join('')}</div>
      <div class="hmap-leg" aria-hidden="true">Less <i></i>${[1, 2, 3, 4, 5].map(k => `<i class="h${k}"></i>`).join('')} More</div></div></div>
      <div class="heat-tip" aria-live="polite"></div>`;
    const hm = el.querySelector('.hmap'), ch = el.querySelector('.chart'), tip = el.querySelector('.heat-tip');
    const fit = () => hm.style.setProperty('--cell', Math.max(1, Math.floor((ch.offsetHeight - 14 - 3 - (rows - 1) * 3) / rows)) + 'px');
    const on = i => {
      el.querySelectorAll('.on').forEach(e => e.classList.remove('on'));
      tip.textContent = '';
      if (i == null) return;
      el.querySelectorAll(`[data-i="${i}"]`).forEach(e => e.classList.add('on'));
      tip.textContent = `${fmtD(days[i].d)}: ${days[i].t}M tokens`;
    };
    el.addEventListener('pointerover', e => on(e.target.dataset?.i ?? null));
    el.addEventListener('pointerleave', () => on(null));
    fit(); addEventListener('resize', fit);
  }

  document.addEventListener('DOMContentLoaded', () => {
    const b = document.body, sprite = `<svg width="0" height="0" style="position:absolute" aria-hidden="true">${Object.entries(ICONS).map(([k, p]) => `<symbol id="i-${k}" viewBox="0 0 24 24">${p}</symbol>`).join('')}</svg>`;
    const banner = BANNERS[q.get('banner') || b.dataset.banner];
    const bar = b.dataset.bar === 'slim' ? platbar() : b.dataset.bar === 'none' ? '' : topbar(b.dataset.tab);
    const bannerHtml = banner ? `<div class="banner ${banner[0]}" role="status">${icon(banner[1])}<span>${banner[2]}</span><a class="btn" href="admin-user.html">${banner[3]}</a></div>` : '';
    b.insertAdjacentHTML('afterbegin', sprite + bannerHtml + bar);
    if (b.dataset.bar !== 'none') b.insertAdjacentHTML('beforeend', settings());
    document.querySelectorAll('[data-cal]').forEach(cal);
    document.querySelectorAll('[data-chart]').forEach(chart);
    syncTheme();

    document.addEventListener('click', e => {
      const t = e.target;
      if (t.closest('[data-act=theme]')) { toggleTheme(); return; }
      const acc = t.closest('[data-acc]');
      if (acc) { setAccent(acc.dataset.acc); document.querySelectorAll('.pal button').forEach(p => p.setAttribute('aria-pressed', p.dataset.acc === acc.dataset.acc)); document.getElementById('acc-val').textContent = acc.dataset.acc; return; }
      const ab = t.closest('.acct-btn');
      document.querySelectorAll('.acct').forEach(a => { const open = ab && a.contains(ab) && !a.classList.contains('open'); a.classList.toggle('open', open); a.querySelector('.acct-btn').setAttribute('aria-expanded', open); });
      const bg = t.closest('.burger');
      if (bg) { const tb = bg.closest('.topbar'); tb.classList.toggle('menu-open'); bg.setAttribute('aria-expanded', tb.classList.contains('menu-open')); }
    });
    document.addEventListener('input', e => {
      if (!e.target.dataset.hsl) return;
      const v = [0, 1, 2].map(i => document.getElementById('hsl' + i).value);
      const c = `hsl(${v[0]} ${v[1]}% ${v[2]}%)`;
      e.target.nextElementSibling.textContent = e.target.value + (e.target.dataset.hsl === '0' ? '°' : '%');
      setAccent(c); document.getElementById('acc-val').textContent = c;
      document.querySelectorAll('.pal button').forEach(p => p.setAttribute('aria-pressed', false));
    });
    document.addEventListener('keydown', e => { if (e.key === 'Escape') { document.querySelectorAll('.acct.open').forEach(a => a.classList.remove('open')); if (location.hash) location.hash = ''; } });
  });
})();
