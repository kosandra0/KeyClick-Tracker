/* Analytics page and its insight engine.
   Everything here is plain arithmetic on the counts the app already keeps (no text is ever stored, nothing
   leaves this computer). The numbers are recomputed on every live update, so the page moves as you work. */
(function () {
  'use strict';
  const { $, $$, state, fmt, esc, keyLabel } = KC;
  const P = KC.pg, n0 = P.n0, svg = P.svg;
  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
  const pad = n => String(n).padStart(2, '0');
  const ymd = d => d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
  const sum = a => a.reduce((s, x) => s + x, 0);
  const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  const hrTotal = h => n0(h.keys) + n0(h.clicks) + n0(h.scrolls);
  const hourName = KC.hourName;
  let mode = 'trend';

  /* ---------------------------------------------------------------- engine */
  const empty = () => ({ ready: false, days: 0, score: 0, cons: 0, momAbs: 0, mom: 0, proj: 0, today: 0, avg: 0, insights: [], traits: [], persona: { name: 'Still learning', sub: 'Keep working and the engine will describe your style.' }, paceRatio: null, cs: 0 });

  function keyCat(id) {
    if (/^[A-Z]$/.test(id)) return 'Letters';
    if (/^[0-9]$/.test(id) || /^Numpad/.test(id)) return 'Numbers';
    if (id === 'Space') return 'Space';
    if (id === 'Enter') return 'Enter';
    if (/^(Shift|Control|Alt|Meta)/.test(id) || id === 'CapsLock') return 'Modifiers';
    if (/^(Backspace|Delete|Insert|Tab)$/.test(id)) return 'Editing';
    if (/^(Arrow|Home|End|Page)/.test(id)) return 'Navigation';
    if (/^F\d+$/.test(id)) return 'Function keys';
    return 'Symbols & other';
  }
  const CATS = ['Letters', 'Numbers', 'Space', 'Enter', 'Editing', 'Navigation', 'Modifiers', 'Function keys', 'Symbols & other'];

  function compute() {
    const a = state.all, s = state.stats;
    if (!a || !s) return (state.an = state.an || empty());
    const an = empty();
    const now = new Date();
    const daily = (a.series || []).slice().sort((x, y) => (x.key < y.key ? -1 : 1));
    const byKey = new Map(daily.map(p => [p.key, n0(p.events)]));
    const first = daily.length ? daily[0].key : ymd(now);
    const dense = [];
    for (let i = 59; i >= 0; i--) { const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() - i); dense.push({ key: ymd(d), v: byKey.get(ymd(d)) || 0, dow: (d.getDay() + 6) % 7 }); }
    const today = dense[59].v, hist = dense.slice(0, 59).filter(d => d.key >= first);
    const act = hist.map(d => d.v).filter(v => v > 0);
    an.days = hist.length; an.today = today;
    const mean = act.length ? sum(act) / act.length : 0;
    const sd = act.length > 1 ? Math.sqrt(sum(act.map(v => (v - mean) ** 2)) / act.length) : 0;
    an.avg = Math.round(mean);
    an.ready = hist.length >= 3 && act.length >= 2;

    /* typical shape of a day (all-time hour of day) -> how much of a normal day is usually done by now */
    const hs = (a.hourOfDay || []).map(hrTotal); while (hs.length < 24) hs.push(0);
    const tot = sum(hs), frac = now.getHours() + now.getMinutes() / 60;
    let done = 0; for (let h = 0; h < 24; h++) done += h + 1 <= frac ? hs[h] : h < frac ? hs[h] * (frac - h) : 0;
    const cs = tot ? done / tot : frac / 24; an.cs = cs;
    an.proj = today ? Math.round(cs >= .08 ? Math.max(today, today / cs) : Math.max(today, mean)) : 0;
    an.paceRatio = mean && cs >= .1 ? today / (mean * cs) : null;

    /* consistency, momentum, trend, score */
    const last30 = hist.slice(-30), activeShare = last30.length ? last30.filter(d => d.v > 0).length / last30.length : 0;
    const cv = mean ? sd / mean : 1.5;
    an.cons = Math.round(100 * (.6 * activeShare + .4 * (1 - clamp(cv, 0, 1.5) / 1.5)));
    const rec = dense.slice(52, 59), prv = dense.slice(45, 52), rs = sum(rec.map(d => d.v)), ps = sum(prv.map(d => d.v));
    const haveWeeks = hist.length >= 14;
    an.mom = haveWeeks ? (ps ? (rs - ps) / ps * 100 : rs ? 100 : 0) : 0;
    an.momAbs = Math.round(Math.abs(an.mom));
    const volume = mean ? clamp(today / (mean * Math.max(cs, .15)), 0, 1.3) / 1.3 * 100 : today ? 50 : 0;
    an.score = an.ready ? Math.round(.4 * an.cons + .35 * volume + .25 * clamp(50 + an.mom / 2, 0, 100)) : 0;
    an.kScore = an.ready ? an.score : null; an.kCons = an.ready ? an.cons : null; an.kMom = haveWeeks ? an.momAbs : null;
    an.act30 = last30.filter(d => d.v > 0).length; an.n30 = last30.length;

    /* rhythm facts */
    const share = (lo, hi) => tot ? sum(hs.filter((_, h) => (lo <= hi ? h >= lo && h <= hi : h >= lo || h <= hi))) / tot : 0;
    const part = { morning: share(5, 11), afternoon: share(12, 16), evening: share(17, 21), night: share(22, 4) };
    const topPart = Object.keys(part).sort((x, y) => part[y] - part[x])[0];
    let bw = 0, bs = -1; for (let h = 0; h < 24; h++) { const v = hs[h] + hs[(h + 1) % 24] + hs[(h + 2) % 24]; if (v > bs) { bs = v; bw = h; } }
    const focus = tot ? bs / tot : 0;

    /* weekday vs weekend (per day, so an uneven number of days does not skew it) */
    const dowTot = [0, 0, 0, 0, 0, 0, 0], dowN = [0, 0, 0, 0, 0, 0, 0];
    daily.forEach(p => { const d = new Date(p.key + 'T00:00'), w = (d.getDay() + 6) % 7; dowTot[w] += n0(p.events); dowN[w]++; });
    const wk = sum(dowTot.slice(0, 5)) / Math.max(1, sum(dowN.slice(0, 5))), we = sum(dowTot.slice(5)) / Math.max(1, sum(dowN.slice(5)));
    const dowAvg = dowTot.map((v, i) => (dowN[i] ? v / dowN[i] : 0)); const bestDow = dowAvg.indexOf(Math.max(...dowAvg));

    /* keyboard / mouse / habits */
    const T = a.totals || {}, keys = n0(T.keys), clicks = n0(T.clicks), scrolls = n0(T.scrolls), ev = keys + clicks + scrolls;
    const kShare = keys + clicks ? keys / (keys + clicks) : 0.5;
    const ks = Object.fromEntries((a.keys || []).map(k => [k.id, n0(k.count)]));
    const corr = keys ? ((ks.Backspace || 0) + (ks.Delete || 0)) / keys : 0;
    const wpm = n0((a.rates || {}).avgWpm), peakWpm = n0((a.rates || {}).peakWpm);

    /* anomaly: yesterday against the usual spread */
    const yv = dense[58], z = sd ? (yv.v - mean) / sd : 0;
    const best = daily.reduce((b, p) => (n0(p.events) > b.v ? { v: n0(p.events), k: p.key, l: p.label } : b), { v: 0 });
    const st = a.streak || {};

    /* ------------ insights ------------ */
    const out = [];
    const add = (id, icon, tone, title, text, tag) => out.push({ id, icon, tone, title, text, tag });
    if (!an.ready) add('learn', 'spark', 'note', 'The engine is still learning', (hist.length ? 'Only ' + hist.length + ' earlier day' + (hist.length === 1 ? '' : 's') + ' of history so far.' : 'No earlier days of history yet.') + ' Patterns become reliable after about two weeks of normal use.', 'Needs more days');
    if (an.ready && an.paceRatio != null) {
      const pr = Math.round((an.paceRatio - 1) * 100);
      if (!today) add('pace', 'clock', 'note', 'Nothing recorded yet today', 'A normal day brings about ' + fmt(an.avg) + ' events. Start working and this finding will track your pace live.', 'Live');
      else if (an.paceRatio >= 1.15) add('pace', 'flame', 'good', 'Ahead of your usual pace', 'You are about ' + pr + '% ahead of where a typical day is by now. At this rate today finishes near ' + fmt(an.proj) + ' events (a normal day is ' + fmt(an.avg) + ').', 'Live');
      else if (an.paceRatio <= .7) add('pace', 'clock', 'warn', 'A quieter day so far', 'You are about ' + Math.abs(pr) + '% below your usual pace for this hour. A normal day is ' + fmt(an.avg) + ' events; you are on track for ' + fmt(an.proj) + '.', 'Live');
      else add('pace', 'target', 'note', 'Right on your usual pace', 'Today is tracking close to a typical day, heading for about ' + fmt(an.proj) + ' events.', 'Live');
    }
    if (tot > 200 && an.ready) {
      const names = { morning: ['an early-bird', 'before noon'], afternoon: ['an afternoon', 'between noon and 5 PM'], evening: ['an evening', 'between 5 and 10 PM'], night: ['a night-owl', 'late at night'] };
      add('chrono', 'clock', 'note', 'You work like ' + names[topPart][0] + ' type', Math.round(part[topPart] * 100) + '% of everything you do happens ' + names[topPart][1] + '.', 'All time');
      add('window', 'target', 'good', 'Your sharpest window: ' + hourName(bw) + ' to ' + hourName((bw + 3) % 24), Math.round(focus * 100) + '% of all your activity falls in these three hours. Protect that slot for deep work.', 'All time');
    }
    if (an.ready) {
      add('cons', 'calendar', an.cons >= 70 ? 'good' : an.cons >= 40 ? 'note' : 'warn', 'Consistency: ' + an.cons + ' out of 100', 'You were active on ' + an.act30 + ' of the last ' + an.n30 + ' days' + (cv > .9 ? ', but the size of your days swings a lot.' : cv < .45 ? ', with very even days.' : '.'), 'Last 30 days');
      if (haveWeeks && (rs || ps)) add('mom', 'trend', an.mom >= 8 ? 'good' : an.mom <= -8 ? 'warn' : 'note', an.mom >= 8 ? 'You are speeding up' : an.mom <= -8 ? 'Activity is easing off' : 'Steady week on week', 'The last 7 full days made ' + fmt(rs) + ' events against ' + fmt(ps) + ' the week before (' + (an.mom >= 0 ? '+' : '-') + an.momAbs + '%).', 'Week on week');
      if (Math.abs(z) >= 1.8 && yv.v + mean > 0) add('anom', z > 0 ? 'flame' : 'clock', z > 0 ? 'good' : 'warn', z > 0 ? 'Yesterday was an unusually busy day' : 'Yesterday was unusually quiet', fmt(yv.v) + ' events, ' + (z > 0 ? 'well above' : 'well below') + ' your usual ' + fmt(an.avg) + '.', 'Anomaly check');
    }
    if (dowN.some(n => n > 1) && wk + we > 0) {
      const r = we ? wk / we : 9;
      add('week', 'calendar', 'note', r > 1.6 ? 'A weekday worker' : r < .7 ? 'You are busier at weekends' : 'Weekdays and weekends look alike', 'A weekday averages ' + fmt(Math.round(wk)) + ' events and a weekend day ' + fmt(Math.round(we)) + '. Your strongest day is ' + KC.dayNames[bestDow] + '.', 'All time');
    }
    if (ev > 200) {
      add('style', 'keyboard', 'note', kShare > .8 ? 'Keyboard first' : kShare > .55 ? 'Keyboard and mouse together' : kShare > .35 ? 'A balanced pointer-and-keys style' : 'Mouse-led', 'Keys are ' + Math.round(kShare * 100) + '% of your key and click events. Scrolling makes up ' + Math.round(scrolls / ev * 100) + '% of all activity' + (scrolls / ev > .35 ? ', typical of a lot of reading and browsing.' : '.'), 'All time');
    }
    if (keys > 300) add('corr', 'keyboard', corr > .12 ? 'warn' : 'good', corr > .12 ? 'You edit a lot while typing' : corr < .06 ? 'Clean typing' : 'A normal amount of correcting', (corr * 100).toFixed(1) + '% of your key presses are Backspace or Delete. Most people sit between 6% and 12%.', 'All time');
    if (wpm) add('speed', 'speed', wpm >= 60 ? 'good' : 'note', 'Typing speed: ' + (wpm >= 70 ? 'fast' : wpm >= 40 ? 'comfortable' : 'relaxed'), 'You average ' + KC.rate(wpm, 'w', 1) + ' ' + KC.unitLabel('w') + ' while typing, with a best minute of ' + KC.rate(peakWpm, 'w', 1) + '.', 'Typing minutes');
    if (n0(st.current) >= 2 || n0(st.longest) >= 3) add('streak', 'flame', 'good', 'Streak: ' + n0(st.current) + ' day' + (n0(st.current) === 1 ? '' : 's'), 'Your longest run is ' + n0(st.longest) + ' days in a row, out of ' + n0(st.activeDays) + ' active days in total.', 'All time');
    if (best.v > 0 && mean > 0) add('best', 'trend', 'note', 'Your biggest day: ' + fmt(best.v) + ' events', (best.l || best.k) + ' is the record. A typical day is about ' + fmt(an.avg) + ', so that day was ' + (mean ? (best.v / mean).toFixed(1) : '–') + ' times normal.', 'All time');
    an.insights = out;

    /* ------------ profile ------------ */
    const trait = (name, v, hint) => ({ name, v: Math.round(clamp(v, 0, 100)), hint });
    an.traits = [
      trait('Keyboard-driven', kShare * 100, 'Share of key presses against clicks'),
      trait('Consistency', an.ready ? an.cons : 0, 'How evenly you show up day to day'),
      trait('Focus', focus * 100 * 2.6, 'How concentrated your activity is in your best 3 hours'),
      trait('Early bird', part.morning * 100 * 1.6, 'Share of activity before noon'),
      trait('Night owl', (part.night + part.evening * .35) * 100 * 1.6, 'Share of activity late in the day'),
      trait('Weekend energy', we && wk ? we / (wk + we) * 200 : 0, 'Weekend days against weekdays'),
      trait('Typing speed', wpm / 80 * 100, 'Average words per minute against 80')
    ];
    if (tot > 200 && an.ready) {
      const adj = an.cons > 70 ? 'Steady ' : '';
      const when = { morning: 'Early-bird', afternoon: 'Daytime', evening: 'Evening', night: 'Night-owl' }[topPart];
      const noun = kShare > .8 ? 'Typist' : kShare > .55 ? 'Keyboard Worker' : kShare > .35 ? 'All-rounder' : 'Pointer Navigator';
      an.persona = { name: adj + when + ' ' + noun, sub: 'Built from ' + fmt(ev) + ' events over ' + fmt(daily.length) + ' recorded day' + (daily.length === 1 ? '' : 's') + '. It updates as you work.' };
    }
    an.traits.forEach(t => { an['t_' + t.name.replace(/\W/g, '')] = t.v; });
    an.cats = CATS.map(c => ({ c, n: 0 }));
    (s.keys || []).forEach(k => { an.cats[CATS.indexOf(keyCat(k.id))].n += n0(k.count); });
    return an;
  }
  KC.computeAnalytics = () => { state.an = compute(); return state.an; };

  /* ---------------------------------------------------------------- chart pieces */
  const KPI = (label, path, icon, color, id, sub) => '<div class="stat-card an-kpi" style="--c:' + color + '"><div class="stat-top"><span>' + label + '</span><span class="stat-icon">' + svg(icon) + '</span></div><div class="stat-value"><span data-odo="' + path + '"></span></div><div class="stat-sub" id="' + id + '">' + sub + '</div></div>';

  /* smooth line through points (Catmull-Rom to bezier) */
  function smooth(p) {
    if (p.length < 3) return p.map((q, i) => (i ? 'L' : 'M') + q[0].toFixed(1) + ' ' + q[1].toFixed(1)).join('');
    let d = 'M' + p[0][0].toFixed(1) + ' ' + p[0][1].toFixed(1);
    for (let i = 0; i < p.length - 1; i++) {
      const p0 = p[i - 1] || p[i], p1 = p[i], p2 = p[i + 1], p3 = p[i + 2] || p2, t = .18;
      d += 'C' + (p1[0] + (p2[0] - p0[0]) * t).toFixed(1) + ' ' + (p1[1] + (p2[1] - p0[1]) * t).toFixed(1) + ',' + (p2[0] - (p3[0] - p1[0]) * t).toFixed(1) + ' ' + (p2[1] - (p3[1] - p1[1]) * t).toFixed(1) + ',' + p2[0].toFixed(1) + ' ' + p2[1].toFixed(1);
    }
    return d;
  }
  function lineHTML(pts, cumulative) {
    const W = 700, H = 230, L = 10, R = 10, T = 16, B = 26;
    if (pts.length < 2) return '<div class="empty-mini">A trend needs at least two points.<br>Pick a longer range or a smaller unit.</div>';
    let vals = pts.map(p => p.events); if (cumulative) { let c = 0; vals = vals.map(v => (c += v)); }
    const max = Math.max(1, ...vals), n = vals.length, iw = W - L - R, ih = H - T - B;
    const X = i => L + i * iw / (n - 1), Y = v => T + ih * (1 - v / max);
    const xy = vals.map((v, i) => [X(i), Y(v)]);
    const w = n >= 21 ? 7 : n >= 8 ? 3 : 2, ma = vals.map((_, i) => { const lo = Math.max(0, i - w + 1), seg = vals.slice(lo, i + 1); return sum(seg) / seg.length; });
    const mxy = ma.map((v, i) => [X(i), Y(v)]);
    /* least-squares trend */
    const mx = (n - 1) / 2, my = sum(vals) / n; let nu = 0, de = 0; vals.forEach((v, i) => { nu += (i - mx) * (v - my); de += (i - mx) ** 2; });
    const sl = de ? nu / de : 0, t0 = my - sl * mx, ty = i => clamp(Y(t0 + sl * i), T, T + ih);
    const drawn = document.getElementById('an-line') && document.getElementById('an-line').dataset.drawn;
    let h = '<svg' + (drawn ? '' : ' class="an-reveal"') + ' viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none"><defs><linearGradient id="anfill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="var(--accent)" stop-opacity=".32"/><stop offset="1" stop-color="var(--accent)" stop-opacity="0"/></linearGradient></defs>';
    [0, .33, .66, 1].forEach(t => { const y = T + ih * (1 - t); h += '<line class="chart-gridline" x1="' + L + '" y1="' + y + '" x2="' + (W - R) + '" y2="' + y + '"/>'; });
    const base = T + ih, line = smooth(xy);
    h += '<path class="an-area" d="' + line + 'L' + X(n - 1) + ' ' + base + 'L' + X(0) + ' ' + base + 'Z" fill="url(#anfill)"/><path class="an-line" d="' + line + '"/>';
    if (!cumulative) h += '<path class="an-ma" d="' + smooth(mxy) + '"/><line class="an-trend" x1="' + X(0) + '" y1="' + ty(0) + '" x2="' + X(n - 1) + '" y2="' + ty(n - 1) + '"/>';
    const every = Math.ceil(n / 8); pts.forEach((p, i) => { if (i % every === 0) h += '<text class="chart-label" x="' + X(i) + '" y="' + (H - 7) + '" text-anchor="' + (i === 0 ? 'start' : 'middle') + '">' + esc(String(p.label).replace(/\s+\d{4}$/, '').replace(/^Week of /, 'Wk ')) + '</text>'; });
    if (n <= 70) xy.forEach((q, i) => { h += '<circle class="an-dot" cx="' + q[0].toFixed(1) + '" cy="' + q[1].toFixed(1) + '" r="3.2" data-tip="' + esc(pts[i].label) + '\n' + fmt(vals[i]) + (cumulative ? ' events so far' : ' events') + '"/>'; });
    return h + '</svg>';
  }

  const hoursBars = () => { let h = '<div class="hbars">'; for (let i = 0; i < 24; i++) h += '<div class="hb" data-h="' + i + '"><i></i><span>' + (i % 3 === 0 ? (i % 12 || 12) + (i < 12 ? 'a' : 'p') : '') + '</span></div>'; return h + '</div>'; };
  const dowBars = () => { let h = '<div class="dbars">'; for (let i = 0; i < 7; i++) h += '<div class="db" data-d="' + i + '"><b></b><i></i><span>' + KC.dayNames[i].slice(0, 3) + '</span></div>'; return h + '</div>'; };
  function radialHTML() {
    let h = '<svg viewBox="-40 -14 320 268" class="radial" aria-label="24 hour rhythm"><circle class="rr" cx="120" cy="120" r="34"/><circle class="rr" cx="120" cy="120" r="76"/><circle class="rr" cx="120" cy="120" r="116"/>';
    for (let i = 0; i < 24; i++) { const a = i / 24 * 2 * Math.PI - Math.PI / 2, c = Math.cos(a), s = Math.sin(a); h += '<line class="rl" data-h="' + i + '" x1="' + (120 + c * 38).toFixed(2) + '" y1="' + (120 + s * 38).toFixed(2) + '" x2="' + (120 + c * 118).toFixed(2) + '" y2="' + (120 + s * 118).toFixed(2) + '"/>'; }
    [[0, '12 AM'], [6, '6 AM'], [12, '12 PM'], [18, '6 PM']].forEach(([i, t]) => { const a = i / 24 * 2 * Math.PI - Math.PI / 2; h += '<text class="chart-label" x="' + (120 + Math.cos(a) * 130).toFixed(1) + '" y="' + (123 + Math.sin(a) * 130).toFixed(1) + '" text-anchor="middle">' + t + '</text>'; });
    return h + '</svg>';
  }
  const DR = 2 * Math.PI * 54;
  const donutHTML = () => '<div class="donut"><svg viewBox="0 0 140 140"><circle class="dtrack" cx="70" cy="70" r="54"/>' + ['k', 'c', 's'].map(k => '<circle class="darc d-' + k + '" data-k="' + k + '" cx="70" cy="70" r="54" stroke-dasharray="0 ' + DR.toFixed(2) + '" transform="rotate(-90 70 70)"/>').join('') + '</svg><div class="dmid"><b data-odo="stats.totals.events"></b><small>events</small></div></div>' +
    '<div class="dlegend"><div><i class="d-k"></i><span>Key presses</span><b id="dp-k">0%</b></div><div><i class="d-c"></i><span>Mouse clicks</span><b id="dp-c">0%</b></div><div><i class="d-s"></i><span>Scrolls</span><b id="dp-s">0%</b></div></div>';

  /* a year of days, GitHub style */
  const CAL_WEEKS = 53;
  function calHTML() {
    const now = new Date(), mon = new Date(now.getFullYear(), now.getMonth(), now.getDate() - ((now.getDay() + 6) % 7) - (CAL_WEEKS - 1) * 7);
    let months = '', cells = '', lastM = -1;
    for (let w = 0; w < CAL_WEEKS; w++) {
      const d0 = new Date(mon.getFullYear(), mon.getMonth(), mon.getDate() + w * 7);
      if (d0.getMonth() !== lastM) { lastM = d0.getMonth(); months += '<span style="grid-column:' + (w + 1) + '">' + MON[lastM] + '</span>'; }
      for (let d = 0; d < 7; d++) { const dt = new Date(d0.getFullYear(), d0.getMonth(), d0.getDate() + d); cells += dt > now ? '<i class="cc fut"></i>' : '<i class="cc" data-d="' + ymd(dt) + '"></i>'; }
    }
    return '<div class="cal-year"><div class="cal-months">' + months + '</div><div class="cal-grid">' + cells + '</div></div>';
  }

  /* ---------------------------------------------------------------- patching */
  const sigOf = (el, s) => { if (el._sig === s) return false; el._sig = s; return true; };
  function patchHours(root) {
    const hs = P.S().hourOfDay || [], max = Math.max(1, ...hs.map(hrTotal));
    const box = $('.hbars', root); if (!box || !sigOf(box, hs.map(hrTotal).join())) return;
    $$('.hb', box).forEach(b => { const h = hs[+b.dataset.h] || {}, v = hrTotal(h); b.firstChild.style.height = v ? Math.max(4, v / max * 100) + '%' : '0'; b.dataset.tip = hourName(+b.dataset.h) + '\n' + fmt(v) + ' events\n' + fmt(n0(h.keys)) + ' keys · ' + fmt(n0(h.clicks)) + ' clicks · ' + fmt(n0(h.scrolls)) + ' scrolls'; });
    const rad = $('.radial', root); if (rad) $$('.rl', rad).forEach(l => { const v = hrTotal(hs[+l.dataset.h] || {}); l.style.strokeDasharray = (v ? Math.max(5, v / max * 78) : 2) + ' 200'; l.style.opacity = v ? .45 + .55 * v / max : .25; l.dataset.tip = hourName(+l.dataset.h) + '\n' + fmt(v) + ' events'; });
  }
  function patchDow(root) {
    const ds = P.S().dayOfWeek || [], tot = ds.map(hrTotal), max = Math.max(1, ...tot), box = $('.dbars', root); if (!box || !sigOf(box, tot.join())) return;
    $$('.db', box).forEach(b => { const d = ds[+b.dataset.d] || {}, v = hrTotal(d); b.children[1].style.height = v ? Math.max(4, v / max * 100) + '%' : '0'; b.children[0].textContent = v ? fmt(v) : ''; b.dataset.tip = KC.dayNames[+b.dataset.d] + '\n' + fmt(v) + ' events'; });
  }
  function patchDonut(root) {
    const t = P.S().totals || {}, k = n0(t.keys), c = n0(t.clicks), s = n0(t.scrolls), all = k + c + s, box = $('.donut', root); if (!box || !sigOf(box, [k, c, s].join())) return;
    let off = 0;
    [['k', k], ['c', c], ['s', s]].forEach(([id, v]) => { const len = all ? v / all * DR : 0, el = $('.darc[data-k="' + id + '"]', box); el.style.strokeDasharray = Math.max(0, len - (all && len > 3 ? 2 : 0)).toFixed(2) + ' ' + DR.toFixed(2); el.style.strokeDashoffset = (-off).toFixed(2); off += len; const p = $('#dp-' + id, root); if (p) p.textContent = (all ? Math.round(v / all * 100) : 0) + '%'; });
  }
  function patchCal(root) {
    const a = state.all, box = $('.cal-grid', root); if (!a || !box) return;
    const map = new Map((a.series || []).map(p => [p.key, n0(p.events)])), vals = [...map.values()].filter(v => v > 0).sort((x, y) => x - y), cap = vals.length ? vals[Math.min(vals.length - 1, Math.floor(vals.length * .92))] : 1;
    const tk = ymd(new Date()), full = sigOf(box, vals.length + ':' + cap + ':' + tk);
    const set = c => { const v = map.get(c.dataset.d) || 0; c.style.setProperty('--h', v ? Math.round(16 + Math.min(1, v / cap) * 72) : 0); c.dataset.tip = new Date(c.dataset.d + 'T00:00').toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' }) + '\n' + (v ? fmt(v) + ' events' : 'No activity'); };
    if (full) $$('.cc[data-d]', box).forEach(set); else { const t = $('.cc[data-d="' + tk + '"]', box); if (t) set(t); }
  }
  function patchLine(root) {
    const el = $('#an-line', root); if (!el) return;
    const t = performance.now(); if (el._t && t - el._t < 250) return; el._t = t;
    const pts = P.seriesPoints(), html = lineHTML(pts, mode === 'cum');
    if (el.dataset.h === html) return;
    el.dataset.h = html; el.innerHTML = html;
    if (!el.dataset.drawn) el.dataset.drawn = '1';
  }
  function patchCats(root) {
    const an = state.an, box = $('#an-cats', root); if (!an || !box) return;
    const max = Math.max(1, ...an.cats.map(c => c.n)), tot = Math.max(1, sum(an.cats.map(c => c.n)));
    if (!box.firstChild) box.innerHTML = an.cats.map((c, i) => '<div class="cat"><span>' + c.c + '</span><div class="cbar"><i></i></div><b></b></div>').join('');
    $$('.cat', box).forEach((row, i) => { const c = an.cats[i]; row.querySelector('i').style.width = (c.n / max * 100).toFixed(1) + '%'; row.querySelector('b').textContent = Math.round(c.n / tot * 100) + '%'; row.dataset.tip = c.c + '\n' + fmt(c.n) + ' key presses'; });
  }
  /* The rows below are built once and then patched in place, so live updates never restart an animation. */
  const txt = (el, v) => { if (el && el.textContent !== v) el.textContent = v; };
  function patchTopKeys(root) {
    const box = $('#an-top', root); if (!box) return;
    const ks = (P.S().keys || []).slice().sort((x, y) => y.count - x.count).slice(0, 8), tot = Math.max(1, n0((P.S().totals || {}).keys));
    if (!box.firstChild) box.innerHTML = '<div class="an-sub">Most used keys</div>' + '<div class="tk"><span class="key-chip"></span><div class="cbar"><i></i></div><b></b><small></small></div>'.repeat(8);
    const rows = $$('.tk', box), max = ks.length ? ks[0].count : 1;
    rows.forEach((row, i) => {
      const k = ks[i]; row.style.display = k ? '' : 'none'; if (!k) return;
      txt(row.children[0], keyLabel(k.id)); row.children[1].firstChild.style.width = (k.count / max * 100).toFixed(1) + '%';
      txt(row.children[2], fmt(k.count)); txt(row.children[3], Math.round(k.count / tot * 100) + '%');
    });
  }
  function patchInsights(root, an) {
    const box = $('#an-insights', root); if (!box) return;
    /* a finding's identity is its stable id, never its wording (the wording holds live numbers) */
    const sig = an.insights.map(x => x.id).join('|');
    if (box._sig !== sig) {
      const first = box._sig === undefined;
      box._sig = sig; box._ic = [];
      box.innerHTML = an.insights.map((x, i) => '<div class="an-ins' + (first ? '' : ' still') + '" style="--i:' + Math.min(i, 8) + '"><span class="an-ic"></span><div><div class="an-it"></div><p></p></div><span class="an-tag"></span></div>').join('');
    }
    $$('.an-ins', box).forEach((row, i) => {
      const x = an.insights[i]; if (!x) return;
      ['good', 'warn', 'note'].forEach(t => row.classList.toggle(t, t === x.tone));
      if (box._ic[i] !== x.icon) { box._ic[i] = x.icon; row.querySelector('.an-ic').innerHTML = svg(x.icon); }
      txt(row.querySelector('.an-it'), x.title); txt(row.querySelector('p'), x.text); txt(row.querySelector('.an-tag'), x.tag);
    });
  }
  function profileHTML() {
    const names = [['Keyboard-driven', 'Share of key presses against clicks'], ['Consistency', 'How evenly you show up day to day'], ['Focus', 'How concentrated your activity is in your best 3 hours'], ['Early bird', 'Share of activity before noon'], ['Night owl', 'Share of activity late in the day'], ['Weekend energy', 'Weekend days against weekdays'], ['Typing speed', 'Average words per minute against 80']];
    return '<div class="an-persona"><span class="an-pi">' + svg('brain') + '</span><div><div class="an-pn" id="an-pn"></div><div class="an-ps" id="an-ps"></div></div></div><div class="an-traits">' +
      names.map(t => '<div class="an-trait" data-tip="' + t[1] + '"><span>' + t[0] + '</span><div class="cbar"><i></i></div><b data-odo="an.t_' + t[0].replace(/\W/g, '') + '"></b></div>').join('') + '</div>';
  }
  function patchProfile(root, an) {
    txt($('#an-pn', root), an.persona.name); txt($('#an-ps', root), an.persona.sub);
    $$('.an-trait', root).forEach((row, i) => { const t = an.traits[i]; if (t) row.querySelector('i').style.width = t.v + '%'; });
  }

  /* ---------------------------------------------------------------- page */
  KC.pages.analytics = {
    html() {
      KC.computeAnalytics();
      const unitWord = { hour: 'hour', daily: 'day', weekly: 'week', monthly: 'month', quarterly: 'quarter', yearly: 'year' }[state.unit];
      return '<div class="page-wrap">' + P.header('ANALYTICS', 'Your patterns, understood', 'An automatic engine studies every key, click and scroll as it happens and keeps these charts and findings up to date. Only counts are used, never what you type.', P.rangeControls()) +
        '<div class="grid stats-grid an-kpis" style="--i:1">' + KPI('Activity score', 'an.kScore', 'target', 'var(--accent)', 'an-s-score', '') + KPI('Consistency', 'an.kCons', 'calendar', 'var(--green)', 'an-s-cons', '') + KPI('Momentum', 'an.kMom', 'trend', 'var(--accent2)', 'an-s-mom', '') + KPI('Projected today', 'an.proj', 'flame', 'var(--amber)', 'an-s-proj', '') + '</div>' +
        '<div class="grid split-grid section-gap" style="--i:2"><section class="panel"><div class="panel-head"><div><div class="panel-title">Insight engine</div><div class="panel-hint">Findings written from your own numbers. They refresh as you work.</div></div><span class="an-live"><i></i>Live</span></div><div class="an-insights" id="an-insights"></div></section>' +
        '<section class="panel"><div class="panel-head"><div><div class="panel-title">Your profile</div><div class="panel-hint">How the engine describes your style.</div></div></div><div id="an-profile">' + profileHTML() + '</div></section></div>' +
        '<div class="section-gap">' + P.chartPanel('Activity over time', 'Each bar is one ' + unitWord + '. Change the range and unit above, from hourly to yearly.') + '</div>' +
        '<section class="panel section-gap" style="--i:4"><div class="panel-head"><div><div class="panel-title">Trend and smoothing</div><div class="panel-hint">The line is your activity, the dashed curve is a moving average, and the straight line is the overall direction.</div></div><div class="segmented" id="an-mode"><button data-an="trend" class="' + (mode === 'trend' ? 'active' : '') + '">Trend</button><button data-an="cum" class="' + (mode === 'cum' ? 'active' : '') + '">Cumulative</button></div></div><div class="chart-area an-chart" id="an-line"></div><div class="legend"><span><i></i>Activity</span><span><i class="l2"></i>Moving average</span><span><i class="l4"></i>Direction</span></div></section>' +
        '<div class="grid split-grid an-rhythm section-gap" style="--i:5"><section class="panel"><div class="panel-head"><div><div class="panel-title">Hourly rhythm</div><div class="panel-hint">Which hours of the day you are most active.</div></div></div>' + hoursBars() + '</section>' +
        '<section class="panel"><div class="panel-head"><div><div class="panel-title">24-hour clock</div><div class="panel-hint">The same rhythm, wrapped around the clock.</div></div></div>' + radialHTML() + '</section></div>' +
        '<div class="grid split-grid section-gap" style="--i:6"><section class="panel"><div class="panel-head"><div><div class="panel-title">Weekly rhythm</div><div class="panel-hint">Busiest days of the week.</div></div></div>' + dowBars() + '</section>' +
        '<section class="panel"><div class="panel-head"><div><div class="panel-title">Hour by weekday</div><div class="panel-hint">Darker squares mean busier hours.</div></div></div>' + P.heatHTML() + '</section></div>' +
        '<section class="panel section-gap" style="--i:7"><div class="panel-head"><div><div class="panel-title">The last year</div><div class="panel-hint">One square per day. Hover a square for the exact count.</div></div></div>' + calHTML() + '</section>' +
        '<div class="grid split-grid an-mix section-gap" style="--i:8"><section class="panel"><div class="panel-head"><div><div class="panel-title">Activity mix</div><div class="panel-hint">What your activity is made of in this range.</div></div></div><div class="donut-wrap">' + donutHTML() + '</div></section>' +
        '<section class="panel"><div class="panel-head"><div><div class="panel-title">How you type</div><div class="panel-hint">Key presses grouped by kind, and your most used keys.</div></div></div><div class="an-cats" id="an-cats"></div><div class="an-top" id="an-top"></div></section></div>' +
        '<section class="panel section-gap" style="--i:9"><div class="panel-head"><div><div class="panel-title">All-time records</div><div class="panel-hint">Your personal bests since you started counting.</div></div></div><div class="records" id="records"></div></section></div>';
    },
    mount(root) {
      root.addEventListener('click', e => {
        const b = e.target.closest('[data-an]'); if (!b) return;
        mode = b.dataset.an; $$('#an-mode button', root).forEach(x => x.classList.toggle('active', x === b));
        const el = $('#an-line', root); if (el) { el._t = 0; el.dataset.h = ''; } patchLine(root);
      });
    },
    update(root) {
      const an = state.an || KC.computeAnalytics(), set = (id, v) => { const el = $('#' + id, root); if (el && el.textContent !== v) el.textContent = v; };
      P.drawChart($('#main-chart', root));
      patchLine(root); patchHours(root); patchDow(root); patchDonut(root); patchCal(root); patchCats(root); P.updateHeat(root);
      patchTopKeys(root); patchInsights(root, an); patchProfile(root, an);
      KC.updateRecords($('#records', root), P.recordsHTML());
      set('an-s-score', !an.ready ? 'Learning your habits' : an.score >= 75 ? 'Excellent: strong pace and rhythm' : an.score >= 50 ? 'Good: steady and on track' : 'Room to build: a lighter stretch');
      set('an-s-cons', an.ready ? 'Active ' + an.act30 + ' of the last ' + an.n30 + ' days' : 'Needs a few more days');
      set('an-s-mom', an.days >= 14 ? (an.mom >= 8 ? '↑ ' : an.mom <= -8 ? '↓ ' : '→ ') + 'Percent change on the previous 7 days' : 'Needs two weeks of history');
      set('an-s-proj', an.today ? 'Events so far today: ' + fmt(an.today) : 'Nothing yet today');
    }
  };
})();
