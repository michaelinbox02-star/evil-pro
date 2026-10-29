package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// engineBase returns the base URL of a profile's engine API.
func (a *App) engineBase(p *Profile) string {
	if base := strings.TrimRight(p.EngineURL, "/"); base != "" {
		return base
	}
	return "http://" + net.JoinHostPort(p.Host, "5000")
}

// engineCall performs a server-side call against the engine API of a profile,
// using the same credentials and TLS tolerance as the request proxy. This is what
// lets the panel build the front Worker from engine state without a browser hop.
func (a *App) engineCall(p *Profile, method, path string, body interface{}, out interface{}) (int, error) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	do := func(pr *Profile) (*http.Response, error) {
		req, err := http.NewRequest(method, a.engineBase(pr)+"/api/v1/"+path, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if pr.DashboardUser != "" || pr.DashboardPass != "" {
			req.SetBasicAuth(pr.DashboardUser, pr.DashboardPass)
		}
		return client.Do(req)
	}

	resp, err := do(p)
	if err != nil {
		return 0, err
	}
	// A 401 usually means the stored credentials are stale (adopted server).
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		a.syncCredsFromServer(p)
		if np := a.store.Get(p.ID); np != nil {
			p = np
			resp, err = do(p)
			if err != nil {
				return 0, err
			}
		}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode, nil
}

// engineRedirect is the subset of the engine's redirect DTO the front needs.
type engineRedirect struct {
	ID        int    `json:"id"`
	Subdomain string `json:"subdomain"`
	Host      string `json:"host"`
	TargetURL string `json:"target_url"`
	FrontOn   bool   `json:"front_on"`
	FrontURL  string `json:"front_url"`
	// FrontWorker is the engine's effective workers.dev label for this page.
	FrontWorker string `json:"front_worker"`
	Turnstile   bool   `json:"turnstile"`
}

func (a *App) engineRedirects(p *Profile) ([]engineRedirect, error) {
	var out struct {
		Redirects []engineRedirect `json:"redirects"`
	}
	code, err := a.engineCall(p, "GET", "redirects", nil, &out)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, err
	}
	return out.Redirects, nil
}
