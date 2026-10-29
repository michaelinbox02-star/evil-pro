package core

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Preflight: a self-check the operator runs before mailing a campaign, so the
// crawler-facing behaviour is proven rather than assumed.
//
// Rationale: Safe Browsing files "deceptive pages" when a crawler ends up on our
// phish. Every entry point is therefore probed twice - once as a crawler (which
// must leave our infrastructure) and once as a browser (which must stay on the
// phish) - and anything a reputation crawler would judge is reported.

const (
	preflightScannerUA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	preflightBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// PreflightProbe is one URL requested with one User-Agent.
type PreflightProbe struct {
	Status   int    `json:"status"`
	Location string `json:"location,omitempty"`
	Hops     int    `json:"hops"`
	External bool   `json:"external"`
	Err      string `json:"err,omitempty"`
}

// PreflightCheck is a single entry point with both probes and a verdict.
type PreflightCheck struct {
	Kind    string         `json:"kind"` // lure | redirect | front | apex
	Label   string         `json:"label"`
	URL     string         `json:"url"`
	Scanner PreflightProbe `json:"scanner"`
	Browser PreflightProbe `json:"browser"`
	Problem string         `json:"problem,omitempty"`
}

type PreflightReport struct {
	BaseDomain string           `json:"base_domain"`
	Checks     []PreflightCheck `json:"checks"`
	Problems   []string         `json:"problems"`
	OK         bool             `json:"ok"`
	Note       string           `json:"note"`
}

func preflightClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// isExternalHost reports whether host is outside our own domain, i.e. the genuine
// site rather than our phish.
func (p *HttpProxy) isExternalHost(host string) bool {
	base := strings.ToLower(p.cfg.GetBaseDomain())
	host = strings.ToLower(hostOnly(host))
	if base == "" || host == "" {
		return true
	}
	return host != base && !strings.HasSuffix(host, "."+base)
}

// probe requests target with ua, following redirects by hand so every hop is
// visible. It stops early once a hop leaves our domain.
func (p *HttpProxy) probe(target string, ua string, maxHops int) PreflightProbe {
	pr := PreflightProbe{}
	client := preflightClient()
	cur := target
	for hop := 0; hop <= maxHops; hop++ {
		req, err := http.NewRequest("GET", cur, nil)
		if err != nil {
			pr.Err = err.Error()
			return pr
		}
		req.Header.Set("User-Agent", ua)
		resp, err := client.Do(req)
		if err != nil {
			pr.Err = err.Error()
			return pr
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		pr.Status = resp.StatusCode
		pr.Location = resp.Header.Get("Location")
		pr.Hops = hop
		if u, err := url.Parse(cur); err == nil && p.isExternalHost(u.Host) {
			pr.External = true
			return pr
		}
		if pr.Location == "" || resp.StatusCode < 300 || resp.StatusCode > 399 {
			return pr
		}
		next, err := resp.Request.URL.Parse(pr.Location)
		if err != nil {
			return pr
		}
		cur = next.String()
	}
	return pr
}

// Preflight probes every public entry point and reports what a crawler would see.
func (p *HttpProxy) Preflight() PreflightReport {
	base := p.cfg.GetBaseDomain()
	rep := PreflightReport{
		BaseDomain: base,
		Note:       "scanner = a crawler must leave our domain; browser = a victim must stay on our phish",
	}

	add := func(kind, label, u string) {
		if u == "" {
			return
		}
		chk := PreflightCheck{Kind: kind, Label: label, URL: u}
		chk.Scanner = p.probe(u, preflightScannerUA, 5)
		chk.Browser = p.probe(u, preflightBrowserUA, 3)
		rep.Checks = append(rep.Checks, chk)
	}

	for _, l := range p.cfg.GetLures() {
		host := l.Hostname
		if host == "" {
			if pl, err := p.cfg.GetPhishlet(l.Phishlet); err == nil {
				host = pl.GetLandingPhishHost()
			}
		}
		if host != "" {
			add("lure", "lure #"+l.Id+" ("+l.Phishlet+")", "https://"+host+l.Path)
		}
	}
	for i, rp := range p.cfg.GetRedirectPages() {
		if rp.Subdomain == "" {
			continue
		}
		id := strconv.Itoa(i)
		add("redirect", "redirect page #"+id+" ("+rp.Subdomain+")", "https://"+rp.Subdomain+"."+base+"/")
		// Only a front that is actually switched on is a live entry point. An off
		// page can still carry a FrontURL written by an earlier deploy, and that
		// stale link must not be reported as a problem.
		if p.cfg.FrontEnabled(rp) && rp.FrontURL != "" {
			add("front", "front link for #"+id+" ("+rp.Subdomain+")", rp.FrontURL)
		}
	}
	if base != "" {
		// the apex, over both schemes: this is the URL Safe Browsing sampled
		add("apex", "apex http", "http://"+base+"/")
		add("apex", "apex https", "https://"+base+"/")
	}

	for i := range rep.Checks {
		chk := &rep.Checks[i]
		switch {
		case chk.Scanner.Err != "":
			chk.Problem = "scanner probe failed: " + chk.Scanner.Err
		case chk.Scanner.Status >= 400:
			chk.Problem = "scanner got " + strconv.Itoa(chk.Scanner.Status)
		case !chk.Scanner.External:
			chk.Problem = "scanner never left our domain - the crawler evaluates our phish"
		}
		if chk.Browser.Err != "" {
			if chk.Problem == "" {
				chk.Problem = "browser probe failed: " + chk.Browser.Err
			}
		} else if chk.Browser.Status >= 400 {
			chk.Problem += " (browser got " + strconv.Itoa(chk.Browser.Status) + ")"
		}
		if chk.Problem != "" {
			rep.Problems = append(rep.Problems, chk.Label+": "+chk.Problem)
		}
	}
	rep.OK = len(rep.Problems) == 0
	return rep
}
