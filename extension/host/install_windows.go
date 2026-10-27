//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// registerManifests writes the manifest files and points each browser registry
// key at the right one. Returns the paths written.
func registerManifests(chrome, firefox map[string]interface{}) ([]string, []error) {
	var written []string
	var errs []error
	dir := exeDir()
	for key, isFF := range windowsRegistryKeys {
		manifest := chrome
		suffix := "chrome"
		if isFF {
			manifest = firefox
			suffix = "firefox"
		}
		path := filepath.Join(dir, hostName+"."+suffix+".json")
		if err := writeJSON(path, manifest); err != nil {
			errs = append(errs, fmt.Errorf("write %s: %v", path, err))
			continue
		}
		if out, err := exec.Command("reg", "add", `HKCU\`+key, "/ve", "/d", path, "/f").CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("reg add %s: %v %s", key, err, string(out)))
			continue
		}
		written = append(written, `HKCU\`+key)
	}
	if len(written) == 0 {
		errs = append(errs, fmt.Errorf("nothing registered"))
	}
	return written, errs
}

var _ = os.Getenv
