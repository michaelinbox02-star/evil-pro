package core

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ServeRedirectPage reads the redirect page HTML file, injects the redirect
// logic (with optional Cloudflare Turnstile gate), and writes the response.
// siteKey is the Cloudflare Turnstile site key; pass "" to disable the gate.
func ServeRedirectPage(w http.ResponseWriter, r *http.Request, rp *RedirectPage, redirectsDir string, siteKey string) {
	fpath := filepath.Join(redirectsDir, filepath.Base(rp.File))
	content, err := os.ReadFile(fpath)
	if err != nil {
		http.Error(w, fmt.Sprintf("redirect page file not found: %s", rp.File), http.StatusNotFound)
		return
	}

	html := string(content)

	// If no target URL is set, serve the file as-is — the page handles its own redirect.
	if rp.TargetURL == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
		return
	}

	var injection string

	if rp.Turnstile && siteKey != "" {
		// Client-side Turnstile gate — overlay shown until challenge passes,
		// then JS redirects to the target URL.
		safeKey := htmlAttrEscape(siteKey)
		safeURL := jsStringEscape(rp.TargetURL)
		injection = fmt.Sprintf(`
<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>
<style>
#_ts_overlay{position:fixed;top:0;left:0;width:100%%;height:100%%;background:rgba(0,0,0,0.75);
display:flex;align-items:center;justify-content:center;z-index:2147483647}
</style>
<div id="_ts_overlay">
  <div class="cf-turnstile" data-sitekey="%s" data-callback="_tsPass"></div>
</div>
<script>
function _tsPass(){
  var el=document.getElementById('_ts_overlay');
  if(el)el.remove();
  window.location.replace('%s');
}
</script>`, safeKey, safeURL)
	} else {
		// No gate — immediate redirect via JS (faster than meta-refresh).
		safeURL := jsStringEscape(rp.TargetURL)
		injection = fmt.Sprintf(`<script>window.location.replace('%s');</script>`, safeURL)
	}

	// Inject before </body> when present; otherwise append.
	lower := strings.ToLower(html)
	if idx := strings.LastIndex(lower, "</body>"); idx != -1 {
		html = html[:idx] + injection + html[idx:]
	} else {
		html += injection
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
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
