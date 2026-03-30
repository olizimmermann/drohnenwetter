package handler

import (
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/olizimmermann/drone-weather/internal/api"
)

// ZoneInfoHandler proxies a DiPUL WMS GetFeatureInfo request for a given
// lat/lon and returns the raw GeoJSON FeatureCollection.
// GET /zone-info?lat=X&lon=Y
func ZoneInfoHandler(w http.ResponseWriter, r *http.Request) {
	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if errLat != nil || errLon != nil ||
		math.IsNaN(lat) || math.IsInf(lat, 0) ||
		math.IsNaN(lon) || math.IsInf(lon, 0) ||
		lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		http.Error(w, "invalid coordinates", http.StatusBadRequest)
		return
	}

	body, err := api.FetchZoneInfo(lat, lon)
	if err != nil {
		log.Printf("[zone-info] %.5f,%.5f: %v", lat, lon, err)
		http.Error(w, "zone info unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(body); err != nil {
		log.Printf("[zone-info] write error: %v", err)
	}
}
