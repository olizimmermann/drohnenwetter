package api

import (
	"fmt"
	"net/url"
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

// FetchZoneInfo queries the DiPUL WMS GetFeatureInfo endpoint for a given
// lat/lon and returns the raw GeoJSON FeatureCollection bytes.
func FetchZoneInfo(lat, lon float64) ([]byte, error) {
	const delta = 0.002 // ~200 m radius bbox
	bbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", lat-delta, lon-delta, lat+delta, lon+delta)

	params := url.Values{}
	params.Set("SERVICE", "WMS")
	params.Set("VERSION", "1.3.0")
	params.Set("REQUEST", "GetFeatureInfo")
	params.Set("LAYERS", zoneInfoLayers)
	params.Set("QUERY_LAYERS", zoneInfoLayers)
	params.Set("INFO_FORMAT", "application/json")
	params.Set("CRS", "EPSG:4326")
	params.Set("BBOX", bbox)
	params.Set("WIDTH", "256")
	params.Set("HEIGHT", "256")
	params.Set("I", "128")
	params.Set("J", "128")

	return doGet("https://uas-betrieb.de/geoservices/dipul/wms?"+params.Encode(), nil)
}
