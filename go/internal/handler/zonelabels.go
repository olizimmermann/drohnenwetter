package handler

// zoneLabels maps DiPUL zone type codes to the names used in the map legend,
// so the zone list shows "Kontrollzonen" instead of "U_CTR".
var zoneLabels = map[string][2]string{ // code -> {de, en}
	"U_NFZ":              {"Flugverbotszonen", "No-fly Zones"},
	"FLIGHT_RESTRICTION": {"Flugbeschränkungsgebiete", "Flight Restriction Areas"},
	"CONTROL_ZONE":       {"Kontrollzonen", "Control Zones"},
	"U_CTR":              {"Kontrollzonen", "Control Zones"},
	"AIRPORT":            {"Flughäfen", "Airports"},
	"AIRFIELD_LAW":       {"Flugplätze", "Airfields"},
	"MILITARY":           {"Militärische Anlagen", "Military Facilities"},
	"FED_STATE_AUTH":     {"Behörden", "Authorities"},
	"POLICE":             {"Polizei", "Police"},
	"PRISON":             {"Justizvollzugsanstalten", "Prisons"},
	"SEC_ORG_OTHERS":     {"Sicherheitsbehörden", "Security Authorities"},
	"EMBASSY":            {"Diplomatische Vertretungen", "Diplomatic Representations"},
	"HOSPITAL":           {"Krankenhäuser", "Hospitals"},
	"INDUSTRY":           {"Industrieanlagen", "Industrial Facilities"},
	"POPULATEDAREA":      {"Wohngrundstücke", "Residential Properties"},
	"NATIONAL_PARK":      {"Nationalparks", "National Parks"},
	"NATURE_RESERVE":     {"Naturschutzgebiete", "Nature Reserves"},
	"BIRD_RESERVE":       {"Vogelschutzgebiete", "Bird Protection Areas"},
	"FFH_AREA":           {"FFH-Gebiete", "FFH Areas"},
	"MOTORWAY":           {"Bundesautobahnen", "Federal Motorways"},
	"HIGHWAY":            {"Bundesstraßen", "Federal Roads"},
	"RAILWAY_FACILITY":   {"Bahnanlagen", "Railway Facilities"},
}

// ZoneLabel returns the human-readable name of a DiPUL zone type code in
// lang ("de" or "en"). Unknown codes are returned unchanged, so new DiPUL
// codes still show up instead of disappearing.
func ZoneLabel(code, lang string) string {
	l, ok := zoneLabels[code]
	if !ok {
		return code
	}
	if lang == "en" {
		return l[1]
	}
	return l[0]
}
