/* Live overlay: shows current typing speed and click rate; stays on top of other windows. */
(function () {
  'use strict';
  const q = new URLSearchParams(location.search), token = q.get('token') || '';
  try { if (token) sessionStorage.setItem('kc_token', token); } catch (e) {}
  const $ = s => document.querySelector(s);
  const H = { 'X-KeyScope-Token': token };
  const odoW = new KCOdo($('#n-w')), odoC = new KCOdo($('#n-c'));
  let units = { w: 'wpm', c: 'cpm' }, live = { wpm: 0, cpm: 0 }, preset = 'detailed';
  const hist = [];   // the last 60 seconds of typing speed, for the sparkline style
  const call = (fn, ...a) => { if (typeof window[fn] === 'function') window[fn](...a); };

  function paint() {
    odoW.set(KCU.format(live.wpm, units.w, 0), true);
    odoC.set(KCU.format(live.cpm, units.c, 0), true);
    $('#u-w').textContent = KCU.LABEL[units.w];
    $('#u-c').textContent = KCU.LABEL[units.c];
  }
  async function loadSettings() {
    try {
      const r = await (await fetch('/api/settings', { headers: H, cache: 'no-store' })).json();
      units = { w: r.settings.typing_unit || 'wpm', c: r.settings.click_unit || 'cpm' };
      preset = ['compact', 'detailed', 'sparkline'].includes(r.settings.overlay_preset) ? r.settings.overlay_preset : 'detailed';
      $('#ov').dataset.preset = preset;
      applyLook(r.settings);
      document.documentElement.classList.toggle('lite', !!r.settings.lite_mode); // v2.2.0 Lite mode: no effects
      paint();
    } catch (e) {}
  }
  /* The box has its own colour theme (Settings / Appearance), separate from the app theme. */
  function applyLook(s) {
    const t = (window.KC_OV_THEMES || []).find(x => x[0] === s.overlay_theme) || (window.KC_OV_THEMES || [])[0];
    const root = document.documentElement;
    if (t) {
      root.style.setProperty('--ov-top', t[3]);
      root.style.setProperty('--ov-bottom', t[4]);
      root.style.setProperty('--ov-ink', t[5]);
      root.style.setProperty('--ov-muted', t[6]);
      root.style.setProperty('--ov-accent', t[7]);
      root.style.setProperty('--ov-line', t[8]);
    }
    const f = (window.KC_FONTS || []).find(x => x[0] === s.font);
    if (f) root.style.setProperty('--font', f[2]);
  }
  window.kcOvReload = loadSettings; // called by the window host after a settings change
  function connect() {
    const es = new EventSource('/api/live?token=' + encodeURIComponent(token));
    es.onmessage = e => { try { const f = JSON.parse(e.data); lastFrameAt = Date.now(); live = { wpm: f.wpm || 0, cpm: f.cpm || 0 }; paint(); } catch (err) {} };
    es.onopen = loadSettings;
  }
  /* The server sends a frame at least every 2 s. With none for 5 s the old speed is wrong: show 0. */
  let lastFrameAt = Date.now();
  setInterval(() => { if (Date.now() - lastFrameAt > 5000 && (live.wpm || live.cpm)) { live = { wpm: 0, cpm: 0 }; paint(); } }, 1000);
  const ov = $('#ov');
  ov.addEventListener('mouseenter', () => call('kcOvHover', true));
  ov.addEventListener('mouseleave', () => call('kcOvHover', false));
  ov.addEventListener('mousedown', e => {
    if (e.button !== 0 || e.target.closest('.x,.grip')) return;
    call('kcOvDrag');
  });
  $('#grip').addEventListener('mousedown', e => { e.preventDefault(); e.stopPropagation(); call('kcOvResize'); });
  $('#x').addEventListener('click', e => { e.stopPropagation(); call('kcOvClose'); });
  /* Right-click switches to the next style (compact, detailed, sparkline). */
  window.addEventListener('contextmenu', async e => {
    e.preventDefault();
    const order = ['compact', 'detailed', 'sparkline'], next = order[(order.indexOf(preset) + 1) % order.length];
    try { await fetch('/api/settings', { method: 'POST', headers: Object.assign({ 'Content-Type': 'application/json' }, H), body: JSON.stringify({ overlay_preset: next }) }); loadSettings(); } catch (err) {}
  });
  setInterval(() => {   // one sample a second; the line is drawn only in the sparkline style
    hist.push(live.wpm); if (hist.length > 60) hist.shift();
    if (preset !== 'sparkline') return;
    const max = Math.max(10, ...hist), n = hist.length;
    $('#spark-l').setAttribute('points', hist.map((v, i) => ((120 * i / Math.max(1, 59)).toFixed(1) + ',' + (28 - 26 * v / max).toFixed(1))).join(' '));
  }, 1000);
  setInterval(loadSettings, 5000);   // picks up unit changes made in the main window
  paint(); loadSettings(); connect();
})();
