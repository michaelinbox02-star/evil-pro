package core

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed redirect_templates
var redirectTemplatesFS embed.FS

const redirectTemplatesRoot = "redirect_templates"

// RedirectTemplate describes a bundled redirect-page design.
type RedirectTemplate struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Delay       int    `json:"delay"`
	HasSlot     bool   `json:"has_slot"`
	Accent      string `json:"accent"`
	Logo        string `json:"logo"`
}

// ListRedirectTemplates returns the bundled templates, sorted by category/title.
func ListRedirectTemplates() []RedirectTemplate {
	out := []RedirectTemplate{}
	entries, err := fs.ReadDir(redirectTemplatesFS, redirectTemplatesRoot)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, _, err := loadRedirectTemplate(e.Name())
		if err != nil {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			return out[i].Title < out[j].Title
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func loadRedirectTemplate(slug string) (RedirectTemplate, string, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	slug = strings.ReplaceAll(slug, "/", "")
	if slug == "" {
		return RedirectTemplate{}, "", fmt.Errorf("empty template slug")
	}
	metaRaw, err := redirectTemplatesFS.ReadFile(redirectTemplatesRoot + "/" + slug + "/meta.json")
	if err != nil {
		return RedirectTemplate{}, "", fmt.Errorf("unknown template: %s", slug)
	}
	var meta RedirectTemplate
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return RedirectTemplate{}, "", fmt.Errorf("template %s: bad meta.json", slug)
	}
	meta.Slug = slug
	html, err := redirectTemplatesFS.ReadFile(redirectTemplatesRoot + "/" + slug + "/index.html")
	if err != nil {
		return RedirectTemplate{}, "", fmt.Errorf("template %s: missing index.html", slug)
	}
	meta.HasSlot = strings.Contains(string(html), "__ts_slot__")
	if meta.Delay <= 0 {
		meta.Delay = 3
	}
	return meta, string(html), nil
}

// GetRedirectTemplate returns the metadata and raw HTML for a bundled template.
func GetRedirectTemplate(slug string) (RedirectTemplate, string, error) {
	return loadRedirectTemplate(slug)
}

// WriteRedirectTemplate writes a template (with placeholders intact) to the
// redirects directory and returns the file name it was written as.
func WriteRedirectTemplate(slug, redirectsDir, fileName string) (string, error) {
	_, html, err := loadRedirectTemplate(slug)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".html") {
		fileName += ".html"
	}
	if err := os.MkdirAll(redirectsDir, 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(redirectsDir, fileName), []byte(html), 0644); err != nil {
		return "", err
	}
	return fileName, nil
}
