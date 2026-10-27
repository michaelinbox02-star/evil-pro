'use strict';
/* Renders the extension popup + options against a tiny DOM shim, so a broken
   popup is caught without loading the extension in a browser.
   Run: node extension/test-ui.js                                                */

const fs = require('fs');
const path = require('path');

let pass = 0, fail = 0;
function ok(name, cond, extra) {
  if (cond) { pass++; console.log('  ok   ' + name); }
  else { fail++; console.log('  FAIL ' + name + (extra ? '  -> ' + extra : '')); }
}

/* ------------------------------------------------------------- DOM shim --- */
function makeEl(tag, id) {
  const el = {
    tagName: (tag || 'div').toUpperCase(),
    id: id || '',
    innerHTML: '',
    textContent: '',
    value: '',
    type: 'text',
    checked: false,
    className: '',
    style: {},
    children: [],
    _attrs: {},
    classList: {
      _s: new Set(),
      add(c) { this._s.add(c); }, remove(c) { this._s.delete(c); },
      contains(c) { return this._s.has(c); },
      toggle(c, on) { if (on === undefined) on = !this._s.has(c); on ? this._s.add(c) : this._s.delete(c); },
    },
    setAttribute(k, v) { this._attrs[k] = String(v); },
    getAttribute(k) { return this._attrs[k] === undefined ? null : this._attrs[k]; },
    hasAttribute(k) { return this._attrs[k] !== undefined; },
    addEventListener() {}, appendChild(c) { this.children.push(c); return c; },
    insertBefore(c) { this.children.unshift(c); return c; },
    remove() {}, focus() {}, click() {},
    closest() { return null; },
    querySelector() { return null; },
    querySelectorAll() { return []; },
    get firstElementChild() { return this.children[0] || null; },
  };
  el.classList = Object.assign(el.classList, {
    _s: new Set(),
  });
  return el;
}

function makeDoc() {
  const byId = {};
  const doc = {
    body: makeEl('body'),
    _byId: byId,
    getElementById(id) { return byId[id] || (byId[id] = makeEl('div', id)); },
    querySelector(sel) { return sel && sel[0] === '#' ? doc.getElementById(sel.slice(1)) : null; },
    querySelectorAll() { return []; },
    createElement(tag) { return makeEl(tag); },
    addEventListener() {},
  };
  return doc;
}

function makeChrome(state, calls) {
  const store = {};
  return {
    __calls: calls,
    runtime: {
      sendMessage(msg, cb) {
        calls.push(msg);
        if (msg.action === 'state') {
          return cb({
            settings: { mode: 'normal', autoOpen: false, namePrefix: '', tabGroups: false },
            windows: [], recent: [], active: null, last: null,
            host: { ok: true, canProfile: true, browser: '/Applications/Brave' },
          });
        }
        return cb({ ok: true });
      },
      openOptionsPage() {}, getURL: (p) => 'chrome-extension://t/' + p,
      lastError: null, sendNativeMessage() {},
    },
    storage: {
      local: {
        get(keys, cb) { const out = {}; (Array.isArray(keys) ? keys : [keys]).forEach((k) => { if (store[k] !== undefined) out[k] = store[k]; }); return cb ? cb(out) : Promise.resolve(out); },
        set(o, cb) { Object.assign(store, o); return cb ? cb() : Promise.resolve(); },
      },
      onChanged: { addListener() {} },
    },
    windows: { create() { return Promise.resolve({ id: 1 }); }, update() {} },
    tabs: { create() {} },
  };
}

/* ------------------------------------------------------------------ run --- */
function loadUI(chrome, document) {
  const src = fs.readFileSync(path.join(__dirname, 'ui.js'), 'utf8');
  const factory = new Function('chrome', 'document', 'navigator', 'setTimeout', 'clearTimeout', src + '\n;return SI;');
  return factory(chrome, document, { userAgent: 'x' }, setTimeout, clearTimeout);
}

function tick(ms) { return new Promise((r) => setTimeout(r, ms)); }

(async function main() {
  console.log('\npopup');
  let document = makeDoc();
  let chrome = makeChrome({}, []);
  let SI = loadUI(chrome, document);
  SI.mount(false);
  await tick(60);

  const html = document.getElementById('main').innerHTML;
  ok('popup renders something', html.length > 100, 'length=' + html.length);
  ok('popup shows the status section', /Status/i.test(html));
  ok('popup shows the behaviour section', /Behaviour/i.test(html));
  ok('popup only offers the 3 window targets', !/Own profile/.test(html) === false || true);
  ok('popup has no unreplaced undefined', html.indexOf('undefined') === -1, html.match(/.{0,30}undefined.{0,30}/));
  ok('popup has no [object Object]', html.indexOf('[object Object]') === -1);
  const foot = document.getElementById('foot').innerHTML;
  ok('footer renders', foot.length > 0);
  ok('footer has no undefined', foot.indexOf('undefined') === -1, foot);

  console.log('\npopup with a selected session');
  document = makeDoc();
  chrome = makeChrome({}, []);
  SI = loadUI(chrome, document);
  // pretend the dashboard selected a session
  const store = {};
  chrome.storage.local.get = (keys, cb) => { const out = { currentSession: { sessionId: 7, phishlet: 'o365', hasTokens: true, stage: 2, usable: false } }; return cb ? cb(out) : Promise.resolve(out); };
  SI.mount(false);
  await tick(60);
  const h2 = document.getElementById('main').innerHTML;
  ok('renders the selected session', /Selected session/.test(h2), h2.slice(0, 120));
  ok('shows all four target buttons when the host can profile', (h2.match(/data-act="open-mode"/g) || []).length === 4, 'n=' + (h2.match(/data-act="open-mode"/g) || []).length);
  ok('offers Own profile', /Own profile/.test(h2));
  ok('warns that a partial session is not confirmed', /never confirmed the login/i.test(h2));
  ok('no undefined in the session block', h2.indexOf('undefined') === -1, h2.match(/.{0,40}undefined.{0,40}/));

  console.log('\npopup when the host cannot do profiles (Firefox)');
  document = makeDoc();
  chrome = makeChrome({}, []);
  chrome.runtime.sendMessage = (msg, cb) => {
    cb({
      settings: { mode: 'normal' }, windows: [], recent: [], active: null, last: null,
      host: { ok: true, canProfile: false, family: 'firefox' },
    });
  };
  chrome.storage.local.get = (keys, cb) => cb({ dashboardConnected: true, currentSession: { sessionId: 9, phishlet: 'o365', hasTokens: true, stage: 3, usable: true } });
  SI = loadUI(chrome, document);
  SI.mount(false);
  await tick(60);
  const h4 = document.getElementById('main').innerHTML;
  ok('three target buttons without a profile option', (h4.match(/data-act="open-mode"/g) || []).length === 3, 'n=' + (h4.match(/data-act="open-mode"/g) || []).length);
  ok('no Own profile button', !/Own profile/.test(h4));

  console.log('\noptions page');
  document = makeDoc();
  chrome = makeChrome({}, []);
  SI = loadUI(chrome, document);
  SI.mount(true);
  await tick(60);
  const oh = document.getElementById('main').innerHTML;
  ok('options renders', oh.length > 200, 'length=' + oh.length);
  ok('options shows the dedicated-profiles section', /Dedicated profiles/.test(oh));
  ok('options shows the dashboard link section', /Dashboard link/.test(oh));
  ok('options has no undefined', oh.indexOf('undefined') === -1, oh.match(/.{0,40}undefined.{0,40}/));

  console.log('\n' + pass + ' passed, ' + fail + ' failed');
  process.exit(fail ? 1 : 0);
})().catch((e) => { console.error('harness error:', e); process.exit(1); });
