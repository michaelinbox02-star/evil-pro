// Session Injector — popup script
// Reads dashboard connection state and last inject result from chrome.storage.local
// and updates the popup UI. Subscribes to live changes so the popup reflects
// any session selection made on the dashboard while it is open.

function updateUI(data) {
  var connected = data.dashboardConnected === true;
  document.getElementById('statusDot').className = 'dot' + (connected ? ' on' : '');
  document.getElementById('statusText').textContent = connected ? 'Dashboard connected' : 'Not connected';

  var hasSession = data.currentSession && data.currentSession.sessionId;
  var hasInject  = data.lastInject && typeof data.lastInject.count !== 'undefined';

  if (hasSession) {
    var s = data.currentSession;
    document.getElementById('popSid').textContent = '#' + s.sessionId;
    document.getElementById('popPl').textContent  = s.phishlet || '—';
    var tokEl = document.getElementById('popTok');
    if (s.hasTokens) {
      tokEl.className = 'badge badge-g';
      tokEl.textContent = 'captured';
    } else {
      tokEl.className = 'badge badge-d';
      tokEl.textContent = 'none';
    }
    document.getElementById('sessionSection').style.display = '';
  } else {
    document.getElementById('sessionSection').style.display = 'none';
  }

  if (hasInject) {
    var inj = data.lastInject;
    var countText = inj.count + ' injected';
    if (inj.failed) countText += ', ' + inj.failed + ' failed';
    document.getElementById('popCount').textContent = countText;
    var targetEl = document.getElementById('popTarget');
    targetEl.textContent = inj.targetUrl || '—';
    targetEl.title = inj.targetUrl || '';
    document.getElementById('popTime').textContent =
      inj.time ? new Date(inj.time).toLocaleTimeString() : '—';
    document.getElementById('injectSection').style.display = '';
  } else {
    document.getElementById('injectSection').style.display = 'none';
  }

  document.getElementById('emptyState').style.display =
    (hasSession || hasInject) ? 'none' : '';
}

function openDash() {
  chrome.storage.local.get(['dashboardUrl'], function (data) {
    var url = data.dashboardUrl || 'http://localhost:5000';
    chrome.tabs.create({ url: url });
  });
}

// Initial load.
chrome.storage.local.get(
  ['dashboardConnected', 'currentSession', 'lastInject', 'dashboardUrl'],
  function (data) { updateUI(data || {}); }
);

// Live updates while popup is open.
chrome.storage.onChanged.addListener(function (changes, area) {
  if (area !== 'local') return;
  chrome.storage.local.get(
    ['dashboardConnected', 'currentSession', 'lastInject', 'dashboardUrl'],
    function (data) { updateUI(data || {}); }
  );
});
