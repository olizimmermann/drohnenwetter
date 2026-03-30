package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// zoneInfoLayers are queried on map click via GetFeatureInfo.
// Order: least restrictive first, most restrictive last
// (WMS renders last layer on top; same order used for priority sorting client-side).
var zoneInfoLayers = strings.Join([]string{
	// Nature / environment (least restrictive)
	"dipul:ffh-gebiete",
	"dipul:vogelschutzgebiete",
	"dipul:nationalparks",
	"dipul:naturschutzgebiete",
	// Inactive restrictions
	"dipul:inaktive_temporaere_betriebseinschraenkungen",
	// Infrastructure / facilities
	"dipul:stromleitungen",
	"dipul:windkraftanlagen",
	"dipul:umspannwerke",
	"dipul:wohngrundstuecke",
	"dipul:freibaeder",
	"dipul:industrieanlagen",
	"dipul:kraftwerke",
	"dipul:labore",
	"dipul:krankenhaeuser",
	// Authorities / security
	"dipul:behoerden",
	"dipul:justizvollzugsanstalten",
	"dipul:polizei",
	"dipul:sicherheitsbehoerden",
	"dipul:internationale_organisationen",
	"dipul:diplomatische_vertretungen",
	// Military
	"dipul:militaerische_anlagen",
	// Aviation (approach)
	"dipul:haengegleiter",
	"dipul:modellflugplaetze",
	"dipul:flugplaetze",
	"dipul:flughaefen",
	// Airspace (most restrictive — on top)
	"dipul:temporaere_betriebseinschraenkungen",
	"dipul:kontrollzonen",
	"dipul:flugbeschraenkungsgebiete",
}, ",")

// ZoneFeature is a simplified GeoJSON-like feature from WMS GetFeatureInfo.
type ZoneFeature struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
}

// ZoneFeatureCollection is the JSON response returned by FetchZoneInfo.
type ZoneFeatureCollection struct {
	Type     string        `json:"type"`
	Features []ZoneFeature `json:"features"`
}

// FetchZoneInfo queries the DiPUL WMS GetFeatureInfo endpoint for a given
// lat/lon and returns a GeoJSON-like FeatureCollection as JSON bytes.
func FetchZoneInfo(lat, lon float64) ([]byte, error) {
	const delta = 0.002 // ~200 m radius bbox
	bbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", lat-delta, lon-delta, lat+delta, lon+delta)

	params := url.Values{}
	params.Set("SERVICE", "WMS")
	params.Set("VERSION", "1.3.0")
	params.Set("REQUEST", "GetFeatureInfo")
	params.Set("LAYERS", zoneInfoLayers)
	params.Set("QUERY_LAYERS", zoneInfoLayers)
	params.Set("INFO_FORMAT", "text/plain")
	params.Set("CRS", "EPSG:4326")
	params.Set("BBOX", bbox)
	params.Set("WIDTH", "256")
	params.Set("HEIGHT", "256")
	params.Set("I", "128")
	params.Set("J", "128")

	raw, err := doGet("https://uas-betrieb.de/geoservices/dipul/wms?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	fc := parseWMSPlainText(raw)
	return json.Marshal(fc)
}

// parseWMSPlainText converts a WMS text/plain GetFeatureInfo response into a
// ZoneFeatureCollection.  The WMS emits blocks like:
//
//	Results for FeatureType 'de.dfs.dipul:kontrollzonen':
//	--------------------------------------------
//	name = MUENCHEN
//	upper_limit_altitude = 3500.0
//	…
//	--------------------------------------------
func parseWMSPlainText(data []byte) ZoneFeatureCollection {
	fc := ZoneFeatureCollection{Type: "FeatureCollection", Features: []ZoneFeature{}}
	text := strings.TrimSpace(string(data))
	if text == "" || strings.Contains(text, "no features were found") {
		return fc
	}

	featureIdx := 0
	currentLayer := ""
	inBlock := false
	var props map[string]interface{}

	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)

		// Layer header line
		if strings.HasPrefix(trimmed, "Results for FeatureType") {
			start := strings.Index(trimmed, "'")
			end := strings.LastIndex(trimmed, "'")
			if start >= 0 && end > start {
				fullName := trimmed[start+1 : end]
				// Strip namespace prefix (everything up to and including the last colon)
				if i := strings.LastIndex(fullName, ":"); i >= 0 {
					currentLayer = fullName[i+1:]
				} else {
					currentLayer = fullName
				}
			}
			continue
		}

		// Separator line (----…)
		if strings.HasPrefix(trimmed, "---") {
			if inBlock {
				// Close the current feature block
				if len(props) > 0 {
					fc.Features = append(fc.Features, ZoneFeature{
						ID:         fmt.Sprintf("%s.%d", currentLayer, featureIdx),
						Type:       "Feature",
						Properties: props,
					})
					featureIdx++
				}
				inBlock = false
				props = nil
			} else {
				// Open a new feature block
				inBlock = true
				props = make(map[string]interface{})
			}
			continue
		}

		if !inBlock || props == nil {
			continue
		}

		// Parse "key = value"
		if eqIdx := strings.Index(trimmed, " = "); eqIdx >= 0 {
			key := trimmed[:eqIdx]
			val := trimmed[eqIdx+3:]
			if key == "geom" {
				continue // skip geometry blob
			}
			// Skip literal "null" values from WMS
			if val == "null" {
				continue
			}
			// Store numbers as float64, everything else as string
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				props[key] = f
			} else {
				props[key] = val
			}
		}
	}

	return fc
}
