# Optional Cloudflare front for redirect-page links

The emailed link in a campaign is often the redirect page, not the lure. Mailbox
placement and scanner behaviour follow the domain in that link. This feature
serves the page from a Cloudflare Worker on the account's `workers.dev` domain
(Cloudflare's own public-suffix domain) instead of a young custom subdomain.

The **lure never changes**: the page still points at the normal lure URL.

## When to use which link

* **Plain subdomain link** (`https://doc.yoursite.com`) - the default. Use it when
  the domain already has age and reputation, or when you do not want any Cloudflare
  dependency. Always works, front on or off.
* **`.dev` front link** (`https://evilpro-yoursite.yoursite.workers.dev/doc`) - use
  it for cold campaigns where the domain is new and placement is the risk. The
  link then sits on Cloudflare's own domain, and the first label carries your brand.
  Both links stay live at the same time, so you can A/B them.

Neither link changes where the victim ends up.

## Enabling it

1. Put a Cloudflare API token on the server profile (Cloudflare tab). It needs:
   * `Zone:Read` - find the account from the zone
   * `Workers Scripts:Edit` - upload the Worker
   * `Workers Scripts:Read` - read the account's `workers.dev` subdomain
   * `Turnstile:Read` + `Turnstile:Edit` - only if a fronted page uses Turnstile
   If one is missing the panel names the exact permission to add.
2. Set `front: on` on the redirect page (its editor), or globally with
   `config front on`. Global default is `off`, so nothing changes until asked.
3. Cloudflare tab, **Deploy / update front worker**. The `.dev` link is stored on
   the page and appears in the redirect list and in the server hand-off.

The account's `workers.dev` subdomain is read from the API and created if the
account has none (normally the dashboard creates it on first visit, the API does
not). One Worker fronts every enabled page, and the page map is embedded in the
script, so no KV is needed.

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
the host to the widget's allowed domains automatically (`PUT`, not `PATCH`: with a
token, PATCH answers `10405`).

## workers.dev or pages.dev

Both are Cloudflare's own domains and equally reputable, and both are covered by
the `.dev` HSTS preload. **`workers.dev` is the right one here** because the front
executes code (a passthrough), and because the account subdomain is a single
brandable label (`yoursite.workers.dev`) rather than a project name. Pages serves
static files, so it would need a Worker anyway (Pages `_middleware` runs on
Workers) - it gains nothing for this shape.

`.dev` is HSTS-preloaded, so browsers only ever use https; the Worker also sends
`Strict-Transport-Security` explicitly. Cloudflare still answers plain http with a
200 rather than redirecting, so always write `https://` in the campaign.

## Notes

* Turning the front off restores the plain links exactly; the stored `.dev` link
  is kept so turning it back on is one click, and the hand-off stops advertising
  it while off.
* The Worker keeps the slugs (`/doc`) and forwards to the engine origin, so the
  page keeps its branding, delay and Turnstile settings.
* `https://…workers.dev/` alone returns a small index of the published slugs.
