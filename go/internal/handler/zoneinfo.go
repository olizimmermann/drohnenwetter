package handler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// zoneInfoLayers are the WMS layers queried on map click — airspace and
// safety-relevant categories only (infrastructure layers omitted).
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

// ZoneInfoHandler proxies a WMS GetFeatureInfo request to the DiPUL WMS for
// a given lat/lon and returns the raw GeoJSON FeatureCollection.
// GET /zone-info?lat=X&lon=Y
func ZoneInfoHandler(w http.ResponseWriter, r *http.Request) {
	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if errLat != nil || errLon != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		http.Error(w, "invalid coordinates", http.StatusBadRequest)
		return
	}

	// Small bbox centred on the click (~200m radius), click at pixel centre.
	const delta = 0.002
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

	wmsURL := "https://uas-betrieb.de/geoservices/dipul/wms?" + params.Encode()

	resp, err := http.Get(wmsURL) //nolint:noctx
	if err != nil {
		log.Printf("[zone-info] WMS request error: %v", err)
		http.Error(w, "zone info unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[zone-info] read error: %v", err)
		http.Error(w, "zone info unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body) //nolint:errcheck
}
