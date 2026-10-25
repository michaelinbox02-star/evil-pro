package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const cookieName = "ep_cp_session"

type jobRegistry struct {
	mu sync.Mutex
	m  map[string]*Job
}

func newJobRegistry() *jobRegistry { return &jobRegistry{m: map[string]*Job{}} }

func (r *jobRegistry) put(j *Job) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[j.ID] = j
	// prune
	if len(r.m) > 60 {
		var oldest string
		var t int64 = 1 << 62
		for id, jb := range r.m {
			jb.mu.Lock()
			st := jb.Started
			oser := jb.Status
			jb.mu.Unlock()
			_ = oser
			if st < t {
				t = st
				oldest = id
			}
		}
		if oldest != "" {
			delete(r.m, oldest)
		}
	}
}

func (r *jobRegistry) get(id string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[id]
}

func (r *jobRegistry) list() []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0, len(r.m))
	for _, j := range r.m {
		out = append(out, j)
	}
	return out
}

// App is the control panel.
type App struct {
	store *Store
	jobs  *jobRegistry

	user string
	pass string

	mu     sync.Mutex
	tokens map[string]int64

	started time.Time
}

func NewApp(st *Store, user, pass string) *App {
	return &App{store: st, jobs: newJobRegistry(), user: user, pass: pass, tokens: map[string]int64{}, started: time.Now()}
}

// ---------------------------------------------------------------------------
// help
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, status int, msg string) { writeJSON(w, status, map[string]string{"error": msg}) }

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	h := r.RemoteAddr
	if i := strings.LastIndex(h, ":"); i > 0 {
		return h[:i]
	}
	return h
}

func rndHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// auth
// ---------------------------------------------------------------------------

func (a *App) authEnabled() bool { return a.user != "" || a.pass != "" }

func (a *App) tokenValid(t string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.tokens[t]
	if !ok {
		return false
	}
	if time.Now().Unix() > exp {
		delete(a.tokens, t)
		return false
	}
	return true
}

func (a *App) newToken() string {
	t := rndHex(24)
	a.mu.Lock()
	a.tokens[t] = time.Now().Add(24 * time.Hour).Unix()
	a.mu.Unlock()
	return t
}

func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(cookieName); err == nil && a.tokenValid(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		if u, p, ok := r.BasicAuth(); ok && u == a.user && p == a.pass {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if a.user != b.Username || a.pass != b.Password {
		jsonErr(w, 401, "invalid credentials")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: a.newToken(), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 86400})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// server
// ---------------------------------------------------------------------------

func (a *App) buildMux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /login", a.handleLoginPage)
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("GET /static/app.css", a.serveAsset(cpCSS, "text/css; charset=utf-8"))
	mux.HandleFunc("GET /static/app.js", a.serveAsset(cpJS+cpManageJS, "application/javascript; charset=utf-8"))
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		fmt.Fprint(w, cpFavicon)
	})

	h := func(method, path string, fn http.HandlerFunc) { mux.Handle(method+" "+path, a.auth(fn)) }

	h("GET", "/", a.handleIndex)
	h("GET", "/api/state", a.handleState)
	h("GET", "/api/profiles", a.handleProfiles)
	h("POST", "/api/profiles", a.handleProfileSave)
	h("GET", "/api/profiles/{id}", a.handleProfileGet)
	h("DELETE", "/api/profiles/{id}", a.handleProfileDelete)
	h("POST", "/api/profiles/{id}/test", a.handleTest)
	h("GET", "/api/profiles/{id}/status", a.handleStatus)
	h("POST", "/api/profiles/{id}/install", a.handleInstall)
	h("POST", "/api/profiles/{id}/update", a.handleUpdate)
	h("POST", "/api/profiles/{id}/action", a.handleAction)
	h("POST", "/api/profiles/{id}/uninstall", a.handleUninstall)
	h("GET", "/api/profiles/{id}/logs", a.handleLogs)
	h("GET", "/api/profiles/{id}/handoff", a.handleHandoff)
	h("GET", "/api/jobs", a.handleJobs)
	h("GET", "/api/jobs/{id}", a.handleJob)
	h("GET", "/api/extension.zip", a.handleExtensionZip)
	h("POST", "/api/settings", a.handleSettings)
	h("POST", "/api/build", a.handleBuild)
	h("POST", "/api/cf/verify", a.handleCFVerify)

	// engine management proxy — forwards to the selected server's engine API
	h("GET", "/api/profiles/{id}/engine-ping", a.handleEnginePing)
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.Handle(m+" /api/profiles/{id}/engine/{rest...}", a.auth(http.HandlerFunc(a.handleEngineProxy)))
	}

	return mux
}

func (a *App) serveAsset(body, ctype string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprint(w, body)
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, cpIndexHTML)
}

func (a *App) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil && a.tokenValid(c.Value) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, cpLoginHTML)
}

// ---------------------------------------------------------------------------
// extension bundle
// ---------------------------------------------------------------------------

func (a *App) handleExtensionZip(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(a.store.Settings().RepoDir, "extension")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		f, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	_ = zw.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="session-injector-extension.zip"`)
	_, _ = w.Write(buf.Bytes())
}
