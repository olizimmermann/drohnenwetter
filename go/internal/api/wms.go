package api

import (
	"fmt"
	"net/url"
	"strings"
)

var zoneInfoLayers = strings.Join([]string{
	"dipul:kontrollzonen",
	"dipul:flugbeschraenkungsgebiete",
	"dipul:flughaefen",
	"dipul:flugplaetze",
	"dipul:modellflugplaetze",
	"dipul:haengegleiter",
	"dipul:militaerische_anlagen",
	"dipul:temporaere_betriebseinschraenkungen",
	"dipul:inaktive_temporaere_betriebseinschraenkungen",
	"dipul:naturschutzgebiete",
	"dipul:nationalparks",
	"dipul:vogelschutzgebiete",
	"dipul:ffh-gebiete",
	"dipul:wohngrundstuecke",
	"dipul:industrieanlagen",
	"dipul:krankenhaeuser",
	"dipul:windkraftanlagen",
	"dipul:stromleitungen",
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
