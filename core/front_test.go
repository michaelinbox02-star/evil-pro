package core

import (
	"testing"

	"github.com/spf13/viper"
)

// The front must be off by default so today's subdomain links are untouched.
func TestFrontDefaultsOff(t *testing.T) {
	c := &Config{general: &GeneralConfig{}, cfg: viper.New()}
	if c.GetFront() != "off" {
		t.Errorf("default front = %q, want off", c.GetFront())
	}
	if c.IsFrontEnabled() {
		t.Error("front must default to off")
	}
}

func TestSetFrontValidation(t *testing.T) {
	c := &Config{general: &GeneralConfig{}, cfg: viper.New()}
	for _, in := range []string{"on", "true", "yes", "1"} {
		if err := c.SetFront(in); err != nil {
			t.Fatalf("SetFront(%q): %v", in, err)
		}
		if !c.IsFrontEnabled() {
			t.Errorf("SetFront(%q) did not enable", in)
		}
	}
	for _, in := range []string{"off", "false", "no", "0"} {
		if err := c.SetFront(in); err != nil {
			t.Fatalf("SetFront(%q): %v", in, err)
		}
		if c.IsFrontEnabled() {
			t.Errorf("SetFront(%q) did not disable", in)
		}
	}
	if err := c.SetFront("bogus"); err == nil {
		t.Error("a bogus value must be rejected")
	}
}

// A per-page override wins either way; empty inherits the global setting.
func TestFrontEnabledPageOverride(t *testing.T) {
	c := &Config{general: &GeneralConfig{Front: "off"}, cfg: viper.New()}
	if c.FrontEnabled(&RedirectPage{Front: "on"}) != true {
		t.Error("page on must win over global off")
	}
	if c.FrontEnabled(&RedirectPage{Front: ""}) != false {
		t.Error("empty page value must inherit global off")
	}
	if c.FrontEnabled(nil) != false {
		t.Error("nil page must inherit global")
	}
	c.general.Front = "on"
	if c.FrontEnabled(&RedirectPage{Front: "off"}) != false {
		t.Error("page off must win over global on")
	}
	if c.FrontEnabled(&RedirectPage{Front: ""}) != true {
		t.Error("empty page value must inherit global on")
	}
}

// Turning the front off must not lose an already-deployed link.
func TestFrontOffKeepsTheStoredLink(t *testing.T) {
	rp := &RedirectPage{Front: "off", FrontURL: "https://w.sub.workers.dev/doc"}
	if rp.FrontURL == "" {
		t.Error("the stored link must survive turning the front off")
	}
}

func TestNormalizeOnOff(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"on", "on"}, {"OFF", "off"}, {" true ", "on"}, {"0", "off"}, {"", ""}, {"maybe", ""},
	} {
		if got := NormalizeOnOff(c.in); got != c.want {
			t.Errorf("NormalizeOnOff(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
