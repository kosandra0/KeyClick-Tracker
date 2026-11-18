/* KeyClick Tracker 2.0.0 interface additions. Loaded after pages.js and analytics.js.
   It extends the existing pages through small hooks instead of rewriting them. */
(function () {
  'use strict';
  const { $, $$, state, fmt, esc, api, toast } = KC;
  const pages = KC.pages;
  const S = () => state.settings || {};

  /* ---------- small builders ---------- */
  const sw = (id, on) => `<input type="checkbox" class="switch" id="${id}" ${on ? 'checked' : ''}>`;
  const row = (title, help, control) => `<div class="setting-row"><div><div class="setting-title">${title}</div>${help ? `<div class="setting-help">${help}</div>` : ''}</div>${control || ''}</div>`;
  const panel = (title, hint, body, id) => `<section class="panel section-gap"${id ? ` id="${id}"` : ''} style="--i:4"><div class="panel-head"><div><div class="panel-title">${title}</div>${hint ? `<div class="panel-hint">${hint}</div>` : ''}</div></div>${body}</section>`;
  const num = (id, v, min, max, step) => `<input class="text-input num-input" type="number" min="${min}" max="${max}" step="${step || 1}" id="${id}" value="${v}">`;
  const sel = (id, opts, cur) => `<select class="select-control" id="${id}">${opts.map(([v, l]) => `<option value="${v}"${cur === v ? ' selected' : ''}>${l}</option>`).join('')}</select>`;
  const appendPanels = (h, extra) => h.replace(/<\/div>\s*$/, extra + '</div>');
  const dur = m => { m = Math.round(m); const h = Math.floor(m / 60); return h ? `${h} h ${m % 60} m` : `${m} m`; };

  /* Wrap a page's html / mount / update / unmount without replacing them. */
  function hook(name, o) {
    const p = pages[name]; if (!p) return;
    const html0 = p.html, mount0 = p.mount, update0 = p.update, unmount0 = p.unmount;
    if (o.html) p.html = function () { return o.html(html0.call(this)); };
    if (o.mount) p.mount = function (root) { if (mount0) mount0.call(this, root); o.mount(root); };
    if (o.update) p.update = function (root) { if (update0) update0.call(this, root); o.update(root); };
    if (o.unmount) p.unmount = function () { if (unmount0) unmount0.call(this); o.unmount(); };
  }

  /* ---------- Settings: reminders, goals, backups, reports, live box style, quiet hours, insights ---------- */
  function settingsExtra() {
    const s = S();
    const preset = ['compact', 'detailed', 'sparkline'].includes(s.overlay_preset) ? s.overlay_preset : 'detailed';
    return panel('Break reminders', 'Gentle reminders to rest. They use the tray notification and never appear during idle time or while a full-screen program is in front.',
      row('Reminder style', 'Simple: one reminder after the break minutes set above. Cycle: a work block, then a rest, repeated.', sel('v2-remmode', [['', 'Simple'], ['cycle', 'Work and rest cycle']], s.reminder_mode === 'cycle' ? 'cycle' : '')) +
      row('Work block (minutes)', 'Used by the cycle style.', num('v2-work', s.work_minutes || 50, 10, 180)) +
      row('Rest (minutes)', 'A pause this long counts as a rest and starts the next cycle.', num('v2-rest', s.rest_minutes || 10, 1, 60)) +
      row('Snooze', 'After a reminder, the tray menu offers "Remind me in 5 minutes".', '')) +
    panel('Typing speed goal', 'Shown with your daily goals. Compared with your average speed for the day.',
      row('Daily typing speed goal (WPM)', '0 turns it off.', num('v2-gwpm', s.goal_wpm || 0, 0, 300))) +
    panel('Backups and summaries', 'Backups and written summaries of your day or week. Nothing is sent anywhere.',
      row('Daily backups to keep', 'One backup a day is kept for this many days (7 to 90). A copy is also made every six hours while the app runs (the latest eight are kept). Other backups are never removed.', num('v2-bkdays', s.backup_days || 14, 7, 90)) +
      row('Summary', 'A short report with totals, your best hour, top programs and the change from the period before.', sel('v2-rep', [['', 'Off'], ['daily', 'Every evening at 18:00 (today so far)'], ['weekly', 'Every Monday at 09:00 (last week)']], s.report_mode || '')) +
      row('Also save the summary as a file', 'Saved as .md in the Backups\\Reports folder. A file is never overwritten.', sw('v2-repfile', !!s.report_file)) +
      row('Make a summary now', '', '<div class="header-controls"><button class="btn" data-v2="report-day">Today so far</button><button class="btn" data-v2="report-week">Last week</button></div>')) +
    panel('Live box style', 'Compact, detailed, or with a 60-second activity line. Each style remembers its own size. Right-click the live box to switch to the next style.',
      row('Style', '', `<div class="segmented" id="v2-preset">${['compact', 'detailed', 'sparkline'].map(p => `<button data-preset="${p}" class="${p === preset ? 'active' : ''}">${p[0].toUpperCase() + p.slice(1)}</button>`).join('')}</div>`)) +
    panel('Quiet hours', 'Counting is paused between these times. Nothing is deleted. Useful for nights or breaks you do not want measured.',
      row('Pause counting during quiet hours', '', sw('v2-quiet', !!s.quiet_on)) +
      row('From', '', `<input class="text-input" type="time" id="v2-qfrom" value="${esc(s.quiet_from || '23:00')}">`) +
      row('Until', '', `<input class="text-input" type="time" id="v2-qto" value="${esc(s.quiet_to || '07:00')}">`)) +
    panel('Insights', 'Optional extras. Only counts are used, never what you type.',
      row('Busy-stretch notes', 'A suggestion when your busiest 10 minutes are unusually high compared with your own last 30 days. It is a suggestion, never a diagnosis. It needs 14 days of data first.', sw('v2-strain', !!s.strain_on)) +
      row('Also show it once a day in the tray', '', sw('v2-strainnote', !!s.strain_notify)) +
      row('Count keys by keyboard layout', 'Shows which layouts you type on (Keyboard page). Only the layout name is kept.', sw('v2-layout', !s.layout_off)));
  }
  function bindSettings(root) {
    const numSet = (id, key, lo, hi, msg) => { const el = $('#' + id, root); if (!el) return; el.onchange = () => { let v = Math.round(+el.value); if (!isFinite(v)) v = lo; v = Math.min(hi, Math.max(lo, v)); el.value = v; KC.saveSettings({ [key]: v }); if (msg) toast(msg(v)); }; };
    const chkSet = (id, fn) => { const el = $('#' + id, root); if (el) el.onchange = () => fn(el.checked); };
    const rm = $('#v2-remmode', root); if (rm) rm.onchange = () => { KC.saveSettings({ reminder_mode: rm.value }); toast(rm.value === 'cycle' ? 'Work and rest cycle is on.' : 'Simple reminders are on.'); };
    numSet('v2-work', 'work_minutes', 10, 180);
    numSet('v2-rest', 'rest_minutes', 1, 60);
    numSet('v2-gwpm', 'goal_wpm', 0, 300, v => v ? `Typing speed goal: ${v} WPM.` : 'Typing speed goal is off.');
    numSet('v2-bkdays', 'backup_days', 7, 90, v => `Keeping ${v} daily backups.`);
    const rep = $('#v2-rep', root); if (rep) rep.onchange = () => { KC.saveSettings({ report_mode: rep.value }); toast(rep.value ? 'Summaries are on.' : 'Summaries are off.'); };
    chkSet('v2-repfile', on => KC.saveSettings({ report_file: on }));
    $$('#v2-preset button', root).forEach(b => b.onclick = () => { $$('#v2-preset button', root).forEach(x => x.classList.toggle('active', x === b)); KC.saveSettings({ overlay_preset: b.dataset.preset }); toast(`Live box style: ${b.dataset.preset}.`); });
    chkSet('v2-quiet', on => { KC.saveSettings({ quiet_on: on }); toast(on ? 'Quiet hours are on.' : 'Quiet hours are off.'); setTimeout(KC.refreshStatus, 300); });
    const qf = $('#v2-qfrom', root), qt = $('#v2-qto', root);
    if (qf) qf.onchange = () => { if (qf.value) KC.saveSettings({ quiet_from: qf.value }); };
    if (qt) qt.onchange = () => { if (qt.value) KC.saveSettings({ quiet_to: qt.value }); };
    chkSet('v2-strain', on => { KC.saveSettings({ strain_on: on }); toast(on ? 'Busy-stretch notes are on.' : 'Busy-stretch notes are off.'); });
    chkSet('v2-strainnote', on => KC.saveSettings({ strain_notify: on }));
    chkSet('v2-layout', on => { KC.saveSettings({ layout_off: !on }); toast(on ? 'Counting by keyboard layout is on.' : 'Counting by keyboard layout is off.'); });
  }
  hook('settings', { html: h => appendPanels(h, settingsExtra()), mount: bindSettings });

  /* ---------- v2.1.0: app logo. The chosen logo is used in the app, and the native side
     (tray icon, taskbar and window icon) follows the same setting. ---------- */
  let logoList = null;
  KC.logoSrc = function () { const id = (state.settings || {}).app_logo; return id ? '/ui/logos/' + encodeURIComponent(id) : '/ui/logo.png'; };
  KC.applyLogo = function () {
    const src = KC.logoSrc();
    $$('.brand-logo, .about-hero img').forEach(img => { if (img.getAttribute('src') !== src) img.setAttribute('src', src); });
    const link = document.querySelector('link[rel="icon"]');
    if (link && link.getAttribute('href') !== src) link.setAttribute('href', src);
  };
  async function loadLogoList() {
    if (logoList) return logoList;
    try { logoList = await (await fetch('/ui/logos/list.json', { cache: 'no-store' })).json(); } catch (e) { logoList = []; }
    return logoList;
  }
  const logoCard = (id, label, cur) => `<button class="logo-card${cur === id ? ' active' : ''}" data-logo="${esc(id)}" title="${esc(label)}"><img src="${id ? '/ui/logos/' + encodeURIComponent(id) : '/ui/logo.png'}" alt=""><span>${esc(label)}</span></button>`;
  function logoPanel() {
    return `<section class="panel section-gap" style="--i:5"><div class="panel-head"><div><div class="panel-title">App logo</div><div class="panel-hint">Choose the logo for the app, the tray icon, the taskbar, Task Manager and the Start menu. The app, the tray and the taskbar change at once. Task Manager and the Start menu change when the app restarts.</div></div></div><div id="v2-logo-note"></div><div id="v2-logos"><div class="muted">Loading logos…</div></div></section>`;
  }
  /* Task Manager and the Start menu read the icon of the program file. Changing it needs the helper
     restart, so tell the person when the file still carries another logo. */
  async function checkLogoPending(root) {
    const box = $('#v2-logo-note', root); if (!box) return;
    let pending = false;
    try { pending = !!(await api('/api/logo/state')).pending; } catch (e) {}
    if (!pending) { box.innerHTML = ''; return; }
    box.innerHTML = '<div class="setting-row"><div><div class="setting-title">Finish applying the logo</div><div class="setting-help">The app, the tray and the taskbar already show your logo. Task Manager and the Start menu read the icon of the program file, which changes when the app restarts. It takes a few seconds and your counts are saved first.</div></div><button class="btn primary" id="v2-logo-restart">Restart now</button></div>';
    $('#v2-logo-restart', box).onclick = async () => {
      toast('Restarting to apply the logo…');
      try { await api('/api/restart', { method: 'POST', body: JSON.stringify({ icon: true }) }); } catch (e) { toast('Could not restart: ' + e.message); }
    };
  }
  function renderLogos(root) {
    const box = $('#v2-logos', root); if (!box) return;
    const cur = S().app_logo || '';
    const groups = {};
    (logoList || []).forEach(l => { (groups[l.group] = groups[l.group] || []).push(l); });
    let html = '<div class="logo-grid">' + logoCard('', 'Default', cur) + '</div>';
    Object.keys(groups).forEach(g => {
      html += `<div class="logo-group">${esc(g)}</div><div class="logo-grid">` + groups[g].map(l => logoCard(l.id, l.label, cur)).join('') + '</div>';
    });
    box.innerHTML = html;
    $$('.logo-card', box).forEach(b => b.onclick = () => {
      const id = b.dataset.logo || '';
      $$('.logo-card', box).forEach(x => x.classList.toggle('active', x === b));
      KC.saveSettings({ app_logo: id });
      KC.applyLogo();
      toast(id ? 'App logo changed.' : 'Default app logo restored.');
      setTimeout(() => checkLogoPending(root), 400);
    });
    checkLogoPending(root);
  }
  hook('appearance', {
    html: h => appendPanels(h, logoPanel()),
    mount: root => { loadLogoList().then(() => renderLogos(root)); },
  });
  if (!document.getElementById('v2-logo-style')) {
    const st = document.createElement('style');
    st.id = 'v2-logo-style';
    st.textContent = '.logo-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(118px,1fr));gap:12px}' +
      '.logo-group{margin:16px 0 8px;font-size:13px;color:var(--muted);letter-spacing:.02em}' +
      '.logo-card{border:1px solid var(--line);background:var(--panel2);border-radius:16px;padding:10px;cursor:pointer;color:var(--ink);font:inherit;text-align:center;transition:box-shadow .2s,border-color .2s}' +
      '.logo-card img{width:100%;aspect-ratio:1;border-radius:22%;display:block}' +
      '.logo-card span{display:block;margin-top:6px;font-size:12px;color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}' +
      '.logo-card.active{border-color:var(--accent);box-shadow:0 0 0 2px var(--accent)}';
    document.head.appendChild(st);
  }

  /* ---------- v2.2.0: Lite mode, and idling while the window is hidden ---------- */
  KC.liteAtStart = null; // whether Lite mode was on when the app started (the lighter engine only changes at a start)
  KC.applyLite = function () {
    const on = !!(state.settings || {}).lite_mode;
    if (KC.liteAtStart === null) KC.liteAtStart = on;
    document.documentElement.classList.toggle('lite', on);
  };
  /* The app calls this when the window is hidden to the tray, minimised, or shown again. */
  window.kcVisible = function (v) {
    KC.hidden = !v;
    document.documentElement.classList.toggle('kc-hidden', !v);
    if (v && KC.refreshBindings) { try { KC.refreshBindings(); } catch (e) {} }
  };
  if (!document.getElementById('v2-lite-style')) {
    const st2 = document.createElement('style');
    st2.id = 'v2-lite-style';
    st2.textContent = 'html.lite *,html.lite *::before,html.lite *::after{animation:none !important;transition:none !important;backdrop-filter:none !important;-webkit-backdrop-filter:none !important;filter:none !important;box-shadow:none !important;text-shadow:none !important;scroll-behavior:auto !important}' +
      'html.kc-hidden *,html.kc-hidden *::before,html.kc-hidden *::after{animation-play-state:paused !important}' +
      '.setting-row[hidden]{display:none !important}';
    document.head.appendChild(st2);
  }
  function litePanel() {
    const s = S();
    return panel('Lite mode', 'For older or slower computers. It lowers processor and memory use. Counting is not affected.',
      row('Lite mode', 'Turns off animations and visual effects at once, and uses a lighter browser engine after the app restarts.', sw('v2-lite', !!s.lite_mode)) +
      `<div class="setting-row" id="v2-lite-restart"${!!s.lite_mode !== KC.liteAtStart ? '' : ' hidden'}><div><div class="setting-title">Restart to finish</div><div class="setting-help">The lighter browser engine starts with the next start of the app. Your counts are saved first.</div></div><button class="btn primary" id="v2-lite-restart-btn">Restart now</button></div>`);
  }
  function bindLite(root) {
    const on = $('#v2-lite', root); if (!on) return;
    on.onchange = () => {
      KC.saveSettings({ lite_mode: on.checked });
      KC.applyLite();
      const r = $('#v2-lite-restart', root); if (r) r.hidden = (on.checked === KC.liteAtStart);
      toast(on.checked ? 'Lite mode is on.' : 'Lite mode is off.');
    };
    const b = $('#v2-lite-restart-btn', root);
    if (b) b.onclick = async () => {
      toast('Restarting…');
      try { await api('/api/restart', { method: 'POST', body: JSON.stringify({}) }); } catch (e) { toast('Could not restart: ' + e.message); }
    };
  }
  /* Placed after the first panel so it is easy to find. */
  hook('settings', { html: h => h.replace('</section>', '</section>' + litePanel()), mount: bindLite });

  /* ---------- reports (buttons are used on Settings) ---------- */
  async function showReport(period) {
    try {
      const r = await api(`/api/reports/now?period=${period}`, { method: 'POST' });
      const go = await KC.modal({ title: r.title, body: `<pre class="selectable v2-pre">${esc(r.text)}</pre>`, actions: [{ label: 'Close', value: false }, { label: 'Save as file', value: true, primary: true }] });
      if (go) { const s = await api(`/api/reports/now?period=${period}&save=1`, { method: 'POST' }); toast(`Saved ${s.saved} in Backups\\Reports.`); }
    } catch (e) { toast('Could not make the summary: ' + e.message); }
  }
  document.addEventListener('click', async e => {
    const b = e.target.closest && e.target.closest('[data-v2]'); if (!b) return;
    switch (b.dataset.v2) {
      case 'report-day': return showReport('day');
      case 'report-week': return showReport('week');
      case 'csv': return exportCsv();
    }
  });

  /* ---------- Keyboard: shortcuts and layouts ---------- */
  async function loadLayouts() {
    const box = $('#v2-lay'); if (!box) return;
    try {
      const r = await api('/api/layouts?' + KC.statsQuery());
      if (!r.tracking) { box.innerHTML = '<div class="empty-mini">Counting by layout is off. Turn it on in Settings.</div>'; return; }
      const tot = r.layouts.reduce((a, l) => a + l.keys, 0);
      box.innerHTML = (r.layouts.length ? r.layouts.map(l => `<div class="file-row"><span class="selectable">${esc(l.name)}</span><span class="muted">${fmt(l.keys)} keys · ${tot ? Math.round(100 * l.keys / tot) : 0}%</span></div>`).join('') : '<div class="empty-mini">Nothing recorded for this period yet.</div>') +
        (r.current ? `<div class="panel-hint" style="margin-top:8px">In use now: ${esc(r.current)}.</div>` : '');
    } catch (e) { box.innerHTML = '<div class="empty-mini">Could not load this card.</div>'; }
  }
  hook('keyboard', {
    html: h => appendPanels(h, panel('Shortcuts', 'Key presses made while holding Ctrl, Alt or the Windows key. Shift alone does not count, because it is used for capital letters. Keyboards where AltGr sends Ctrl+Alt are not counted as shortcuts.', '<div id="v2-short"></div>') + panel('Keyboard layouts', 'Which keyboard layouts you typed on in this period. Only the layout name is kept.', '<div id="v2-lay"><div class="empty-mini">Loading…</div></div>')),
    mount: () => loadLayouts(),
    update: root => {
      const el = $('#v2-short', root), t = (state.stats || {}).totals; if (!el || !t) return;
      const typing = Math.max(0, (t.keys || 0) - (t.modKeys || 0)), pct = typing ? Math.round(1000 * (t.shortcuts || 0) / typing) / 10 : 0;
      const html = `<div class="kv"><div class="k">Shortcuts</div><div class="v">${fmt(t.shortcuts || 0)}</div><div class="k">Share of your keys</div><div class="v" title="Shortcuts divided by key presses, not counting Ctrl, Alt and Win themselves">${pct}%</div><div class="k">Ctrl, Alt and Win presses</div><div class="v">${fmt(t.modKeys || 0)}</div></div>`;
      if (el.dataset.h !== html) { el.dataset.h = html; el.innerHTML = html; }
    }
  });

  /* ---------- Overview: goal history, today by hour, busy-stretch note ---------- */
  const CELL = { today: 'var(--panel2)', met: 'var(--green)', partly: 'var(--amber)', missed: 'color-mix(in srgb,var(--red) 60%,var(--panel2))', nodata: 'var(--line)', none: 'var(--line)' };
  const isToday = d => d.date === today() && d.state !== 'met';   // today is still in progress, not missed
  async function loadGoalHistory() {
    const box = $('#v2-goalbox'); if (!box) return;
    try {
      const r = await api('/api/goals?days=30');
      const s = S();
      box.innerHTML = `<div class="v2-strip">${r.days.map(d => `<i style="background:${isToday(d) ? CELL.today : (CELL[d.state] || CELL.nodata)};${isToday(d) ? 'outline:1px solid var(--accent);' : ''}" title="${esc(d.date)}: ${({ met: 'goals met', partly: 'some goals met', missed: 'goals missed', none: 'no goals set', nodata: 'no data' })[d.state] || ''}"></i>`).join('')}</div>` +
        `<div class="kv" style="margin-top:10px"><div class="k">Goal streak</div><div class="v">${r.streak} day${r.streak === 1 ? '' : 's'}</div><div class="k">Typing speed goal</div><div class="v">${s.goal_wpm ? s.goal_wpm + ' WPM' : 'off'}</div></div>` +
        `<div class="panel-hint" style="margin-top:8px"><span class="v2-dot" style="background:var(--green)"></span> met <span class="v2-dot" style="background:var(--amber)"></span> partly <span class="v2-dot" style="background:${CELL.missed}"></span> missed <span class="v2-dot" style="background:var(--line)"></span> no data. A changed goal never rewrites past days.</div>`;
    } catch (e) { box.innerHTML = '<div class="empty-mini">Could not load this card.</div>'; }
    const note = $('#v2-strain-note');
    if (note) {
      try {
        const r = await api('/api/strain');
        note.innerHTML = r.on && r.message ? `<div class="panel-hint" style="margin-top:6px">${esc(r.message)}</div>` : '';
      } catch (e) { note.innerHTML = ''; }
    }
  }
  async function loadTimeline() {
    const box = $('#v2-tlbox'); if (!box) return;
    const dEl = $('#v2-tldate'); const date = dEl && dEl.value ? dEl.value : '';
    try {
      const r = await api('/api/timeline' + (date ? '?date=' + encodeURIComponent(date) : ''));
      const peak = Math.max(1, r.peak);
      box.innerHTML = `<div class="v2-tl">${r.hours.map(h => `<div class="v2-tlc" title="${String(h.hour).padStart(2, '0')}:00 · ${fmt(h.keys)} keys, ${fmt(h.clicks)} clicks, ${fmt(h.scrolls)} scrolls"><i style="height:${Math.round(100 * h.total / peak)}%"></i><span>${h.hour % 3 === 0 ? h.hour : ''}</span></div>`).join('')}</div>` +
        `<div class="panel-hint" style="margin-top:6px">Idle time that day: ${dur(r.idle_min)}.</div>`;
    } catch (e) { box.innerHTML = '<div class="empty-mini">Could not load this card.</div>'; }
  }
  const today = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`; };
  let ovTimer = 0;
  hook('overview', {
    html: h => {
      const mine = panel('Goal history', 'The last 30 days: each square is a day. A square is green when every goal you set was met.', '<div id="v2-goalbox"><div class="empty-mini">Loading…</div></div><div id="v2-strain-note"></div>') +
        panel('Day timeline', 'Activity hour by hour. Pick another day to compare.', `<div style="margin-bottom:8px"><input class="date-control" type="date" id="v2-tldate" value="${today()}" max="${today()}"></div><div id="v2-tlbox"><div class="empty-mini">Loading…</div></div>`);
      const key = '<section class="panel section-gap" style="--i:6">';
      return h.includes(key) ? h.replace(key, mine + key) : appendPanels(h, mine);
    },
    mount: root => {
      loadGoalHistory(); loadTimeline();
      const d = $('#v2-tldate', root); if (d) d.onchange = loadTimeline;
      clearInterval(ovTimer); ovTimer = setInterval(() => { loadGoalHistory(); loadTimeline(); }, 60000);
    },
    unmount: () => clearInterval(ovTimer)
  });

  /* ---------- Data & Backup: CSV, displays, startup log ---------- */
  async function exportCsv() {
    try {
      const r = await api('/api/export/csv');
      const blob = await r.blob(), cd = r.headers.get('content-disposition') || '', m = /filename=([^;]+)/.exec(cd);
      if (await KC.saveFile(m ? m[1].trim() : 'keyclick-tracker-days.csv', blob)) toast('Exported CSV file.');
    } catch (e) { toast('Export failed: ' + e.message); }
  }
  async function loadDisplays() {
    const box = $('#v2-disp'); if (!box) return;
    try {
      const r = await api('/api/displays'), h = await api('/api/health');
      box.innerHTML = r.monitors.map((m, i) => `<div class="file-row"><span>Display ${i + 1}${m.primary ? ' (main)' : ''}</span><span class="muted">${m.w} × ${m.h} · ${m.scale_percent}% scale</span></div>`).join('') +
        `<div class="file-row"><span>Floating live box</span><span class="muted">${r.live_box.on ? (r.live_box.on_a_screen ? 'on, on a visible screen' : 'on, but off every screen (it will be brought back)') : 'off'}</span></div>` +
        (h.boot && h.boot.length ? `<div class="panel-hint" style="margin-top:10px">Start-up steps this session:</div>` + h.boot.map(b => `<div class="file-row"><span class="selectable">${esc(b.step)}</span><span class="muted">${esc(b.at)}</span></div>`).join('') : '');
    } catch (e) { box.innerHTML = '<div class="empty-mini">Could not load this card.</div>'; }
  }
  hook('data', {
    html: h => appendPanels(h, panel('Spreadsheet export (CSV)', 'Daily totals for a spreadsheet: date, keys, clicks, scrolls, words, active and idle minutes, shortcuts. This file is for reading only: it cannot be imported and is not a backup. Use JSON for backups.', '<button class="btn" data-v2="csv">Export daily totals (CSV)</button>') + panel('Displays and start-up', 'Your screens, and the start-up steps of this session. Useful when the live box or a hotkey does not appear where you expect.', '<div id="v2-disp"><div class="empty-mini">Loading…</div></div>')),
    mount: () => loadDisplays()
  });

  /* ---------- import preview (called by pages.js before importing) ---------- */
  KC.importPreviewModal = async function (text, mode, name) {
    let p;
    try { p = await api('/api/import/preview?mode=' + mode, { method: 'POST', body: text, headers: { 'Content-Type': 'text/plain' } }); }
    catch (e) { await KC.modal({ title: 'That file could not be read', body: `<p>${esc(e.message)}</p><p>Nothing was changed.</p>` }); return false; }
    const f = p.file || {}, c = p.current || {};
    let body = `<p><b>${esc(name)}</b> contains ${fmt(f.keys)} key presses, ${fmt(f.clicks)} clicks and ${fmt(f.scrolls)} scrolls over ${fmt(f.days)} days${f.from ? ` (${esc(f.from)} to ${esc(f.to)})` : ''}.</p><p>Right now you have ${fmt(c.keys)} key presses and ${fmt(c.clicks)} clicks over ${fmt(c.days)} days.</p>`;
    if (mode === 'replace') body += `<p><b>Replace:</b> your current data is swapped for the file. ${p.daysOnlyInCurrent ? `<b>${fmt(p.daysOnlyInCurrent)} day${p.daysOnlyInCurrent === 1 ? '' : 's'}</b> that are only in your current data will no longer be in the main history (they stay in the safety backup).` : 'No day is lost.'} A safety backup of your current data is saved first.</p>`;
    else body += `<p><b>Merge:</b> ${fmt(p.hoursAdded)} new hours would be added, ${fmt(p.hoursUpdated)} hours updated and ${fmt(p.daysApplied)} days of speed data applied. Nothing is double-counted. A safety backup is saved first.</p>`;
    return !!(await KC.modal({ title: mode === 'replace' ? 'Replace all current data?' : 'Import this file?', body, actions: [{ label: 'Cancel', value: false }, { label: mode === 'replace' ? 'Replace data' : 'Import', value: true, primary: true, danger: mode === 'replace' }] }));
  };

  /* ---------- quiet hours in the status pill ---------- */
  const rs = KC.refreshStatus;
  KC.refreshStatus = async function () {
    await rs.apply(this, arguments);
    const s = state.status; if (s && s.quiet && !s.paused) { const t = $('#status-text'); if (t) t.textContent = 'Quiet hours'; }
  };
  setInterval(() => { KC.refreshStatus(); }, 30000);

  /* ---------- Practice page (F8): lessons are scored in this page; only scores are saved ---------- */
  const LESSONS = [
    ['home', 'Home row', 'asdf jkl; asdf jkl; fall sad lads flask dad ask fads gas flags'],
    ['words', 'Common words', 'the of and to in is you that it he was for on are as with his they at be this from'],
    ['sentence', 'Short sentences', 'The quick brown fox jumps over the lazy dog. Pack my box with five dozen liquor jugs.'],
    ['numbers', 'Numbers', '1234 5678 9012 3456 7890 2024 1999 3.14 42 100 2500 360 87 910'],
    ['punct', 'Punctuation', 'Hello, world! Wait: is this right? Yes; it is. (Really) "Quote" it\'s fine - truly.'],
    ['para1', 'Paragraph: a quiet morning', 'The morning was quiet. A soft light came through the window, and the town slowly woke up. Somewhere a kettle began to sing, and a bicycle rattled past on the wet road.'],
    ['para2', 'Paragraph: good habits', 'Small habits add up. Ten minutes of careful practice every day is worth more than one long session once a month. Keep your wrists relaxed, your back straight, and your eyes on the text.'],
    ['code', 'Brackets and symbols', 'if (a > b) { return a + b * 2; } else { return [a, b]; } // done']
  ];
  const pr = { lesson: LESSONS[0], t0: 0, typed: '', total: 0, errors: 0, done: false };
  function prRender() {
    const box = $('#pr-text'); if (!box) return;
    const text = pr.lesson[2], v = pr.typed;
    box.innerHTML = [...text].map((ch, i) => `<span class="tc ${i < v.length ? (v[i] === ch ? 'ok' : 'bad') : i === v.length ? 'cur' : ''}">${ch === ' ' ? '&nbsp;' : esc(ch)}</span>`).join('');
    const secs = pr.t0 ? Math.max(1, (Date.now() - pr.t0) / 1000) : 0;
    let ok = 0; for (let i = 0; i < v.length; i++) if (v[i] === text[i]) ok++;
    const wpm = secs ? (ok / 5) / (secs / 60) : 0, acc = pr.total ? 100 * (pr.total - pr.errors) / pr.total : 100;
    const live = $('#pr-live'); if (live) live.textContent = `${wpm.toFixed(0)} WPM · ${acc.toFixed(0)}% accuracy · ${pr.errors} mistake${pr.errors === 1 ? '' : 's'}`;
    const bar = $('#pr-bar'); if (bar) bar.style.width = Math.round(100 * v.length / text.length) + '%';
  }
  function prReset(keepFocus) {
    pr.t0 = 0; pr.typed = ''; pr.total = 0; pr.errors = 0; pr.done = false;
    const ta = $('#pr-input'); if (ta) { ta.value = ''; ta.disabled = false; if (keepFocus) ta.focus(); }
    const res = $('#pr-result'); if (res) res.innerHTML = '';
    prRender();
  }
  async function prFinish() {
    pr.done = true;
    const ta = $('#pr-input'); if (ta) ta.disabled = true;
    const text = pr.lesson[2], secs = Math.max(1, Math.round((Date.now() - pr.t0) / 1000));
    let ok = 0; for (let i = 0; i < pr.typed.length; i++) if (pr.typed[i] === text[i]) ok++;
    const wpm = Math.round(10 * (ok / 5) / (secs / 60)) / 10, acc = Math.round(10 * (pr.total ? 100 * (pr.total - pr.errors) / pr.total : 100)) / 10;
    const res = $('#pr-result');
    if (res) res.innerHTML = `<div class="panel" style="margin-top:12px"><div class="panel-title">Lesson finished</div><div class="kv" style="margin-top:8px"><div class="k">Speed</div><div class="v">${wpm} WPM</div><div class="k">Accuracy</div><div class="v">${acc}%</div><div class="k">Mistakes</div><div class="v">${pr.errors}</div><div class="k">Time</div><div class="v">${secs} s</div></div><div class="header-controls" style="margin-top:10px"><button class="btn primary" data-v2p="again">Try again</button><button class="btn" data-v2p="next">Next lesson</button></div></div>`;
    if (wpm > 300) { toast('That was faster than anyone can type, so the score was not saved. Type the lesson yourself to get a score.'); return; }
    try { await api('/api/practice', { method: 'POST', body: JSON.stringify({ lesson: pr.lesson[0], wpm, acc, errors: pr.errors, secs }) }); prHistory(); }
    catch (e) { toast('The score could not be saved: ' + e.message); }
  }
  async function prHistory() {
    const box = $('#pr-hist'); if (!box) return;
    try {
      const r = await api('/api/practice');
      if (!r.count) { box.innerHTML = '<div class="empty-mini">No lessons finished yet. Your scores appear here.</div>'; return; }
      const pts = r.scores.map(s => s.wpm), max = Math.max(10, ...pts), W = 300, H = 60;
      const line = pts.length > 1 ? pts.map((v, i) => `${(i * W / (pts.length - 1)).toFixed(1)},${(H - 4 - (H - 8) * v / max).toFixed(1)}`).join(' ') : '';
      const name = id => (LESSONS.find(l => l[0] === id) || [0, id])[1];
      box.innerHTML = `<div class="kv"><div class="k">Lessons finished</div><div class="v">${fmt(r.count)}</div><div class="k">Best speed</div><div class="v">${r.best_wpm.toFixed(1)} WPM</div><div class="k">Average accuracy</div><div class="v">${r.avg_acc.toFixed(1)}%</div></div>` +
        (line ? `<svg class="v2-spark" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="Speed of your recent lessons"><polyline points="${line}" fill="none" stroke="var(--accent)" stroke-width="2" vector-effect="non-scaling-stroke"/></svg>` : '') +
        `<div style="margin-top:8px">${r.scores.slice(-8).reverse().map(s => `<div class="file-row"><span>${esc(name(s.lesson))}</span><span class="muted">${s.wpm.toFixed(1)} WPM · ${s.acc.toFixed(1)}% · ${new Date(s.at).toLocaleDateString()}</span></div>`).join('')}</div>`;
    } catch (e) { box.innerHTML = '<div class="empty-mini">Could not load your scores.</div>'; }
  }
  pages.practice = {
    html() {
      return '<div class="page-wrap">' + KC.pg.header('PRACTICE', 'Typing practice', 'Short lessons that score your speed and accuracy. Only the scores are saved: what you type here is never stored.', '') +
        panel('Lesson', 'Pick a lesson, click the box below and start typing. The timer starts with your first key.',
          `<div class="v2-lessons">${LESSONS.map(l => `<button class="btn v2-lesson${l === pr.lesson ? ' active' : ''}" data-lesson="${l[0]}">${esc(l[1])}</button>`).join('')}</div>` +
          `<div class="v2-textbox" id="pr-text" aria-label="Lesson text"></div><div class="v2-bar"><i id="pr-bar"></i></div>` +
          `<textarea class="text-input v2-input" id="pr-input" rows="3" spellcheck="false" autocomplete="off" autocapitalize="off" placeholder="Click here and start typing…" aria-label="Type the lesson text here"></textarea>` +
          `<div class="panel-hint" id="pr-live" style="margin-top:8px"></div><div id="pr-result"></div>`, 'pr-panel') +
        panel('Your progress', 'Speed of your recent lessons and your best scores.', '<div id="pr-hist"><div class="empty-mini">Loading…</div></div>') + '</div>';
    },
    mount(root) {
      const ta = $('#pr-input', root);
      ta.addEventListener('paste', e => e.preventDefault());
      ta.addEventListener('keydown', e => { if (e.key === 'Escape') { prReset(true); e.preventDefault(); } });
      ta.addEventListener('input', () => {
        if (pr.done) return;
        const text = pr.lesson[2];
        let v = ta.value.slice(0, text.length);
        if (!pr.t0 && v.length) pr.t0 = Date.now();
        if (v.length > pr.typed.length) for (let i = pr.typed.length; i < v.length; i++) { pr.total++; if (v[i] !== text[i]) pr.errors++; }
        pr.typed = v; if (ta.value !== v) ta.value = v;
        prRender();
        if (v.length >= text.length) prFinish();
      });
      prReset(false); prHistory();
    }
  };
  document.addEventListener('click', e => {
    const l = e.target.closest && e.target.closest('[data-lesson]');
    if (l) { pr.lesson = LESSONS.find(x => x[0] === l.dataset.lesson) || LESSONS[0]; $$('.v2-lesson').forEach(b => b.classList.toggle('active', b === l)); prReset(true); return; }
    const a = e.target.closest && e.target.closest('[data-v2p]');
    if (a) { if (a.dataset.v2p === 'next') { pr.lesson = LESSONS[(LESSONS.indexOf(pr.lesson) + 1) % LESSONS.length]; $$('.v2-lesson').forEach(b => b.classList.toggle('active', b.dataset.lesson === pr.lesson[0])); } prReset(true); }
  });
})();
