package main

const cpFavicon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#16181d"/><path d="M18 5.5 9.5 18h5.2l-1 8.5L22.5 14h-5.4z" fill="none" stroke="#3b82f6" stroke-width="2.2" stroke-linejoin="round" stroke-linecap="round"/></svg>`

const cpCSS = `*{box-sizing:border-box;margin:0;padding:0}
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
.side-head svg.mark{width:18px;height:18px;flex-shrink:0;color:var(--accent)}
.side-brand{font-size:12.5px;font-weight:600;line-height:1.2}
.side-brand small{display:block;font-size:10.5px;font-weight:400;color:var(--dim)}
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
.top h2{font-size:14px;font-weight:600}
.top .desc{font-size:11.5px;color:var(--dim);margin-top:1px}
.top-actions{margin-left:auto;display:flex;gap:6px;flex-wrap:wrap}
.content{padding:18px;max-width:1560px;width:100%}

/* buttons */
.btn{display:inline-flex;align-items:center;gap:6px;padding:5px 10px;border:1px solid var(--border);border-radius:var(--r);background:var(--surface2);color:var(--text);cursor:pointer;font-size:12px;font-weight:500;white-space:nowrap;transition:background .1s,border-color .1s}
.btn:hover{background:var(--surface3);border-color:var(--border2)}
.btn.primary{background:var(--accent-h);border-color:var(--accent-h);color:#fff}
.btn.primary:hover{background:#1d4ed8}
.btn.danger{color:#f87171;border-color:rgba(220,38,38,.3);background:var(--err-bg)}
.btn.danger:hover{background:rgba(220,38,38,.2)}
.btn.ok{color:#4ade80;border-color:rgba(34,197,94,.3);background:var(--ok-bg)}
.btn.sm{padding:4px 8px;font-size:11.5px}
.btn.xs{padding:2px 7px;font-size:11px}
.btn.icon{padding:5px 7px}
.btn:disabled{opacity:.45;cursor:default}

/* surfaces */
.grid{display:grid;gap:12px}
.cards{grid-template-columns:repeat(auto-fill,minmax(330px,1fr))}
.stats{grid-template-columns:repeat(auto-fit,minmax(160px,1fr))}
.card{background:var(--surface2);border:1px solid var(--border);border-radius:var(--r2);padding:13px 15px}
.card.pad0{padding:0;overflow:hidden}
.card-head{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:10px 14px;border-bottom:1px solid var(--border)}
.card-head h3{font-size:12.5px;font-weight:600}
.card-body{padding:14px}
.stat-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.07em;color:var(--dim)}
.stat-value{font-size:20px;font-weight:600;font-family:var(--mono);margin-top:4px}
.stat-sub{font-size:10.5px;color:var(--dim);margin-top:2px}

/* tables */
table{width:100%;border-collapse:collapse}
th{text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--dim);padding:8px 12px;border-bottom:1px solid var(--border);background:transparent;white-space:nowrap}
td{padding:7px 12px;border-bottom:1px solid var(--border)}
tbody tr:last-child td{border-bottom:none}
tbody tr:hover{background:var(--surface3)}
.clickable{cursor:pointer}
.mono{font-family:var(--mono);font-size:11.5px}
.trunc{max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:inline-block;vertical-align:middle}
.copy{cursor:pointer}.copy:hover{color:var(--accent)}

/* badges */
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
input,select,textarea{padding:6px 9px;border:1px solid var(--border);border-radius:var(--r);background:var(--surface);color:var(--text);font-size:12.5px;font-family:inherit;outline:none;width:100%}
input:focus,select:focus,textarea:focus{border-color:var(--accent);box-shadow:0 0 0 2px var(--accent-bg)}
textarea{font-family:var(--mono);font-size:11.5px;resize:vertical}
.row{display:flex;gap:10px;flex-wrap:wrap;align-items:flex-end}
.row>.field{flex:1;min-width:140px}
.switch{display:inline-flex;align-items:center;gap:8px;cursor:pointer;font-size:12.5px}
.switch input{display:none}
.switch .track{width:32px;height:18px;border-radius:10px;background:var(--surface4);border:1px solid var(--border2);position:relative;flex-shrink:0}
.switch .track::after{content:"";position:absolute;top:2px;left:2px;width:12px;height:12px;border-radius:50%;background:var(--dim);transition:.12s}
.switch input:checked+.track{background:var(--accent-bg);border-color:var(--accent-bd)}
.switch input:checked+.track::after{transform:translateX(14px);background:var(--accent)}

/* tabs */
.tabs{display:flex;gap:2px;border-bottom:1px solid var(--border);margin-bottom:14px;flex-wrap:wrap}
.tab{padding:7px 11px;font-size:12.5px;font-weight:500;color:var(--muted);cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px}
.tab:hover{color:var(--text)}
.tab.active{color:var(--text);border-bottom-color:var(--accent);font-weight:550}

/* misc */
.empty{text-align:center;padding:42px 20px;color:var(--dim)}
.empty .big{font-size:22px;opacity:.5;margin-bottom:8px}
.kv{display:grid;grid-template-columns:130px 1fr;gap:6px 12px;font-size:12.5px}
.kv .k{color:var(--dim)}
.kv .v{font-family:var(--mono);word-break:break-all}
.log,.pre{background:#0b0e12;border:1px solid var(--border);border-radius:var(--r);padding:10px;font-family:var(--mono);font-size:11px;white-space:pre-wrap;word-break:break-word;max-height:320px;overflow:auto;color:#c8d1dc}
.actions{display:flex;gap:6px;flex-wrap:wrap}
.spin{display:inline-block;width:11px;height:11px;border:2px solid var(--accent-bd);border-top-color:var(--accent);border-radius:50%;animation:s .8s linear infinite;vertical-align:-1px}
@keyframes s{to{transform:rotate(360deg)}}

/* template gallery */
.tplgrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(215px,1fr));gap:12px}
.tplcard{background:var(--surface2);border:1px solid var(--border);border-radius:var(--r2);overflow:hidden;cursor:pointer;transition:border-color .1s}
.tplcard:hover{border-color:var(--accent-bd)}
.tplthumb{height:140px;overflow:hidden;background:#fff;border-bottom:1px solid var(--border)}
.tplthumb iframe{width:200%;height:200%;transform:scale(.5);transform-origin:0 0;border:0}
.tplmeta{padding:10px 12px}
.tplmeta b{font-size:12.5px;font-weight:600}
.tplmeta .dim{font-size:10px;text-transform:uppercase;letter-spacing:.05em;margin-left:6px}

/* drawer + modal */
.scrim{position:fixed;inset:0;background:rgba(6,8,11,.66);z-index:900;opacity:0;pointer-events:none;transition:opacity .12s}
.scrim.open{opacity:1;pointer-events:auto}
.drawer{position:fixed;top:0;right:0;bottom:0;width:min(560px,95vw);background:var(--surface);border-left:1px solid var(--border);z-index:950;transform:translateX(101%);transition:transform .16s;display:flex;flex-direction:column}
.drawer.open{transform:none}
.drawer-head{display:flex;align-items:center;gap:10px;padding:12px 14px;border-bottom:1px solid var(--border)}
.drawer-head h3{font-size:13px;font-weight:600}
.drawer-body{padding:14px;overflow-y:auto;flex:1}
.modal{position:fixed;inset:0;z-index:960;display:none;align-items:center;justify-content:center;padding:18px}
.modal.open{display:flex}
.modal .box{background:var(--surface);border:1px solid var(--border2);border-radius:10px;width:min(620px,96vw);max-height:90vh;overflow:auto;box-shadow:0 18px 48px rgba(0,0,0,.5)}
.modal .box.wide{width:min(860px,97vw)}

/* toast */
.toast{position:fixed;right:16px;bottom:16px;z-index:2000;display:flex;flex-direction:column;gap:8px}
.toast .t{padding:9px 13px;border-radius:var(--r);font-size:12.5px;font-weight:500}
.toast .t.ok{background:#14532d;color:#bbf7d0;border:1px solid rgba(34,197,94,.3)}
.toast .t.err{background:#7f1d1d;color:#fecaca;border:1px solid rgba(220,38,38,.35)}
.toast .t.info{background:var(--surface4);border:1px solid var(--border2)}

.hide{display:none!important}.muted{color:var(--muted)}.dim{color:var(--dim)}.right{text-align:right}.center{text-align:center}
.mt8{margin-top:8px}.mt14{margin-top:14px}.mb8{margin-bottom:8px}.mb14{margin-bottom:14px}
.mobile-only{display:none}
@media(max-width:900px){
 .side{position:fixed;z-index:1000;transform:translateX(-101%);transition:transform .16s}
 .side.open{transform:none}
 .mobile-only{display:inline-flex}
 .cards,.tplgrid{grid-template-columns:1fr}
 .content{padding:14px}
}`

const cpLoginHTML = `<!DOCTYPE html><html lang="en" data-theme="dark"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sign in · evil-pro control panel</title><link rel="icon" href="/favicon.ico">
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,system-ui,sans-serif;background:#0e1116;color:#e6eaf0;min-height:100vh;display:flex;align-items:center;justify-content:center}
.box{width:330px;background:#12161c;border:1px solid #232a33;border-radius:10px;padding:26px 24px;box-shadow:0 18px 48px rgba(0,0,0,.45)}
.brand{display:flex;align-items:center;gap:9px;margin-bottom:3px}.brand svg.mark{width:20px;height:20px;color:#3b82f6}
.brand h1{font-size:14px;font-weight:600}.sub{color:#6b7683;font-size:11.5px;margin-bottom:20px}
label{display:block;font-size:10.5px;font-weight:600;text-transform:uppercase;letter-spacing:.05em;color:#6b7683;margin:13px 0 5px}
input{width:100%;padding:8px 10px;border-radius:6px;border:1px solid #232a33;background:#0e1116;color:#e6eaf0;font-size:13px;outline:none}
input:focus{border-color:#3b82f6;box-shadow:0 0 0 2px rgba(59,130,246,.12)}
button{width:100%;margin-top:18px;padding:9px;border:none;border-radius:6px;background:#2563eb;color:#fff;font-size:13px;font-weight:500;cursor:pointer}
button:hover{background:#1d4ed8}
button:disabled{opacity:.6}
.err{margin-top:12px;font-size:12px;color:#f87171;min-height:15px}
</style></head><body>
<form class="box" id="form">
  <div class="brand"><svg class="mark" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M13.4 2.6 6 13.2h4.6L9.4 21.4 17 10.6h-4.6z" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round" stroke-linecap="round"/></svg><h1>evil-pro control panel</h1></div>
  <div class="sub">Deploy &amp; manage servers</div>
  <label for="u">Username</label><input id="u" autocomplete="username" autofocus>
  <label for="p">Password</label><input id="p" type="password" autocomplete="current-password">
  <button id="btn" type="submit">Sign in</button><div class="err" id="err"></div>
</form>
<script>
document.getElementById('form').addEventListener('submit',async function(e){e.preventDefault();
 var b=document.getElementById('btn'),er=document.getElementById('err');b.disabled=true;b.textContent='Signing in...';er.textContent='';
 try{var r=await fetch('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:document.getElementById('u').value,password:document.getElementById('p').value})});
  if(r.ok){location.href='/';return;} var d=await r.json().catch(function(){return{}});er.textContent=d.error||'Invalid credentials';}
 catch(x){er.textContent='Network error';} b.disabled=false;b.textContent='Sign in';});
</script></body></html>`

const cpIndexHTML = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>evil-pro · control panel</title><link rel="icon" href="/favicon.ico">
<link rel="stylesheet" href="/static/app.css"></head>
<body data-app="ctrl-panel"><div id="app">
  <aside class="side">
    <div class="side-head">
      <svg class="mark" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M13.4 2.6 6 13.2h4.6L9.4 21.4 17 10.6h-4.6z" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round" stroke-linecap="round"/></svg>
      <div class="side-brand">evil-pro<small>control panel</small></div>
    </div>
    <nav class="nav" id="nav"></nav>
    <div class="side-foot" id="foot"></div>
  </aside>
  <div class="main">
    <header class="top">
      <button class="btn icon mobile-only" id="menuBtn">≡</button>
      <div><h2 id="title">Servers</h2><div class="desc" id="desc"></div></div>
      <div class="top-actions" id="actions"></div>
    </header>
    <main class="content" id="content"></main>
  </div>
</div>
<div class="scrim" id="scrim"></div>
<aside class="drawer" id="drawer"><div class="drawer-head" id="drawerHead"></div><div class="drawer-body" id="drawerBody"></div></aside>
<div class="modal" id="modal"><div class="box" id="modalBox"></div></div>
<div class="toast" id="toast"></div>
<script src="/static/app.js"></script></body></html>`
