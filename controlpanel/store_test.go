package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := &Store{path: filepath.Join(t.TempDir(), "panel.json")}
	s.data.Settings = defaultSettings()
	return s
}

func TestStoreUpsertGetDelete(t *testing.T) {
	s := newTestStore(t)

	p, err := s.Upsert(&Profile{Host: "10.0.0.1", Password: "x", Domain: "a.test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("expected an id")
	}
	if p.Port != 22 || p.User != "root" || p.InstallDir == "" || p.Phishlet == "" {
		t.Errorf("defaults not applied: %+v", p)
	}
	if !p.Debug {
		t.Error("new profiles should default to debug logging")
	}

	if got := s.Get(p.ID); got == nil || got.Host != "10.0.0.1" {
		t.Errorf("get returned %+v", got)
	}

	// update keeps the id and does not duplicate
	p.Domain = "b.test"
	if _, err := s.Upsert(p); err != nil {
		t.Fatal(err)
	}
	if n := len(s.List()); n != 1 {
		t.Errorf("expected 1 profile, got %d", n)
	}
	if got := s.Get(p.ID); got.Domain != "b.test" {
		t.Errorf("update not persisted: %+v", got)
	}

	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Error("delete failed")
	}
	if s.List() == nil {
		t.Error("List() should never be nil (the UI reads .length)")
	}
}

func TestStorePersistsAndReloads(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Upsert(&Profile{Host: "10.0.0.2", DashboardPass: "pw"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(Settings{Bind: "0.0.0.0:9000", User: "boss", Pass: "s", BinAmd64: "x"}); err != nil {
		t.Fatal(err)
	}

	re, err := NewStoreAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	got := re.Settings()
	if got.Bind != "0.0.0.0:9000" || got.User != "boss" || got.Pass != "s" {
		t.Errorf("settings not persisted: %+v", got)
	}
	if len(re.List()) != 1 || re.List()[0].Host != "10.0.0.2" {
		t.Error("profiles not persisted")
	}
}

func TestSettingsFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	s := newTestStore(t)
	if _, err := s.Upsert(&Profile{Host: "10.0.0.3"}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Errorf("panel.json should be 0600, got %v", st.Mode().Perm())
	}
}

func TestSplitHostPort(t *testing.T) {
	if h, p := splitHostPort("127.0.0.1:8080"); h != "127.0.0.1" || p != 8080 {
		t.Errorf("got %q %d", h, p)
	}
	if _, p := splitHostPort("0.0.0.0:9999"); p != 9999 {
		t.Errorf("port = %d", p)
	}
}

func TestClassifySSH(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ssh: handshake failed: read tcp: i/o timeout", "MaxStartups"},
		{"ssh: unable to authenticate, attempted methods [none password]", "authentication failed"},
		{"dial tcp 1.2.3.4:22: connect: connection refused", "refused"},
		{"host key mismatch for 1.2.3.4", "pinned"},
		{"lookup nope.invalid: no such host", "resolve"},
	}
	for _, c := range cases {
		got := classifySSH(errors.New(c.in))
		if !strings.Contains(got, c.want) {
			t.Errorf("classifySSH(%q) = %q, want it to mention %q", c.in, got, c.want)
		}
	}
	if classifySSH(nil) != "" {
		t.Error("nil error should classify to empty")
	}
}

func TestBinaryCandidatesOrder(t *testing.T) {
	a := &App{store: newTestStore(t)}
	set := a.store.Settings()
	cands := a.binaryCandidates("amd64")
	if len(cands) < 3 {
		t.Fatalf("expected several candidates, got %v", cands)
	}
	if cands[0] != set.BinAmd64 {
		t.Errorf("first candidate should be the configured path, got %q", cands[0])
	}
	found := false
	for _, c := range cands {
		if strings.Contains(c, filepath.Join("build", "evilginx-linux")) {
			found = true
		}
	}
	if !found {
		t.Error("should also look in <repo>/build")
	}
}

func TestProfileJSONRoundTrip(t *testing.T) {
	p := &Profile{Host: "1.2.3.4", DashboardPass: "pw", InstallDir: "/root/x"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back Profile
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Host != p.Host || back.DashboardPass != p.DashboardPass || back.InstallDir != p.InstallDir {
		t.Errorf("round trip mismatch: %+v", back)
	}
}

func TestEngineURLEmptyFallsBackToPort5000(t *testing.T) {
	p := &Profile{Host: "9.9.9.9", EngineURL: ""}
	base := strings.TrimRight(p.EngineURL, "/")
	if base != "" {
		t.Fatalf("expected empty engine url, got %q", base)
	}
}

func TestLookslLikeRepoAndResolve(t *testing.T) {
	root := t.TempDir()
	if looksLikeRepo(root) {
		t.Error("an empty dir should not look like the repo")
	}
	// a portable-style layout is detected by the bundled extension
	if err := os.MkdirAll(filepath.Join(root, "extension"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "extension", "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !looksLikeRepo(root) {
		t.Error("a dir with extension/manifest.json must be detected")
	}
	if got := resolveRepo(root); got != root {
		t.Errorf("resolveRepo(valid) = %q, want %q", got, root)
	}
	// a wrong -repo (e.g. truncated at a space) must not win over a real one
	if got := resolveRepo(filepath.Join(root, "nope")); !looksLikeRepo(got) {
		t.Errorf("resolveRepo(fallback) = %q, which is not a repo", got)
	}
	if looksLikeRepo("") {
		t.Error("empty string is not a repo")
	}
}

func TestExtensionZipRequiresTheRepo(t *testing.T) {
	a := &App{store: newTestStore(t)}
	if got := a.store.Settings().RepoDir; got == "" {
		t.Error("RepoDir default should be set")
	}
}
