package handler

import (
	"strings"
	"testing"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

// Values measured on DiPUL (2026-10-17): zone floors are aerodrome
// elevation + 25/45/100 m; EDDH elevation 16.2 m, EDDF 110.9 m.
func TestCTRMaxAGL(t *testing.T) {
	cases := []struct {
		name          string
		zone          int
		floor, ground float64
		want          float64
	}{
		{"EDDH zone 4, take-off below aerodrome", 4, 116.2, 5.4, 100},
		{"EDDH zone 4, higher terrain", 4, 116.2, 30.3, 82},         // 116.2-33.3=82.9
		{"zone 4, much higher terrain", 4, 116.2, 90, 50},           // max(23.2, 50)
		{"EDDH zone 3, terrain above aerodrome", 3, 61.2, 24.2, 34}, // min(45, 34)
		{"EDDF zone 2, flat", 2, 135.6, 108.6, 24},                  // min(25, 24)
		{"zone 2, terrain far above", 2, 135.6, 140, 0},
		{"zone 1 has no general clearance", 1, 0, 100, 0},
	}
	for _, c := range cases {
		got, _, ok := ctrMaxAGL(c.zone, c.floor, c.ground, 3)
		if c.zone == 1 {
			if ok {
				t.Errorf("%s: ok for zone 1", c.name)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("%s: got %v ok=%v, want %v", c.name, got, ok, c.want)
		}
	}
}

func TestEnrichZones(t *testing.T) {
	ctr := map[string]interface{}{"name": "Hamburg (EDDH) Zone 4", "legalReference": "NfL 2026-1-3960", "externalReference": "DFSXYZ"}
	police := map[string]interface{}{"name": "Polizeikommissariat 14", "legalReference": "§ 21h, Abs. 3 (4.) LuftVO", "externalReference": "zGc2CYA"}
	zones := []api.AffectedArea{{TypeCode: "U_CTR", Areas: []map[string]interface{}{ctr}}, {TypeCode: "POLICE", Areas: []map[string]interface{}{police}}}
	ed := []api.UASZone{
		{ID: "14769_ctr", Name: "Hamburg (EDDH) Zone 4", LowerM: 116.2, LowerRef: "AMSL", LinkDE: "https://dipul.de/de/ctr", LinkEN: "https://dipul.de/en/ctr"},
		{ID: "zGc2CYA", Name: "Polizeikommissariat 14", LowerRef: "AGL", LinkDE: "https://dipul.de/de/behoerden"},
	}
	enrichZones(zones, ed, &api.Elevation{MSL: 5.4, RMSE: 3})

	if ctr[keyCTRMaxAGL] != 100.0 || ctr[keyLinkDE] != "https://dipul.de/de/ctr" {
		t.Errorf("ctr not enriched: %v", ctr)
	}
	if police[keyLinkDE] != "https://dipul.de/de/behoerden" || police[keyCTRMaxAGL] != nil {
		t.Errorf("police enrichment wrong: %v", police)
	}
	r := ZoneRule(ctr)
	if !strings.Contains(r.LocalDE, "max. ca. 100 m über Grund") || r.LinkEN != "https://dipul.de/en/ctr" {
		t.Errorf("rule: %+v", r)
	}
	if r := ZoneRule(police); r.LinkEN != "https://dipul.de/de/behoerden" || r.LocalDE != "" {
		t.Errorf("police rule: %+v", r)
	}

	// Without elevation: links still attached, no local height.
	ctr2 := map[string]interface{}{"name": "Hamburg (EDDH) Zone 4", "legalReference": "NfL 2026-1-3960"}
	enrichZones([]api.AffectedArea{{Areas: []map[string]interface{}{ctr2}}}, ed, nil)
	if ctr2[keyCTRMaxAGL] != nil || ctr2[keyLinkDE] == nil {
		t.Errorf("no-elevation fallback wrong: %v", ctr2)
	}
}
