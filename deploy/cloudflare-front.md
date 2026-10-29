# Optional Cloudflare front for redirect-page links

The emailed link in a campaign is often the redirect page, not the lure. Mailbox
placement and scanner behaviour follow the domain in that link. This feature
serves the page from a Cloudflare Worker on the account's `workers.dev` domain
(Cloudflare's own public-suffix domain) instead of a young custom subdomain.

The **lure never changes**: the page still points at the normal lure URL.

## Naming is per page, not global

Each redirect page publishes **its own Worker under its own label**, so its link
reads like a hostname and carries nothing about the engine:

```
page "doc"          -> https://doc.platoveriour.workers.dev
page "arrival"      -> label "share-files" -> https://share-files.platoveriour.workers.dev
```

The label defaults to the page's own subdomain, which mirrors the campaign
(`doc.platoveriour.com` -> `doc.platoveriour.workers.dev`). Set anything you like
in the page's editor (**Redirects -> page -> Edit -> workers.dev label**); it is
sanitised to a valid DNS label (lowercase, letters/digits/hyphens, max 54).

Two pages may deliberately share one label, in which case a single Worker serves
both and each link carries the page path
(`https://<label>.<account>.workers.dev/<page>`). The panel says so when it sees
sharing.

Unavoidably account-wide: the `workers.dev` **suffix** itself
(`platoveriour.workers.dev`). It is one per Cloudflare account, read from the API
and created on first deploy if the account has none.

## When to use which link

* **Plain subdomain link** (`https://doc.yoursite.com`) - the default. Use it when
  the domain already has age and reputation, or when you do not want any Cloudflare
  dependency. Always works, front on or off.
* **`.dev` front link** - use it for cold campaigns where the domain is new and
  placement is the risk. The link then sits on Cloudflare's own domain. Both links
  stay live at the same time, so you can A/B them.

Neither link changes where the victim ends up.

## Enabling it

1. Put a Cloudflare API token on the server profile (Cloudflare tab). It needs:
   * `Zone:Read` - find the account from the zone
   * `Workers Scripts:Edit` - upload the Worker
   * `Workers Scripts:Read` - read the account's `workers.dev` subdomain
   * `Turnstile:Read` + `Turnstile:Edit` - only if a fronted page uses Turnstile
   If one is missing the panel names the exact permission to add.
2. Set the page to `front: on` in its editor and give it a **workers.dev label**
   (empty = its own subdomain). The global default lives in **Settings**
   (*cloudflare front*) and is `off`, so nothing changes until asked.
3. Cloudflare tab, **Deploy / update front worker**. One Worker is uploaded per
   label; the link is stored on each page and appears in the redirect list and in
   the server hand-off.

## How it stays exact

The Worker is deliberately thin, so there is **one** implementation of the bot
logic, of branding/template placeholders and of the Turnstile injection, and it
lives in the engine:

* the client `User-Agent` is forwarded, never replaced by Cloudflare's own;
* `redirect: "manual"`, so the engine's 302 is passed through rather than followed;
* `cf: { cacheTtl: 0 }` on the subrequest and `Cache-Control: no-store` on the
  outer response. A cached copy served to a scanner would silently defeat the
  anti-bot path, so caching is disabled at both ends.

## Turnstile

Turnstile validates the page hostname. A brand new `workers.dev` host is not on
the widget's allowed domains, so the widget renders but shows "Unable to connect
to website", `__tsOnPass` never fires and the visitor is parked. The deploy adds
every published host to the widget's allowed domains automatically (`PUT`, not
`PATCH`: with a token, PATCH answers `10405`).

Hosts are only ever added, never pruned, so a host belonging to another server on
the same account is not yanked out from under it. Remove stale ones by hand in the
Cloudflare dashboard if you want.

## workers.dev or pages.dev

Both are Cloudflare's own domains and equally reputable, and both are covered by
the `.dev` HSTS preload. **`workers.dev` is the right one here** because the front
executes code (a passthrough), and because each page's label becomes the first
part of the link. Pages serves static files, so it would need a Worker anyway
(Pages `_middleware` runs on Workers) - it gains nothing for this shape.

`.dev` is HSTS-preloaded, so browsers only ever use https; the Worker also sends
`Strict-Transport-Security` explicitly. Cloudflare still answers plain http with a
200 rather than redirecting, so always write `https://`.

## Notes

* Turning a page's front off restores its plain link exactly; the stored `.dev`
  link is kept so turning it back on is one click, and the hand-off stops
  advertising it while off.
* The Worker forwards to the page's engine origin, so branding, delay and
  Turnstile settings stay on the engine.
* A single-page Worker answers on every path, so its link has no suffix. Its bare
  host also shows a small index of published paths if you hit a shared Worker's
  root.
* Deploying replaces the older single-Worker layout (`evilpro-<domain>`). The
  panel tells you when that name is no longer used by any page so you can delete
  it in the dashboard.
