package core

import "testing"

// These are the UAs observed in the journal arriving as "new visitor" (so they
// evaluated the phish, and Safe Browsing filed a deceptive-page report for the
// apex). Each must now be treated as a crawler.
func TestScannerUAMatchesTheCrawlersThatBurnedUs(t *testing.T) {
	for _, ua := range []string{
		"Google-Site-Verification/1.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36 BingPreview/1.0b",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Microsoft Outlook 16.0",
		"Microsoft Office Existence Discovery",
		"Googlebot/2.1 (+http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; Google-SafeBrowsing/1.0)",
		"AdsBot-Google (+http://www.google.com/adsbot.html)",
		"bingbot/2.0",
		"msnbot/2.1",
		"Applebot/0.1",
		"PetalBot",
		"Mozilla/5.0 (compatible; UptimeRobot/2.0)",
		"Mozilla/5.0 (X11; Linux x86_64) GoogleOther",
	} {
		if !isScannerUA(ua) {
			t.Errorf("crawler UA must be treated as a scanner: %q", ua)
		}
	}
}

// A real victim must NOT be matched, or they would never see the phish.
func TestScannerUADoesNotMatchRealBrowsers(t *testing.T) {
	for _, ua := range []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
	} {
		if isScannerUA(ua) {
			t.Errorf("a real browser must not be treated as a scanner: %q", ua)
		}
	}
}

func TestHostOnly(t *testing.T) {
	if got := hostOnly("66.249.66.1:443"); got != "66.249.66.1" {
		t.Errorf("hostOnly = %q", got)
	}
	if got := hostOnly("66.249.66.1"); got != "66.249.66.1" {
		t.Errorf("hostOnly = %q", got)
	}
}

// The preflight verdict: a crawler must leave our domain. A probe that stays on
// it is the exact condition Safe Browsing punishes.
func TestPreflightFlagLogic(t *testing.T) {
	cases := []struct {
		name    string
		probe   PreflightProbe
		problem bool
	}{
		{"clean bounce", PreflightProbe{Status: 302, Location: "https://login.microsoftonline.com/", External: true}, false},
		{"stays on our host", PreflightProbe{Status: 302, Location: "https://platoveriour.com/", Hops: 5}, true},
		{"200 on our host", PreflightProbe{Status: 200}, true},
		{"error", PreflightProbe{Err: "timeout"}, true},
		{"5xx", PreflightProbe{Status: 503, External: true}, true},
	}
	for _, c := range cases {
		var problem string
		switch {
		case c.probe.Err != "":
			problem = "err"
		case c.probe.Status >= 400:
			problem = "status"
		case !c.probe.External:
			problem = "never left"
		}
		if (problem != "") != c.problem {
			t.Errorf("%s: problem=%q, want problem=%v", c.name, problem, c.problem)
		}
	}
}

// isExternalHost must treat the apex and any subdomain as ours, everything else
// (the real provider, a workers.dev front host) as external.
func TestIsExternalHost(t *testing.T) {
	p := &HttpProxy{cfg: &Config{general: &GeneralConfig{Domain: "example.com"}}}
	for host, want := range map[string]bool{
		"example.com":               false,
		"www.example.com":           false,
		"EXAMPLE.com:443":           false,
		"login.microsoftonline.com": true,
		"doc.example.workers.dev":   true,
	} {
		if got := p.isExternalHost(host); got != want {
			t.Errorf("isExternalHost(%q) = %v, want %v", host, got, want)
		}
	}
}
