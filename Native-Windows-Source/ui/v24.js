/* KeyClick Tracker 2.4.0: input hold durations, scroll sessions, History and Analytics additions,
   and the Applications page. Loaded after v2.js. It extends the existing pages through hooks and
   uses the same classes, colours and animations as the rest of the app.

   LIVE NUMBERS. Every duration and count on these screens is a "slot": a small element that stays
   in the page and is updated in place by the live service below, with the app's own odometer
   (KCOdo, odometer.js), so digits roll up instead of the number being redrawn. The service keeps
   the page current in real time:
   - /api/held is polled every 200 ms. A key or button that is held right now adds its running
     time to the totals on every 100 ms tick, so "total held" climbs while you hold;
   - a hold that has just ended is added from the 'released' rows until the next /api/duration
     (or /api/apps2) answer includes it, so a total never dips or waits for a refresh;
   - /api/duration and /api/apps2 are refreshed every second, and at once when a hold ends.
   Animations follow the user's Animations / Live numbers / Lite mode settings. */
(function () {
  'use strict';
  const { $, $$, state, fmt, esc, api } = KC;
  const P = KC.pg, pages = KC.pages;
  const q = () => KC.statsQuery();
  const SEP = '¦';   // separates the parts of a slot name

  /* ================= formatting ================= */
  const ms = v => {
    v = Math.round(v || 0);
    if (v < 1000) return v + ' ms';
    if (v < 60000) return (v / 1000).toFixed(v < 10000 ? 2 : 1) + ' s';
    /* tenths of a second up to an hour, so a running total ticks ten times a second, not once a second */
    const m = Math.floor(v / 60000), sec = (v % 60000) / 1000;
    if (m < 60) return m + ' m ' + (sec < 10 ? '0' : '') + sec.toFixed(1) + ' s';
    return Math.floor(m / 60) + ' h ' + String(m % 60).padStart(2, '0') + ' m ' + String(Math.floor(sec)).padStart(2, '0') + ' s';
  };
  const avg = (t, n) => n ? ms(t / n) : '–';
  const btnName = { left: 'Left button', right: 'Right button', middle: 'Middle (wheel click)', x1: 'Back (X1)', x2: 'Forward (X2)' };
  const dirName = { up: 'Scroll up', down: 'Scroll down', left: 'Scroll left', right: 'Scroll right', mixed: 'Mixed directions' };
  const when = t => { const d = new Date(t); return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) + ' ' + d.toLocaleTimeString(); };
  const lastFmt = t => { if (!t) return '–'; const d = new Date(t * 1000), n = new Date(), hm = { hour: 'numeric', minute: '2-digit' }; return d.toDateString() === n.toDateString() ? 'today ' + d.toLocaleTimeString([], hm) : d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) + ' ' + d.toLocaleTimeString([], hm); };
  const EST = 'Estimated. A mouse wheel has no real hold time. This is the time from the first to the last wheel event of a burst (events less than 0.4 s apart).';
  const NA = 'Not recorded (hold times start with version 2.4.0)';
  const estBadge = '<span class="est-badge" title="' + EST + '">estimated</span>';
  const label = id => P.keyLabel ? P.keyLabel(id) : id;
  const setHTML = (el, h) => { if (el && el.dataset.h !== h) { el.dataset.h = h; el.innerHTML = h; } };
  /* KeyClick Tracker lists itself like any other program. */
  const isSelf = n => /^keyclick[-_ ]?tracker/i.test(n || '');
  const appLabel = n => !n ? 'Unknown application' : n === '(protected)' ? 'Protected program' : isSelf(n) ? 'KeyClick Tracker' : n;
  const icon = n => n && n !== '(protected)' ? '<span class="app-ico"><img src="/api/appicon?name=' + encodeURIComponent(n) + '&token=' + encodeURIComponent(KC.token || '') + '" alt="" width="24" height="24" data-letter="' + esc((n[0] || '?').toUpperCase()) + '"></span>' : '<span class="app-ico fb">?</span>';
  /* A program without an icon shows a neutral letter badge instead of a broken image. */
  document.addEventListener('error', e => {
    const t = e.target;
    if (t && t.tagName === 'IMG' && t.dataset && t.dataset.letter) { const p = t.parentNode; p.classList.add('fb'); p.textContent = t.dataset.letter; }
  }, true);
  const sumMs = o => Object.values(o || {}).reduce((a, h) => a + (h.ms || 0), 0), cntN = o => Object.values(o || {}).reduce((a, h) => a + (h.n || 0), 0), maxMs = o => Object.values(o || {}).reduce((a, h) => Math.max(a, h.mx || 0), 0);

  /* ================= sliding numbers ================= */
  const animOK = () => { const s = state.settings || {}; return !!(s.animations && s.live_numbers && !s.lite_mode); };
  const zeros = t => String(t).replace(/[0-9]/g, '0');
  const memo = new Map();   // the last text of every slot, so a slot that is re-created starts from what it showed
  /* A number in an element that stays alive. A new element starts from start, then rolls to text. */
  function setNum(el, text, start) {
    if (!el) return;
    let o = el._o;
    if (!o) {
      o = el._o = new KCOdo(el);
      if (!animOK() || start === text) { o.set(text, false); return; }
      o.set(start != null ? start : zeros(text), false);
      requestAnimationFrame(() => o.set(text, true));
      return;
    }
    o.set(text, animOK());
  }
  const slot = (name, cls) => '<b class="nv' + (cls ? ' ' + cls : '') + '" data-slot="' + esc(name) + '">–</b>';
  const tslot = (name, cls) => '<span class="tv' + (cls ? ' ' + cls : '') + '" data-tslot="' + esc(name) + '"></span>';
  const S = (...p) => p.join(SEP);

  /* ================= the live data service ================= */
  const LIVE = {
    root: null, page: null, timers: [],
    held: null, items: new Map(), scr: { active: false, span: 0, events: 0, dir: '' }, scrPend: [], rel: [], relSeen: new Set(),
    dur: null, durStart: 0, durBusy: false, durAgain: false, scrSeen: 0,
    apps: null, appMap: new Map(), appsStart: 0, appsBusy: false,
    addD: null, addA: null, addX: {}, data: {}, heldBusy: false, heldAgain: false
  };
  const epoch = () => Date.now();
  /* urgent: something was just pressed, so ask again as soon as the answer in flight has landed */
  function pollHeld(urgent) {
    if (KC.hidden) return;
    if (LIVE.heldBusy) { if (urgent) LIVE.heldAgain = true; return; }
    LIVE.heldBusy = true;
    api('/api/held').then(onHeld).catch(() => { /* server restarting */ }).then(() => { LIVE.heldBusy = false; if (LIVE.heldAgain) { LIVE.heldAgain = false; pollHeld(); } });
  }
  /* extra data a page wants polled (History: hold time per period, per day) */
  function pollData(def) {
    if (def.busy) return;
    def.busy = true;
    api(def.url()).then(r => { LIVE.data[def.key] = r; if (def.on) def.on(r); }).catch(() => {}).then(() => { def.busy = false; });
  }
  function onHeld(j) {
    const t0 = performance.now(), items = new Map();
    const add = (kind, it) => { const key = kind + ':' + it.id, old = LIVE.items.get(key); items.set(key, { kind, id: it.id, app: it.app, base: it.ms, t0, total: it.total_ms, holds: it.holds, shown: old ? old.shown : 0 }); };
    (j.keys || []).forEach(it => add('k', it));
    (j.mouse || []).forEach(it => add('m', it));
    LIVE.held = j; LIVE.items = items;
    let need = false;
    (j.released || []).forEach(r => {
      const key = r.k + '|' + r.id + '|' + r.st;
      if (LIVE.relSeen.has(key)) return;
      LIVE.relSeen.add(key); LIVE.rel.push({ k: r.k, id: r.id, ms: r.ms, app: r.app, seq: r.seq, doneAt: r.st + r.ms, total: r.total_ms, holds: r.holds }); need = true;
    });
    const s = j.scroll || {}, cl = s.closed;
    if (cl && cl.seq && cl.seq !== LIVE.scrSeen) { LIVE.scrSeen = cl.seq; LIVE.scrPend.push({ seq: cl.seq, span: cl.events > 1 ? cl.span_ms : 0, doneAt: epoch() }); need = true; }
    LIVE.scr = { active: !!s.active, span: s.span_ms || 0, events: s.events || 0, dir: s.dir || '' };
    const cut = epoch() - 15000;
    LIVE.rel = LIVE.rel.filter(r => r.doneAt > cut); LIVE.scrPend = LIVE.scrPend.filter(p => p.doneAt > cut);
    if (LIVE.relSeen.size > 400) LIVE.relSeen = new Set(LIVE.rel.map(r => r.k + '|' + r.id + '|' + (r.doneAt - r.ms)));
    syncTiles(j);
    if (need) {   // a hold ended: fetch the totals now
      pollDur(true); if (LIVE.page && LIVE.page.apps) pollApps(true);
      if (LIVE.page && LIVE.page.polls) LIVE.page.polls.forEach(d => setTimeout(() => pollData(d), 150));
    }
  }
  function pollDur(soon) {
    if (LIVE.durBusy) { LIVE.durAgain = true; return; }
    const go = () => {
      LIVE.durBusy = true; const start = epoch();
      api('/api/duration?' + q()).then(r => { LIVE.dur = r; LIVE.durStart = start; if (LIVE.page && LIVE.page.onDur) LIVE.page.onDur(); }).catch(() => {}).then(() => { LIVE.durBusy = false; if (LIVE.durAgain) { LIVE.durAgain = false; pollDur(true); } });
    };
    soon ? setTimeout(go, 150) : go();   // 150 ms lets the recorder store the hold that just ended
  }
  function pollApps(soon) {
    if (LIVE.appsBusy) return;
    const go = () => {
      LIVE.appsBusy = true; const start = epoch();
      api('/api/apps2?' + q()).then(r => { LIVE.apps = r; LIVE.appsStart = start; LIVE.appMap = new Map(r.apps.map(a => [a.name, a])); if (LIVE.page && LIVE.page.onApps) LIVE.page.onApps(); }).catch(() => {}).then(() => { LIVE.appsBusy = false; });
    };
    soon ? setTimeout(go, 150) : go();
  }
  /* What is not in the fetched totals yet: the holds in progress, and the holds recorded after the
     totals were read. Every recorded hold has a sequence number and every answer says which one it
     contains up to (seq), so a hold is added exactly until a total contains it: never twice, never
     missing in between. */
  function computeAdd(nowP, seq) {
    const a = { k: {}, m: {}, kDone: {}, mDone: {}, kN: {}, mN: {}, kLong: {}, mLong: {}, kTot: 0, mTot: 0, kDoneTot: 0, mDoneTot: 0, kCnt: 0, mCnt: 0, kMax: 0, mMax: 0, app: {}, scrMs: 0, scrN: 0, scrMax: 0 };
    const bump = (kind, id, v, app, done) => {
      const o = a[kind], dn = a[kind + 'Done'], nn = a[kind + 'N'], lg = a[kind + 'Long'];
      o[id] = (o[id] || 0) + v; lg[id] = Math.max(lg[id] || 0, v);
      a[kind + 'Tot'] += v; a[kind + 'Max'] = Math.max(a[kind + 'Max'], v);
      if (done) { dn[id] = (dn[id] || 0) + v; nn[id] = (nn[id] || 0) + 1; a[kind + 'DoneTot'] += v; a[kind + 'Cnt']++; }
      if (app) { const x = a.app[app] || (a.app[app] = { k: 0, m: 0, kid: {}, mid: {} }); x[kind] += v; (kind === 'k' ? x.kid : x.mid)[id] = ((kind === 'k' ? x.kid : x.mid)[id] || 0) + v; }
    };
    LIVE.items.forEach(it => { it.shown = Math.max(it.shown || 0, it.base + (nowP - it.t0)); bump(it.kind, it.id, it.shown, it.app, false); });   // never runs backwards
    LIVE.rel.forEach(r => { if (r.seq > seq) bump(r.k === 'key' ? 'k' : 'm', r.id, r.ms, r.app, true); });
    if (LIVE.scr.active) { a.scrN++; if (LIVE.scr.events > 1) { a.scrMs += LIVE.scr.span; a.scrMax = Math.max(a.scrMax, LIVE.scr.span); } }
    LIVE.scrPend.forEach(p => { if (p.seq > seq) { a.scrN++; a.scrMs += p.span; a.scrMax = Math.max(a.scrMax, p.span); } });
    return a;
  }

  /* ---------- slot values ---------- */
  const none = '–';
  function gval(f) {
    const d = LIVE.dur, a = LIVE.addD; if (!d || !a) return null;
    const kn = cntN(d.keys) + a.kCnt, mn = cntN(d.mouse) + a.mCnt, sc = d.scroll || {};
    switch (f) {
      case 'kt': return kn || a.kTot ? ms(sumMs(d.keys) + a.kTot) : none;
      case 'kn': return fmt(kn);
      case 'ka': return kn ? avg(sumMs(d.keys) + a.kDoneTot, kn) : none;
      case 'kl': return kn || a.kMax ? ms(Math.max(maxMs(d.keys), a.kMax)) : none;
      case 'mt': return mn || a.mTot ? ms(sumMs(d.mouse) + a.mTot) : none;
      case 'mn': return fmt(mn);
      case 'ma': return mn ? avg(sumMs(d.mouse) + a.mDoneTot, mn) : none;
      case 'ml': return mn || a.mMax ? ms(Math.max(maxMs(d.mouse), a.mMax)) : none;
      case 'st': return (sc.n || a.scrN) ? ms((sc.ms || 0) + a.scrMs) : none;
      case 'sn': return fmt((sc.n || 0) + a.scrN);
      case 'sl': return (sc.n || a.scrN) ? ms(Math.max(sc.mx || 0, a.scrMax)) : none;
      case 'lg': return kn + mn || a.kMax || a.mMax ? ms(Math.max(maxMs(d.keys), maxMs(d.mouse), a.kMax, a.mMax)) : none;
    }
    if (f.indexOf('dir_') === 0) return fmt(((d.scroll || {}).d || {})[f.slice(4)] || 0);
    return null;
  }
  function kval(id, f) {
    const d = LIVE.dur, a = LIVE.addD; if (!d || !a) return null;
    const h = (d.keys || {})[id], n = (h ? h.n : 0) + (a.kN[id] || 0), live = a.k[id] || 0;
    if (f === 'h') return n || live ? ms((h ? h.ms : 0) + live) : none;
    if (f === 'a') return n ? avg((h ? h.ms : 0) + (a.kDone[id] || 0), n) : none;
    if (f === 'l') return n || live ? ms(Math.max(h ? h.mx : 0, a.kLong[id] || 0)) : none;
    return null;
  }
  function bval(ids, f) {
    const d = LIVE.dur, a = LIVE.addD; if (!d || !a) return null;
    let msT = 0, n = 0, mx = 0, live = 0, done = 0;
    ids.split('+').forEach(id => { const h = (d.mouse || {})[id]; if (h) { msT += h.ms; n += h.n; mx = Math.max(mx, h.mx); } n += a.mN[id] || 0; live += a.m[id] || 0; done += a.mDone[id] || 0; mx = Math.max(mx, a.mLong[id] || 0); });
    if (f === 'h') return n || live ? ms(msT + live) : none;
    if (f === 'a') return n ? avg(msT + done, n) : none;
    if (f === 'l') return n || live ? ms(mx) : none;
    return null;
  }
  const appTotal = a => a.keys + a.clicks + a.scroll;
  function pval(name, f) {
    const app = LIVE.appMap.get(name); if (!app) return null;
    const x = LIVE.addA && LIVE.addA.app[name];
    switch (f) {
      case 'kh': return Object.keys(app.kh || {}).length || (x && x.k) ? ms(sumMs(app.kh) + (x ? x.k : 0)) : none;
      case 'mh': return Object.keys(app.mh || {}).length || (x && x.m) ? ms(sumMs(app.mh) + (x ? x.m : 0)) : none;
      case 'sh': return app.sess ? ms(app.sess_ms) : none;
      case 'kc': return fmt(app.keys);
      case 'cc': return fmt(app.clicks);
      case 'sc': return fmt(app.scroll);
      case 'sp': { const all = Math.max(1, LIVE.apps.apps.reduce((s, y) => s + appTotal(y), 0)); return Math.round(100 * appTotal(app) / all) + '%'; }
    }
    return null;
  }
  function qval(name, kind, id, f) {
    const app = LIVE.appMap.get(name); if (!app) return null;
    const x = LIVE.addA && LIVE.addA.app[name], live = x ? (kind === 'k' ? x.kid : x.mid)[id] || 0 : 0;
    const cnt = (kind === 'k' ? app.ki : app.mi) || {}, hold = ((kind === 'k' ? app.kh : app.mh) || {})[id];
    if (f === 'p') return fmt(cnt[id] || 0);
    if (f === 'h') return hold || live ? ms((hold ? hold.ms : 0) + live) : none;
    return null;
  }
  function aval(f) {
    const r = LIVE.apps; if (!r) return null;
    switch (f) {
      case 'programs': return fmt(r.apps.filter(a => appTotal(a) > 0).length);
      case 'keys': return fmt(r.total_keys); case 'clicks': return fmt(r.total_clicks); case 'scroll': return fmt(r.total_scroll);
    }
    return null;
  }
  const EXT = {};   // other files (v24h.js) add their own slots: EXT.val(parts, name), EXT.tval(name), EXT.bar(name)
  function val(name) {
    const p = name.split(SEP);
    let r = null;
    switch (p[0]) {
      case 'G': r = gval(p[1]); break; case 'K': r = kval(p[1], p[2]); break; case 'B': r = bval(p[1], p[2]); break;
      case 'P': r = pval(p[1], p[2]); break; case 'Q': r = qval(p[1], p[2], p[3], p[4]); break; case 'A': r = aval(p[1]); break;
    }
    if (r == null && EXT.val) r = EXT.val(p, name);
    return r;
  }
  /* text (not rolling) values */
  const FIXED = { kPer: 'Per measured press', kLong: 'A single key held down', mPer: 'Per measured click', mLong: 'A single button held down' };
  function tval(name) {
    if (FIXED[name] !== undefined) return FIXED[name];
    if (EXT.tval) { const r = EXT.tval(name); if (r != null) return r; }
    const p = name.split(SEP), d = LIVE.dur, a = LIVE.addD;
    if (p[0] === 'P' && p[2] === 'last') { const app = LIVE.appMap.get(p[1]); return app ? (app.last ? 'Last input ' + lastFmt(app.last) : 'Last input –') : null; }
    if (!d || !a) return null;
    const kn = cntN(d.keys) + a.kCnt, mn = cntN(d.mouse) + a.mCnt, sc = d.scroll || {}, sn = (sc.n || 0) + a.scrN;
    switch (name) {
      case 'kSub': return kn ? 'Measured over ' + fmt(kn) + ' presses' : 'Not recorded yet';
      case 'mSub': return mn ? 'Measured over ' + fmt(mn) + ' clicks' : 'Not recorded yet';
      case 'sSub': return sn ? fmt(sn) + ' scroll sessions' : 'Not recorded yet';
      case 'kAvgSub': return kn ? 'Average ' + avg(sumMs(d.keys) + a.kDoneTot, kn) + ' per press' : 'Not recorded yet';
      case 'mAvgSub': return mn ? 'Average ' + avg(sumMs(d.mouse) + a.mDoneTot, mn) + ' per click' : 'Not recorded yet';
      case 'dkSub': return fmt(kn) + ' measured presses';
      case 'dmSub': return fmt(mn) + ' measured clicks';
      case 'dsSub': return fmt(sn) + ' sessions';
      case 'dlSub': return 'Longest scroll session ' + ms(Math.max(sc.mx || 0, a.scrMax));
    }
    return null;
  }
  /* bars follow a slot's value */
  function barVal(name) {
    if (EXT.bar) { const r = EXT.bar(name); if (r != null) return r; }
    const p = name.split(SEP), d = LIVE.dur, a = LIVE.addD; if (!d || !a) return 0;
    if (p[0] === 'K') { const live = (d.keys[p[1]] ? d.keys[p[1]].ms : 0) + (a.k[p[1]] || 0), top = Math.max(1, ...Object.keys(d.keys).map(k => d.keys[k].ms + (a.k[k] || 0))); return Math.max(2, Math.round(100 * live / top)); }
    if (p[0] === 'M') { const live = (d.mouse[p[1]] ? d.mouse[p[1]].ms : 0) + (a.m[p[1]] || 0), top = Math.max(1, ...Object.keys(d.mouse).map(k => d.mouse[k].ms + (a.m[k] || 0))); return Math.max(2, Math.round(100 * live / top)); }
    return 0;
  }
  /* A total of held time only ever grows. Re-syncing with the server, or a hold that ends a few
     milliseconds shorter than the running clock showed, must never make it step backwards. */
  const GROWS = /^(G¦(kt|mt|st|lg|kl|ml|sl)|K¦[^¦]+¦[hl]|B¦[^¦]+¦[hl]|P¦[^¦]+¦(kh|mh)|Q¦[^¦]+¦[km]¦[^¦]+¦h)$/;
  const prevMs = new Map();
  function parseMs(t) {
    if (!t || t === '–') return null;
    let m = t.match(/^([\d.]+) ms$/); if (m) return +m[1];
    m = t.match(/^([\d.]+) s$/); if (m) return Math.round(+m[1] * 1000);
    m = t.match(/^(\d+) m ([\d.]+) s$/); if (m) return Math.round((+m[1] * 60 + +m[2]) * 1000);
    m = t.match(/^(\d+) h (\d+) m (\d+) s$/); if (m) return ((+m[1] * 60 + +m[2]) * 60 + +m[3]) * 1000;
    return null;
  }
  function refreshAdd() {
    const nowP = performance.now();
    LIVE.addD = computeAdd(nowP, LIVE.dur && LIVE.dur.seq != null ? LIVE.dur.seq : Infinity);
    LIVE.addA = computeAdd(nowP, LIVE.apps && LIVE.apps.seq != null ? LIVE.apps.seq : Infinity);
    LIVE.addX = {};   // for each extra polled answer that carries a seq (History)
    Object.keys(LIVE.data || {}).forEach(k => { const r = LIVE.data[k]; if (r && r.seq != null) LIVE.addX[k] = computeAdd(nowP, r.seq); });
  }
  function fillAll() {
    const root = LIVE.root; if (!root) return;
    refreshAdd();   // always from the newest totals: a total that just arrived must not be added to twice
    $$('[data-slot]', root).forEach(el => {
      const key = el.dataset.slot; let t = val(key);
      if (t == null) return;
      if (GROWS.test(key)) {
        const n = parseMs(t), p = prevMs.get(key);
        if (n != null) { if (p != null && n < p && p - n < 400) t = ms(p); else prevMs.set(key, n); }
      }
      setNum(el, t, memo.get(key)); memo.set(key, t);
    });
    $$('[data-tslot]', root).forEach(el => { const t = tval(el.dataset.tslot); if (t != null && el.textContent !== t) el.textContent = t; });
    $$('[data-bar]', root).forEach(el => { const w = barVal(el.dataset.bar) + '%'; if (el.style.width !== w) el.style.width = w; });
  }
  /* Keys and buttons glow while they are held: on the Keyboard and Mouse pages for anything held, and in
     an opened program for what is held in that program. A press that was just made pulses (see below). */
  function glowHeld() {
    const root = LIVE.root; if (!root) return;
    const held = LIVE.items;
    $$('.keyboard-board:not(.ad-board) .keycap', root).forEach(c => c.classList.toggle('holding', held.has('k:' + c.dataset.key)));
    ['left', 'right', 'middle', 'x1', 'x2'].forEach(b => { const el = $('#mp-' + b, root); if (el) el.classList.toggle('holding', held.has('m:' + b)); });
    A.rows.forEach((r, n) => {
      if (!A.open.has(n) || !r.built) return;
      $$('.keycap', r.inn).forEach(c => { const it = held.get('k:' + c.dataset.key); c.classList.toggle('holding', !!it && it.app === n); });
      $$('.mpart', r.inn).forEach(p => { const it = held.get('m:' + p.dataset.b); p.classList.toggle('holding', !!it && it.app === n); });
    });
  }
  /* A key or button that was just pressed in the program that has focus pulses in that program's open detail. */
  function pulseApps(f) {
    const fg = LIVE.held && LIVE.held.fg, hit = el => { el.classList.remove('hit'); void el.getBoundingClientRect(); el.classList.add('hit'); };
    A.rows.forEach((r, n) => {
      if (!A.open.has(n) || !r.built || (fg && n !== fg)) return;
      if (f.ki) for (const id in f.ki) { const c = r.inn.querySelector('.keycap[data-key="' + (window.CSS && CSS.escape ? CSS.escape(id) : id) + '"]'); if (c) hit(c); }
      if (f.mi) for (const id in f.mi) { const p = r.inn.querySelector('.mpart[data-b="' + id + '"]'); if (p) hit(p); }
    });
  }
  function tick() {
    tilesTick(performance.now());
    fillAll();
    glowHeld();
  }
  function startService(page) {
    stopService();
    LIVE.root = $('#content'); LIVE.page = page;
    LIVE.items = new Map(); LIVE.rel = []; LIVE.relSeen = new Set(); LIVE.scrPend = []; LIVE.scrSeen = 0; LIVE.scr = { active: false, span: 0, events: 0, dir: '' };
    LIVE.dur = null; LIVE.durStart = 0; LIVE.apps = page.apps ? LIVE.apps : null;
    prevMs.clear(); LIVE.data = {};
    pollHeld(); pollDur(false); if (page.apps) pollApps(false);
    /* held keys every 100 ms (and at once on every input), the screen every 80 ms */
    LIVE.timers = [setInterval(pollHeld, 100), setInterval(() => pollDur(false), 1000), setInterval(tick, 80)];
    if (page.apps) LIVE.timers.push(setInterval(() => pollApps(false), 1000));
    (page.polls || []).forEach(d => { d.busy = false; pollData(d); LIVE.timers.push(setInterval(() => pollData(d), d.every || 1000)); });
  }
  function stopService() { LIVE.timers.forEach(clearInterval); LIVE.timers = []; LIVE.page = null; tiles.map.clear(); tiles.grid = null; }
  /* a page lifecycle that starts and stops the service */
  function withService(page, opts, extra) {
    const m0 = page.mount, u0 = page.unmount;
    page.mount = function (root) { if (m0) m0.call(this, root); if (extra && extra.mount) extra.mount(root); startService(opts); };
    page.unmount = function () { if (u0) u0.call(this); stopService(); };
  }

  /* ================= held tiles ================= */
  /* kind: keys (Keyboard page), mouse (Mouse page, with scrolling) or both (Applications). Tiles stay
     alive: they rise in with a glow, count up while held, flash when released (so even a quick tap
     shows) and fade out. */
  const LIVE_TEXT = {
    keys: ['Key holds', 'Keys that are down at this moment, how long each has been down, and its total held time. The strip shows the total for the range you picked and keeps counting while you hold.'],
    mouse: ['Button holds and scrolling', 'Mouse buttons that are down at this moment, how long each has been down, and its total held time. The strip shows the totals for the range you picked and keeps counting while you hold.'],
    both: ['Held right now', 'Keys and mouse buttons that are down at this moment, with how long each has been down and its total held time today.']
  };
  const EMPTY_TEXT = { keys: 'No keys are held right now', mouse: 'No mouse buttons are held right now', both: 'No keys or mouse buttons are held right now' };
  const hs = (l, sl, sub, est) => '<div class="held-stat"><small>' + l + (est ? ' ' + estBadge : '') + '</small>' + slot(sl) + '<em>' + tslot(sub) + '</em></div>';
  function totalsHTML(kind) {
    return kind === 'keys'
      ? hs('Total key-held time', 'G' + SEP + 'kt', 'kSub') + hs('Average hold', 'G' + SEP + 'ka', 'kPer') + hs('Longest hold', 'G' + SEP + 'kl', 'kLong')
      : hs('Total button-held time', 'G' + SEP + 'mt', 'mSub') + hs('Average hold', 'G' + SEP + 'ma', 'mPer') + hs('Longest hold', 'G' + SEP + 'ml', 'mLong') + hs('Scroll-active time', 'G' + SEP + 'st', 'sSub', true);
  }
  const liveHTML = kind => '<section class="panel section-gap live-held" id="live-held" data-kind="' + kind + '" style="--i:2"><div class="panel-head"><div><div class="panel-title">' + LIVE_TEXT[kind][0] + '</div><div class="panel-hint">' + LIVE_TEXT[kind][1] + '</div></div><span class="live-led" id="live-led"></span></div>' +
    (kind === 'both' ? '' : '<div class="held-totals" id="held-totals">' + totalsHTML(kind) + '</div>') + '<div class="held-grid" id="live-held-body"><div class="held-empty show" id="held-empty">' + EMPTY_TEXT[kind] + '</div></div></section>';
  const tiles = { map: new Map(), grid: null, empty: null, kind: 'both', paused: false };
  const RELEASE_MS = 1100;   // how long a released tile stays (it glows once, then fades)
  function bindTiles() {
    const box = $('#live-held'); if (!box) { tiles.grid = null; return false; }
    if (tiles.grid !== $('#live-held-body')) { tiles.map.clear(); tiles.grid = $('#live-held-body'); tiles.empty = $('#held-empty'); }
    tiles.kind = box.dataset.kind; return true;
  }
  function makeTile(key, name, scroll) {
    const el = document.createElement('div');
    el.className = 'held-tile' + (scroll ? ' scroll' : '');
    el.innerHTML = '<div class="ht-name"></div><div class="ht-now"></div><div class="ht-lbl"></div><div class="ht-tot"><span>' + (scroll ? 'Session span ' + estBadge : 'Total held today') + '</span><b class="ht-total">–</b></div><div class="ht-meta"></div>';
    el.querySelector('.ht-name').textContent = name;
    tiles.grid.appendChild(el);
    const t = { key, el, now: el.querySelector('.ht-now'), lbl: el.querySelector('.ht-lbl'), tot: el.querySelector('.ht-total'), meta: el.querySelector('.ht-meta'), state: 'held', base: 0, t0: 0, tot0: 0, expire: 0, scroll: !!scroll };
    tiles.map.set(key, t);
    return t;
  }
  function toHeld(t, it) {
    t.state = 'held'; t.apiRel = false; t.leaving = false;
    t.el.classList.remove('released', 'leaving'); t.el.classList.add('held');
    t.base = it.ms; t.t0 = performance.now(); t.tot0 = it.total_ms;
    t.lbl.textContent = 'Held now';
    t.meta.textContent = fmt(it.holds) + (it.holds === 1 ? ' hold' : ' holds') + (it.app ? ' · ' + appLabel(it.app) : '');
  }
  function toRelease(t, finalMs, life, r) {
    t.state = 'released'; t.expire = performance.now() + life;
    t.el.classList.remove('held'); t.el.classList.add('released');
    if (!t.scroll) { t.now.textContent = ms(finalMs); t.lbl.textContent = 'Released'; } else t.lbl.textContent = 'Scroll finished';
    if (r && !t.scroll) {
      const k = 'lt|' + t.key, txt = ms(r.total_ms); setNum(t.tot, txt, memo.get(k)); memo.set(k, txt);
      t.meta.textContent = fmt(r.holds) + (r.holds === 1 ? ' hold' : ' holds') + (r.app ? ' · ' + appLabel(r.app) : '');
    }
  }
  function syncTiles(j) {
    if (!bindTiles()) return;
    const kind = tiles.kind, now = performance.now(), act = new Set();
    tiles.paused = !!j.paused;
    const nameOf = (k, id) => k === 'k' ? label(id) : (btnName[id] || id);
    if (kind !== 'mouse') (j.keys || []).forEach(it => { const key = 'k:' + it.id; act.add(key); toHeld(tiles.map.get(key) || makeTile(key, nameOf('k', it.id)), it); });
    if (kind !== 'keys') {
      (j.mouse || []).forEach(it => { const key = 'm:' + it.id; act.add(key); toHeld(tiles.map.get(key) || makeTile(key, nameOf('m', it.id)), it); });
      const s = j.scroll || {};
      if (s.active) {
        act.add('s');
        const t = tiles.map.get('s') || makeTile('s', dirName[s.dir] || 'Scrolling', true);
        t.state = 'held'; t.leaving = false; t.el.classList.remove('released', 'leaving'); t.el.classList.add('held');
        t.now.textContent = fmt(s.events) + (s.events === 1 ? ' event' : ' events'); t.lbl.textContent = 'Scroll in progress';
        const k = 'lt|s', txt = s.events > 1 ? ms(s.span_ms) : '–'; setNum(t.tot, txt, memo.get(k)); memo.set(k, txt);
        t.el.querySelector('.ht-name').textContent = dirName[s.dir] || 'Scrolling';
      }
    }
    /* holds that ended a moment ago: a quick tap still shows and glows */
    (j.released || []).forEach(r => {
      const pre = r.k === 'key' ? 'k:' : 'm:', key = pre + r.id;
      if (act.has(key) || r.ago_ms > RELEASE_MS) return;
      if ((r.k === 'key' && kind === 'mouse') || (r.k === 'mouse' && kind === 'keys')) return;
      let t = tiles.map.get(key);
      if (t && t.state === 'released' && t.apiRel) return;
      if (!t) t = makeTile(key, nameOf(pre[0], r.id));
      toRelease(t, r.ms, RELEASE_MS - r.ago_ms, r); t.apiRel = true;
    });
    tiles.map.forEach((t, key) => { if (t.state === 'held' && !act.has(key)) toRelease(t, t.base + (now - t.t0), RELEASE_MS); });
  }
  function tilesTick(now) {
    if (!bindTiles()) return;
    tiles.map.forEach((t, key) => {
      if (t.state === 'held' && !t.scroll) {
        const el = now - t.t0;
        t.now.textContent = ms(t.base + el);                               // the running time
        const k = 'lt|' + key, txt = ms(t.tot0 + el); setNum(t.tot, txt, memo.get(k)); memo.set(k, txt);   // the total keeps counting too
      }
      if (t.state === 'released' && now >= t.expire && !t.leaving) {
        t.leaving = true; t.el.classList.add('leaving');
        setTimeout(() => { t.el.remove(); if (tiles.map.get(key) === t) tiles.map.delete(key); }, 340);
      }
    });
    const visible = [...tiles.map.values()].some(t => !t.leaving);
    if (tiles.empty) { tiles.empty.classList.toggle('show', !visible); tiles.empty.textContent = tiles.paused ? 'Recording is paused' : EMPTY_TEXT[tiles.kind]; }
    const led = $('#live-led'); if (led) led.classList.toggle('on', !tiles.paused);
  }
  function addLive(name, marker, kind, decorate) {
    const p = pages[name]; if (!p) return;
    const h0 = p.html, up0 = p.update;
    p.html = function () { const h = h0.call(this); return h.includes(marker) ? h.replace(marker, liveHTML(kind) + marker) : h; };
    withService(p, {});
    if (decorate) p.update = function (root) { if (up0) up0.call(this, root); decorate(root); };
  }

  /* ---------- Keyboard: Most used keys gets Held and Average ---------- */
  function decorateKeys(root) {
    const t = $('#top-keys table', root); if (!t) return;
    const heads = [...t.querySelectorAll('thead th')], pi = heads.findIndex(h => /presses/i.test(h.textContent)); if (pi < 0) return;
    if (!t.dataset.hold) { t.dataset.hold = '1'; heads[pi].insertAdjacentHTML('afterend', '<th class="num">Held</th><th class="num">Average</th>'); }
    const map = new Map();
    ((P.S().keys) || []).forEach(k => map.set(label(k.id), k.id));
    t.querySelectorAll('tbody tr').forEach(tr => {
      const chip = tr.querySelector('.key-chip'), id = chip ? map.get(chip.textContent) : null;
      if (tr.querySelector('.hold-cell')) return;
      tr.cells[pi].insertAdjacentHTML('afterend', '<td class="num hold-cell">' + (id ? slot(S('K', id, 'h')) : '–') + '</td><td class="num avg-cell">' + (id ? slot(S('K', id, 'a')) : '–') + '</td>');
    });
    fillAll();   // the new cells show their value at once
  }

  /* ---------- Mouse: each button shows how long its clicks were held ---------- */
  function decorateMouse(root) {
    const rows = $$('#mouse-rows .mouse-row', root), groups = ['left', 'right', 'middle', 'x1+x2'];
    rows.forEach((row, i) => {
      if (!groups[i] || row.querySelector('.hv')) return;
      const hv = document.createElement('em'); hv.className = 'hv';
      hv.innerHTML = 'Held ' + slot(S('B', groups[i], 'h')) + '<span class="hsep"> · average </span>' + slot(S('B', groups[i], 'a'));
      row.insertBefore(hv, row.querySelector('strong'));
    });
    const box = $('#scroll-rows', root);
    if (box && !box.querySelector('.hv-extra')) box.insertAdjacentHTML('beforeend', '<div class="mouse-row hv-extra"><span>Scroll-active time ' + estBadge + '</span><strong>' + slot(S('G', 'st')) + '</strong></div><div class="mouse-row hv-extra"><span>Scroll sessions</span><strong>' + slot(S('G', 'sn')) + '</strong></div>');
    fillAll();
  }
  /* The Keyboard list and the Mouse rows are drawn by v24h.js with rows that are never rebuilt (the old
     cells were re-created every time a key was pressed, which made their numbers vanish and reappear). */
  addLive('keyboard', '<section class="panel section-gap" style="--i:2">', 'keys');
  addLive('mouse', '<div class="grid split-grid section-gap"', 'mouse');

  /* ---------- Overview: the three cards show how long their input was held ---------- */
  function decorateOverview(root) {
    const cards = $$('.stats-grid .stat-card', root).slice(0, 3);
    const defs = [['Time held', S('G', 'kt'), 'kAvgSub'], ['Time held', S('G', 'mt'), 'mAvgSub'], ['Scrolling time ' + estBadge, S('G', 'st'), 'sSub']];
    cards.forEach((c, i) => {
      if (c.querySelector('.stat-held')) return;
      const h = document.createElement('div'); h.className = 'stat-held';
      h.innerHTML = '<span class="sh-l">' + defs[i][0] + '</span>' + slot(defs[i][1], 'sh-v') + '<em class="sh-s">' + tslot(defs[i][2]) + '</em>';
      c.appendChild(h);
    });
    fillAll();
  }
  (function () {
    const p = pages.overview; if (!p) return;
    withService(p, {}, { mount: root => decorateOverview(root) });
  })();

  /* ---------- Analytics: Input duration ---------- */
  const dcard = (t, sl, sub, est) => '<div class="dur-card"><div class="dur-t">' + t + (est ? ' ' + estBadge : '') + '</div><div class="dur-v">' + slot(sl) + '</div><div class="dur-s">' + tslot(sub) + '</div></div>';
  function durationHTML() {
    return '<section class="panel section-gap" id="dur-panel" style="--i:5"><div class="panel-head"><div><div class="panel-title">Input duration</div><div class="panel-hint">How long keys and mouse buttons were held in the range above. Measured from press to release, and counting live while you hold.</div></div></div><div id="dur-body"><div class="empty-mini">Loading…</div></div></section>';
  }
  /* The structure (which keys, which buttons) is drawn once and only redrawn when it changes. Every
     number in it is a slot, so the numbers are never redrawn: they just keep rolling. */
  function buildDuration() {
    const root = LIVE.root, box = root && $('#dur-body', root), r = LIVE.dur; if (!box || !r) return;
    const kN = cntN(r.keys), mN = cntN(r.mouse), sc = r.scroll || {};
    if (!kN && !mN && !sc.n && !LIVE.items.size) { setHTML(box, '<div class="empty-mini">No hold times recorded in this range yet. Hold times are measured from version 2.4.0 on; earlier days show Not recorded.</div>'); return; }
    const keys = Object.entries(r.keys).sort((a, b) => b[1].ms - a[1].ms || a[0].localeCompare(b[0])).slice(0, 10).map(e => e[0]);
    const btns = Object.entries(r.mouse).sort((a, b) => b[1].ms - a[1].ms || a[0].localeCompare(b[0])).map(e => e[0]);
    const dirs = Object.entries(sc.d || {}).filter(x => x[1]).map(([d]) => '<span>' + esc(dirName[d] || d) + ' ' + slot(S('G', 'dir_' + d)) + '</span>').join('');
    const krows = keys.map(id => '<tr><td><span class="key-chip">' + esc(label(id)) + '</span></td><td class="num">' + slot(S('K', id, 'h')) + '</td><td class="num">' + slot(S('K', id, 'a')) + '</td><td class="num">' + slot(S('K', id, 'l')) + '</td><td><div class="bar"><i data-bar="' + esc(S('K', id)) + '"></i></div></td></tr>').join('');
    const brows = btns.map(id => '<tr><td>' + esc(btnName[id] || id) + '</td><td class="num">' + slot(S('B', id, 'h')) + '</td><td class="num">' + slot(S('B', id, 'a')) + '</td><td class="num">' + slot(S('B', id, 'l')) + '</td><td><div class="bar"><i data-bar="' + esc(S('M', id)) + '"></i></div></td></tr>').join('');
    setHTML(box,
      '<div class="dur-cards">' + dcard('Key-held time', S('G', 'kt'), 'dkSub') + dcard('Mouse-button-held time', S('G', 'mt'), 'dmSub') + dcard('Scroll-active time', S('G', 'st'), 'dsSub', true) + dcard('Longest hold', S('G', 'lg'), 'dlSub') + '</div>' +
      '<div class="grid two-col" style="margin-top:16px"><div><div class="panel-title" style="margin-bottom:8px">Most-held keys</div>' +
      (keys.length ? '<table class="table"><thead><tr><th>Key</th><th class="num">Held</th><th class="num">Average</th><th class="num">Longest</th><th style="width:26%">&nbsp;</th></tr></thead><tbody>' + krows + '</tbody></table>' : '<div class="empty-mini">No key holds yet.</div>') + '</div>' +
      '<div><div class="panel-title" style="margin-bottom:8px">Mouse-button hold time</div>' +
      (btns.length ? '<table class="table"><thead><tr><th>Button</th><th class="num">Held</th><th class="num">Average</th><th class="num">Longest</th><th style="width:26%">&nbsp;</th></tr></thead><tbody>' + brows + '</tbody></table>' : '<div class="empty-mini">No button holds yet.</div>') +
      '<div class="panel-title" style="margin:18px 0 8px">Scroll sessions ' + estBadge + '</div><div class="legend dur-legend"><span>Sessions ' + slot(S('G', 'sn')) + '</span><span>Longest ' + slot(S('G', 'sl')) + '</span>' + dirs + '</div></div></div>' +
      '<div class="dur-note">These totals add up every hold, so they can be longer than the time that passed (several keys can be held at once). They are not time spent working. Scroll time is estimated from the gaps between wheel events. Hold times are measured from ' + esc(r.since || 'version 2.4.0') + ' on; earlier days show Not recorded and are left out of averages.</div>');
    fillAll();
  }
  (function () {
    const p = pages.analytics; if (!p) return;
    const h0 = p.html;
    p.html = function () { return h0.call(this).replace(/<\/div>\s*$/, durationHTML() + '</div>'); };
    withService(p, { onDur: buildDuration });
  })();

  /* ================= Applications page ================= */
  const A = { open: new Set(), sort: 'used', q: '', hide0: true, kmode: 'count', filter: 'all', rows: new Map() };
  const heldMs = a => sumMs(a.kh) + sumMs(a.mh);
  const sorters = {
    used: (a, b) => appTotal(b) - appTotal(a), recent: (a, b) => b.last - a.last, oldest: (a, b) => (a.last || 1e12) - (b.last || 1e12),
    az: (a, b) => appLabel(a.name).localeCompare(appLabel(b.name)), held: (a, b) => heldMs(b) - heldMs(a)
  };
  const scardS = (lbl, sl, sub, ic, color) => '<div class="stat-card" style="--c:' + color + '"><div class="stat-top"><span>' + lbl + '</span><span class="stat-icon">' + P.svg(ic) + '</span></div><div class="stat-value">' + slot(sl) + '</div><div class="stat-sub">' + sub + '</div></div>';
  /* The cells of a row never change: names and labels are fixed, every number is a slot. */
  function rowCells(a) {
    const n = a.name, m = (lbl, big, count, unit, tip) => '<span class="am"><small>' + lbl + '</small><b' + (tip ? ' title="' + tip + '"' : '') + '>' + slot(S('P', n, big)) + '</b><em>' + slot(S('P', n, count)) + ' ' + unit + '</em></span>';
    return '<span class="app-id"><b title="' + esc(n) + '">' + esc(appLabel(n)) + '</b><small>' + esc(n) + (isSelf(n) ? ' · this app' : '') + '</small></span>' +
      m('Keys held', 'kh', 'kc', 'key presses', 'Measured from press to release. ' + NA) + m('Buttons held', 'mh', 'cc', 'mouse clicks', 'Measured from press to release. ' + NA) +
      m('Scrolling ' + estBadge, 'sh', 'sc', 'scroll events', EST) +
      '<span class="am share"><small>Share of input</small><b class="plain">' + slot(S('P', n, 'sp')) + '</b><em>' + tslot(S('P', n, 'last')) + '</em></span>';
  }
  function makeRow(a) {
    const el = document.createElement('div');
    el.className = 'app-card' + (isSelf(a.name) ? ' self' : ''); el.dataset.app = a.name;
    el.innerHTML = '<button type="button" class="app-top" data-toggle aria-expanded="false">' + icon(a.name) + '<span class="app-cells">' + rowCells(a) + '</span><span class="chev" aria-hidden="true"></span></button><div class="app-detail-wrap"><div class="app-detail-in"></div></div>';
    return { el, btn: el.querySelector('.app-top'), inn: el.querySelector('.app-detail-in'), built: false };
  }
  function mouseSVG(uid) {
    return '<svg viewBox="0 0 120 200"><defs><clipPath id="mc' + uid + '"><rect x="10" y="6" width="100" height="188" rx="50"/></clipPath></defs><rect class="mbody" x="10" y="6" width="100" height="188" rx="50"/><g clip-path="url(#mc' + uid + ')"><rect class="mpart" data-b="left" x="10" y="6" width="49" height="76"/><rect class="mpart" data-b="right" x="61" y="6" width="49" height="76"/></g><rect class="mpart" data-b="middle" x="54" y="26" width="12" height="28" rx="6"/><rect class="mpart" data-b="x1" x="3" y="92" width="9" height="24" rx="4"/><rect class="mpart" data-b="x2" x="3" y="120" width="9" height="24" rx="4"/></svg>';
  }
  /* The detail is built once, when a program is first opened; later it is only updated in place. */
  function buildDetail(r, a) {
    const caps = (P.KEYROWS || []).map(row => '<div class="keyboard-row">' + row.map(k => { const id = Array.isArray(k) ? k[0] : k, cls = Array.isArray(k) ? k[1] : ''; return '<div class="keycap ' + cls + '" data-key="' + esc(id) + '" tabindex="0"><span>' + esc(label(id)) + '</span><i class="key-count"></i></div>'; }).join('') + '</div>').join('');
    const uid = a.name.replace(/[^a-z0-9]/gi, '');
    r.inn.innerHTML = '<div class="app-detail">' +
      '<div class="ad-sec"><div class="ad-head"><div class="panel-title">Keyboard</div><div class="segmented ad-kmode"><button type="button" data-km="count">Most pressed</button><button type="button" data-km="held">Most held</button></div></div><div class="keyboard-board ad-board">' + caps + '</div><div class="ad-ktable"></div></div>' +
      '<div class="ad-sec"><div class="ad-head"><div class="panel-title">Mouse buttons</div></div><div class="ad-mouse"><div class="mouse-illustration">' + mouseSVG(uid) + '</div><div class="ad-mtable"></div></div></div>' +
      '<div class="ad-sec"><div class="ad-head"><div class="panel-title">Scrolling ' + estBadge + '</div></div><div class="ad-scroll"></div></div></div>';
    r.built = true;
  }
  const kAbbr = c => c ? (c >= 1e4 ? Math.round(c / 1e3) + 'k' : c >= 1e3 ? (c / 1e3).toFixed(1) + 'k' : String(c)) : '';
  function updateDetail(r, a) {
    if (!r.built) buildDetail(r, a);
    const n = a.name, byMs = A.kmode === 'held', ki = a.ki || {}, kh = a.kh || {};
    const m = byMs ? Object.fromEntries(Object.entries(kh).map(([k, h]) => [k, h.ms])) : ki, max = Math.max(1, ...Object.values(m));
    $$('.ad-kmode button', r.inn).forEach(b => b.classList.toggle('active', b.dataset.km === A.kmode));
    $$('.keycap', r.inn).forEach(c => {
      const id = c.dataset.key, v = m[id] || 0, cnt = ki[id] || 0, h = kh[id], kc = c.querySelector('.key-count'), txt = kAbbr(cnt);
      c.classList.toggle('used', v > 0); c.style.setProperty('--heat', (v ? 14 + 66 * v / max : 0).toFixed(0) + '%');
      if (kc.textContent !== txt) kc.textContent = txt;
      c.title = label(id) + ': ' + fmt(cnt) + ' presses' + (h ? ', held ' + ms(h.ms) + ' in total, longest ' + ms(h.mx) : '');
    });
    const tot = Math.max(1, Object.values(ki).reduce((s, x) => s + x, 0));
    const list = Object.keys(ki).sort((x, y) => byMs ? ((kh[y] || {}).ms || 0) - ((kh[x] || {}).ms || 0) : ki[y] - ki[x] || x.localeCompare(y)).slice(0, 15);
    setHTML($('.ad-ktable', r.inn), list.length ? '<div class="table-scroll"><table class="table"><thead><tr><th>Key</th><th class="num">Presses</th><th class="num">Share</th><th class="num">Held</th><th class="num">Average</th><th class="num">Longest</th></tr></thead><tbody>' + list.map(id => { const h = kh[id]; return '<tr><td><span class="key-chip">' + esc(label(id)) + '</span></td><td class="num">' + slot(S('Q', n, 'k', id, 'p')) + '</td><td class="num">' + Math.round(100 * ki[id] / tot) + '%</td><td class="num">' + slot(S('Q', n, 'k', id, 'h')) + '</td><td class="num">' + (h ? avg(h.ms, h.n) : '–') + '</td><td class="num">' + (h ? ms(h.mx) : '–') + '</td></tr>'; }).join('') + '</tbody></table></div>' : '<div class="empty-mini">No key presses in this program for the range.</div>');
    const mi = a.mi || {}, mh = a.mh || {}, mmax = Math.max(1, ...Object.values(mi));
    $$('.mpart', r.inn).forEach(p => { p.style.setProperty('--heat', (mi[p.dataset.b] ? 14 + 70 * mi[p.dataset.b] / mmax : 0).toFixed(0) + '%'); });
    const mrows = Object.keys(mi).sort((x, y) => mi[y] - mi[x] || x.localeCompare(y));
    setHTML($('.ad-mtable', r.inn), mrows.length ? '<div class="table-scroll"><table class="table"><thead><tr><th>Button</th><th class="num">Presses</th><th class="num">Held</th><th class="num">Average</th><th class="num">Longest</th></tr></thead><tbody>' + mrows.map(id => { const h = mh[id]; return '<tr><td>' + esc(btnName[id] || id) + '</td><td class="num">' + slot(S('Q', n, 'm', id, 'p')) + '</td><td class="num">' + slot(S('Q', n, 'm', id, 'h')) + '</td><td class="num">' + (h ? avg(h.ms, h.n) : '–') + '</td><td class="num">' + (h ? ms(h.mx) : '–') + '</td></tr>'; }).join('') + '</tbody></table></div>' : '<div class="empty-mini">No mouse clicks in this program for the range.</div>');
    const sd = a.sd || {};
    const tile = (lbl, v) => '<div class="ad-tile"><small>' + lbl + '</small><b>' + v + '</b></div>';
    setHTML($('.ad-scroll', r.inn), '<div class="ad-tiles">' + ['up', 'down', 'left', 'right'].map(d => tile(dirName[d], fmt(sd[d] || 0))).join('') + tile('Scroll events', slot(S('P', n, 'sc'))) + tile('Sessions', fmt(a.sess || 0)) + tile('Scroll-active time', a.sess ? slot(S('P', n, 'sh')) : (a.scroll ? 'Not recorded' : '–')) + '</div>');
  }
  function renderApps() {
    const box = $('#apps-list'), c = $('#content'), d = LIVE.apps; if (!box || !d) return;
    const top = c ? c.scrollTop : 0;
    if (!d.tracking) { box.innerHTML = '<div class="empty-mini">Application tracking is off. Turn it on in Settings.</div>'; A.rows.clear(); return; }
    let list = d.apps.filter(a => !A.hide0 || appTotal(a) > 0);
    const needle = A.q.trim().toLowerCase();
    if (needle) list = list.filter(a => (a.name + ' ' + appLabel(a.name)).toLowerCase().includes(needle));
    if (A.filter === 'keys') list = list.filter(a => a.keys > 0); else if (A.filter === 'mouse') list = list.filter(a => a.clicks > 0); else if (A.filter === 'scroll') list = list.filter(a => a.scroll > 0);
    list = list.slice().sort(sorters[A.sort]);
    /* the notice and the empty message are separate elements, so the rows are never rebuilt */
    let note = $('#apps-note'), empty = $('#apps-empty');
    if (!note) { box.insertAdjacentHTML('afterend', '<div class="apps-other" id="apps-note"></div>'); note = $('#apps-note'); }
    const keysSum = d.apps.reduce((s, a) => s + a.keys, 0), other = Math.max(0, (d.total_keys || 0) - keysSum);
    note.textContent = other ? fmt(other) + ' key presses are not tied to a program (counted before application tracking existed, or while no window had focus).' : '';
    if (!empty) { box.insertAdjacentHTML('beforebegin', '<div class="empty-mini" id="apps-empty" hidden></div>'); empty = $('#apps-empty'); }
    empty.hidden = list.length > 0;
    empty.textContent = d.apps.length ? 'No program matches.' : 'No program has received input in this range yet.';
    const loading = box.querySelector('.empty-mini'); if (loading) loading.remove();
    /* remember where every row is, so the rows that move can slide */
    const before = new Map(); A.rows.forEach((r, n) => { if (r.el.isConnected) before.set(n, r.el.getBoundingClientRect().top); });
    const want = new Set(list.map(a => a.name)), firstPaint = A.rows.size === 0;
    A.rows.forEach((r, n) => { if (!want.has(n)) { r.el.classList.add('leaving'); setTimeout(() => r.el.remove(), 300); A.rows.delete(n); } });
    let cursor = box.firstElementChild, i = 0;
    list.forEach(a => {
      let r = A.rows.get(a.name), fresh = false;
      if (!r) { r = makeRow(a); A.rows.set(a.name, r); fresh = true; }
      const open = A.open.has(a.name);
      if (fresh) { r.el.classList.add('enter'); if (firstPaint && i < 14) r.el.style.animationDelay = (i * 45) + 'ms'; if (open) { r.el.classList.add('open'); r.btn.setAttribute('aria-expanded', 'true'); } }
      if (open) updateDetail(r, a);
      while (cursor && cursor.classList.contains('leaving')) cursor = cursor.nextElementSibling;
      if (cursor === r.el) cursor = cursor.nextElementSibling; else box.insertBefore(r.el, cursor);   // only rows that must move are moved
      i++;
    });
    if (c && c.scrollTop !== top) c.scrollTop = top;   // a redraw must never move the page
    fillAll();                                          // new rows show their numbers at once
    if (animOK()) A.rows.forEach((r, n) => {
      const was = before.get(n); if (was == null) return;
      const dy = was - r.el.getBoundingClientRect().top; if (Math.abs(dy) < 2) return;
      r.el.style.transition = 'none'; r.el.style.transform = 'translateY(' + dy + 'px)';
      void r.el.offsetWidth;
      r.el.style.transition = 'transform .55s cubic-bezier(.2,.85,.25,1)'; r.el.style.transform = '';
      setTimeout(() => { r.el.style.transition = ''; }, 600);
    });
  }
  function toggleRow(name) {
    const r = A.rows.get(name), a = LIVE.appMap.get(name); if (!r || !a) return;
    if (A.open.has(name)) { A.open.delete(name); r.el.classList.remove('open'); r.btn.setAttribute('aria-expanded', 'false'); return; }
    A.open.add(name);
    updateDetail(r, a); fillAll();                         // build it while still folded, then unfold
    requestAnimationFrame(() => requestAnimationFrame(() => { r.el.classList.add('open'); r.btn.setAttribute('aria-expanded', 'true'); }));
  }
  pages.apps = {
    html() {
      const opt = (v, l, c) => '<option value="' + v + '"' + (c === v ? ' selected' : '') + '>' + l + '</option>';
      return '<div class="page-wrap">' + P.header('APPLICATIONS', 'Where your input went', 'Which programs received your keys, clicks and scrolls. Open a program to see exactly what you pressed in it.', P.rangeControls()) +
        '<div class="grid stats-grid" style="--i:1" id="apps-stats">' + scardS('Programs', S('A', 'programs'), 'Received input in this period', 'folder', 'var(--accent)') + scardS('Key presses', S('A', 'keys'), 'In this period', 'keyboard', 'var(--accent2)') + scardS('Mouse clicks', S('A', 'clicks'), 'In this period', 'click', 'var(--amber)') + scardS('Scroll events', S('A', 'scroll'), 'Wheel movements', 'scroll', 'var(--green)') + '</div>' + liveHTML('both') +
        '<section class="panel section-gap" style="--i:3"><div class="panel-head"><div><div class="panel-title">Programs</div><div class="panel-hint">Open a program to see its keyboard, mouse buttons and scrolling. The numbers update live.</div></div></div><div class="apps-tools"><input class="text-input" id="app-q" type="search" placeholder="Search programs" autocomplete="off" value="' + esc(A.q) + '"><select class="select-control" id="app-sort">' + opt('used', 'Most used', A.sort) + opt('recent', 'Most active recently', A.sort) + opt('oldest', 'Oldest activity', A.sort) + opt('az', 'A to Z', A.sort) + opt('held', 'Longest hold time', A.sort) + '</select><select class="select-control" id="app-filter">' + opt('all', 'All input', A.filter) + opt('keys', 'With keyboard', A.filter) + opt('mouse', 'With mouse', A.filter) + opt('scroll', 'With scrolling', A.filter) + '</select></div><div id="apps-list" class="apps-list"><div class="empty-mini">Loading…</div></div>' +
        '<div class="dur-note">Every program that had focus is counted, including KeyClick Tracker itself. Only the program file name and its icon are kept: never window titles, paths or what you typed. Each hold belongs to the program in front when the key or button went down. Hold times start with version 2.4.0; earlier days show Not recorded. Scrolling time is estimated.</div></section></div>';
    },
    mount(root) {
      A.rows = new Map();
      /* on this page's own element (see History): a click must fire once, however often the page was visited */
      const wrap = $('.page-wrap', root);
      if (wrap) {
        wrap.addEventListener('click', e => {
          const t = e.target.closest('[data-toggle]'); if (t) { toggleRow(t.closest('.app-card').dataset.app); return; }
          const km = e.target.closest('[data-km]');
          if (km) { A.kmode = km.dataset.km; A.rows.forEach((r, n) => { const a = LIVE.appMap.get(n); if (A.open.has(n) && a) updateDetail(r, a); }); fillAll(); }
        });
        wrap.addEventListener('input', e => { if (e.target.id === 'app-q') { A.q = e.target.value; renderApps(); } });
        wrap.addEventListener('change', e => { if (e.target.id === 'app-sort') { A.sort = e.target.value; renderApps(); } if (e.target.id === 'app-filter') { A.filter = e.target.value; renderApps(); } });
      }
      startService({ apps: true, onApps: renderApps });
    },
    onKeys(f) { pulseApps(f); },
    unmount() { stopService(); }
  };

  /* ================= keyed lists (rows that are never rebuilt) ================= */
  /* Keeps the rows of a list alive: a new row rises in, a row that goes fades out, a row that moves
     slides to its place. items: [{ key, html }], html is ONE element. Rows are identified by data-rk. */
  function reconcile(box, items) {
    const cur = new Map();
    [...box.children].forEach(el => { if (el.dataset && el.dataset.rk != null) cur.set(el.dataset.rk, el); });
    const before = new Map(); cur.forEach((el, k) => before.set(k, el.getBoundingClientRect().top));
    const want = new Set(items.map(i => i.key));
    cur.forEach((el, k) => { if (!want.has(k)) { el.classList.add('leaving'); setTimeout(() => el.remove(), 300); cur.delete(k); } });
    const first = before.size === 0;
    let cursor = box.firstElementChild, n = 0;
    items.forEach(it => {
      let el = cur.get(it.key);
      if (!el) {
        const t = document.createElement('div'); t.innerHTML = it.html; el = t.firstElementChild;
        el.dataset.rk = it.key; el.classList.add('enter'); if (first && n < 14) el.style.animationDelay = (n * 40) + 'ms';
        el.addEventListener('animationend', () => { el.classList.remove('enter'); el.style.animationDelay = ''; }, { once: true });
        cur.set(it.key, el);
      }
      while (cursor && cursor.classList.contains('leaving')) cursor = cursor.nextElementSibling;
      if (cursor === el) cursor = cursor.nextElementSibling; else box.insertBefore(el, cursor);
      n++;
    });
    if (animOK()) cur.forEach((el, k) => {
      const was = before.get(k); if (was == null) return;
      const dy = was - el.getBoundingClientRect().top; if (Math.abs(dy) < 2) return;
      el.style.transition = 'none'; el.style.transform = 'translateY(' + dy + 'px)';
      void el.offsetWidth;
      el.style.transition = 'transform .5s cubic-bezier(.2,.85,.25,1)'; el.style.transform = '';
      setTimeout(() => { el.style.transition = ''; }, 560);
    });
  }

  /* Input that just happened is looked at at once, not at the next poll. */
  const onKeys0 = KC.onKeys;
  KC.onKeys = function () { if (onKeys0) onKeys0.apply(this, arguments); if (LIVE.page) pollHeld(true); };

  /* for the files loaded after this one (v24h.js) */
  KC.v24 = { S, SEP, EXT, LIVE, slot, tslot, fillAll, setNum, memo, animOK, withService, startService, stopService, pollData, reconcile, ms, avg, sumMs, cntN, maxMs, btnName, dirName, estBadge, when, lastFmt, label, appLabel, icon, setHTML, pollHeld };
})();
