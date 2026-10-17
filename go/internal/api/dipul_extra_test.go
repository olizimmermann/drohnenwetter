package api

import (
	"math"
	"testing"
)

// Trimmed from a live ED-318 response for Hamburg Rathaus (2026-10-17).
const edFixture = `{"genericReply":{},"uasZones":{"type":"FeatureCollection","features":[
 {"type":"Feature","id":"14769_ctr","geometry":{"type":"MultiPolygon","layer":{"upper":2500.0,"upperReference":"AMSL","lower":381.233608,"lowerReference":"AMSL","uom":"ft"}},
  "properties":{"identifier":"14769_ctr","name":[{"text":"Hamburg (EDDH) Zone 4","lang":"de"},{"text":"Hamburg (EDDH) zone 4","lang":"en"}],
   "extendedProperties":{"externalReference":"14769_ctr","typeCode":"UAS_KONTROLLZONE","links":[
    {"text":"Kontrollzonen","lang":"de-DE","url":"https://dipul.de/homepage/de/informationen/geografische-gebiete/rechtsgrundlagen/#accordion-1-9"},
    {"text":"Control zones","lang":"en-GB","url":"https://dipul.de/homepage/en/information/geographical-zones/legal-basis/#accordion-1-9"}]}}},
 {"type":"Feature","id":"zGc2CYA","geometry":{"type":"MultiPolygon","layer":{"upper":99999.0,"upperReference":"AGL","lower":0.0,"lowerReference":"AGL","uom":"m"}},
  "properties":{"identifier":"zGc2CYA","name":[{"text":"Polizeikommissariat 14 - Außenstelle Rathaus","lang":"de"}],
   "extendedProperties":{"typeCode":"POLIZEI","links":[{"lang":"de-DE","url":"javascript:alert(1)"}]}}}]}}`

func TestParseUASZones(t *testing.T) {
	zones, err := parseUASZones([]byte(edFixture))
	if err != nil || len(zones) != 2 {
		t.Fatalf("zones=%v err=%v", zones, err)
	}
	ctr := zones[0]
	if ctr.Name != "Hamburg (EDDH) Zone 4" || ctr.LowerRef != "AMSL" || math.Abs(ctr.LowerM-116.2) > 0.05 {
		t.Errorf("ctr = %+v", ctr)
	}
	if ctr.LinkDE == "" || ctr.LinkEN == "" {
		t.Errorf("ctr links missing: %+v", ctr)
	}
	if p := zones[1]; p.ID != "zGc2CYA" || p.LinkDE != "" {
		t.Errorf("non-https link must be dropped: %+v", p)
	}
}
