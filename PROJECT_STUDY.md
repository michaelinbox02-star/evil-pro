# Code study — "Evilginx Pro Edition 4.0" (`evilginx2-master`)

**Subject:** a fork of Kuba Gretzky's `evilginx2`, rebranded by `@officialmonsterz`.
**Method:** static reading of the non-vendored Go sources (~16k lines across `core/`, `database/`, `evilfeed/`, `parser/`, `log/`, `main.go`), the YAML/HTML assets, build files and docs. No code was executed. Nothing in the tree was modified except adding this file.

> **Framing.** This is an adversary-in-the-middle (AiTM) phishing framework: it is designed to sit between a victim and a real login page and steal the resulting session. Studying it is legitimate for defenders, incident responders and authorised red teams. Running it against anyone who has not given written permission is a serious crime in essentially every jurisdiction. Nothing below is deployment guidance.

---

## 1. What the project actually is

Mechanically, the original evilginx2 is a **TLS-terminating reverse proxy with a rule engine for rewriting a specific site's traffic**. That is all an AiTM kit is:

1. The victim resolves `login.attacker.tld` to the operator's server.
2. The server presents a valid certificate for that name and **proxies every request to the real site** (`login.microsoftonline.com`), rewriting hosts, URLs, cookies and CSP in both directions so the browser thinks it is talking directly to the real service.
3. The victim performs a **real** login, including MFA, against the real IdP.
4. When the IdP issues its session cookies, the proxy recognises them (by name/domain, from the phishlet) and **copies them out**. The victim is then redirected to the real site and notices nothing.
5. The stolen cookies are replayed by the operator — no password or OTP is needed at that point.

Everything in this repository is either infrastructure for that loop (DNS, ACME, dashboard, notifications) or marketing.

### Provenance / packaging oddities

- The Go module path is still `github.com/kgretzky/evilginx2` (`go.mod:1`) — the fork never re-moduled itself.
- **No `.git` directory**: this is a flattened snapshot, not a clone, so there is no history to attribute changes.
- **No tests at all** (`**/*_test.go` → none), and no CI workflow (only `.github/FUNDING.yml`), although the README renders a GitHub Actions build badge.
- Dependencies are fully vendored (1,154 Go files under `vendor/`), including the author's own `kgretzky/goproxy` fork (`go.mod:70`).
- Ships a **1,153-line `DEPLOYMENT.md`** ("From a fresh VPS to a fully operational phishing framework") covering VPS setup, Cloudflare, Telegram bot creation, systemd and cert renewal. Combined with a ready-made Microsoft 365 phishlet, this is turnkey criminal infrastructure, not a research harness — the "authorized testing only" notice in the README sits uneasily next to it.

---

## 2. Inventory of the tree

| Path | Lines | Role |
|---|---:|---|
| `main.go` | 320 | flags, wiring, startup order |
| `core/http_proxy.go` | 2,699 | **the engine**: goproxy handlers, capture logic, rewriting |
| `core/terminal.go` | 2,528 | interactive CLI (config/phishlets/lures/sessions/…) |
| `core/dashboard.go` | 1,430 | single-file HTML+JS admin panel, Basic Auth, HTTP |
| `core/webapi.go` + `webapi_auth.go` | 1,856 | a second, better admin API — **never started** (see §8) |
| `core/config.go` | 1,262 | viper-backed config, lures, phishlet registry |
| `core/phishlet.go` | 1,108 | phishlet YAML parser/validator |
| `core/certdb.go` | 410 | certmagic/ACME, self-signed dev certs, wildcard loader |
| `core/notify.go`, `tele.go`, `telegram_queue.go` | 638 | Telegram formatting/sending |
| `core/geoip.go` | 288 | MaxMind lookups, VPN/DC heuristics |
| `core/validator.go` | 163 | **replays captured creds against Microsoft/Google** |
| `core/randomizer.go`, `validator.go`, `redirects.go`, `auto_export.go` | ~470 | fork "features", mostly inert |
| `database/` | ~700 | BuntDB persistence (sessions, users) |
| `evilfeed/` | ~300 | separate `localhost:1337` websocket viewer |
| `phishlets/o365new.yaml` | 135 | **one** working M365/Okta/GoDaddy/OneLogin/Duo phishlet |
| `redirectors/`, `post_redirectors/`, `templates/`, `web/` | — | decoy pages, Turnstile templates, dashboard assets |

Listeners: **53/udp** (embedded DNS, `nameserver.go:41`), **:80** (ACME HTTP-01, Turnstile, HTTP→HTTPS redirect, `http_server.go:46`), **:443** (the proxy, `http_proxy.go:138`), **:5000** (dashboard, `main.go:39`), **:1337** (evilfeed, `evilfeed/evilfeed.go:127`), **:2030** (Web API — code exists, never launched).

---

## 3. The loop, line by line

### 3.1 Session creation (`http_proxy.go:194-593`)

- Requests arrive via goproxy's `NonproxyHandler` (`:161`), which forces scheme `https` and hands off to the MITM proxy. `HandleConnect(goproxy.AlwaysMitm)` is set at `:184`, so the box also behaves as a forward MITM proxy if something CONNECTs to it.
- Client IP is taken from the socket **or any of eight `X-Forwarded-For`-style headers** (`:210-217`) — spoofable, which matters for blacklisting and GeoIP.
- Blacklist modes (`off`/`unauth`/`all`) and the scanner-UA check run before anything else (`:219-255`).
- Host → phishlet resolution is by phished hostname (`getPhishletByPhishHost`, `:265`), with a lure-path override (`:346`) because several phishlets can share a subdomain.
- **Session identity is a cookie** whose *name* is `sha256(phishlet + "-" + random8)[:8]` (`:2578-2582`, `:155`) and whose value is a 32-byte random token (`GenRandomToken`). It is set on the base domain with a 60-minute expiry (`:1102-1108`). Sessions live in `p.sessions`/`p.sids` maps, mirrored into BuntDB (`:564`).
- A new visitor that hits a valid lure gets a session, GeoIP enrichment (`:491-526`), optional GoPhish "email opened/link clicked" callbacks (`:528-561`), and is redirected to the phishlet's login URL (`:730-757`). Non-lure paths get a 302 to the same place, or a 403/spoof if blacklisted.

### 3.2 Credential capture (`:847-1049`)

The request body is buffered, URL-rewritten, and then matched against the phishlet's `credentials` rules:

- **JSON bodies**: regex on the whole body; username/password/custom fields extracted and written to the session and DB (`:865-905`).
- **Form bodies**: `ParseForm`, match on field *name* regex, then extract from the value (`:948-990`).
- `force_post` rules inject extra fields (e.g. `DontShowAgain=true`) to smooth the flow (`:907-943`).
- Each hit calls `go p.sendTelegramNotificationForSession(...)` (`:945`).

The request is then forwarded upstream **unchanged in intent** — that is the whole trick: the real IdP sees a genuine login and issues genuine cookies.

### 3.3 Token capture (`:1092-1311`)

On every response:

1. Session cookie is attached if the session was just created.
2. `Location`, `Access-Control-Allow-Origin`, CSP and `X-Frame-Options` are rewritten from real→phish (`:1114-1154`).
3. Every `Set-Cookie` is inspected against the phishlet's `auth_tokens` list (domain + name, or `.*` with `regexp`). Matches are stored as `CookieTokens[domain][name]` (`:1166-1207`), then the cookie's `Domain` is rewritten to the phishing domain and re-emitted.
4. `body_auth_tokens` (regex over the response body, `:1216-1226`) and `http_auth_tokens` (request headers such as `Authorization`, `:1228-1236`) are collected the same way.
5. **Completion** = every non-optional cookie token captured **and** all body tokens **and** all HTTP tokens (`:1241-1252`, `session.go:116-153`). Alternatively, a regex from `auth_urls` matching the request path marks the session done (`:1062-1072`, `:1446-1497`).
6. On completion: tokens go to BuntDB, the session is `Finish()`ed (`session.go:155`), GoPhish is told "credentials submitted", the **credential validator is fired in a goroutine**, and the Telegram queue is fed (`:1252-1311`).
7. After completion, the next HTML response is replaced by a JS redirect to `redirect_url` so the victim lands on the real site (`:1503-1516`).

### 3.4 Rewriting

- Host/Origin/Referer/X-Embedding-Uri rewritten real→phish on the way in (`:794-845`); `Set-Cookie` domains, `Location` and ACAO on the way out (`:1104-1154`).
- `sub_filters` are per-host regex substitutions with `{hostname}`, `{domain}`, `{basedomain}`, `{subdomain}` placeholders and optional MIME/param gating (`:1313-1384`).
- `auto_filter` runs the generic URL rewriter over the body (`patchUrls`, `:1915`).
- Injected into HTML: `<script src="/s/<session>.js">` and `<script src="/s/<session>/<id>.js">` (`:1426-1432`) — these are served by the proxy itself (`:271-338`) and are the mechanism for the post-capture redirect and phishlet JS.
- `rewriteSensitivePaths` strips the phishing origin out of bodies so the address bar/DOM look cleaner (`:2598-2609`).
- Proxy-identifying headers are deleted on both legs (`:1074-1084`, `:1397-1410`), including `Server`.

---

## 4. Session model and what is stored

`core/session.go:9-39` — one struct per victim: `Username`, `Password`, `Custom` (GeoIP, validation), `Params` (lure parameters), `BodyTokens`, `HttpTokens`, `CookieTokens[domain][name]{Value,Path,HttpOnly}`, `RedirectURL`, `IsDone`, `RemoteAddr`, `UserAgent`, timestamps.

Persistence is BuntDB in `~/.evilginx/data.db`, JSON-serialised, **unencrypted** (`database/db_session.go`). The dashboard's "cookie export" endpoint emits a Cookie-Editor–compatible JSON plus a ready `Cookie:` header string (`dashboard.go:332-430`), i.e. the last mile of replay is built in.

Other files of interest to an investigator: `~/.evilginx/config.json` (Telegram bot token, dashboard password, Turnstile secret, GoPhish API key — all plaintext), `blacklist.txt`, `crt/`, and optional on-disk exports.

---

## 5. The phishlet DSL

Parsed into `ConfigPhishlet` (`phishlet.go:221-234`), validated for `proxy_hosts`/`auth_tokens`/`credentials`/`login` presence (`:377-394`).

| Section | Meaning |
|---|---|
| `proxy_hosts` | (phish_sub, orig_sub, domain, session, is_landing, auto_filter) — which real hosts are proxied and under which phishing subdomain |
| `sub_filters` | per-host regex substitutions with MIME gating, `redirect_only`, `with_params` |
| `auth_tokens` | (domain, keys[]) — which cookies are the prize; `.*,regexp` wildcards allowed; `optional` flag |
| `body_auth_tokens` / `http_auth_tokens` | tokens that appear in bodies or headers instead of cookies |
| `auth_urls` | URL regexes that mean "authentication finished" |
| `credentials` | (username, password, custom) × (key regex, search regex, `post`/`json`) |
| `force_post` | inject parameters into matching POSTs |
| `login` | domain+path the lure redirects to |
| `js_inject` | JS injected into matching pages, with `trigger_domains`/`paths`/`params` |
| `intercept` | return a canned body for a matching path |
| `params` | template parameters (makes a phishlet a template requiring `subphishlets`) |

The bundled `o365new.yaml` chains Microsoft (`microsoftonline.com`, `live.com`, `office.com`) with **Okta, GoDaddy, OneLogin and Duo** hosts, wildcard-captures `.*` cookies on the SSO domains, and injects JS that pre-fills the email field from the URL fragment and adds a fake "verify your password" banner. It also contains tenant-specific artefacts (`dotfoods`, `empowermm`, `ulgroup`) — i.e. it was used against at least one real organisation before being published.

---

## 6. Operator surface

- **CLI** (`terminal.go:89-201`): `config`, `proxy`, `sessions`, `phishlets`, `lures`, `autolures`, `redirects`, `blacklist`, `test-certs`, `help`, `exit`. Lures gained custom subdomains, `ua_filter`, `active_hours/days/tz`, OG metadata and **automatic path rotation** (`monitorAutoLureRotation`, `:1753`) — rotate the URL every ~45–90 min so the captured link dies quickly.
- **Dashboard** (`:5000`, HTTP): session table with password column, search, filter, CSV/JSON export, cookie export. Auth is HTTP Basic with a **plaintext string comparison** (`dashboard.go:113-127`); if user and password are empty it is disabled entirely.
- **Web API** (`webapi.go`): bcrypt users, roles, audit log, stats, phishlet editing — a genuinely better design, but `NewWebAPI` is never called from `main.go`, so none of it runs.
- **Telegram**: queue → `Notify()` → multipart document upload of a `session_tokens.txt` plus a MarkdownV2 caption (`tele.go:20-51`, `telegram_queue.go`). Token/chat are per-config, not hardcoded — no hidden exfiltration to the fork author was found.
- **GeoIP**: MaxMind City DB; `IsVPN/IsProxy/IsDatacenter` are derived (see `geoip.go`), used for tracking and optional blocking (`-block-vpn`, `-block-countries`).
- **Credential validator** (`validator.go`): on capture, performs a real password login to Microsoft (OAuth2 ROPC to `login.microsoftonline.com/common/oauth2/v2.0/token`) or Google (`accounts.google.com/_/signin/sl/challenge`) and stores `valid: true/false`. Functionally weak — the Microsoft `client_id` is all zeros (`:91`), which most tenants reject — but legally it is a second unauthorised authentication attempt per victim.
- **GoPhish**: reports open/click/submit back to a GoPhish instance (`gophish.go`), plus a `replace_rid.sh` helper that rewrites a campaign RID through sources and rebuilds.
- **evilfeed**: a separate binary serving `localhost:1337` with a websocket that streams captures to a browser UI; no authentication.

---

## 7. Claimed vs. implemented

The README is the least trustworthy artefact in the repository. Verified against the code:

| README claim | Reality |
|---|---|
| "30+ detection signals incl. **JA3/JA3S** fingerprinting, sandbox/VM, headless" | 38 **User-Agent regexes** (`http_proxy.go:1620-1668`). No TLS fingerprinting, no VM/sandbox checks anywhere in the tree. |
| "Privacy extension detection (uBlock, Ghostery, NoScript, headless, Puppeteer)" | Absent. No such code exists. |
| "Anti-screenshot CSS randomization: hue, brightness, skew, translate, opacity" | `injectRandomCSS` appends a `0×0`, `visibility:hidden` div (`:1736-1766`); `Randomizer.RandomizeResponseBody` adds a `--rnd:` custom property (`randomizer.go:46-67`) **and is never called**. Rendered pixels are unchanged — the anti-screenshot feature is fiction. |
| "JS obfuscation — base64+eval" | `func obfuscateJS(s string) string { return s }` (`:2584-2586`). |
| "Auto-export to JSON+CSV in real time" | `Enabled:false` default (`auto_export.go:31-38`); `SetAutoExportConfig` is never called, so the code path is unreachable. |
| "Full SPA + REST API, multi-user RBAC, audit log" | Implemented in `webapi.go`/`webapi_auth.go` — and never instantiated. Dead code. |
| "40+ pre-built phishlets" | **One** (`phishlets/o365new.yaml`). |
| "Docker ~18MB Alpine" | `Dockerfile:2` uses `golang:1.22-alpine`; `go.mod:3` declares `go 1.25.7`. The image cannot build. |
| "Wildcard SSL, hidden from crt.sh" | Real, but only if the operator supplies `crt/wildcard/fullchain.pem` + `privkey.pem` themselves (`certdb.go:62-90`). |
| "Header stripping" | Real (`:1074-1084`, `:1397-1410`). |
| "URL rewriting cleans the address bar" | Real, but a naive `strings.ReplaceAll` of two prefixes (`:2598-2609`). |
| "Dynamic content spoofing — blocked visitors see the real site" | Real: fetches `unauth_url`, rewrites host strings, returns it (`:1551-1608`). |
| "BuntDB, Telegram queue, GeoIP, GoPhish, Turnstile, autolures" | Real. |
| "60+ engineering improvements" | Many are comments; several "features" are inert; the fork introduces new defects (below). |

The pattern — README tables that describe features not present, comments like `// WHAT THIS DOES (baby steps)` and `// ADD THIS:` (`validator.go:5`, `geoip.go:5,82,165`, `http_proxy.go:1491`), a "FIX: … eliminates the compile error" comment (`:483-487`) — is consistent with a largely LLM-generated layer placed over the upstream codebase.

---

## 8. Defects worth knowing

**Concurrency**
- `p.sessions` / `p.sids` are written without a lock (`:548-549`) while the Telegram path reads them under `session_mtx` (`:2632`), and `p.sids[...]` is read unlocked in request handling (`:325`). That is a textbook Go map race — `-race` would fire, and under load it can crash the process mid-campaign.
- `p.session_mtx` is locked, unlocked, then locked again around the same read (`:2632-2684`); the `exists` check is stale by the time it is used.
- `time.Sleep(2 * time.Second)` inside the notification path (`:2629`) with a **single** queue worker (`telegram_queue.go:45-51`) serialises and stalls capture notifications.

**Memory / correctness**
- Every response body is `io.ReadAll`'d regardless of MIME, then re-wrapped (`:1212`, `:1443`) — no streaming, no size cap; a large download is buffered in RAM, and on read error the body is lost.
- `session.Cmsgid = "req.Header.Get()"` (`:570`) assigns a literal string instead of a header.
- `waitForRedirectUrl` returns `("", false)` after 30 s because `break` exits the `select`, not the loop (`:1533-1549`).
- `AllCookieAuthTokensCaptured` mutates a slice while ranging over it (`session.go:127-147`).
- `ReadLatestSession` parses `data.db` as line-oriented text with `$` separators (`tsession.go:31-79`) — nonsense for a BuntDB file; only reachable from `readFile`, which nothing calls.

**Operational security of the tool itself**
- Dashboard binds `0.0.0.0:5000` by default with username `admin` and **empty password** (`main.go:39-41`, `dashboard.go:113-127`); plain HTTP, no CSRF, no rate limiting, no session expiry. Anyone who can reach the port sees every captured password and cookie.
- Credentials are written to the CLI log at `Success` level (`:874-899`) and stored in plaintext in `data.db`; config secrets in plaintext `config.json`.
- `InsecureSkipVerify: true` in the validator, the unauth-page fetcher and developer certs (`validator.go:49`, `:1573`).
- Web auth tokens never expire (`db_users.go:177-205`).

**Build / hygiene**
- No tests, no CI, toolchain mismatch with the Dockerfile, dead packages, duplicated dashboards (`web/index.html` plus a 1,400-line HTML string in `dashboard.go`).

---

## 9. Detection notes (the useful part for defenders)

- **IdP telemetry is the strongest signal.** The validator performs a *second* authentication with the victim's password from the operator's IP using a null `client_id`. In Entra ID that is an ROPC grant with `client_id 00000000-0000-0000-c000-000000000000` — highly anomalous and loggable; correlate "successful session created from IP A" with "password grant from IP B".
- **Session-cookie replay** is what actually hurts. Cookies arrive from a different IP/ASN/device than the one that performed the login, with no matching device state. Conditional Access session controls, token protection (binding to device), continuous access evaluation and FIDO2/ passkeys are the real mitigations; MFA alone does not stop this class of attack.
- **Proxy fingerprints** (still present despite header stripping): injected `<script src="/s/<32-hex>.js">` tags, `Cache-Control: no-cache, no-store` forced onto HTML/JSON, injected `Access-Control-Allow-Origin` when upstream has none, stripped `Server` header, and a session cookie with an 8-hex name and 32-byte value.
- **Network**: Go's `crypto/tls` ClientHello is distinctive (JA3/JA4), so upstream connections from a Go proxy differ from a browser's; egress to `api.telegram.org`; DNS served from the same host.
- **CT logs** still work unless the operator supplies their own wildcard cert — per-hostname ACME issuance exposes every phished subdomain.
- **Page artefacts**: the `redirectors/o365_turnstilev3` decoy set and the injected "Because you're accessing sensitive info you need to verify your password" banner in the bundled phishlet are reusable signatures/hashes.

---

## 10. Bottom line

- It is a **functional, dangerous AiTM framework** — the core loop (proxy, credential extraction, cookie harvest, replay-ready export) is real and works.
- It is **not** the "most advanced fork" it advertises: several headline features are inert or unreachable, the phishlet collection is one file, the Docker build is broken, there are no tests, and the fork adds concurrency bugs to otherwise-stable upstream code.
- Its **own security is poor** (open dashboard, plaintext secrets, credential logging), which is both an operator risk and an investigative gift.
- For study purposes the interesting parts are `http_proxy.go` (the entire technique in ~1,500 lines), `phishlet.go` (the declarative attack surface) and `session.go` (the definition of "captured").
- For defence, the repository is a good specimen of the current phishing commodity: it shows exactly which controls fail (password + OTP) and which hold (phishing-resistant credentials, device-bound sessions, IdP-side anomaly correlation).
