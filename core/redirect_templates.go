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
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Delay       int      `json:"delay"`
	HasSlot     bool     `json:"has_slot"`
	Accent      string   `json:"accent"`
	Logo        string   `json:"logo"`
	Brand       string   `json:"brand"`
	TitleText   string   `json:"title_text"`
	Subtitle    string   `json:"subtitle"`
	Button      string   `json:"button"`
	Messages    []string `json:"messages"`
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

// TemplateDefaults returns the meta for a slug, or an empty struct if unknown.
func TemplateDefaults(slug string) RedirectTemplate {
	m, _, err := loadRedirectTemplate(slug)
	if err != nil {
		return RedirectTemplate{}
	}
	return m
}

// Logos lists the built-in logo marks the UI can offer.
func Logos() []string {
	return []string{"mark", "microsoft", "docusign", "cloud", "file", "ring", "none"}
}

// logoHTML renders a logo selection as self-contained HTML. Values that look like
// a URL become an <img>; anything else is one of the built-in inline marks.
func logoHTML(sel string) string {
	switch strings.ToLower(strings.TrimSpace(sel)) {
	case "", "mark":
		return `<span class="__logo_mark" style="width:34px;height:34px;border-radius:10px;background:var(--accent);display:inline-flex;align-items:center;justify-content:center"><svg viewBox="0 0 24 24" style="width:17px;height:17px;fill:none;stroke:#fff;stroke-width:2.1;stroke-linecap:round;stroke-linejoin:round"><path d="M12 3l7 3v5.2c0 4.3-2.9 8.1-7 9.3-4.1-1.2-7-5-7-9.3V6z"/><path d="M9.2 12.2l2 2 3.6-3.9"/></svg></span>`
	case "microsoft":
		return `<svg viewBox="0 0 108 24" style="height:28px;width:auto" xmlns="http://www.w3.org/2000/svg"><rect x="0" y="1" width="10" height="10" fill="#f25022"/><rect x="11" y="1" width="10" height="10" fill="#7fba00"/><rect x="0" y="12" width="10" height="10" fill="#00a4ef"/><rect x="11" y="12" width="10" height="10" fill="#ffb900"/><text x="27" y="18" font-family="Segoe UI,sans-serif" font-size="17" font-weight="600" fill="#5e5e5e">Microsoft</text></svg>`
	case "docusign":
		return `<span style="font-size:24px;font-weight:700;letter-spacing:-.6px;color:var(--text)">docusign</span>`
	case "cloud":
		return `<span class="__logo_mark" style="width:38px;height:38px;border-radius:11px;background:color-mix(in srgb,var(--accent) 12%,transparent);display:inline-flex;align-items:center;justify-content:center"><svg viewBox="0 0 24 24" style="width:20px;height:20px;fill:none;stroke:var(--accent);stroke-width:1.9;stroke-linecap:round;stroke-linejoin:round"><path d="M7 18a4 4 0 0 1 0-8 5.5 5.5 0 0 1 10.5-1.2A4 4 0 0 1 18 18z"/></svg></span>`
	case "file":
		return `<span class="__logo_mark" style="width:40px;height:40px;border-radius:12px;background:color-mix(in srgb,var(--accent) 10%,transparent);display:inline-flex;align-items:center;justify-content:center"><svg viewBox="0 0 24 24" style="width:20px;height:20px;fill:none;stroke:var(--accent);stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg></span>`
	case "ring", "none":
		return `<span></span>`
	default:
		if strings.HasPrefix(sel, "http://") || strings.HasPrefix(sel, "https://") {
			return `<img src="` + htmlAttrEscape(sel) + `" alt="" style="height:30px;width:auto">`
		}
		return `<span>` + htmlAttrEscape(sel) + `</span>`
	}
}

func sanitizeHex(s, def string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if s[0] != '#' {
		s = "#" + s
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return def
		}
	}
	if len(s) != 4 && len(s) != 7 {
		return def
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
