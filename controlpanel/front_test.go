package main

import (
	"strings"
	"testing"
)

// The worker forwards; the bot decision, branding and turnstile stay on the
// engine. These pin the parts that would silently break the anti-bot path.
func TestBuildFrontWorkerForwardsUAManuallyAndNeverCaches(t *testing.T) {
	src := buildFrontWorker("doc", map[string]string{"": "https://doc.example.com"})
	for _, want := range []string{
		`redirect: "manual"`,            // keep the upstream 302 instead of following it
		"cacheTtl: 0",                   // no caching of the UA-conditional response
		`cacheEverything: false`,        //
		`req.headers.get("User-Agent")`, // the client UA, not Cloudflare's own
		`"Cache-Control", "no-store"`,   //
		"Strict-Transport-Security",     //
		`"": "https://doc.example.com"`, // no KV: the page map is embedded
	} {
		if !strings.Contains(src, want) {
			t.Errorf("worker source missing %q", want)
		}
	}
	if strings.Contains(src, "if (ua") || strings.Contains(src, "isScanner") {
		t.Error("the worker must not reimplement the bot logic")
	}
}

// A single-page worker answers on any path (catch-all), so its link is the bare
// host. A worker shared by several pages instead keys them by subdomain.
func TestBuildFrontWorkerCatchAllOnlyWhenSinglePage(t *testing.T) {
	single := buildFrontWorker("doc", map[string]string{"": "https://doc.example.com"})
	if !strings.Contains(single, `PAGES[""] !== undefined`) {
		t.Error("single-page worker must fall back to the catch-all")
	}

	shared := buildFrontWorker("shared", map[string]string{"doc": "https://doc.example.com", "arrival": "https://a.example.com"})
	if strings.Contains(shared, `"":`) {
		t.Error("shared worker should not carry a catch-all")
	}
	for _, k := range []string{`"doc"`, `"arrival"`} {
		if !strings.Contains(shared, k) {
			t.Errorf("shared worker missing page key %s", k)
		}
	}
}

func TestBuildFrontWorkerIsStableAndSorted(t *testing.T) {
	a := buildFrontWorker("w", map[string]string{"b": "https://b.test", "a": "https://a.test"})
	b := buildFrontWorker("w", map[string]string{"a": "https://a.test", "b": "https://b.test"})
	if a != b {
		t.Error("worker source must be deterministic so re-deploys do not churn")
	}
	if strings.Index(a, `"a"`) > strings.Index(a, `"b"`) {
		t.Error("page map should be sorted")
	}
}

// sanitizeLabel must always produce a usable single DNS label.
func TestSanitizeLabel(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Doc", "doc"},
		{"secure-files", "secure-files"},
		{"my_page", "my-page"},
		{"a.b.c", "a-b-c"},
		{"-edge-", "edge"},
		{"weird!!name", "weirdname"},
		{"  spaced  ", "spaced"},
		{"", ""},
		{"!!!", ""},
	} {
		if got := sanitizeLabel(c.in); got != c.want {
			t.Errorf("sanitizeLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := sanitizeLabel(strings.Repeat("a", 80)); len(got) != 54 {
		t.Errorf("long label should be clipped to 54, got %d", len(got))
	}
}

// Per page naming: an explicit label wins, empty falls back to the page's own
// subdomain so the link mirrors the campaign, and nothing leaks the engine name.
func TestWorkerLabelFor(t *testing.T) {
	if got := workerLabelFor(engineRedirect{Subdomain: "doc", FrontWorker: "Secure-Files"}); got != "secure-files" {
		t.Errorf("explicit label: got %q", got)
	}
	if got := workerLabelFor(engineRedirect{Subdomain: "doc"}); got != "doc" {
		t.Errorf("empty label should fall back to the page subdomain, got %q", got)
	}
	if got := workerLabelFor(engineRedirect{Subdomain: "!!!"}); got != "page" {
		t.Errorf("unusable subdomain should fall back to page, got %q", got)
	}
	if got := workerLabelFor(engineRedirect{Subdomain: "doc"}); strings.HasPrefix(got, "evilpro") {
		t.Errorf("per-page label must not carry the engine name: %q", got)
	}
}

// A single-page worker's link is the bare host; a shared worker's link carries the
// page path, so the two never collide.
func TestFrontPublicLink(t *testing.T) {
	pg := engineRedirect{Subdomain: "doc"}
	if got := frontPublicLink("doc", "example", true, pg); got != "https://doc.example.workers.dev" {
		t.Errorf("single-page link = %q", got)
	}
	if got := frontPublicLink("shared", "example", false, pg); got != "https://shared.example.workers.dev/doc" {
		t.Errorf("shared-worker link = %q", got)
	}
}

func TestBaseLabelSuggestion(t *testing.T) {
	if got := baseLabel("platoveriour.com"); got != "platoveriour" {
		t.Errorf("baseLabel = %q", got)
	}
	if got := baseLabel(""); got != "front" {
		t.Errorf("empty domain should fall back, got %q", got)
	}
}

// The legacy single-worker name is only kept to tell the operator what to delete.
func TestFrontWorkerName(t *testing.T) {
	if got := frontWorkerName("platoveriour.com"); got != "evilpro-platoveriour" {
		t.Errorf("legacy name = %q", got)
	}
	if got := frontWorkerName("My-Site.CO.UK"); got != "evilpro-my-site" {
		t.Errorf("legacy name should sanitise, got %q", got)
	}
}
