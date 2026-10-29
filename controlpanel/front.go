package main

import (
	"fmt"
	"net/http"
	"strings"
)

// The Cloudflare front, panel side.
//
// The Control Panel owns the Cloudflare calls because the API token already
// lives here (profile Cloudflare tab) and the link surfaces (redirect list,
// hand-off) are here too. The engine owns the per-page switch and the stored
// link, so nothing about the plain subdomain links changes.

type frontLink struct {
	ID        int    `json:"id"`
	Subdomain string `json:"subdomain"`
	URL       string `json:"url"`
	Target    string `json:"target_url"`
	Turnstile bool   `json:"turnstile"`
}

// frontLinkFor builds a page's public link from the worker's full public host
// (worker + workers.dev subdomain). Keep it in step with public_host in the
// deploy response: the .workers.dev suffix belongs here, not in the caller.
func frontLinkFor(publicHost string, r engineRedirect) string {
	return "https://" + publicHost + "/" + r.Subdomain
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
		"worker":    frontWorkerName(p.Domain),
	}

	pages, err := a.engineRedirects(p)
	if err != nil {
		out["engine_error"] = err.Error()
	}
	out["pages"] = pages

	if out["token_set"] == true && p.Domain != "" {
		if acct, err := cfAccountIDFromZone(p.CFToken, p.Domain); err == nil {
			if sub, err := cfWorkersSubdomain(p.CFToken, acct); err == nil {
				out["workers_subdomain"] = sub
				out["public_host"] = out["worker"].(string) + "." + sub + ".workers.dev"
			} else {
				out["workers_error"] = err.Error()
			}
		} else {
			out["workers_error"] = err.Error()
		}
	}
	writeJSON(w, 200, out)
}

// handleFrontDeploy generates, uploads and enables the front Worker, then stores
// each page's public link back on the engine.
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

	// Only pages explicitly switched on are published through the front.
	origin := map[string]string{}
	on := []engineRedirect{}
	for _, pg := range pages {
		if pg.FrontOn && pg.Host != "" && pg.TargetURL != "" {
			origin[pg.Subdomain] = "https://" + pg.Host
			on = append(on, pg)
		}
	}

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
	worker := frontWorkerName(p.Domain)
	publicHost := worker + "." + sub + ".workers.dev"

	src := buildFrontWorker(worker, origin)
	if err := cfPutWorkerScript(p.CFToken, acct, worker, src); err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	if err := cfEnableWorkerRoute(p.CFToken, acct, worker, true); err != nil {
		jsonErr(w, 502, err.Error())
		return
	}

	links := make([]frontLink, 0, len(on))
	saved, failed := 0, 0
	for _, pg := range on {
		url := frontLinkFor(publicHost, pg)
		code, err := a.engineCall(p, "POST",
			fmt.Sprintf("redirects/%d/edit", pg.ID),
			map[string]string{"front_url": url}, nil)
		if err == nil && (code == 200 || code == 204) {
			saved++
		} else {
			failed++
		}
		links = append(links, frontLink{ID: pg.ID, Subdomain: pg.Subdomain, URL: url, Target: pg.TargetURL, Turnstile: pg.Turnstile})
	}

	// A Turnstile-gated page needs the new host on the widget's allowed domains,
	// otherwise the widget renders but never passes and the visitor is parked.
	// Best effort: a token without Turnstile:Edit still gets a working front.
	res := map[string]interface{}{
		"status":        "deployed",
		"worker":        worker,
		"public_host":   worker + "." + sub + ".workers.dev",
		"pages":         len(on),
		"links":         links,
		"links_saved":   saved,
		"links_failed":  failed,
		"skipped":       len(pages) - len(on),
		"cache_policy":  "no-store on the outer response, cf.cacheTtl 0 on the subrequest",
		"ua_forwarded":  true,
		"workers_scope": sub + ".workers.dev",
	}
	tsGated := 0
	for _, pg := range on {
		if pg.Turnstile {
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
			if changed, err := cfAllowTurnstileHost(p.CFToken, acct, key, publicHost); err != nil {
				res["turnstile_warning"] = err.Error()
			} else if changed {
				res["turnstile_host_added"] = publicHost
			}
		} else {
			res["turnstile_warning"] = fmt.Sprintf("%d fronted page(s) use Turnstile but the engine has no turnstile site key set", tsGated)
		}
	}
	writeJSON(w, 200, res)
}

// baseLabel is a DNS-safe suggestion for the account's workers.dev subdomain.
func baseLabel(domain string) string {
	l := strings.ToLower(strings.TrimSpace(domain))
	if i := strings.Index(l, "."); i > 0 {
		l = l[:i]
	}
	var b strings.Builder
	for _, c := range l {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) < 3 {
		return "evilpro"
	}
	return s
}
