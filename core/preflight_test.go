package core

import (
	"net"
	"testing"
)

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

// The Safe Browsing social-engineering renderer fetches with a browser UA from
// Google's general ranges, so range detection (not just UA) is the layer that
// keeps it off the phish. General Google ranges must be covered, and real
// eyeballs on consumer ISPs must not be.
func TestScannerIPGoogleRanges(t *testing.T) {
	for _, ip := range []string{
		"66.249.66.1",  // googlebot crawler range
		"142.250.1.1",  // Google general (was NOT covered before)
		"172.217.1.1",  // Google general (was NOT covered before)
		"216.58.192.1", // Google
		"8.8.8.8",      // Google
	} {
		if !isScannerIP(ip) {
			t.Errorf("Google ip %s should be treated as a scanner", ip)
		}
	}
	for _, ip := range []string{
		"102.88.166.38",   // MTN Nigeria (a real visitor)
		"105.112.127.132", // Airtel Nigeria (a real visitor)
		"207.174.1.232",   // Dynu host
		"203.0.113.7",     // TEST-NET
	} {
		if isScannerIP(ip) {
			t.Errorf("non-Google ip %s should not be treated as a scanner", ip)
		}
	}
	if len(scannerIPNets) < 100 {
		t.Errorf("expected the full Google range set, got %d nets", len(scannerIPNets))
	}
	if isScannerIP("") || isScannerIP("not-an-ip") {
		t.Error("garbage ip should be false")
	}
}

func TestGoogleScannerCIDRsParse(t *testing.T) {
	if len(googleScannerCIDRs) < 100 {
		t.Fatalf("googleScannerCIDRs too small: %d", len(googleScannerCIDRs))
	}
	for _, c := range googleScannerCIDRs {
		if _, _, err := net.ParseCIDR(c); err != nil {
			t.Errorf("bad CIDR %q: %v", c, err)
		}
	}
}

// Microsoft's prefetch beacons must be recognised (so a Content-Type is set and
// mobile Chrome stops downloading them), and nothing else must be.
func TestPrefetchBeaconPath(t *testing.T) {
	for _, p := range []string{"/Prefetch/Prefetch.aspx", "/owa/prefetch.aspx", "/Prefetch/prefetch.aspx", "/foo/PREFETCH.ASPX"} {
		if !prefetchBeaconPath(p) {
			t.Errorf("should be a beacon: %s", p)
		}
	}
	for _, p := range []string{"/", "/kmsi", "/common/js/Login.js", "/ppsecure/post.srf"} {
		if prefetchBeaconPath(p) {
			t.Errorf("should not be a beacon: %s", p)
		}
	}
}
