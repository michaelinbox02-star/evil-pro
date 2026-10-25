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
--bg:#0a0d16;--panel:#111524;--panel2:#151a2b;--panel3:#1a2033;--border:#1e2438;--border2:#2a3350;
--text:#e6ebf5;--muted:#8b98b6;--dim:#5b6785;--accent:#6366f1;--accent-h:#4f46e5;--accent-bg:rgba(99,102,241,.12);--accent-bd:rgba(99,102,241,.35);
--ok:#22c55e;--ok-bg:rgba(34,197,94,.12);--warn:#f59e0b;--warn-bg:rgba(245,158,11,.12);--err:#ef4444;--err-bg:rgba(239,68,68,.12);
--mono:ui-monospace,SFMono-Regular,Consolas,monospace;--r:9px;--sb:216px;
}
[data-theme="light"]{--bg:#f4f6fb;--panel:#ffffff;--panel2:#f7f9fc;--panel3:#eef2f9;--border:#e2e7f0;--border2:#cfd7e6;--text:#141a2b;--muted:#5b6785;--dim:#8b98b6;--accent-bg:rgba(99,102,241,.1);--accent-bd:rgba(99,102,241,.3)}
html,body{height:100%}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;background:var(--bg);color:var(--text);font-size:13px;line-height:1.5;-webkit-font-smoothing:antialiased}
a{color:var(--accent);text-decoration:none}
#app{display:flex;min-height:100vh}

/* sidebar */
.side{width:var(--sb);flex-shrink:0;background:var(--panel);border-right:1px solid var(--border);display:flex;flex-direction:column;position:sticky;top:0;height:100vh}
.side-head{padding:16px 16px 14px;display:flex;align-items:center;gap:9px;border-bottom:1px solid var(--border)}
.side-head svg{width:24px;height:24px;border-radius:7px;flex-shrink:0}
.side-brand{font-size:13px;font-weight:700;letter-spacing:-.2px;line-height:1.2}
.side-brand small{display:block;font-size:10px;font-weight:500;color:var(--dim)}
.nav{flex:1;overflow-y:auto;padding:10px 8px}
.nav-group{font-size:9.5px;font-weight:700;text-transform:uppercase;letter-spacing:.1em;color:var(--dim);padding:12px 10px 5px}
.nav-item{display:flex;align-items:center;gap:9px;padding:7px 10px;border-radius:7px;color:var(--muted);cursor:pointer;font-size:12.5px;font-weight:500;user-select:none;transition:background .12s,color .12s}
.nav-item:hover{background:var(--panel2);color:var(--text)}
.nav-item.active{background:var(--accent-bg);color:var(--accent);font-weight:600}
.nav-item .ico{width:15px;text-align:center;font-size:13px;opacity:.9}
.nav-item .count{margin-left:auto;font-family:var(--mono);font-size:10px;background:var(--panel3);border:1px solid var(--border);color:var(--dim);padding:0 6px;border-radius:9px;min-width:20px;text-align:center}
.side-foot{padding:10px 12px;border-top:1px solid var(--border);font-size:11px;color:var(--dim);display:flex;align-items:center;justify-content:space-between}
.dot{width:7px;height:7px;border-radius:50%;background:var(--ok);display:inline-block}
.dot.off{background:var(--dim)}

/* main */
.main{flex:1;min-width:0;display:flex;flex-direction:column}
.top{display:flex;align-items:center;gap:12px;padding:12px 20px;border-bottom:1px solid var(--border);background:var(--panel);position:sticky;top:0;z-index:50}
.top h2{font-size:15px;font-weight:700;letter-spacing:-.2px}
.top .desc{font-size:11.5px;color:var(--muted);margin-top:1px}
.top-actions{margin-left:auto;display:flex;gap:7px;align-items:center;flex-wrap:wrap}
.content{padding:20px;max-width:1500px;width:100%}

/* buttons */
.btn{display:inline-flex;align-items:center;gap:6px;padding:6px 11px;border:1px solid var(--border);border-radius:var(--r);background:var(--panel2);color:var(--text);cursor:pointer;font-size:12px;font-family:inherit;font-weight:500;transition:all .12s;white-space:nowrap}
.btn:hover{border-color:var(--border2);background:var(--panel3)}
.btn.primary{background:var(--accent);border-color:var(--accent);color:#fff}
.btn.primary:hover{background:var(--accent-h)}
.btn.danger{color:var(--err);border-color:var(--err-bg);background:var(--err-bg)}
.btn.danger:hover{background:rgba(239,68,68,.2)}
.btn.ok{color:var(--ok);border-color:var(--ok-bg);background:var(--ok-bg)}
.btn.sm{padding:4px 9px;font-size:11.5px}
.btn.xs{padding:2px 7px;font-size:11px}
.btn.icon{padding:6px 8px}
.btn:disabled{opacity:.5;cursor:default}

/* cards + stats */
.grid{display:grid;gap:12px}
.stats{grid-template-columns:repeat(auto-fit,minmax(160px,1fr))}
.card{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:14px 16px}
.card.pad0{padding:0;overflow:hidden}
.card-head{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:13px 16px;border-bottom:1px solid var(--border)}
.card-head h3{font-size:12.5px;font-weight:700}
.card-body{padding:16px}
.stat-label{font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.09em;color:var(--dim)}
.stat-value{font-size:23px;font-weight:700;font-family:var(--mono);margin-top:5px;line-height:1.1}
.stat-value.ok{color:var(--ok)}.stat-value.warn{color:var(--warn)}.stat-value.accent{color:var(--accent)}
.stat-sub{font-size:10.5px;color:var(--dim);margin-top:3px}
.two{grid-template-columns:1.4fr 1fr}
@media(max-width:1050px){.two{grid-template-columns:1fr}}

/* table */
table{width:100%;border-collapse:collapse}
th{text-align:left;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.07em;color:var(--dim);padding:9px 12px;border-bottom:1px solid var(--border);white-space:nowrap;background:var(--panel2)}
th.sortable{cursor:pointer}th.sortable:hover{color:var(--muted)}
td{padding:8px 12px;border-bottom:1px solid var(--border);font-size:12px;vertical-align:middle}
tbody tr:last-child td{border-bottom:none}
tbody tr:hover{background:var(--panel2)}
tbody tr.clickable{cursor:pointer}
.mono{font-family:var(--mono);font-size:11.5px}
.trunc{max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:inline-block;vertical-align:middle}
.copy{cursor:pointer}.copy:hover{color:var(--accent)}

/* badges + chips */
.badge{display:inline-block;font-size:10px;font-weight:700;padding:2px 7px;border-radius:6px;text-transform:uppercase;letter-spacing:.04em}
.badge.ok{background:var(--ok-bg);color:var(--ok)}
.badge.warn{background:var(--warn-bg);color:var(--warn)}
.badge.err{background:var(--err-bg);color:var(--err)}
.badge.mut{background:var(--panel3);color:var(--muted);border:1px solid var(--border)}
.badge.acc{background:var(--accent-bg);color:var(--accent)}
.chip{display:inline-flex;align-items:center;gap:6px;padding:4px 9px;border:1px solid var(--border);border-radius:20px;background:var(--panel2);font-size:11.5px;cursor:pointer;color:var(--muted)}
.chip:hover{border-color:var(--border2);color:var(--text)}
.chip.active{background:var(--accent-bg);border-color:var(--accent-bd);color:var(--accent)}

/* forms */
.field{display:flex;flex-direction:column;gap:5px;margin-bottom:13px}
.field label{font-size:10.5px;font-weight:700;text-transform:uppercase;letter-spacing:.07em;color:var(--dim)}
.field .hint{font-size:11px;color:var(--dim)}
input,select,textarea{padding:8px 10px;border:1px solid var(--border);border-radius:8px;background:var(--panel2);color:var(--text);font-size:12.5px;font-family:inherit;outline:none;transition:border-color .12s;width:100%}
input:focus,select:focus,textarea:focus{border-color:var(--accent)}
textarea{font-family:var(--mono);font-size:11.5px;resize:vertical}
.row{display:flex;gap:10px;flex-wrap:wrap;align-items:flex-end}
.row > .field{flex:1;min-width:150px}
.switch{display:inline-flex;align-items:center;gap:8px;cursor:pointer;user-select:none;font-size:12.5px}
.switch input{display:none}
.switch .track{width:34px;height:19px;border-radius:20px;background:var(--panel3);border:1px solid var(--border);position:relative;transition:background .15s;flex-shrink:0}
.switch .track::after{content:"";position:absolute;top:2px;left:2px;width:13px;height:13px;border-radius:50%;background:var(--muted);transition:transform .15s,background .15s}
.switch input:checked + .track{background:var(--accent-bg);border-color:var(--accent-bd)}
.switch input:checked + .track::after{transform:translateX(15px);background:var(--accent)}

/* toolbar */
.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:14px}
.toolbar .grow{flex:1;min-width:180px}
.search{position:relative}
.search input{padding-left:30px}
.search::before{content:"⌕";position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--dim);font-size:14px}

/* misc */
.empty{text-align:center;padding:46px 20px;color:var(--dim)}
.empty .big{font-size:26px;opacity:.4;margin-bottom:8px}
.pre{background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:11px;font-family:var(--mono);font-size:11px;white-space:pre-wrap;word-break:break-all;max-height:320px;overflow:auto}
.tabs{display:flex;gap:4px;border-bottom:1px solid var(--border);margin-bottom:16px;flex-wrap:wrap}
.tab{padding:8px 13px;font-size:12.5px;font-weight:600;color:var(--muted);cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px}
.tab:hover{color:var(--text)}
.tab.active{color:var(--accent);border-bottom-color:var(--accent)}
.kv{display:grid;grid-template-columns:150px 1fr;gap:7px 14px;font-size:12.5px}
.kv .k{color:var(--dim)}
.kv .v{font-family:var(--mono);word-break:break-all}
.bar{height:7px;border-radius:5px;background:var(--panel3);overflow:hidden}
.bar > span{display:block;height:100%;background:var(--accent)}
.spark{display:flex;align-items:flex-end;gap:3px;height:52px}
.spark .b{flex:1;background:var(--accent);border-radius:3px 3px 0 0;min-height:2px;opacity:.75}
.spark .b:hover{opacity:1}

/* drawer + modal */
.scrim{position:fixed;inset:0;background:rgba(4,6,14,.6);z-index:900;opacity:0;pointer-events:none;transition:opacity .15s}
.scrim.open{opacity:1;pointer-events:auto}
.drawer{position:fixed;top:0;right:0;bottom:0;width:min(520px,94vw);background:var(--panel);border-left:1px solid var(--border);z-index:950;transform:translateX(100%);transition:transform .18s;display:flex;flex-direction:column}
.drawer.open{transform:none}
.drawer-head{display:flex;align-items:center;gap:10px;padding:14px 16px;border-bottom:1px solid var(--border)}
.drawer-head h3{font-size:13.5px;font-weight:700}
.drawer-body{padding:16px;overflow-y:auto;flex:1}
.modal{position:fixed;inset:0;z-index:960;display:none;align-items:center;justify-content:center;padding:20px}
.modal.open{display:flex}
.modal .box{background:var(--panel);border:1px solid var(--border);border-radius:14px;width:min(560px,96vw);max-height:90vh;overflow:auto;box-shadow:0 30px 80px rgba(0,0,0,.55)}
.modal .box.wide{width:min(720px,96vw)}

.toast{position:fixed;right:18px;bottom:18px;z-index:2000;display:flex;flex-direction:column;gap:8px}
.toast .t{padding:10px 14px;border-radius:9px;font-size:12.5px;font-weight:600;box-shadow:0 10px 30px rgba(0,0,0,.4);animation:in .15s}
.toast .t.ok{background:#0f7a3d;color:#fff}.toast .t.err{background:#b91c1c;color:#fff}.toast .t.info{background:var(--panel3);color:var(--text);border:1px solid var(--border)}
@keyframes in{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}

.mobile-only{display:none}
@media(max-width:860px){
 .side{position:fixed;z-index:1000;transform:translateX(-100%);transition:transform .18s}
 .side.open{transform:none}
 .mobile-only{display:inline-flex}
}
.hide{display:none!important}
.muted{color:var(--muted)}.dim{color:var(--dim)}
.right{text-align:right}.center{text-align:center}
.mt8{margin-top:8px}.mt14{margin-top:14px}.mb8{margin-bottom:8px}.mb14{margin-bottom:14px}
.actions{display:flex;gap:6px;flex-wrap:wrap}
.bulk{display:flex;align-items:center;gap:8px}
.chk{width:14px;height:14px;accent-color:var(--accent);cursor:pointer}
.clickable{cursor:pointer}`

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
