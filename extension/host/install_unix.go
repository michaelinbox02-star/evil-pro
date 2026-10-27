//go:build !windows

package main

import (
	"fmt"
	"path/filepath"
)

// registerManifests drops a manifest into every browser's native-messaging folder.
func registerManifests(chrome, firefox map[string]interface{}) ([]string, []error) {
	var written []string
	var errs []error
	for _, dir := range chromiumDirs() {
		manifest := chrome
		if filepath.Base(dir) == "native-messaging-hosts" {
			manifest = firefox
		}
		path := filepath.Join(dir, hostName+".json")
		if err := writeJSON(path, manifest); err != nil {
			errs = append(errs, fmt.Errorf("write %s: %v", path, err))
			continue
		}
		written = append(written, path)
	}
	return written, errs
}
