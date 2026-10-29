package core

import (
	"net/http"
	"testing"
)

// dumbResponseWriter is handed to goproxy, which type asserts it to
// http.Hijacker and panics with "httpserver does not support hijacking" when that
// assertion fails.
//
// Hijack has NO textual reference anywhere (the assertion happens in vendor
// code), which makes it look like dead code - it was removed once and the
// service crash-looped. Pin it so removing it fails a test instead.
func TestDumbResponseWriterImplementsHijacker(t *testing.T) {
	var rw http.ResponseWriter = dumbResponseWriter{}
	if _, ok := rw.(http.Hijacker); !ok {
		t.Error("dumbResponseWriter must implement http.Hijacker or goproxy panics")
	}
}
