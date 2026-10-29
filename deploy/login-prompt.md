# login_prompt

Controls what the identity provider is asked to show on the authorization hop.

Set globally with `config login_prompt <mode>`, the Control Panel settings screen,
`general.login_prompt` in the config file, or `POST /api/v1/config`; override it
per lure (lure field `login_prompt`, or the lure editor in the Control Panel).
An empty lure value inherits the global setting.

## Modes

* `off` - never touch the authorization request. A visitor who already has a
  provider session goes straight to the app, so a fresh visit yields a
  **session-only, anonymous capture**: their session cookies and nothing typed.
* `login` - add `prompt=login`, so the provider always asks for the password even
  when the browser already holds a session. `wfresh=0` is added on SAML / WS-Fed
  hops, which have no `prompt`. Default.
* `select_account` - add `prompt=select_account`, so the provider shows its account
  picker. A returning victim clicks their own tile and skips typing the email;
  username and password still get captured. SAML / WS-Fed hops still get
  `wfresh=0`, since there is no equivalent.

Existing values are never overwritten: an app that asks for its own `prompt`
keeps it.

## Compatibility

`force_login` still works and is kept truthful in the config file: `on` means
`login`, `off` means `off`. A config that only carries `force_login` is read with
its original meaning, so adopted servers do not change behaviour.

`general.force_login` is mirrored on every write and `IsForceLoginEnabled()` still
reports whether any prompt is applied (i.e. the mode is not `off`).

## Modes are per visit

The mode is resolved per request from the lure that started the visit, so a
returning-visitor lure can use `select_account` while a cold lure uses `login` on
the same deployment.

The resolved mode is also pinned on the lure entry URL (`/?prompt=<mode>`), not
only on the authorization hop. Microsoft accepts `prompt` on its login entry
point and forwards it, and this is what makes the account picker render for an
entry that does not pass through an OAuth authorize (a redirect template, for
example). The mode is applied only while the login is not yet authenticated: an
authorize reached *after* the portal round-trip would otherwise force a second
credential prompt, which Microsoft answers with `AADSTS90100`.

## Caveats, stated plainly

* A redirect template that appends `login_hint` (the `file-signin` template does)
  names the account for Microsoft, and Microsoft then renders the password step
  directly instead of the picker. The mode still applies; the picker is simply
  not needed when the account is already known.
* `off` is not "never authenticate". It means "never ask for a prompt". With no
  session to reuse, Microsoft still asks, and that is correct.
* SAML / WS-Fed hops always get `wfresh=0` for a non-off mode, including
  `select_account`, because there is no equivalent parameter.

## Follow-up

`auto` / `prompt=none` is not implemented. A `prompt=none` mode would let the
provider answer silently when possible and fall back when not, which needs the
error path (`interaction_required`) handled; `off` is the closest thing today and
is honest about the trade-off (session-only capture).
