package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// install writes the native-messaging manifests and registers them with every
// browser we know about. Run it once: `evilpro-host install`.
func install(cfg config) response {
	if fileExists(filepath.Join(cfg.ExtensionsDir, "manifest.json")) {
		// remember where the extension lives so the host can find it later
		hj := filepath.Join(exeDir(), "host.json")
		if !fileExists(hj) {
			b, _ := json.MarshalIndent(config{ExtensionsDir: cfg.ExtensionsDir}, "", "  ")
			_ = os.WriteFile(hj, b, 0o644)
		}
	} else {
		return response{Error: "cannot find the extension next to this host (expected manifest.json in " + cfg.ExtensionsDir + ")"}
	}

	id := extID(cfg)
	if id == "" {
		return response{Error: "the extension manifest has no \"key\", so its id is not stable"}
	}
	exe, err := os.Executable()
	if err != nil {
		return response{Error: err.Error()}
	}
	exe, _ = filepath.Abs(exe)

	manifest := map[string]interface{}{
		"name":            hostName,
		"description":     "evil-pro session host: launches a dedicated browser profile",
		"path":            exe,
		"type":            "stdio",
		"allowed_origins": []string{"chrome-extension://" + id + "/"},
	}
	ffManifest := map[string]interface{}{
		"name":               hostName,
		"description":        manifest["description"],
		"path":               exe,
		"type":               "stdio",
		"allowed_extensions": []string{geckoID},
	}

	written, errs := registerManifests(manifest, ffManifest)
	if len(errs) > 0 {
		return response{OK: len(written) > 0, ExtID: id, ExtPath: cfg.ExtensionsDir, Error: fmt.Sprint(errs)}
	}
	return response{OK: true, ExtID: id, ExtPath: cfg.ExtensionsDir, Profiles: written}
}

const (
	hostName = "com.evilpro.host"
	geckoID  = "session-injector@evil-pro.local"
)

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// chromiumDirs are the per-browser native-messaging folders.
func chromiumDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		return []string{
			filepath.Join(base, "Google", "Chrome", "NativeMessagingHosts"),
			filepath.Join(base, "Chromium", "NativeMessagingHosts"),
			filepath.Join(base, "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
			filepath.Join(base, "Microsoft Edge", "NativeMessagingHosts"),
			filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts"),
		}
	default:
		base := filepath.Join(home, ".config")
		return []string{
			filepath.Join(base, "google-chrome", "NativeMessagingHosts"),
			filepath.Join(base, "chromium", "NativeMessagingHosts"),
			filepath.Join(base, "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
			filepath.Join(base, "microsoft-edge", "NativeMessagingHosts"),
			filepath.Join(home, ".mozilla", "native-messaging-hosts"),
		}
	}
}

// windowsRegistryKeys maps a registry key to whether it is a Firefox host entry.
var windowsRegistryKeys = map[string]bool{
	`Software\Google\Chrome\NativeMessagingHosts\` + hostName:               false,
	`Software\Chromium\NativeMessagingHosts\` + hostName:                    false,
	`Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + hostName: false,
	`Software\Microsoft\Edge\NativeMessagingHosts\` + hostName:              false,
	`Software\Mozilla\NativeMessagingHosts\` + hostName:                     true,
}
