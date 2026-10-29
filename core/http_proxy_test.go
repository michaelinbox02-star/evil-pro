package core

import (
	"net/url"
	"testing"
)

const authorizeRaw = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=abc&response_type=code%20id_token"

func newForceProxy(on bool) *HttpProxy {
	return &HttpProxy{cfg: &Config{general: &GeneralConfig{ForceLogin: on}}}
}

// forceLoginPrompt must apply the mode it is given, never overwrite a prompt the
// app asked for itself, and leave SAML/WS-Fed on the wfresh=0 equivalent.
func TestForceLoginPromptModes(t *testing.T) {
	p := &HttpProxy{cfg: &Config{general: &GeneralConfig{LoginPrompt: LOGIN_PROMPT_LOGIN}}}

	cases := []struct {
		name     string
		mode     string
		in       string
		wantKey  string
		wantVal  string
		modified bool
	}{
		{"login on oauth v2", LOGIN_PROMPT_LOGIN, authorizeRaw, "prompt", "login", true},
		{"login on oauth", LOGIN_PROMPT_LOGIN, "https://login.microsoftonline.com/common/oauth2/authorize?client_id=abc", "prompt", "login", true},
		{"login on live srf", LOGIN_PROMPT_LOGIN, "https://login.live.com/oauth20_authorize.srf?client_id=abc", "prompt", "login", true},
		{"select_account on oauth", LOGIN_PROMPT_SELECT_ACCOUNT, authorizeRaw, "prompt", "select_account", true},
		{"off leaves the url alone", LOGIN_PROMPT_OFF, authorizeRaw, "", "", false},
		// an app that asks for the picker keeps its own value
		{"existing prompt preserved", LOGIN_PROMPT_LOGIN, authorizeRaw + "&prompt=select_account", "prompt", "select_account", false},
		{"already forced", LOGIN_PROMPT_LOGIN, "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?prompt=login", "prompt", "login", false},
		// unrelated hop (asset, api, ...)
		{"unrelated hop", LOGIN_PROMPT_LOGIN, "https://login.microsoftonline.com/common/handlers/foo.js?v=2", "", "", false},
		// SAML / WS-Fed equivalent, any non-off mode
		{"saml2", LOGIN_PROMPT_LOGIN, "https://login.microsoftonline.com/tenant/saml2?SAMLRequest=x", "wfresh", "0", true},
		{"wsfed", LOGIN_PROMPT_LOGIN, "https://login.microsoftonline.com/tenant/wsfed?wa=wsignin1.0", "wfresh", "0", true},
		{"wsfed select_account still uses wfresh", LOGIN_PROMPT_SELECT_ACCOUNT, "https://login.microsoftonline.com/tenant/wsfed?wa=wsignin1.0", "wfresh", "0", true},
	}

	for _, c := range cases {
		u, err := url.Parse(c.in)
		if err != nil {
			t.Fatalf("%s: parse %q: %v", c.name, c.in, err)
		}
		before := u.RawQuery
		got := p.forceLoginPrompt(u, c.mode)
		if got != c.modified {
			t.Errorf("%s: modified=%v, want %v", c.name, got, c.modified)
			continue
		}
		if !c.modified {
			if u.RawQuery != before {
				t.Errorf("%s: query changed but reported no change", c.name)
			}
			continue
		}
		if q := u.Query(); q.Get(c.wantKey) != c.wantVal {
			t.Errorf("%s: %s=%q, want %q (%q)", c.name, c.wantKey, q.Get(c.wantKey), c.wantVal, u.RawQuery)
		}
		// the rest of the URL must survive intact
		if q := u.Query(); q.Get("client_id") == "" && q.Get("wa") == "" && q.Get("SAMLRequest") == "" {
			t.Errorf("%s: dropped the original params: %q", c.name, u.String())
		}
	}
}

// An empty mode means "use the global setting".
func TestForceLoginPromptEmptyModeUsesGlobal(t *testing.T) {
	off := &HttpProxy{cfg: &Config{general: &GeneralConfig{LoginPrompt: LOGIN_PROMPT_OFF}}}
	u, _ := url.Parse(authorizeRaw)
	if off.forceLoginPrompt(u, "") {
		t.Error("global off must leave the url alone")
	}
	sel := &HttpProxy{cfg: &Config{general: &GeneralConfig{LoginPrompt: LOGIN_PROMPT_SELECT_ACCOUNT}}}
	u2, _ := url.Parse(authorizeRaw)
	if !sel.forceLoginPrompt(u2, "") {
		t.Error("global select_account must apply")
	}
	if u2.Query().Get("prompt") != LOGIN_PROMPT_SELECT_ACCOUNT {
		t.Errorf("want select_account, got %q", u2.Query().Get("prompt"))
	}
}

// The legacy force_login alias still drives the behaviour, both ways.
func TestForceLoginAlias(t *testing.T) {
	off := newForceProxy(false)
	u, _ := url.Parse(authorizeRaw)
	if off.forceLoginPrompt(u, "") {
		t.Error("force_login off must not touch the URL")
	}
	if u.Query().Get("prompt") != "" {
		t.Error("force_login off added a param")
	}
	if newForceProxy(false).forceLoginPrompt(nil, LOGIN_PROMPT_LOGIN) {
		t.Error("nil URL should be a no-op")
	}
}

// A per-lure override wins over the global setting; empty inherits.
func TestLoginPromptForLureOverride(t *testing.T) {
	c := &Config{general: &GeneralConfig{LoginPrompt: LOGIN_PROMPT_LOGIN}}
	if got := c.LoginPromptFor(nil); got != LOGIN_PROMPT_LOGIN {
		t.Errorf("no lure: got %q, want login", got)
	}
	if got := c.LoginPromptFor(&Lure{}); got != LOGIN_PROMPT_LOGIN {
		t.Errorf("empty lure override: got %q, want login (inherit)", got)
	}
	if got := c.LoginPromptFor(&Lure{LoginPrompt: LOGIN_PROMPT_SELECT_ACCOUNT}); got != LOGIN_PROMPT_SELECT_ACCOUNT {
		t.Errorf("lure override: got %q, want select_account", got)
	}
	if got := c.LoginPromptFor(&Lure{LoginPrompt: "on"}); got != LOGIN_PROMPT_LOGIN {
		t.Errorf("legacy boolean spelling: got %q, want login", got)
	}
}

// SetLoginPrompt validates and mirrors into force_login; SetForceLogin is the
// alias. Off must clear force_login so both stay truthful.
func TestSetLoginPromptAndAlias(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"off", LOGIN_PROMPT_OFF},
		{"login", LOGIN_PROMPT_LOGIN},
		{"select_account", LOGIN_PROMPT_SELECT_ACCOUNT},
		{"SELECT-ACCOUNT", LOGIN_PROMPT_SELECT_ACCOUNT},
		{"on", LOGIN_PROMPT_LOGIN},
		{"true", LOGIN_PROMPT_LOGIN},
		{"false", LOGIN_PROMPT_OFF},
	} {
		if got := NormalizeLoginPrompt(c.in); got != c.want {
			t.Errorf("NormalizeLoginPrompt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if NormalizeLoginPrompt("bogus") != "" {
		t.Error("a bogus value must normalize to empty (inherit)")
	}
}

// The nop-clash guard: forcing must not lose the response_type space encoding
// in a way that breaks Microsoft's parser.
func TestForceLoginPromptKeepsOAuthParams(t *testing.T) {
	p := newForceProxy(true)
	raw := "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=4765445b&redirect_uri=https%3A%2F%2Fwww.office.com%2Flandingv2&response_type=code%20id_token&scope=openid%20profile&state=abc123&nonce=n1&login_hint=alice%40contoso.com"
	u, _ := url.Parse(raw)
	if !p.forceLoginPrompt(u, LOGIN_PROMPT_LOGIN) {
		t.Fatal("expected the URL to be modified")
	}
	q := u.Query()
	for _, k := range []string{"client_id", "redirect_uri", "response_type", "scope", "state", "nonce"} {
		if q.Get(k) == "" {
			t.Errorf("param %s lost: %q", k, u.String())
		}
	}
	if q.Get("prompt") != "login" {
		t.Error("prompt=login not added")
	}
	if q.Get("login_hint") != "alice@contoso.com" {
		t.Errorf("login_hint mangled: %q", q.Get("login_hint"))
	}
	if q.Get("state") != "abc123" {
		t.Error("state must be preserved byte for byte")
	}
}
