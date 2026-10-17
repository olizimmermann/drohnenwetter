package handler

import (
	"reflect"
	"strings"
	"testing"
)

// Areas as DiPUL actually returns them (values taken from live responses).
func TestZoneRule(t *testing.T) {
	cases := []struct {
		name     string
		area     map[string]interface{}
		wantDE   string // substring of the rule
		wantZone int
		wantH    string // substring of the CTR height rule
	}{
		{"CTR zone 1", map[string]interface{}{"legalReference": "NfL 2026-1-3960", "name": "Frankfurt Main (EDDF) Zone 1"},
			"Zonen 2 bis 4", 1, "individuelle"},
		{"CTR zone 2", map[string]interface{}{"legalReference": "NfL 2026-1-3960", "name": "Hamburg (EDDH) Zone 2"},
			"Zonen 2 bis 4", 2, "25 m"},
		{"CTR zone 4", map[string]interface{}{"legalReference": "NfL 2026-1-3960", "name": "München (EDDM) Zone 4"},
			"Zonen 2 bis 4", 4, "50 m über Grund"},
		{"CTR other NfL", map[string]interface{}{"legalReference": "NfL 2024-1-3081", "name": "Linz"},
			"Flugverkehrskontrollfreigabe", 0, ""},
		{"police", map[string]interface{}{"legalReference": "§ 21h, Abs. 3 (4.) LuftVO"}, "100 m seitlich", 0, ""},
		{"residential", map[string]interface{}{"legalReference": "§ 21h, Abs. 3 (7.) LuftVO"}, "0,25 kg", 0, ""},
		{"hospital", map[string]interface{}{"legalReference": "§ 21h, Abs. 3 (10.) LuftVO"}, "Betreibers", 0, ""},
		{"restricted", map[string]interface{}{"legalReference": "§ 17, Abs. 1 LuftVO"}, "Beschränkungen", 0, ""},
	}
	for _, c := range cases {
		got := ZoneRule(c.area)
		if !got.HasRule || !strings.Contains(got.DE, c.wantDE) || got.EN == "" {
			t.Errorf("%s: rule = %+v", c.name, got)
		}
		if got.CTRZone != c.wantZone || !strings.Contains(got.HeightDE, c.wantH) {
			t.Errorf("%s: zone %d height %q", c.name, got.CTRZone, got.HeightDE)
		}
	}
	if r := ZoneRule(map[string]interface{}{"legalReference": "something else"}); r.HasRule {
		t.Errorf("unknown reference produced a rule: %+v", r)
	}
	if r := ZoneRule(map[string]interface{}{}); r.HasRule {
		t.Errorf("missing reference produced a rule: %+v", r)
	}
}

func TestZoneNames(t *testing.T) {
	got := ZoneNames("Botschaft der Französischen Republik||Botschaft der Republik Polen|| ")
	want := []string{"Botschaft der Französischen Republik", "Botschaft der Republik Polen"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ZoneNames = %q", got)
	}
	if ZoneNames(nil) != nil {
		t.Error("nil name should give no names")
	}
}
