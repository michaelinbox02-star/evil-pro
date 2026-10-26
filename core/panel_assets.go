package core

const panelFavicon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#6366f1"/><path d="M18 6 9 18h5l-1 8 9-12h-5z" fill="#fff"/></svg>`

const panelLoginCSS = `*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0a0d16;--panel:#111524;--border:#1e2438;--text:#e6ebf5;--muted:#7c89a6;--accent:#6366f1;--accent-h:#4f46e5;--err:#f87171}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;display:flex;align-items:center;justify-content:center;background-image:radial-gradient(900px 500px at 50% -10%,rgba(99,102,241,.16),transparent)}
.box{width:340px;background:var(--panel);border:1px solid var(--border);border-radius:16px;padding:28px 26px;box-shadow:0 30px 70px rgba(0,0,0,.5)}
.brand{display:flex;align-items:center;gap:10px;margin-bottom:4px}
.brand svg{width:30px;height:30px;border-radius:8px}
.brand h1{font-size:15px;font-weight:700;letter-spacing:-.2px}
.sub{color:var(--muted);font-size:12px;margin-bottom:22px}
label{display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin:14px 0 6px}
input{width:100%;padding:10px 12px;border-radius:9px;border:1px solid var(--border);background:#0c1019;color:var(--text);font-size:13px;outline:none;transition:border-color .15s}
input:focus{border-color:var(--accent)}
button{width:100%;margin-top:20px;padding:11px;border:none;border-radius:9px;background:var(--accent);color:#fff;font-size:13px;font-weight:600;cursor:pointer;transition:background .15s}
button:hover{background:var(--accent-h)}
button:disabled{opacity:.6;cursor:default}
.err{margin-top:14px;font-size:12px;color:var(--err);min-height:16px}`

const panelLoginHTML = `<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sign in · evil-pro dashboard</title>
<link rel="icon" href="/favicon.ico">
<link rel="stylesheet" href="/static/login.css">
</head>
<body>
<form class="box" id="form">
  <div class="brand"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#6366f1"/><path d="M18 6 9 18h5l-1 8 9-12h-5z" fill="#fff"/></svg><h1>evil-pro dashboard</h1></div>
  <div class="sub">Session capture · cookie takeover</div>
  <label for="u">Username</label>
  <input id="u" name="u" autocomplete="username" autofocus>
  <label for="p">Password</label>
  <input id="p" name="p" type="password" autocomplete="current-password">
  <button id="btn" type="submit">Sign in</button>
  <div class="err" id="err"></div>
</form>
<script>
document.getElementById('form').addEventListener('submit', async function(e){
  e.preventDefault();
  var btn=document.getElementById('btn'),err=document.getElementById('err');
  btn.disabled=true; btn.textContent='Signing in...'; err.textContent='';
  try{
    var r=await fetch('/api/v1/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:document.getElementById('u').value,password:document.getElementById('p').value})});
    if(r.ok){location.href='/';return;}
    var d=await r.json().catch(function(){return{}});
    err.textContent=d.error||'Invalid credentials';
  }catch(ex){err.textContent='Network error';}
  btn.disabled=false; btn.textContent='Sign in';
});
</script>
</body></html>`

const panelCSS = `*{box-sizing:border-box;margin:0;padding:0}
:root{
--bg:#0e1116;--surface:#12161c;--surface2:#161a21;--surface3:#1a1f27;--surface4:#1f2530;
--border:#232a33;--border2:#2e3742;
--text:#e6eaf0;--muted:#9aa5b1;--dim:#6b7683;
--accent:#3b82f6;--accent-h:#2563eb;--accent-bg:rgba(59,130,246,.12);--accent-bd:rgba(59,130,246,.32);
--ok:#22c55e;--ok-bg:rgba(34,197,94,.12);--warn:#d97706;--warn-bg:rgba(217,119,6,.12);--err:#dc2626;--err-bg:rgba(220,38,38,.12);
--mono:ui-monospace,SFMono-Regular,"SF Mono",Consolas,monospace;--r:6px;--r2:8px;--sb:208px;
--font:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,system-ui,sans-serif;
}
[data-theme="light"]{--bg:#f6f7f9;--surface:#ffffff;--surface2:#ffffff;--surface3:#f3f5f8;--surface4:#eceff3;--border:#e3e7ec;--border2:#d3d9e0;--text:#1a1f26;--muted:#5b6672;--dim:#8b95a1}
html,body{height:100%}
body{font-family:var(--font);background:var(--bg);color:var(--text);font-size:12.5px;line-height:1.5;-webkit-font-smoothing:antialiased}
a{color:var(--accent);text-decoration:none}
svg.ic{width:15px;height:15px;flex-shrink:0;stroke:currentColor;fill:none;stroke-width:1.6;stroke-linecap:round;stroke-linejoin:round}
#app{display:flex;min-height:100vh}

/* sidebar */
.side{width:var(--sb);flex-shrink:0;background:var(--surface);border-right:1px solid var(--border);display:flex;flex-direction:column;position:sticky;top:0;height:100vh}
.side-head{padding:13px 14px;display:flex;align-items:center;gap:9px;border-bottom:1px solid var(--border)}
.side-head svg.logo{width:22px;height:22px;border-radius:5px;flex-shrink:0}
.side-brand{font-size:12.5px;font-weight:600;letter-spacing:-.1px;line-height:1.2}
.side-brand small{display:block;font-size:10.5px;font-weight:400;color:var(--dim);letter-spacing:.02em}
.nav{flex:1;overflow-y:auto;padding:8px 8px 12px}
.nav-group{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.07em;color:var(--dim);padding:12px 8px 5px}
.nav-item{display:flex;align-items:center;gap:9px;padding:6px 8px;border-radius:var(--r);color:var(--muted);cursor:pointer;font-size:12.5px;font-weight:450;user-select:none;transition:background .1s,color .1s}
.nav-item:hover{background:var(--surface2);color:var(--text)}
.nav-item.active{background:var(--surface3);color:var(--text);font-weight:550}
.nav-item.active .ic{color:var(--accent)}
.nav-item .count{margin-left:auto;font-family:var(--mono);font-size:10.5px;color:var(--dim)}
.side-foot{padding:9px 12px;border-top:1px solid var(--border);font-size:11px;color:var(--dim);display:flex;align-items:center;justify-content:space-between}
.dot{width:6px;height:6px;border-radius:50%;background:var(--ok);display:inline-block}

/* shell */
.main{flex:1;min-width:0;display:flex;flex-direction:column}
.top{display:flex;align-items:center;gap:12px;padding:11px 18px;border-bottom:1px solid var(--border);background:var(--surface);position:sticky;top:0;z-index:40}
.top h2{font-size:14px;font-weight:600;letter-spacing:-.1px}
.top .desc{font-size:11.5px;color:var(--dim);margin-top:1px}
.top-actions{margin-left:auto;display:flex;gap:6px;align-items:center;flex-wrap:wrap}
.content{padding:18px;max-width:1560px;width:100%}

/* buttons */
.btn{display:inline-flex;align-items:center;gap:6px;padding:5px 10px;border:1px solid var(--border);border-radius:var(--r);background:var(--surface2);color:var(--text);cursor:pointer;font-size:12px;font-family:inherit;font-weight:500;transition:background .1s,border-color .1s;white-space:nowrap}
.btn:hover{background:var(--surface3);border-color:var(--border2)}
.btn.primary{background:var(--accent-h);border-color:var(--accent-h);color:#fff}
.btn.primary:hover{background:#1d4ed8;border-color:#1d4ed8}
.btn.danger{color:#f87171;border-color:rgba(220,38,38,.3);background:var(--err-bg)}
.btn.danger:hover{background:rgba(220,38,38,.2)}
.btn.ok{color:#4ade80;border-color:rgba(34,197,94,.3);background:var(--ok-bg)}
.btn.sm{padding:4px 8px;font-size:11.5px}
.btn.xs{padding:2px 7px;font-size:11px}
.btn:disabled{opacity:.45;cursor:default}

/* surfaces */
.grid{display:grid;gap:12px}
.stats{grid-template-columns:repeat(auto-fit,minmax(160px,1fr))}
.card{background:var(--surface2);border:1px solid var(--border);border-radius:var(--r2);padding:13px 15px}
.card.pad0{padding:0;overflow:hidden}
.card-head{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:10px 14px;border-bottom:1px solid var(--border);background:var(--surface2)}
.card-head h3{font-size:12.5px;font-weight:600}
.card-body{padding:14px}
.stat-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.07em;color:var(--dim)}
.stat-value{font-size:20px;font-weight:600;font-family:var(--mono);margin-top:4px;line-height:1.15}
.stat-value.ok{color:var(--ok)}.stat-value.warn{color:var(--warn)}.stat-value.accent{color:var(--accent)}
.stat-sub{font-size:10.5px;color:var(--dim);margin-top:2px}
.two{grid-template-columns:1.5fr 1fr}
@media(max-width:1050px){.two{grid-template-columns:1fr}}

/* tables */
table{width:100%;border-collapse:collapse}
th{text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--dim);padding:8px 12px;border-bottom:1px solid var(--border);white-space:nowrap;background:var(--surface3)}
td{padding:7px 12px;border-bottom:1px solid var(--border);vertical-align:middle}
tbody tr:last-child td{border-bottom:none}
tbody tr:hover{background:var(--surface3)}
tbody tr.clickable{cursor:pointer}
.mono{font-family:var(--mono);font-size:11.5px}
.trunc{max-width:230px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:inline-block;vertical-align:middle}
.copy{cursor:pointer}.copy:hover{color:var(--accent)}
.chk{width:13px;height:13px;accent-color:var(--accent);cursor:pointer}

/* badges + chips */
.badge{display:inline-block;font-size:10px;font-weight:600;padding:1px 6px;border-radius:4px;text-transform:uppercase;letter-spacing:.03em}
.badge.ok{background:var(--ok-bg);color:#4ade80}
.badge.warn{background:var(--warn-bg);color:#fbbf24}
.badge.err{background:var(--err-bg);color:#f87171}
.badge.mut{background:var(--surface4);color:var(--muted)}
.badge.acc{background:var(--accent-bg);color:#93b8f8}

/* forms */
.field{display:flex;flex-direction:column;gap:4px;margin-bottom:12px}
.field label{font-size:10.5px;font-weight:600;text-transform:uppercase;letter-spacing:.05em;color:var(--dim)}
.field .hint{font-size:11px;color:var(--dim)}
input,select,textarea{padding:6px 9px;border:1px solid var(--border);border-radius:var(--r);background:var(--surface);color:var(--text);font-size:12.5px;font-family:inherit;outline:none;transition:border-color .1s,box-shadow .1s;width:100%}
input:focus,select:focus,textarea:focus{border-color:var(--accent);box-shadow:0 0 0 2px var(--accent-bg)}
textarea{font-family:var(--mono);font-size:11.5px;resize:vertical}
.row{display:flex;gap:10px;flex-wrap:wrap;align-items:flex-end}
.row>.field{flex:1;min-width:150px}
.switch{display:inline-flex;align-items:center;gap:8px;cursor:pointer;user-select:none;font-size:12.5px}
.switch input{display:none}
.switch .track{width:32px;height:18px;border-radius:10px;background:var(--surface4);border:1px solid var(--border2);position:relative;transition:background .12s;flex-shrink:0}
.switch .track::after{content:"";position:absolute;top:2px;left:2px;width:12px;height:12px;border-radius:50%;background:var(--dim);transition:transform .12s,background .12s}
.switch input:checked + .track{background:var(--accent-bg);border-color:var(--accent-bd)}
.switch input:checked + .track::after{transform:translateX(14px);background:var(--accent)}

/* toolbar */
.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:12px}
.toolbar .grow{flex:1;min-width:180px}
.bulk{display:flex;align-items:center;gap:8px}
.search{position:relative}
.search input{padding-left:28px}
.search::before{content:"";position:absolute;left:9px;top:50%;width:12px;height:12px;margin-top:-7px;border:1.5px solid var(--dim);border-radius:50%}
.search::after{content:"";position:absolute;left:19px;top:50%;width:4px;height:1.5px;margin-top:3px;background:var(--dim);transform:rotate(45deg)}

/* misc */
.empty{text-align:center;padding:42px 20px;color:var(--dim)}
.empty .big{font-size:22px;opacity:.5;margin-bottom:8px}
.pre,.log{background:#0b0e12;border:1px solid var(--border);border-radius:var(--r);padding:10px;font-family:var(--mono);font-size:11px;white-space:pre-wrap;word-break:break-all;max-height:320px;overflow:auto}
.kv{display:grid;grid-template-columns:140px 1fr;gap:6px 14px;font-size:12.5px}
.kv .k{color:var(--dim)}
.kv .v{font-family:var(--mono);word-break:break-all}
.bar{height:6px;border-radius:3px;background:var(--surface4);overflow:hidden}
.bar > span{display:block;height:100%;background:var(--accent)}
.spark{display:flex;align-items:flex-end;gap:3px;height:48px}
.spark .b{flex:1;background:var(--accent);opacity:.7;border-radius:2px 2px 0 0;min-height:2px}
.spark .b:hover{opacity:1}

/* drawer + modal */
.scrim{position:fixed;inset:0;background:rgba(6,8,11,.66);z-index:900;opacity:0;pointer-events:none;transition:opacity .12s}
.scrim.open{opacity:1;pointer-events:auto}
.drawer{position:fixed;top:0;right:0;bottom:0;width:min(520px,94vw);background:var(--surface);border-left:1px solid var(--border);z-index:950;transform:translateX(101%);transition:transform .16s;display:flex;flex-direction:column}
.drawer.open{transform:none}
.drawer-head{display:flex;align-items:center;gap:10px;padding:12px 14px;border-bottom:1px solid var(--border)}
.drawer-head h3{font-size:13px;font-weight:600}
.drawer-body{padding:14px;overflow-y:auto;flex:1}
.modal{position:fixed;inset:0;z-index:960;display:none;align-items:center;justify-content:center;padding:18px}
.modal.open{display:flex}
.modal .box,.modal .modal-box{background:var(--surface);border:1px solid var(--border2);border-radius:10px;width:min(560px,96vw);max-height:90vh;overflow:auto;box-shadow:0 18px 48px rgba(0,0,0,.5)}
.modal .box.wide{width:min(760px,96vw)}
#modalBox{border-radius:10px}

/* toast */
.toast{position:fixed;right:16px;bottom:16px;z-index:2000;display:flex;flex-direction:column;gap:8px}
.toast .t{padding:9px 13px;border-radius:var(--r);font-size:12.5px;font-weight:500;box-shadow:0 6px 20px rgba(0,0,0,.45)}
.toast .t.ok{background:#14532d;color:#bbf7d0;border:1px solid rgba(34,197,94,.3)}
.toast .t.err{background:#7f1d1d;color:#fecaca;border:1px solid rgba(220,38,38,.35)}
.toast .t.info{background:var(--surface4);color:var(--text);border:1px solid var(--border2)}

.mobile-only{display:none}
@media(max-width:860px){
 .side{position:fixed;z-index:1000;transform:translateX(-101%);transition:transform .16s}
 .side.open{transform:none}
 .mobile-only{display:inline-flex}
 .content{padding:14px}
}
.hide{display:none!important}
.muted{color:var(--muted)}.dim{color:var(--dim)}
.right{text-align:right}.center{text-align:center}
.mt8{margin-top:8px}.mt14{margin-top:14px}.mb8{margin-bottom:8px}.mb14{margin-bottom:14px}
.actions{display:flex;gap:6px;flex-wrap:wrap}`

const panelIndexHTML = `<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>evil-pro · dashboard</title>
<link rel="icon" href="/favicon.ico">
<link rel="stylesheet" href="/static/app.css">
</head>
<body data-app="ctrl-panel">
<div id="app">
  <aside class="side" id="side">
    <div class="side-head">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#6366f1"/><path d="M18 6 9 18h5l-1 8 9-12h-5z" fill="#fff"/></svg>
      <div class="side-brand">evil-pro<small>dashboard</small></div>
    </div>
    <nav class="nav" id="nav"></nav>
    <div class="side-foot">
      <span><span class="dot" id="liveDot"></span> <span id="liveText">live</span></span>
      <span id="build"></span>
    </div>
  </aside>

  <div class="main">
    <header class="top">
      <button class="btn icon mobile-only" id="menuBtn">☰</button>
      <div><h2 id="pageTitle">Overview</h2><div class="desc" id="pageDesc"></div></div>
      <div class="top-actions" id="topActions"></div>
    </header>
    <main class="content" id="content"></main>
  </div>
</div>

<div class="scrim" id="scrim"></div>
<aside class="drawer" id="drawer">
  <div class="drawer-head" id="drawerHead"></div>
  <div class="drawer-body" id="drawerBody"></div>
</aside>
<div class="modal" id="modal"><div class="box" id="modalBox"></div></div>
<div class="toast" id="toast"></div>

<script src="/static/app.js"></script>
</body></html>`
