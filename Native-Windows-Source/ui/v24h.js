/* KeyClick Tracker 2.4.0, part 2: lists whose rows are never rebuilt, and the History page.
   Loaded after v24.js, which provides the live number slots (KC.v24).

   - Keyboard "Most used keys" and the Mouse rows used to be redrawn by the page every time a key was
     pressed, so their numbers vanished and came back. They are drawn here instead: every row stays in
     place, new rows rise in, rows that change order slide, and every number rolls where it is.
   - History: hold time for every period, a day panel for the day you pick in the calendar, and the
     input history as rows that slide in as new holds are recorded. */
(function () {
  'use strict';
  const V = KC.v24; if (!V) return;
  const { $, $$, state, fmt, esc, api } = KC;
  const { S, SEP, EXT, LIVE, slot, fillAll, withService, reconcile, pollData, ms, avg, sumMs, cntN, btnName, dirName, estBadge, when, label, appLabel, icon, setHTML } = V;
  const P = KC.pg, pages = KC.pages;
  const st = () => P.S();
  const pad2 = n => String(n).padStart(2, '0');
  const localKey = d => d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate());

  /* ================= slots for these pages ================= */
  let seriesRef = null, seriesMap = new Map();
  const seriesPoint = key => {
    const arr = st().series || [];
    if (arr !== seriesRef || seriesMap.size !== arr.length) { seriesRef = arr; seriesMap = new Map(arr.map(p => [p.key, p])); }
    return seriesMap.get(key);
  };
  const mouseCount = ids => ids.split('+').reduce((s, id) => { const m = (st().mouse || []).find(x => x.id === id); return s + (m ? m.count : 0); }, 0);
  function rval(p) {                       // R¦<period>¦field: a row of the By period table
    const pt = seriesPoint(p[1]), f = p[2]; if (!pt) return null;
    switch (f) {
      case 'k': return fmt(pt.keys); case 'c': return fmt(pt.clicks); case 's': return fmt(pt.scrolls); case 't': return fmt(pt.events);
    }
    const d = LIVE.data && LIVE.data.series; if (!d || d.hourly) return '–';
    const r = (d.rows || {})[p[1]] || {}, a = d.now_key === p[1] && LIVE.addX && LIVE.addX.series;   // the running period also counts what is held right now
    if (f === 'kh') { const v = (r.k_ms || 0) + (a ? a.kTot : 0); return v || r.k_n ? ms(v) : '–'; }
    if (f === 'mh') { const v = (r.m_ms || 0) + (a ? a.mTot : 0); return v || r.m_n ? ms(v) : '–'; }
    if (f === 'sh') { const v = (r.s_ms || 0) + (a ? a.scrMs : 0); return v || r.s_n || (a && a.scrN) ? ms(v) : '–'; }
    return null;
  }
  const isToday = () => HS.sel === localKey(new Date());
  function dval(p) {                       // D¦…: the day panel
    const d = LIVE.data && LIVE.data.day; if (!d || d.date !== HS.sel) return null;
    const a = isToday() && LIVE.addX && LIVE.addX.day, f = p[1];
    switch (f) {
      case 'k': return fmt(d.keys || 0); case 'c': return fmt(d.clicks || 0); case 's': return fmt(d.scrolls || 0);
      case 'kh': { const v = sumMs(d.key_hold) + (a ? a.kTot : 0); return v || cntN(d.key_hold) ? ms(v) : '–'; }
      case 'mh': { const v = sumMs(d.mouse_hold) + (a ? a.mTot : 0); return v || cntN(d.mouse_hold) ? ms(v) : '–'; }
      case 'sh': { const sc = d.scroll, n = (sc ? sc.n : 0) + (a ? a.scrN : 0); return n ? ms((sc ? sc.ms : 0) + (a ? a.scrMs : 0)) : '–'; }
      case 'K': {                          // D¦K¦<key>¦h|n
        const h = (d.key_hold || {})[p[2]], live = a ? a.k[p[2]] || 0 : 0;
        if (p[3] === 'h') return h || live ? ms((h ? h.ms : 0) + live) : '–';
        if (p[3] === 'n') return fmt((h ? h.n : 0) + (a ? a.kN[p[2]] || 0 : 0));
        return null;
      }
      case 'A': {                          // D¦A¦<program>¦k|kh|mh
        const app = (d.apps || []).find(x => x.name === p[2]); if (!app) return null;
        const x = a && a.app[p[2]];
        if (p[3] === 'k') return fmt(app.keys);
        if (p[3] === 'kh') return app.k_ms || (x && x.k) ? ms(app.k_ms + (x ? x.k : 0)) : '–';
        if (p[3] === 'mh') return app.m_ms || (x && x.m) ? ms(app.m_ms + (x ? x.m : 0)) : '–';
      }
    }
    return null;
  }
  EXT.val = function (p) {
    switch (p[0]) {
      case 'K': {                          // K¦<key>¦c|s: presses and share, straight from the live counters
        const k = (st().keys || []).find(x => x.id === p[1]);
        if (p[2] === 'c') return fmt(k ? k.count : 0);
        if (p[2] === 's') return (100 * (k ? k.count : 0) / Math.max(1, (st().totals || {}).keys || 0)).toFixed(1) + '%';
        return null;
      }
      case 'B': return p[2] === 'c' ? fmt(mouseCount(p[1])) : null;
      case 'C': {                          // C¦up|down|left|right|notches
        if (p[1] === 'notches') return fmt((st().totals || {}).scrollDistance || 0);
        const x = (st().scroll || []).find(y => y.id === p[1]); return fmt(x ? x.count : 0);
      }
      case 'R': return rval(p);
      case 'D': return dval(p);
    }
    return null;
  };
  EXT.bar = function (name) {
    const p = name.split(SEP);
    if (p[0] === 'KC') { const keys = st().keys || [], top = Math.max(1, ...keys.map(k => k.count)), k = keys.find(x => x.id === p[1]); return Math.max(2, Math.round(100 * (k ? k.count : 0) / top)); }
    if (p[0] === 'R') { const arr = st().series || [], top = Math.max(1, ...arr.map(x => x.events)), pt = seriesPoint(p[1]); return pt ? Math.max(2, Math.round(100 * pt.events / top)) : 0; }
    if (p[0] === 'DK') { const d = LIVE.data && LIVE.data.day; if (!d) return 0; const hs = d.key_hold || {}, top = Math.max(1, ...Object.keys(hs).map(k => hs[k].ms)); return hs[p[1]] ? Math.max(2, Math.round(100 * hs[p[1]].ms / top)) : 0; }
    return null;
  };

  /* ================= Keyboard: Most used keys ================= */
  const TK = { sig: null };
  function renderTopKeys(root) {
    const host = $('#top-keys', root); if (!host) return;
    let my = $('#top-keys2', root);
    if (!my) {
      host.style.display = 'none';
      host.insertAdjacentHTML('afterend', '<div class="gt gt-keys" id="top-keys2"><div class="gt-head"><span>Key</span><span class="num">Presses</span><span class="num">Held</span><span class="num">Average</span><span class="num">Share</span><span></span></div><div class="gt-body" id="top-keys-body"></div></div>');
      my = $('#top-keys2', root); TK.sig = null;
    }
    const keys = (st().keys || []).slice().sort((a, b) => b.count - a.count || a.id.localeCompare(b.id)).slice(0, 15);
    const sig = keys.map(k => k.id).join('|'); if (sig === TK.sig) return; TK.sig = sig;
    reconcile($('#top-keys-body', my), keys.map(k => ({ key: k.id, html: '<div class="gt-row"><span><span class="key-chip">' + esc(label(k.id)) + '</span></span><span class="num">' + slot(S('K', k.id, 'c')) + '</span><span class="num">' + slot(S('K', k.id, 'h')) + '</span><span class="num">' + slot(S('K', k.id, 'a')) + '</span><span class="num">' + slot(S('K', k.id, 's')) + '</span><span class="gt-bar"><div class="bar-cell"><i data-bar="' + esc(S('KC', k.id)) + '"></i></div></span></div>' })));
    fillAll();
  }
  (function () {
    const p = pages.keyboard; if (!p) return;
    const m0 = p.mount, u0 = p.update;
    p.mount = function (root) { TK.sig = null; if (m0) m0.call(this, root); };
    p.update = function (root) { if (u0) u0.call(this, root); renderTopKeys(root); };
  })();

  /* ================= Mouse: buttons and scrolling ================= */
  function renderMouse(root) {
    const rows = $('#mouse-rows', root), srows = $('#scroll-rows', root);
    if (rows && !$('#mouse-rows2', root)) {
      rows.style.display = 'none';
      const mk = (cls, name, grp) => '<div class="mouse-row"><span><i class="dot ' + cls + '"></i>' + name + '</span><em class="hv">Held ' + slot(S('B', grp, 'h')) + '<span class="hsep"> · average </span>' + slot(S('B', grp, 'a')) + '</em><strong>' + slot(S('B', grp, 'c')) + '</strong></div>';
      rows.insertAdjacentHTML('afterend', '<div class="mouse-rows" id="mouse-rows2">' + mk('', 'Left button', 'left') + mk('l2', 'Right button', 'right') + mk('l3', 'Middle button', 'middle') + mk('l4', 'Back and forward', 'x1+x2') + '</div>');
    }
    if (srows && !$('#scroll-rows2', root)) {
      srows.style.display = 'none';
      const sr = (cls, name, f) => '<div class="mouse-row"><span><i class="dot ' + cls + '"></i>' + name + '</span><strong>' + slot(S('C', f)) + '</strong></div>';
      srows.insertAdjacentHTML('afterend', '<div class="mouse-rows" id="scroll-rows2">' + sr('', 'Scroll up', 'up') + sr('l2', 'Scroll down', 'down') + sr('l3', 'Scroll left', 'left') + sr('l4', 'Scroll right', 'right') +
        '<div class="mouse-row"><span>Wheel notches in total</span><strong>' + slot(S('C', 'notches')) + '</strong></div><div class="mouse-row"><span>Scroll-active time ' + estBadge + '</span><strong>' + slot(S('G', 'st')) + '</strong></div><div class="mouse-row"><span>Scroll sessions</span><strong>' + slot(S('G', 'sn')) + '</strong></div></div>');
    }
    fillAll();
  }
  (function () {
    const p = pages.mouse; if (!p) return;
    const u0 = p.update;
    p.update = function (root) { if (u0) u0.call(this, root); renderMouse(root); };
  })();

  /* ================= History ================= */
  const HS = { sel: localKey(new Date()), tab: 'key', q: '', sort: 'new', page: 0, rows: [], limit: 2000, perSig: null, ihSig: null, ihView: '' };

  const card = (t, sl, sub, est) => '<div class="dur-card"><div class="dur-t">' + t + (est ? ' ' + estBadge : '') + '</div><div class="dur-v">' + slot(sl) + '</div><div class="dur-s">' + sub + '</div></div>';
  function dayPanelHTML() {
    return '<section class="panel section-gap day-panel" id="day-panel" style="--i:2"><div class="panel-head"><div><div class="panel-title" id="day-title">Today</div><div class="panel-hint">Pick any day in the calendar to see what was held and scrolled on it. Hold times are measured from version 2.4.0 on, and a running day keeps counting live.</div></div></div>' +
      '<div class="dur-cards" id="day-cards">' + card('Key presses', S('D', 'k'), 'On this day') + card('Mouse clicks', S('D', 'c'), 'On this day') + card('Scroll events', S('D', 's'), 'On this day') +
      card('Key-held time', S('D', 'kh'), 'Measured from press to release') + card('Mouse-button-held time', S('D', 'mh'), 'Measured from press to release') + card('Scroll-active time', S('D', 'sh'), 'From the gaps between wheel events', true) + '</div>' +
      '<div class="grid two-col day-lists"><div class="day-sub"><div class="day-sub-title">Most-held keys</div><div class="gt gt-daykeys"><div class="gt-head"><span>Key</span><span class="num">Held</span><span class="num">Holds</span><span></span></div><div class="gt-body" id="day-keys"></div></div></div>' +
      '<div class="day-sub"><div class="day-sub-title">Programs</div><div class="gt gt-dayapps"><div class="gt-head"><span>Program</span><span class="num">Keys</span><span class="num">Keys held</span><span class="num">Buttons held</span></div><div class="gt-body" id="day-apps"></div></div></div></div></section>';
  }
  function periodHTML() {
    return '<div class="gt gt-period" id="history-table2"><div class="gt-head"><span>Period</span><span class="num">Keys</span><span class="num">Clicks</span><span class="num">Scrolls</span><span class="num" title="Measured from press to release">Key held</span><span class="num" title="Measured from press to release">Button held</span><span class="num" title="Estimated from the gaps between wheel events">Scroll time</span><span class="num">Total</span><span></span></div><div class="gt-body" id="period-body"></div><div class="empty-mini" id="period-empty" hidden>No activity in this range.</div><div class="dur-note" id="period-note" hidden>Hold times are kept for each day, so hourly periods show none. Held time starts with version 2.4.0; earlier days show a dash.</div></div>';
  }
  function ihHTML() {
    return '<section class="panel section-gap" id="ih-panel" style="--i:4"><div class="panel-head"><div><div class="panel-title">Input history</div><div class="panel-hint" id="ih-hint"></div></div><div class="segmented" id="ih-tabs"><button data-t="key" class="active">Keys</button><button data-t="mouse">Mouse buttons</button><button data-t="scroll">Scroll sessions</button></div></div>' +
      '<div class="ih-tools"><input class="text-input" id="ih-q" type="search" placeholder="Search key, button or program" autocomplete="off"><select class="select-control" id="ih-sort"><option value="new">Newest</option><option value="old">Oldest</option><option value="long">Longest</option><option value="short">Shortest</option><option value="name">Name</option></select></div>' +
      '<div class="gt gt-ih" id="ih-grid"><div class="gt-head" id="ih-head"></div><div class="gt-body" id="ih-rows"></div></div><div class="empty-mini" id="ih-empty" hidden></div><div class="ih-pager" id="ih-pager"></div></section>';
  }

  /* ----- calendar: pick a day, see what was held on it ----- */
  function decorateCal(root) {
    const cal = $('#cal', root); if (!cal) return;
    const m = state.calMonth, month = (LIVE.data && LIVE.data.month && LIVE.data.month.rows) || {};
    $$('.cal-day', cal).forEach(el => {
      if (el.classList.contains('blank')) return;
      const day = parseInt(($('b', el) || {}).textContent, 10); if (!day) return;
      const date = m.getFullYear() + '-' + pad2(m.getMonth() + 1) + '-' + pad2(day);
      el.dataset.date = date; el.classList.toggle('sel', date === HS.sel);
      /* The tooltip is the page's own text plus the held time, built from scratch each time and never
         appended to (an earlier version appended on every refresh and the tooltip grew without end). */
      if (el.dataset.base === undefined) el.dataset.base = el.title;
      const r = month[date], extra = r && (r.k_ms || r.m_ms || r.s_ms) ? ' · keys held ' + ms(r.k_ms || 0) + ' · buttons held ' + ms(r.m_ms || 0) + (r.s_ms ? ' · scrolling ' + ms(r.s_ms) + ' (est.)' : '') : '';
      const tip = el.dataset.base + extra; if (el.title !== tip) el.title = tip;
    });
  }
  function selectDay(root, date) {
    if (!date || date === HS.sel) return;
    HS.sel = date;
    const panel = $('#day-panel', root); if (panel) { panel.classList.remove('swap'); void panel.offsetWidth; panel.classList.add('swap'); }
    $('#day-keys', root).innerHTML = ''; $('#day-apps', root).innerHTML = ''; HS.daySig = null;
    LIVE.data.day = null; decorateCal(root); renderDay(root); fillAll();
    pollData(DAYPOLL);
  }
  const dayTitle = date => { const d = new Date(date + 'T00:00:00'), t = date === localKey(new Date()) ? 'Today · ' : ''; return t + d.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }); };
  function renderDay(root) {
    root = root || $('#content');
    const title = $('#day-title', root); if (title && title.textContent !== dayTitle(HS.sel)) title.textContent = dayTitle(HS.sel);
    const d = LIVE.data && LIVE.data.day; if (!d || d.date !== HS.sel) return;
    const keys = Object.entries(d.key_hold || {}).sort((a, b) => b[1].ms - a[1].ms || a[0].localeCompare(b[0])).slice(0, 6).map(e => e[0]);
    const sigK = keys.join('|') + '#' + (d.apps || []).slice(0, 6).map(a => a.name).join('|');
    const kb = $('#day-keys', root), ab = $('#day-apps', root); if (!kb || !ab) return;
    if (HS.daySig !== sigK) {
      HS.daySig = sigK;
      reconcile(kb, keys.map(id => ({ key: id, html: '<div class="gt-row"><span><span class="key-chip">' + esc(label(id)) + '</span></span><span class="num">' + slot(S('D', 'K', id, 'h')) + '</span><span class="num muted">' + slot(S('D', 'K', id, 'n')) + '</span><span class="gt-bar"><div class="bar-cell"><i data-bar="' + esc(S('DK', id)) + '"></i></div></span></div>' })));
      reconcile(ab, (d.apps || []).slice(0, 6).map(a => ({ key: a.name, html: '<div class="gt-row"><span class="ih-app">' + icon(a.name) + '<span>' + esc(appLabel(a.name)) + '</span></span><span class="num">' + slot(S('D', 'A', a.name, 'k')) + '</span><span class="num">' + slot(S('D', 'A', a.name, 'kh')) + '</span><span class="num">' + slot(S('D', 'A', a.name, 'mh')) + '</span></div>' })));
      if (!keys.length && !kb.children.length) kb.innerHTML = '<div class="empty-mini">No key holds were measured on this day.</div>';
      if (!(d.apps || []).length && !ab.children.length) ab.innerHTML = '<div class="empty-mini">No program was recorded on this day.</div>';
      fillAll();
    }
  }

  /* ----- By period ----- */
  function renderPeriod(root) {
    root = root || $('#content');
    const host = $('#history-table', root); if (!host) return;
    if (!$('#history-table2', root)) { host.style.display = 'none'; host.insertAdjacentHTML('afterend', periodHTML()); HS.perSig = null; }
    const rows = (st().series || []).slice().sort((a, b) => (a.key < b.key ? 1 : -1)).slice(0, 200);
    const sig = rows.map(r => r.key).join('|') + '#' + (st().unit || '');
    const hourly = (LIVE.data && LIVE.data.series && LIVE.data.series.hourly) || st().unit === 'hour';
    $('#period-empty', root).hidden = rows.length > 0; $('#period-note', root).hidden = !rows.length;
    if (sig === HS.perSig) return; HS.perSig = sig;
    reconcile($('#period-body', root), rows.map(r => ({ key: r.key, html: '<div class="gt-row"><span class="gt-period-label">' + esc(r.label) + '</span><span class="num">' + slot(S('R', r.key, 'k')) + '</span><span class="num">' + slot(S('R', r.key, 'c')) + '</span><span class="num">' + slot(S('R', r.key, 's')) + '</span><span class="num">' + slot(S('R', r.key, 'kh')) + '</span><span class="num">' + slot(S('R', r.key, 'mh')) + '</span><span class="num">' + slot(S('R', r.key, 'sh')) + '</span><span class="num">' + slot(S('R', r.key, 't')) + '</span><span class="gt-bar"><div class="bar-cell"><i data-bar="' + esc(S('R', r.key)) + '"></i></div></span></div>' })));
    fillAll();
  }

  /* ----- input history ----- */
  const rowKey = r => [r.k, r.id, r.st, r.ms, r.app, r.n, r.sts].join('|');
  function ihView() { return [HS.tab, HS.q, HS.sort, HS.page].join('¦'); }
  function renderIH(full) {
    const root = $('#content'), grid = $('#ih-grid', root); if (!grid) return;
    const hint = $('#ih-hint', root); if (hint) hint.textContent = 'The newest ' + fmt(HS.limit) + ' key holds, button holds and scroll sessions in the range, with exact times. Older ones stay in the totals. New ones slide in as they are recorded. Export and import carry all of them.';
    const needle = HS.q.trim().toLowerCase();
    let rows = HS.rows.filter(r => r.k === HS.tab && (!needle || (label(r.id) + ' ' + (btnName[r.id] || '') + ' ' + (dirName[r.id] || '') + ' ' + appLabel(r.app) + ' ' + (r.app || '')).toLowerCase().includes(needle)));
    const d = r => r.ms || 0;
    rows.sort({ new: (a, b) => b.st - a.st, old: (a, b) => a.st - b.st, long: (a, b) => d(b) - d(a), short: (a, b) => d(a) - d(b), name: (a, b) => String(a.id).localeCompare(String(b.id)) }[HS.sort]);
    const per = 20, pages_ = Math.max(1, Math.ceil(rows.length / per)); HS.page = Math.min(HS.page, pages_ - 1);
    const slice = rows.slice(HS.page * per, HS.page * per + per);
    const view = ihView(), tabChanged = view !== HS.ihView;
    const head = $('#ih-head', root), body = $('#ih-rows', root);
    if (tabChanged || full) {
      HS.ihView = view; HS.ihSig = null;
      grid.dataset.tab = HS.tab;
      head.innerHTML = HS.tab === 'scroll' ? '<span>Started</span><span>Direction</span><span class="num">Wheel events</span><span class="num">Span ' + estBadge + '</span><span>Application</span>' : '<span>' + (HS.tab === 'key' ? 'Key' : 'Button') + '</span><span>Pressed at</span><span class="num">Held for</span><span>Application</span><span>Status</span>';
      body.innerHTML = ''; grid.classList.remove('swap'); void grid.offsetWidth; grid.classList.add('swap');
    }
    const app = r => '<span class="ih-app">' + icon(r.app) + '<span>' + esc(appLabel(r.app)) + '</span></span>';
    const st_ = r => r.sts === 'intr' ? '<span class="st intr" title="The key or button release was not received (pause, sleep or a hold over 15 minutes). No time is counted.">Interrupted</span>' : '<span class="st">Completed</span>';
    const html = r => HS.tab === 'scroll'
      ? '<div class="gt-row"><span>' + when(r.st) + '</span><span>' + esc(dirName[r.id] || r.id) + '</span><span class="num">' + fmt(r.n) + '</span><span class="num">' + (r.n > 1 ? ms(r.ms) : '<span class="muted" title="One wheel click has no measurable span">Single event</span>') + '</span>' + app(r) + '</div>'
      : '<div class="gt-row"><span>' + (HS.tab === 'key' ? '<span class="key-chip">' + esc(label(r.id)) + '</span>' : esc(btnName[r.id] || r.id)) + '</span><span>' + when(r.st) + '</span><span class="num">' + (r.sts === 'intr' ? '–' : ms(r.ms)) + '</span>' + app(r) + '<span>' + st_(r) + '</span></div>';
    const sig = slice.map(rowKey).join('~');
    if (sig !== HS.ihSig) { HS.ihSig = sig; reconcile(body, slice.map(r => ({ key: rowKey(r), html: html(r) }))); }
    const empty = $('#ih-empty', root); empty.hidden = rows.length > 0;
    empty.textContent = HS.rows.length ? 'Nothing matches.' : 'No records in this range yet. Durations are recorded from version 2.4.0 on; earlier days show Not recorded.';
    setHTML($('#ih-pager', root), rows.length ? '<span class="muted">' + fmt(rows.length) + ' records</span><span><button class="btn" data-ih="prev"' + (HS.page ? '' : ' disabled') + '>Previous</button><span class="muted"> Page ' + (HS.page + 1) + ' of ' + pages_ + ' </span><button class="btn" data-ih="next"' + (HS.page < pages_ - 1 ? '' : ' disabled') + '>Next</button></span>' : '');
  }

  /* ----- wiring ----- */
  const DAYPOLL = { key: 'day', every: 1000, url: () => '/api/durday?date=' + HS.sel, on: () => renderDay() };
  const monthUrl = () => { const m = state.calMonth, last = new Date(m.getFullYear(), m.getMonth() + 1, 0).getDate(); return '/api/durseries?range=custom&unit=daily&from=' + m.getFullYear() + '-' + pad2(m.getMonth() + 1) + '-01&to=' + m.getFullYear() + '-' + pad2(m.getMonth() + 1) + '-' + pad2(last); };
  const polls = [
    { key: 'series', every: 1000, url: () => '/api/durseries?' + KC.statsQuery(), on: () => renderPeriod() },
    { key: 'month', every: 2000, url: monthUrl, on: () => decorateCal($('#content')) },
    DAYPOLL,
    { key: 'recent', every: 3000, url: () => '/api/recent?' + KC.statsQuery(), on: r => { HS.rows = r.rows || []; HS.limit = r.limit || HS.limit; renderIH(false); } }
  ];
  (function () {
    const p = pages.history; if (!p) return;
    const h0 = p.html, u0 = p.update;
    p.html = function () {
      let h = h0.call(this);
      h = h.replace('<section class="panel section-gap" style="--i:2">', dayPanelHTML() + '<section class="panel section-gap" style="--i:3">');
      return h.replace(/<\/div>\s*$/, ihHTML() + '</div>');
    };
    p.update = function (root) {
      if (u0) u0.call(this, root);
      decorateCal(root); renderPeriod(root); renderDay(root);
    };
    withService(p, { polls }, {
      mount(root) {
        HS.perSig = null; HS.daySig = null; HS.ihView = ''; HS.ihSig = null;
        /* on this page's own element, never on #content (see v24.js) */
        const wrap = $('.page-wrap', root); if (!wrap) return;
        wrap.addEventListener('click', e => {
          const day = e.target.closest('.cal-day[data-date]'); if (day) { selectDay(root, day.dataset.date); return; }
          const t = e.target.closest('#ih-tabs button'); if (t) { HS.tab = t.dataset.t; HS.page = 0; $$('#ih-tabs button', wrap).forEach(b => b.classList.toggle('active', b === t)); renderIH(true); return; }
          const b = e.target.closest('[data-ih]'); if (b) { HS.page += b.dataset.ih === 'next' ? 1 : -1; renderIH(true); }
        });
        wrap.addEventListener('input', e => { if (e.target.id === 'ih-q') { HS.q = e.target.value; HS.page = 0; renderIH(true); } });
        wrap.addEventListener('change', e => { if (e.target.id === 'ih-sort') { HS.sort = e.target.value; HS.page = 0; renderIH(true); } });
        renderIH(true);
      }
    });
  })();
})();
