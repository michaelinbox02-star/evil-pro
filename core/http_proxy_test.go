package core

import (
	"net/url"
	"testing"
)

func newForceProxy(on bool) *HttpProxy {
	return &HttpProxy{cfg: &Config{general: &GeneralConfig{ForceLogin: on}}}
}

func TestForceLoginPromptOAuth(t *testing.T) {
	p := newForceProxy(true)
	cases := []struct {
		in       string
		want     string
		modified bool
	}{
		{
			"https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=abc&response_type=code%20id_token",
			"prompt=login", true,
		},
		{
			"https://login.microsoftonline.com/common/oauth2/authorize?client_id=abc",
			"prompt=login", true,
		},
		{
			"https://login.live.com/oauth20_authorize.srf?client_id=abc",
			"prompt=login", true,
		},
		// an app that asks for the picker keeps its own value
		{
			"https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=abc&prompt=select_account",
			"prompt=select_account", false,
		},
		// already forced
		{
			"https://login.microsoftonline.com/common/oauth2/v2.0/authorize?prompt=login",
			"prompt=login", false,
		},
		// unrelated hop (asset, api, ...)
		{
			"https://login.microsoftonline.com/common/handlers/foo.js?v=2",
			"", false,
		},
		// SAML / WS-Fed equivalent
		{
			"https://login.microsoftonline.com/tenant/saml2?SAMLRequest=x",
			"wfresh=0", true,
		},
		{
			"https://login.microsoftonline.com/tenant/wsfed?wa=wsignin1.0",
			"wfresh=0", true,
		},
	}
	for _, c := range cases {
		u, err := url.Parse(c.in)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		before := u.RawQuery
		got := p.forceLoginPrompt(u)
		if got != c.modified {
			t.Errorf("forceLoginPrompt(%q) modified=%v, want %v", c.in, got, c.modified)
			continue
		}
		if !c.modified {
			if u.RawQuery != before {
				t.Errorf("forceLoginPrompt(%q) changed the query but reported no change", c.in)
			}
			continue
		}
		q := u.Query()
		switch {
		case q.Get("prompt") == "login":
			// ok
		case q.Get("wfresh") == "0":
			// ok
		default:
			t.Errorf("forceLoginPrompt(%q) -> %q, want %s", c.in, u.RawQuery, c.want)
		}
		// the rest of the URL must survive intact
		if q.Get("client_id") == "" && q.Get("wa") == "" && q.Get("SAMLRequest") == "" {
			t.Errorf("forceLoginPrompt(%q) dropped the original params: %q", c.in, u.String())
		}
	}
}

func TestForceLoginPromptDisabledAndNil(t *testing.T) {
	off := newForceProxy(false)
	u, _ := url.Parse("https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=abc")
	if off.forceLoginPrompt(u) {
		t.Error("disabled force_login must not touch the URL")
	}
	if u.Query().Get("prompt") != "" {
		t.Error("disabled force_login added a param")
	}
	if newForceProxy(true).forceLoginPrompt(nil) {
		t.Error("nil URL should be a no-op")
	}
}

// The nop-clash guard: forcing must not lose the response_type space encoding
// in a way that breaks Microsoft's parser.
func TestForceLoginPromptKeepsOAuthParams(t *testing.T) {
	p := newForceProxy(true)
	raw := "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=4765445b&redirect_uri=https%3A%2F%2Fwww.office.com%2Flandingv2&response_type=code%20id_token&scope=openid%20profile&state=abc123&nonce=n1&login_hint=alice%40contoso.com"
	u, _ := url.Parse(raw)
	if !p.forceLoginPrompt(u) {
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
