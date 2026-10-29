package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cfAPI = "https://api.cloudflare.com/client/v4"

var cfHTTP = &http.Client{Timeout: 20 * time.Second}

type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int
		Message string
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

// cfAPIError keeps the HTTP status alongside Cloudflare's own error, so callers
// can turn a 403 into "add this exact permission" instead of a vague failure.
type cfAPIError struct {
	Status  int
	Code    int
	Message string
}

func (e *cfAPIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("cloudflare: %d %s", e.Code, e.Message)
	}
	return fmt.Sprintf("cloudflare: HTTP %d", e.Status)
}

// cfPermHint rewrites a permission failure into the exact token permission to add.
func cfPermHint(err error, permission string) error {
	var ae *cfAPIError
	if errors.As(err, &ae) && (ae.Status == 401 || ae.Status == 403) {
		return fmt.Errorf("the Cloudflare API token is missing a permission: add \"%s\" (Account scoped) to the token and try again. Cloudflare said: %s",
			permission, ae.Message)
	}
	return err
}

func cfDo(token, method, path string, body interface{}) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, cfAPI+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := cfHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env cfEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &cfAPIError{Status: resp.StatusCode}
	}
	if !env.Success {
		first := cfAPIError{Status: resp.StatusCode}
		if len(env.Errors) > 0 {
			first.Code = env.Errors[0].Code
			first.Message = env.Errors[0].Message
		}
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			msgs = append(msgs, fmt.Sprintf("%d %s", e.Code, e.Message))
		}
		if len(msgs) > 0 {
			first.Message = strings.Join(msgs, "; ")
		}
		return nil, &first
	}
	return env.Result, nil
}

// cfVerify checks that the API token is usable.
//
// /user/tokens/verify only works for user-owned tokens: an account-owned token
// (which is what you get when the token is scoped to an account) answers
// "Invalid API Token" there while working perfectly for zones and Workers. So
// fall back to a zone lookup before calling a good token invalid.
func cfVerify(token string) error {
	if token == "" {
		return fmt.Errorf("cloudflare: empty token")
	}
	if _, err := cfDo(token, "GET", "/user/tokens/verify", nil); err == nil {
		return nil
	}
	raw, err := cfDo(token, "GET", "/zones?per_page=1", nil)
	if err != nil {
		return cfPermHint(err, "Zone:Read")
	}
	var zones []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &zones); err != nil {
		return err
	}
	if len(zones) == 0 {
		return fmt.Errorf("cloudflare: token is valid but sees no zones (add Zone:Read for the account)")
	}
	return nil
}

// cfZoneID finds the zone that owns domain (or its closest parent).
func cfZoneID(token, domain string) (string, error) {
	labels := strings.Split(strings.Trim(domain, "."), ".")
	for i := 0; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		raw, err := cfDo(token, "GET", "/zones?name="+url.QueryEscape(candidate), nil)
		if err != nil {
			return "", err
		}
		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &zones); err != nil {
			return "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("cloudflare: no zone found for %s (is the domain on Cloudflare?)", domain)
}

type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// cfEnsureA creates or updates a grey-cloud (DNS only) A record.
func cfEnsureA(token, zoneID, fqdn, ip string) error {
	want := strings.ToLower(fqdn)
	raw, err := cfDo(token, "GET", "/zones/"+zoneID+"/dns_records?type=A&name="+url.QueryEscape(fqdn), nil)
	if err != nil {
		return err
	}
	var recs []cfRecord
	if err := json.Unmarshal(raw, &recs); err != nil {
		return err
	}
	payload := map[string]interface{}{
		"type": "A", "name": fqdn, "content": ip, "ttl": 1, "proxied": false,
	}
	for _, r := range recs {
		if strings.ToLower(r.Name) == want {
			if r.Content == ip && !r.Proxied {
				return nil
			}
			_, err := cfDo(token, "PUT", "/zones/"+zoneID+"/dns_records/"+r.ID, payload)
			return err
		}
	}
	_, err = cfDo(token, "POST", "/zones/"+zoneID+"/dns_records", payload)
	return err
}

// cfPublicIP reads the server's own public IPv4 from Cloudflare's trace endpoint
// as seen from the target VPS (run remotely, not from the panel host).
const cfTraceCmd = "curl -s --max-time 10 https://www.cloudflare.com/cdn-cgi/trace | awk -F= '/^ip=/{print $2}'"
