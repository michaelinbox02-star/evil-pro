package main

import (
	"strings"
	"testing"
)

// The worker only ever forwards; the bot decision, branding and turnstile stay on
// the engine. These pin the parts that would silently break the anti-bot path.
func TestBuildFrontWorkerForwardsUAManuallyAndNeverCaches(t *testing.T) {
	src := buildFrontWorker("evilpro-example", map[string]string{"doc": "https://doc.example.com"})
	for _, want := range []string{
		`redirect: "manual"`,               // keep the upstream 302 instead of following it
		"cacheTtl: 0",                      // no caching of the UA-conditional response
		`cacheEverything: false`,           //
		`req.headers.get("User-Agent")`,    // the client UA, not Cloudflare's own
		`"Cache-Control", "no-store"`,      //
		"Strict-Transport-Security",        //
		`"doc": "https://doc.example.com"`, // no KV: the page map is embedded
	} {
		if !strings.Contains(src, want) {
			t.Errorf("worker source missing %q", want)
		}
	}
	if strings.Contains(src, "if (ua") || strings.Contains(src, "isScanner") {
		t.Error("the worker must not reimplement the bot logic")
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

func TestFrontWorkerNameIsAValidLabel(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"platoveriour.com", "evilpro-platoveriour"},
		{"My-Site.CO.UK", "evilpro-my-site"},
		{"weird_domain!.net", "evilpro-weird-domain"},
		{"", "evilpro-front"},
		{"a.io", "evilpro-a"},
	} {
		got := frontWorkerName(c.in)
		if got != c.want {
			t.Errorf("frontWorkerName(%q) = %q, want %q", c.in, got, c.want)
		}
		if len(got) > 54 {
			t.Errorf("frontWorkerName(%q) too long: %d", c.in, len(got))
		}
	}
}

func TestBaseLabelSuggestion(t *testing.T) {
	if got := baseLabel("platoveriour.com"); got != "platoveriour" {
		t.Errorf("baseLabel = %q", got)
	}
	if got := baseLabel("x.y"); got != "evilpro" {
		t.Errorf("too short label should fall back, got %q", got)
	}
}

func TestFrontLinkShape(t *testing.T) {
	got := frontLinkFor("evilpro-example.example.workers.dev", engineRedirect{ID: 3, Subdomain: "doc"})
	if got != "https://evilpro-example.example.workers.dev/doc" {
		t.Errorf("front link = %q", got)
	}
}
