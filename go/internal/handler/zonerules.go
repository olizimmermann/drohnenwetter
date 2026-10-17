package handler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Plain-language summaries of the rules behind DiPUL zones, keyed by the
// legal reference DiPUL returns per area. Condensed from the official texts:
//   - § 21h Abs. 3 LuftVO (geografische Gebiete)
//   - § 17 LuftVO (Gebiete mit Flugbeschränkungen)
//   - NfL 2026-1-3960, DFS-Allgemeinverfügung UAS in Kontrollzonen, in force
//     since 06.08.2026
// They are orientation only; the legal texts are binding (the UI says so).

type ruleText struct{ DE, EN string }

// paragraph21h holds § 21h Abs. 3 Nr. 1–11 LuftVO.
var paragraph21h = map[int]ruleText{
	1: {"Über und bis 1,5 km um den Flugplatz nur in der speziellen Kategorie oder mit Zustimmung der Luftaufsicht, Flugleitung oder des Flugplatzbetreibers.",
		"Over and within 1.5 km of the airfield only in the specific category or with consent of the aviation authority, airfield operations or operator."},
	2: {"Über und bis 1 km um den Flughafen sowie bis 1 km beiderseits der um 5 km verlängerten Bahnachsen nur in der speziellen Kategorie.",
		"Over and within 1 km of the airport, and within 1 km either side of the runway centrelines extended by 5 km, only in the specific category."},
	3: {"Über und bis 100 m seitlich nur mit ausdrücklicher Zustimmung der zuständigen Stelle oder des Betreibers.",
		"Over and within 100 m laterally only with explicit consent of the responsible body or operator."},
	4: {"Über und bis 100 m seitlich nur mit ausdrücklicher Zustimmung der zuständigen Stelle oder des Betreibers.",
		"Over and within 100 m laterally only with explicit consent of the responsible body or operator."},
	5: {"Über und bis 100 m seitlich erlaubt, wenn die Flughöhe stets kleiner ist als der seitliche Abstand und dieser über 10 m liegt. Sonst nur mit Zustimmung oder in der speziellen Kategorie; Wasserstraßen nur ab 100 m Höhe auf kürzestem Weg queren, ohne Schiffe und Schifffahrtsanlagen zu überfliegen.",
		"Over and within 100 m laterally allowed if the height above ground always stays below the lateral distance and that distance exceeds 10 m. Otherwise only with consent or in the specific category; cross waterways only at 100 m or higher by the shortest route, without overflying vessels or shipping facilities."},
	6: {"Überflug nur mit Zustimmung der Naturschutzbehörde oder nach Landesrecht. Außer in Nationalparks auch ohne Zustimmung, wenn der Flug nicht Sport oder Freizeit dient, über 100 m Höhe stattfindet, den Schutzzweck berücksichtigt und unumgänglich ist.",
		"Overflight only with consent of the nature conservation authority or as state law allows. Except in national parks also without consent if the flight is not for sport or leisure, above 100 m, respects the protection purpose and is unavoidable."},
	7: {"Überflug nur mit Zustimmung des Eigentümers oder Nutzungsberechtigten, mit Drohnen bis 0,25 kg ohne Kamera, Mikrofon oder Funkaufzeichnung, oder ab 100 m Flughöhe unter weiteren Auflagen (u. a. berechtigter Zweck, nicht zwischen 22 und 6 Uhr).",
		"Overflight only with consent of the owner or occupant, with drones up to 0.25 kg without camera, microphone or radio recording, or at 100 m or higher under further conditions (incl. a legitimate purpose, not between 22:00 and 06:00)."},
	8: {"Überflug nur außerhalb der Betriebs- bzw. Badezeiten.",
		"Overflight only outside opening or bathing hours."},
	9: {"Betrieb nur mit Flugverkehrskontrollfreigabe nach § 21 LuftVO.",
		"Operation only with an air traffic control clearance under § 21 LuftVO."},
	10: {"Über und bis 100 m seitlich nur mit ausdrücklicher Zustimmung des Betreibers.",
		"Over and within 100 m laterally only with explicit consent of the operator."},
	11: {"Über und bis 100 m seitlich nur mit Zustimmung des zuständigen Einsatzleiters.",
		"Over and within 100 m laterally only with consent of the incident commander."},
}

var paragraph17 = ruleText{
	"Flug nur, soweit die Beschränkungen des Gebiets es zulassen oder das Bundesaufsichtsamt für Flugsicherung bzw. die Flugverkehrskontrolle den Durchflug genehmigt hat.",
	"Flight only as far as the area's restrictions allow, or with approval from the Federal Supervisory Authority for Air Navigation Services or air traffic control.",
}

// NfL 2026-1-3960: general ATC clearance for UAS in DFS control zones.
const nflCTR2026 = "NfL 2026-1-3960"

var ctrDFS = ruleText{
	"Kontrollzone: Flugverkehrskontrollfreigabe nötig. In den Zonen 2 bis 4 gilt sie unter Auflagen automatisch (allgemeine Freigabe der DFS).",
	"Control zone: an air traffic control clearance is required. In zones 2 to 4 it is granted automatically under conditions (DFS general clearance).",
}

var ctrZoneRules = map[int]ruleText{
	1: {"Zone 1 (Flughafennähe): immer eine individuelle Flugverkehrskontrollfreigabe der DFS nötig, unabhängig von der Flughöhe. Antrag über ais.dfs.de, i. d. R. 14 Tage Vorlauf.",
		"Zone 1 (near the airport): an individual DFS air traffic control clearance is always required, whatever the height. Apply via ais.dfs.de, usually 14 days in advance."},
	2: {"Zone 2: allgemeine Freigabe bis max. 25 m über Flugplatzhöhe bzw. über Grund – der kleinere Wert gilt.",
		"Zone 2: general clearance up to 25 m above aerodrome elevation or above ground, whichever is lower."},
	3: {"Zone 3: allgemeine Freigabe bis max. 45 m über Flugplatzhöhe bzw. über Grund – der kleinere Wert gilt.",
		"Zone 3: general clearance up to 45 m above aerodrome elevation or above ground, whichever is lower."},
	4: {"Zone 4: allgemeine Freigabe bis 100 m über Flugplatzhöhe oder 50 m über Grund – der größere Wert gilt. Liegt der Startort unter der Flugplatzhöhe: 100 m über Grund.",
		"Zone 4: general clearance up to 100 m above aerodrome elevation or 50 m above ground, whichever is higher. If the take-off point is below aerodrome elevation: 100 m above ground."},
}

var ctrGeneralConditions = ruleText{
	"Hindernisnah (max. 30 m seitlich, 15 m oberhalb eines Hindernisses) auch höher. Mindestens 800 m Bodensicht, kein autonomer Betrieb; bemannten Luftfahrzeugen ausweichen.",
	"Close to obstacles (max. 30 m laterally, 15 m above an obstacle) also higher. At least 800 m ground visibility, no autonomous operation; give way to manned aircraft.",
}

var (
	reParagraph21h = regexp.MustCompile(`21h.*Abs\.?\s*3\s*\(\s*(\d+)\s*\.?\s*\)`)
	reParagraph17  = regexp.MustCompile(`§\s*17\b`)
	reCTRZone      = regexp.MustCompile(`(?i)\bZone\s*([1-4])\b`)
)

// ZoneRuleInfo is what the zone list shows under an area.
type ZoneRuleInfo struct {
	DE, EN string
	// CTR: zone-specific clearance and height rule (empty if not applicable).
	CTRZone      int
	HeightDE     string
	HeightEN     string
	ConditionsDE string
	ConditionsEN string
	HasRule      bool
}

// ZoneRule derives the plain-language rule for one DiPUL area from its legal
// reference (and, for DFS control zones, its zone number in the name).
func ZoneRule(area map[string]interface{}) ZoneRuleInfo {
	ref := strings.TrimSpace(fmt.Sprint(area["legalReference"]))
	if area["legalReference"] == nil {
		ref = ""
	}
	var info ZoneRuleInfo

	switch {
	case ref == nflCTR2026:
		info.DE, info.EN, info.HasRule = ctrDFS.DE, ctrDFS.EN, true
		if m := reCTRZone.FindStringSubmatch(fmt.Sprint(area["name"])); m != nil {
			n, _ := strconv.Atoi(m[1])
			if r, ok := ctrZoneRules[n]; ok {
				info.CTRZone = n
				info.HeightDE, info.HeightEN = r.DE, r.EN
				if n > 1 {
					info.ConditionsDE, info.ConditionsEN = ctrGeneralConditions.DE, ctrGeneralConditions.EN
				}
			}
		}
	case reParagraph21h.MatchString(ref):
		n, _ := strconv.Atoi(reParagraph21h.FindStringSubmatch(ref)[1])
		if r, ok := paragraph21h[n]; ok {
			info.DE, info.EN, info.HasRule = r.DE, r.EN, true
		}
	case strings.HasPrefix(ref, "NfL"):
		// A control zone governed by another NfL (e.g. non-DFS tower):
		// clearance is needed, details are in that NfL.
		base := paragraph21h[9]
		info.DE, info.EN, info.HasRule = base.DE, base.EN, true
	case reParagraph17.MatchString(ref):
		info.DE, info.EN, info.HasRule = paragraph17.DE, paragraph17.EN, true
	}
	return info
}

// ZoneNames splits DiPUL's combined names ("A||B||C") into a list.
func ZoneNames(v interface{}) []string {
	if v == nil {
		return nil
	}
	var out []string
	for _, n := range strings.Split(fmt.Sprint(v), "||") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}
