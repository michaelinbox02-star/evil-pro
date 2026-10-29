package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// The Cloudflare front, panel side.
//
// The Control Panel owns the Cloudflare calls because the API token already
// lives here (profile Cloudflare tab) and the link surfaces (redirect list,
// hand-off) are here too. The engine owns the per-page switch, the per-page
// workers.dev label and the stored link.
//
// Naming is deliberately per page, not global: each page publishes its own Worker
// under its own label, so a link reads like a hostname ("doc.example.workers.dev")
// and nothing about the engine leaks into it. Several pages may share one label on
// purpose, in which case that single Worker serves each page under its subdomain.

type frontLink struct {
	ID        int    `json:"id"`
	Subdomain string `json:"subdomain"`
	Worker    string `json:"worker"`
	URL       string `json:"url"`
	Target    string `json:"target_url"`
	Turnstile bool   `json:"turnstile"`
}

// sanitizeLabel mirrors core.SanitizeDNSLabel; the engine already sanitises, this
// is the panel's safety net before the label becomes a Worker name.
func sanitizeLabel(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case c == '-' || c == '.' || c == '_' || c == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 54 {
		out = out[:54]
	}
	return strings.Trim(out, "-")
}

// workerLabelFor is the label a page publishes under: the panel trusts the engine,
// and only falls back to the page subdomain if the engine sent nothing.
func workerLabelFor(pg engineRedirect) string {
	if l := sanitizeLabel(pg.FrontWorker); l != "" {
		return l
	}
	if l := sanitizeLabel(pg.Subdomain); l != "" {
		return l
	}
	return "page"
}

// frontPublicLink is the link for one page: the bare worker host when the worker
// serves a single page, otherwise the worker host plus the page's path.
func frontPublicLink(worker, workersSub string, single bool, pg engineRedirect) string {
	url := "https://" + worker + "." + workersSub + ".workers.dev"
	if !single {
		url += "/" + pg.Subdomain
	}
	return url
}

// handleFrontStatus reports what a deploy would do, without changing anything.
func (a *App) handleFrontStatus(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	out := map[string]interface{}{
		"token_set": strings.TrimSpace(p.CFToken) != "",
		"domain":    p.Domain,
		"legacy_worker": func() string {
			if p.Domain == "" {
				return ""
			}
			return "evilpro-" + strings.SplitN(p.Domain, ".", 2)[0]
		}(),
	}

	pages, err := a.engineRedirects(p)
	if err != nil {
		out["engine_error"] = err.Error()
	}
	type pageView struct {
		ID        int    `json:"id"`
		Subdomain string `json:"subdomain"`
		Host      string `json:"host"`
		FrontOn   bool   `json:"front_on"`
		Worker    string `json:"worker"`
		FrontURL  string `json:"front_url"`
		Preview   string `json:"preview"`
		Turnstile bool   `json:"turnstile"`
	}
	views := make([]pageView, 0, len(pages))
	labels := map[string]int{}
	for _, pg := range pages {
		label := workerLabelFor(pg)
		labels[label]++
	}
	for _, pg := range pages {
		label := workerLabelFor(pg)
		v := pageView{ID: pg.ID, Subdomain: pg.Subdomain, Host: pg.Host,
			FrontOn: pg.FrontOn, Worker: label, FrontURL: pg.FrontURL, Turnstile: pg.Turnstile}
		if sub, ok := out["workers_subdomain"].(string); ok && sub != "" {
			v.Preview = frontPublicLink(label, sub, labels[label] == 1, pg)
		}
		views = append(views, v)
	}
	out["pages"] = views
	out["shared_labels"] = func() map[string][]string {
		sh := map[string][]string{}
		for _, pg := range pages {
			l := workerLabelFor(pg)
			if labels[l] > 1 {
				sh[l] = append(sh[l], pg.Subdomain)
			}
		}
		return sh
	}()

	if out["token_set"] == true && p.Domain != "" {
		if acct, err := cfAccountIDFromZone(p.CFToken, p.Domain); err == nil {
			if sub, err := cfWorkersSubdomain(p.CFToken, acct); err == nil {
				out["workers_subdomain"] = sub
				// recompute previews now that the suffix is known
				for i := range views {
					pg := pages[i]
					views[i].Preview = frontPublicLink(workerLabelFor(pg), sub, labels[workerLabelFor(pg)] == 1, pg)
				}
				out["pages"] = views
			} else {
				out["workers_error"] = err.Error()
			}
		} else {
			out["workers_error"] = err.Error()
		}
	}
	writeJSON(w, 200, out)
}

// handleFrontDeploy uploads one Worker per label, enables its route, adds the
// workers.dev host to any Turnstile widget's allowed domains, and stores each
// page's link back on the engine.
func (a *App) handleFrontDeploy(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	if strings.TrimSpace(p.CFToken) == "" {
		jsonErr(w, 400, "no Cloudflare API token on this profile: add one in the Cloudflare tab first")
		return
	}
	if p.Domain == "" {
		jsonErr(w, 400, "profile has no domain")
		return
	}

	pages, err := a.engineRedirects(p)
	if err != nil {
		jsonErr(w, 502, "cannot read redirect pages from the engine: "+err.Error())
		return
	}

	// Group the enabled pages by worker label. Distinct labels -> distinct
	// workers (each serving one page on any path); a shared label -> one worker
	// serving those pages under their subdomains.
	groups := map[string][]engineRedirect{}
	for _, pg := range pages {
		if !pg.FrontOn || pg.Host == "" || pg.TargetURL == "" {
			continue
		}
		label := workerLabelFor(pg)
		groups[label] = append(groups[label], pg)
	}
	labels := make([]string, 0, len(groups))
	for l := range groups {
		labels = append(labels, l)
	}
	sort.Strings(labels)

	acct, err := cfAccountIDFromZone(p.CFToken, p.Domain)
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	sub, err := cfEnsureWorkersSubdomain(p.CFToken, acct, baseLabel(p.Domain))
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	if sub == "" {
		jsonErr(w, 502, "cloudflare returned no workers.dev subdomain for the account")
		return
	}

	links := []frontLink{}
	saved, failed := 0, 0
	hosts := []string{}

	for _, label := range labels {
		pgs := groups[label]
		single := len(pgs) == 1

		origin := map[string]string{}
		for _, pg := range pgs {
			key := pg.Subdomain
			if single {
				key = "" // catch-all: the link is the bare worker host
			}
			origin[key] = "https://" + pg.Host
		}

		if err := cfPutWorkerScript(p.CFToken, acct, label, buildFrontWorker(label, origin)); err != nil {
			jsonErr(w, 502, err.Error())
			return
		}
		if err := cfEnableWorkerRoute(p.CFToken, acct, label, true); err != nil {
			jsonErr(w, 502, err.Error())
			return
		}
		hosts = append(hosts, label+"."+sub+".workers.dev")

		for _, pg := range pgs {
			url := frontPublicLink(label, sub, single, pg)
			code, cerr := a.engineCall(p, "POST",
				fmt.Sprintf("redirects/%d/edit", pg.ID),
				map[string]string{"front_url": url}, nil)
			if cerr == nil && (code == 200 || code == 204) {
				saved++
			} else {
				failed++
			}
			links = append(links, frontLink{ID: pg.ID, Subdomain: pg.Subdomain, Worker: label,
				URL: url, Target: pg.TargetURL, Turnstile: pg.Turnstile})
		}
	}

	// A Turnstile-gated page needs its new host on the widget's allowed domains,
	// otherwise the widget renders but never passes and the visitor is parked.
	// Best effort: a token without Turnstile:Edit still gets a working front.
	res := map[string]interface{}{
		"status":        "deployed",
		"workers":       labels,
		"workers_scope": sub + ".workers.dev",
		"pages":         len(links),
		"links":         links,
		"links_saved":   saved,
		"links_failed":  failed,
		"skipped":       len(pages) - len(links),
		"cache_policy":  "no-store on the outer response, cf.cacheTtl 0 on the subrequest",
		"ua_forwarded":  true,
	}
	tsGated := 0
	for _, l := range links {
		if l.Turnstile {
			tsGated++
		}
	}
	if tsGated > 0 {
		var ec struct {
			General struct {
				TurnstileSiteKey string `json:"turnstile_site_key"`
			} `json:"general"`
		}
		_, _ = a.engineCall(p, "GET", "config", nil, &ec)
		if key := ec.General.TurnstileSiteKey; key != "" {
			added, warned := []string{}, []string{}
			for _, h := range hosts {
				if changed, err := cfAllowTurnstileHost(p.CFToken, acct, key, h); err != nil {
					warned = append(warned, err.Error())
				} else if changed {
					added = append(added, h)
				}
			}
			if len(added) > 0 {
				res["turnstile_hosts_added"] = added
			}
			if len(warned) > 0 {
				res["turnstile_warning"] = strings.Join(warned, "; ")
			}
		} else {
			res["turnstile_warning"] = fmt.Sprintf("%d fronted page(s) use Turnstile but the engine has no turnstile site key set", tsGated)
		}
	}

	// Older deployments published one Worker named evilpro-<domain>. Once nothing
	// points at it, say so instead of silently leaving it behind.
	legacy := "evilpro-" + strings.SplitN(p.Domain, ".", 2)[0]
	inUse := false
	for _, l := range labels {
		if l == legacy {
			inUse = true
		}
	}
	if !inUse {
		res["unused_worker_hint"] = legacy + " is no longer used by any page (delete it in the Cloudflare dashboard if you like)"
	}

	writeJSON(w, 200, res)
}

// baseLabel is a DNS-safe suggestion for the account's workers.dev subdomain.
func baseLabel(domain string) string {
	l := strings.ToLower(strings.TrimSpace(domain))
	if i := strings.Index(l, "."); i > 0 {
		l = l[:i]
	}
	if s := sanitizeLabel(l); s != "" {
		return s
	}
	return "front"
}
