package main

const cpFavicon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#0ea5e9"/><path d="M9 10h14v3H9zm0 6h10v3H9z" fill="#fff"/><circle cx="23" cy="21" r="3" fill="#fff"/></svg>`

const cpCSS = `*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#080b14;--panel:#101524;--panel2:#161c2e;--panel3:#1c2338;--border:#1e2536;--border2:#2b3550;--text:#e7ecf7;--muted:#8b98b6;--dim:#5b6785;--accent:#0ea5e9;--accent-h:#0284c7;--accent-bg:rgba(14,165,233,.13);--accent-bd:rgba(14,165,233,.4);--ok:#22c55e;--ok-bg:rgba(34,197,94,.13);--warn:#f59e0b;--warn-bg:rgba(245,158,11,.13);--err:#ef4444;--err-bg:rgba(239,68,68,.13);--mono:ui-monospace,SFMono-Regular,Consolas,monospace;--r:9px;--sb:212px}
[data-theme=light]{--bg:#f4f7fb;--panel:#fff;--panel2:#f6f9fc;--panel3:#eef3f9;--border:#e2e8f0;--border2:#cfd8e6;--text:#0f172a;--muted:#5b6785;--dim:#94a3b8}
html,body{height:100%}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;background:var(--bg);color:var(--text);font-size:13px;line-height:1.5;-webkit-font-smoothing:antialiased}
a{color:var(--accent);text-decoration:none}
#app{display:flex;min-height:100vh}
.side{width:var(--sb);flex-shrink:0;background:var(--panel);border-right:1px solid var(--border);display:flex;flex-direction:column;position:sticky;top:0;height:100vh}
.side-head{padding:16px;display:flex;align-items:center;gap:9px;border-bottom:1px solid var(--border)}
.side-head svg{width:24px;height:24px;border-radius:7px}
.side-brand{font-size:13px;font-weight:700}
.side-brand small{display:block;font-size:10px;font-weight:500;color:var(--dim)}
.nav{flex:1;overflow-y:auto;padding:10px 8px}
.nav-group{font-size:9.5px;font-weight:700;text-transform:uppercase;letter-spacing:.1em;color:var(--dim);padding:12px 10px 5px}
.nav-item{display:flex;align-items:center;gap:9px;padding:7px 10px;border-radius:7px;color:var(--muted);cursor:pointer;font-size:12.5px;font-weight:500}
.nav-item:hover{background:var(--panel2);color:var(--text)}
.nav-item.active{background:var(--accent-bg);color:var(--accent);font-weight:600}
.nav-item .count{margin-left:auto;font-family:var(--mono);font-size:10px;background:var(--panel3);border:1px solid var(--border);color:var(--dim);padding:0 6px;border-radius:9px}
.side-foot{padding:10px 12px;border-top:1px solid var(--border);font-size:11px;color:var(--dim)}
.main{flex:1;min-width:0;display:flex;flex-direction:column}
.top{display:flex;align-items:center;gap:12px;padding:12px 20px;border-bottom:1px solid var(--border);background:var(--panel);position:sticky;top:0;z-index:50}
.top h2{font-size:15px;font-weight:700}
.top .desc{font-size:11.5px;color:var(--muted)}
.top-actions{margin-left:auto;display:flex;gap:7px;flex-wrap:wrap}
.content{padding:20px;max-width:1500px;width:100%}
.btn{display:inline-flex;align-items:center;gap:6px;padding:6px 11px;border:1px solid var(--border);border-radius:var(--r);background:var(--panel2);color:var(--text);cursor:pointer;font-size:12px;font-weight:500;white-space:nowrap;transition:.12s}
.btn:hover{border-color:var(--border2);background:var(--panel3)}
.btn.primary{background:var(--accent);border-color:var(--accent);color:#fff}.btn.primary:hover{background:var(--accent-h)}
.btn.danger{color:var(--err);background:var(--err-bg);border-color:var(--err-bg)}
.btn.ok{color:var(--ok);background:var(--ok-bg);border-color:var(--ok-bg)}
.btn.sm{padding:4px 9px;font-size:11.5px}
.btn:disabled{opacity:.5;cursor:default}
.grid{display:grid;gap:12px}
.cards{grid-template-columns:repeat(auto-fill,minmax(340px,1fr))}
.card{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:14px 16px}
.card.pad0{padding:0;overflow:hidden}
.card-head{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:13px 16px;border-bottom:1px solid var(--border)}
.card-head h3{font-size:12.5px;font-weight:700}
.badge{display:inline-block;font-size:10px;font-weight:700;padding:2px 7px;border-radius:6px;text-transform:uppercase;letter-spacing:.04em}
.badge.ok{background:var(--ok-bg);color:var(--ok)}
.badge.warn{background:var(--warn-bg);color:var(--warn)}
.badge.err{background:var(--err-bg);color:var(--err)}
.badge.mut{background:var(--panel3);color:var(--muted);border:1px solid var(--border)}
.badge.acc{background:var(--accent-bg);color:var(--accent)}
table{width:100%;border-collapse:collapse}
th{text-align:left;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.07em;color:var(--dim);padding:9px 12px;border-bottom:1px solid var(--border);background:var(--panel2)}
td{padding:8px 12px;border-bottom:1px solid var(--border);font-size:12px}
tbody tr:last-child td{border-bottom:none}tbody tr:hover{background:var(--panel2)}
.mono{font-family:var(--mono);font-size:11.5px}
.copy{cursor:pointer}.copy:hover{color:var(--accent)}
.kv{display:grid;grid-template-columns:130px 1fr;gap:6px 12px;font-size:12.5px}
.kv .k{color:var(--dim)}.kv .v{font-family:var(--mono);word-break:break-all}
.field{display:flex;flex-direction:column;gap:5px;margin-bottom:12px}
.field label{font-size:10.5px;font-weight:700;text-transform:uppercase;letter-spacing:.07em;color:var(--dim)}
.field .hint{font-size:11px;color:var(--dim)}
input,select,textarea{padding:8px 10px;border:1px solid var(--border);border-radius:8px;background:var(--panel2);color:var(--text);font-size:12.5px;font-family:inherit;outline:none;width:100%}
input:focus,select:focus{border-color:var(--accent)}
.row{display:flex;gap:10px;flex-wrap:wrap}.row>.field{flex:1;min-width:140px}
.actions{display:flex;gap:6px;flex-wrap:wrap}
.switch{display:inline-flex;align-items:center;gap:8px;cursor:pointer;font-size:12.5px}
.switch input{display:none}
.switch .track{width:34px;height:19px;border-radius:20px;background:var(--panel3);border:1px solid var(--border);position:relative;flex-shrink:0}
.switch .track::after{content:"";position:absolute;top:2px;left:2px;width:13px;height:13px;border-radius:50%;background:var(--muted);transition:.15s}
.switch input:checked+.track{background:var(--accent-bg);border-color:var(--accent-bd)}
.switch input:checked+.track::after{transform:translateX(15px);background:var(--accent)}
.tabs{display:flex;gap:4px;border-bottom:1px solid var(--border);margin-bottom:16px;flex-wrap:wrap}
.tab{padding:8px 12px;font-size:12.5px;font-weight:600;color:var(--muted);cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px}
.tab.active{color:var(--accent);border-bottom-color:var(--accent)}
.scrim{position:fixed;inset:0;background:rgba(4,6,14,.6);z-index:900;opacity:0;pointer-events:none;transition:.15s}
.scrim.open{opacity:1;pointer-events:auto}
.drawer{position:fixed;top:0;right:0;bottom:0;width:min(560px,95vw);background:var(--panel);border-left:1px solid var(--border);z-index:950;transform:translateX(100%);transition:.18s;display:flex;flex-direction:column}
.drawer.open{transform:none}
.drawer-head{display:flex;align-items:center;gap:10px;padding:14px 16px;border-bottom:1px solid var(--border)}
.drawer-head h3{font-size:13.5px;font-weight:700}
.drawer-body{padding:16px;overflow-y:auto;flex:1}
.modal{position:fixed;inset:0;z-index:960;display:none;align-items:center;justify-content:center;padding:20px}
.modal.open{display:flex}
.modal .box{background:var(--panel);border:1px solid var(--border);border-radius:14px;width:min(620px,96vw);max-height:90vh;overflow:auto;box-shadow:0 30px 80px rgba(0,0,0,.55)}
.modal .box.wide{width:min(860px,97vw)}
.log{background:#05070d;border:1px solid var(--border);border-radius:9px;padding:11px;font-family:var(--mono);font-size:11.5px;line-height:1.55;white-space:pre-wrap;word-break:break-word;max-height:52vh;overflow:auto;color:#cbd5e1}
.toast{position:fixed;right:18px;bottom:18px;z-index:2000;display:flex;flex-direction:column;gap:8px}
.toast .t{padding:10px 14px;border-radius:9px;font-size:12.5px;font-weight:600}
.toast .t.ok{background:#0f7a3d;color:#fff}.toast .t.err{background:#b91c1c;color:#fff}.toast .t.info{background:var(--panel3);border:1px solid var(--border)}
.spin{display:inline-block;width:12px;height:12px;border:2px solid var(--accent-bd);border-top-color:var(--accent);border-radius:50%;animation:s .8s linear infinite;vertical-align:-1px}
@keyframes s{to{transform:rotate(360deg)}}
.empty{text-align:center;padding:46px 20px;color:var(--dim)}
.empty .big{font-size:26px;opacity:.4;margin-bottom:8px}
.hide{display:none!important}.muted{color:var(--muted)}.dim{color:var(--dim)}.right{text-align:right}.center{text-align:center}.mt8{margin-top:8px}.mt14{margin-top:14px}.mb14{margin-bottom:14px}
.trunc{max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:inline-block;vertical-align:middle}
.clickable{cursor:pointer}
.stats{grid-template-columns:repeat(auto-fit,minmax(170px,1fr))}
.stat-label{font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.09em;color:var(--dim)}
.stat-value{font-size:23px;font-weight:700;font-family:var(--mono);margin-top:5px}
.tplgrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));gap:14px}
.tplcard{background:var(--panel);border:1px solid var(--border);border-radius:12px;overflow:hidden;cursor:pointer;transition:border-color .12s}
.tplcard:hover{border-color:var(--accent)}
.tplthumb{height:150px;overflow:hidden;background:#fff;border-bottom:1px solid var(--border)}
.tplthumb iframe{width:200%;height:200%;transform:scale(.5);transform-origin:0 0;border:0}
.tplmeta{padding:11px 13px}
.tplmeta b{font-size:12.5px}
.tplmeta .dim{font-size:10.5px;text-transform:uppercase;letter-spacing:.05em;margin-left:6px}`

const cpLoginHTML = `<!DOCTYPE html><html lang="en" data-theme="dark"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sign in · evil-pro control panel</title><link rel="icon" href="/favicon.ico">
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;background:#080b14;color:#e7ecf7;min-height:100vh;display:flex;align-items:center;justify-content:center;background-image:radial-gradient(900px 500px at 50% -10%,rgba(14,165,233,.16),transparent)}
.box{width:340px;background:#101524;border:1px solid #1e2536;border-radius:16px;padding:28px 26px;box-shadow:0 30px 70px rgba(0,0,0,.5)}
.brand{display:flex;align-items:center;gap:10px;margin-bottom:4px}.brand svg{width:30px;height:30px;border-radius:8px}
.brand h1{font-size:15px;font-weight:700}.sub{color:#8b98b6;font-size:12px;margin-bottom:22px}
label{display:block;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.08em;color:#8b98b6;margin:14px 0 6px}
input{width:100%;padding:10px 12px;border-radius:9px;border:1px solid #1e2536;background:#0c1019;color:#e7ecf7;font-size:13px;outline:none}
input:focus{border-color:#0ea5e9}
button{width:100%;margin-top:20px;padding:11px;border:none;border-radius:9px;background:#0ea5e9;color:#fff;font-size:13px;font-weight:600;cursor:pointer}
button:disabled{opacity:.6}
.err{margin-top:14px;font-size:12px;color:#f87171;min-height:16px}
</style></head><body>
<form class="box" id="form">
  <div class="brand"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#0ea5e9"/><path d="M9 10h14v3H9zm0 6h10v3H9z" fill="#fff"/><circle cx="23" cy="21" r="3" fill="#fff"/></svg><h1>evil-pro control panel</h1></div>
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
<html lang="en" data-theme="dark"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>evil-pro · control panel</title><link rel="icon" href="/favicon.ico">
<link rel="stylesheet" href="/static/app.css"></head>
<body data-app="ctrl-panel"><div id="app">
  <aside class="side">
    <div class="side-head">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#0ea5e9"/><path d="M9 10h14v3H9zm0 6h10v3H9z" fill="#fff"/><circle cx="23" cy="21" r="3" fill="#fff"/></svg>
      <div class="side-brand">evil-pro<small>control panel</small></div>
    </div>
    <nav class="nav" id="nav"></nav>
    <div class="side-foot" id="foot"></div>
  </aside>
  <div class="main">
    <header class="top">
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
