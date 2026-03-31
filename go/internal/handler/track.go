package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

var icao24Re = regexp.MustCompile(`^[0-9a-f]{6}$`)

// TrackHandler proxies OpenSky /tracks for a single aircraft.
// GET /track?icao24=abc123
func TrackHandler(w http.ResponseWriter, r *http.Request) {
	icao24 := strings.ToLower(r.URL.Query().Get("icao24"))
	if !icao24Re.MatchString(icao24) {
		http.Error(w, "invalid icao24", http.StatusBadRequest)
		return
	}

	track, err := api.FetchTrack(icao24)
	if err != nil {
		log.Printf("[track] %s: %v", icao24, err)
		http.Error(w, "track unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(track); err != nil {
		log.Printf("[track] encode error: %v", err)
	}
}
