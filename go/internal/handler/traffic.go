package handler

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

// TrafficHandler returns live nearby aircraft as JSON.
// GET /traffic?lat=X&lon=Y
func TrafficHandler(w http.ResponseWriter, r *http.Request) {
	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if errLat != nil || errLon != nil ||
		math.IsNaN(lat) || math.IsInf(lat, 0) ||
		math.IsNaN(lon) || math.IsInf(lon, 0) ||
		lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		http.Error(w, "invalid coordinates", http.StatusBadRequest)
		return
	}

	aircraft, err := api.FetchNearbyTraffic(lat, lon)
	if err != nil {
		log.Printf("[traffic] %.5f,%.5f: %v", lat, lon, err)
		http.Error(w, "traffic unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(aircraft); err != nil {
		log.Printf("[traffic] encode error: %v", err)
	}
}
