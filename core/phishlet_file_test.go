package core

import "testing"

// Backups living in the phishlets directory must not be treated as phishlets.
// Before this, "o365new.yaml.bak-opt" resolved to site "o365new" and shadowed the
// real file (the later directory entry wins), so edits silently did nothing.
func TestPhishletSiteFromFile(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"o365new.yaml", "o365new", true},
		{"my.phishlet.yaml", "my.phishlet", true},
		{"o365new.yaml.bak", "", false},
		{"o365new.yaml.bak-opt", "", false},
		{"o365new.yaml.bak-cold", "", false},
		{"o365new.yaml.orig", "", false},
		{"o365new.yml", "", false},
		{"o365new", "", false},
		{".yaml", "", false},
	}
	for _, c := range cases {
		got, ok := PhishletSiteFromFile(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("PhishletSiteFromFile(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
