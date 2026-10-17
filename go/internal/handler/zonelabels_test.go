package handler

import "testing"

func TestZoneLabel(t *testing.T) {
	cases := []struct{ code, lang, want string }{
		{"U_CTR", "de", "Kontrollzonen"},
		{"U_CTR", "en", "Control Zones"},
		{"POPULATEDAREA", "de", "Wohngrundstücke"},
		{"SOMETHING_NEW", "de", "SOMETHING_NEW"}, // unknown codes pass through
	}
	for _, c := range cases {
		if got := ZoneLabel(c.code, c.lang); got != c.want {
			t.Errorf("ZoneLabel(%q, %q) = %q, want %q", c.code, c.lang, got, c.want)
		}
	}
}
