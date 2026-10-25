package main

import (
	"bytes"
	"encoding/json"
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
	Success  bool              `json:"success"`
	Errors   []struct{ Code int; Message string } `json:"errors"`
	Result   json.RawMessage   `json:"result"`
	ResultInfo struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
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
		return nil, fmt.Errorf("cloudflare: unexpected response (%d)", resp.StatusCode)
	}
	if !env.Success {
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			msgs = append(msgs, fmt.Sprintf("%d %s", e.Code, e.Message))
		}
		if len(msgs) == 0 {
			msgs = append(msgs, fmt.Sprintf("HTTP %d", resp.StatusCode))
		}
		return nil, fmt.Errorf("cloudflare: %s", strings.Join(msgs, "; "))
	}
	return env.Result, nil
}

// cfVerify checks that the API token is valid.
func cfVerify(token string) error {
	if token == "" {
		return fmt.Errorf("cloudflare: empty token")
	}
	_, err := cfDo(token, "GET", "/user/tokens/verify", nil)
	return err
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
