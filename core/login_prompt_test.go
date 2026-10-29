package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// A backup sitting next to a phishlet must be named in a warning, and must not
// become a phishlet. Before this it matched a loose regex, resolved to the same
// site name as the real file, and the alphabetically later entry silently won.
func TestScanPhishletsDirWarnsAboutStrays(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.yaml", "a.yaml.bak-opt", "b.yaml", "README.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("name: x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, warnings, err := ScanPhishletsDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, f := range files {
		got[f.Site] = true
	}
	if len(files) != 2 || !got["a"] || !got["b"] {
		t.Fatalf("expected exactly the a and b phishlets, got %v", files)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one warning (the backup), got %v", warnings)
	}
	if !strings.Contains(warnings[0], "a.yaml.bak-opt") {
		t.Errorf("warning must name the stray file: %q", warnings[0])
	}
	if strings.Contains(strings.Join(warnings, " "), "README") {
		t.Errorf("a non-yaml file must not warn: %v", warnings)
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Path, f.Site+".yaml") {
			t.Errorf("resolved path does not match the site: %v", f)
		}
	}
}

// The shipped phishlet must load with a duplicate phish_sub warning that names
// the collision, and those warnings must not be fatal.
func TestDuplicatePhishSubsIsReported(t *testing.T) {
	pl, err := NewPhishlet("o365new", "../phishlets/o365new.yaml", nil, &Config{})
	if err != nil {
		t.Fatalf("parse phishlet: %v", err)
	}
	warnings := pl.DuplicatePhishSubs()
	if len(warnings) == 0 {
		t.Fatal("the stock phishlet declares duplicate phish_subs, expected a warning")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "login.microsoftonline.com") {
		t.Errorf("the 'login' collision must be named:\n%s", joined)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "first wins inbound") || !strings.Contains(w, "last wins in body rewrites") {
			t.Errorf("a warning must say which side wins where: %q", w)
		}
	}
}

// A phishlet with distinct phish_subs must stay warning free.
func TestDuplicatePhishSubsClean(t *testing.T) {
	pl := &Phishlet{proxyHosts: []ProxyHost{
		{phish_subdomain: "a", orig_subdomain: "a", domain: "x.test"},
		{phish_subdomain: "b", orig_subdomain: "b", domain: "x.test"},
	}}
	if got := pl.DuplicatePhishSubs(); len(got) != 0 {
		t.Errorf("expected no warnings, got %v", got)
	}
}

// The real repo phishlets directory must load clean through the scanner.
func TestScanRepoPhishletsDir(t *testing.T) {
	files, _, err := ScanPhishletsDir("../phishlets")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no phishlets found")
	}
	for _, f := range files {
		if _, err := NewPhishlet(f.Site, f.Path, nil, &Config{}); err != nil {
			t.Errorf("phishlet %q failed to parse: %v", f.Site, err)
		}
	}
}

func newPromptConfig(g *GeneralConfig) *Config {
	return &Config{general: g, cfg: viper.New()}
}

// login_prompt is the setting; force_login must stay truthful as its alias.
func TestSetLoginPromptMirrorsForceLogin(t *testing.T) {
	c := newPromptConfig(&GeneralConfig{})
	if err := c.SetLoginPrompt(LOGIN_PROMPT_SELECT_ACCOUNT); err != nil {
		t.Fatal(err)
	}
	if c.GetLoginPrompt() != LOGIN_PROMPT_SELECT_ACCOUNT {
		t.Errorf("got %q", c.GetLoginPrompt())
	}
	if !c.general.ForceLogin {
		t.Error("force_login must mirror on for a non-off mode")
	}
	if !c.IsForceLoginEnabled() {
		t.Error("IsForceLoginEnabled must be true for a non-off mode")
	}

	if err := c.SetLoginPrompt(LOGIN_PROMPT_OFF); err != nil {
		t.Fatal(err)
	}
	if c.general.ForceLogin {
		t.Error("force_login must mirror off for off")
	}
	if c.IsForceLoginEnabled() {
		t.Error("IsForceLoginEnabled must be false for off")
	}

	if err := c.SetLoginPrompt("bogus"); err == nil {
		t.Error("a bogus mode must error")
	}

	// legacy alias
	c.SetForceLogin(true)
	if c.GetLoginPrompt() != LOGIN_PROMPT_LOGIN {
		t.Errorf("SetForceLogin(true) -> %q, want login", c.GetLoginPrompt())
	}
	c.SetForceLogin(false)
	if c.GetLoginPrompt() != LOGIN_PROMPT_OFF {
		t.Errorf("SetForceLogin(false) -> %q, want off", c.GetLoginPrompt())
	}
}

// A legacy config that only carries force_login keeps its meaning.
func TestGetLoginPromptFallsBackToForceLogin(t *testing.T) {
	if got := newPromptConfig(&GeneralConfig{ForceLogin: false}).GetLoginPrompt(); got != LOGIN_PROMPT_OFF {
		t.Errorf("force_login false -> %q, want off", got)
	}
	if got := newPromptConfig(&GeneralConfig{ForceLogin: true}).GetLoginPrompt(); got != LOGIN_PROMPT_LOGIN {
		t.Errorf("force_login true -> %q, want login", got)
	}
	if got := newPromptConfig(&GeneralConfig{LoginPrompt: LOGIN_PROMPT_SELECT_ACCOUNT, ForceLogin: false}).GetLoginPrompt(); got != LOGIN_PROMPT_SELECT_ACCOUNT {
		t.Errorf("explicit login_prompt must win: %q", got)
	}
}

// The default, when nothing is configured, is login.
func TestDefaultLoginPromptIsLogin(t *testing.T) {
	c := newPromptConfig(&GeneralConfig{LoginPrompt: LOGIN_PROMPT_LOGIN})
	if got := c.GetLoginPrompt(); got != LOGIN_PROMPT_LOGIN {
		t.Errorf("default = %q, want login", got)
	}
	if !c.IsForceLoginEnabled() {
		t.Error("the default must keep prompting")
	}
}
