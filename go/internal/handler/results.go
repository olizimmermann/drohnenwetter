package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/olizimmermann/drohnenwetter/internal/api"
	"github.com/olizimmermann/drohnenwetter/internal/assessment"
)

type ResultsHandler struct {
	tmpl       *template.Template
	hereAPIKey string
	owToken    string
}

func NewResultsHandler(tmpl *template.Template, hereAPIKey, owToken string) *ResultsHandler {
	return &ResultsHandler{tmpl: tmpl, hereAPIKey: hereAPIKey, owToken: owToken}
}

type resultsData struct {
	Address        string
	Lat            float64
	Lon            float64
	Assessment     *assessment.Assessment
	Zones          []api.AffectedArea
	ZonesGeoJSON   template.JS // safe JS — injected directly into <script>
	CloudBase      *api.CloudBaseResult
	Traffic        []api.AircraftState
	TrafficJSON    template.JS // safe JS — injected directly into <script>
	OWFailed       bool // OpenWeatherMap unavailable
	KpFailed       bool // Kp-Index unavailable
	DiPULFailed    bool // DiPUL airspace data unavailable
	TrafficFailed  bool // OpenSky live traffic unavailable
	HasRedZone      bool // ED-R / ED-D / ED-P at this location
	HasOrangeZone   bool // CTR / ATZ / ED-LR / MILITARY at this location
	WeatherFlyable  bool // weather-only assessment (ignoring zones)
	ErrorDE        string
	ErrorEN        string
}

type allFetched struct {
	utm        *api.UTMResponse
	ow         *api.OWResponse
	kp         float64
	zones      []api.AffectedArea
	cloudBase  *api.CloudBaseResult
	traffic    []api.AircraftState
	errs       [5]error
}

func (h *ResultsHandler) fetchAll(lat, lon float64, city string) *allFetched {
	var wg sync.WaitGroup
	out := &allFetched{}

	wg.Add(6)
	go func() {
		defer wg.Done()
		out.utm, out.errs[0] = api.FetchUTMForecast(lat, lon)
	}()
	go func() {
		defer wg.Done()
		out.ow, out.errs[1] = api.FetchOpenWeather(lat, lon, h.owToken)
	}()
	go func() {
		defer wg.Done()
		out.kp, out.errs[2] = api.FetchKpIndex()
	}()
	go func() {
		defer wg.Done()
		out.zones, out.errs[3] = api.FetchAllZoneDetails(lat, lon)
	}()
	go func() {
		defer wg.Done()
		out.cloudBase = api.FetchCloudBase(city) // never errors; check .Available
	}()
	go func() {
		defer wg.Done()
		out.traffic, out.errs[4] = api.FetchNearbyTraffic(lat, lon)
	}()
	wg.Wait()
	return out
}

// coordRe matches "lat, lon" or "lat lon" in decimal degree format.
// e.g. "52.5200, 13.4050" / "52.5200 13.4050" / "52.52,13.405"
var coordRe = regexp.MustCompile(`^\s*(-?\d{1,3}(?:\.\d+)?)\s*[,\s]\s*(-?\d{1,3}(?:\.\d+)?)\s*$`)

// parseCoords returns (lat, lon, true) if s looks like a coordinate pair.
func parseCoords(s string) (float64, float64, bool) {
	m := coordRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(m[1], 64)
	lon, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	// Sanity: lat −90…90, lon −180…180
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}
	return lat, lon, true
}

func logLookup(mode string, geo *api.GeocodeResult, ip string) {
	street := geo.Street
	if geo.HouseNumber != "" {
		street += " " + geo.HouseNumber
	}
	log.Printf("[lookup] mode=%s street=%q city=%q zip=%q lat=%.6f lon=%.6f ip=%s",
		mode, street, geo.City, geo.PostalCode, geo.Lat, geo.Lon, ip)
}

func (h *ResultsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var geo *api.GeocodeResult

	latStr := r.FormValue("lat")
	lonStr := r.FormValue("lon")

	if latStr != "" && lonStr != "" {
		// GPS path: coordinates supplied by the browser
		lat, errLat := strconv.ParseFloat(latStr, 64)
		lon, errLon := strconv.ParseFloat(lonStr, 64)
		if errLat != nil || errLon != nil || lat == 0 || lon == 0 {
			h.renderError(w, "Ungültige GPS-Koordinaten.", "Invalid GPS coordinates.")
			return
		}
		var err error
		geo, err = api.ReverseGeocode(lat, lon, h.hereAPIKey)
		if err != nil {
			// Reverse geocode failed — still usable, just show raw coords
			log.Printf("[results] revgeocode error: %v", err)
			geo = &api.GeocodeResult{
				Lat:   lat,
				Lon:   lon,
				Title: fmt.Sprintf("%.5f, %.5f", lat, lon),
			}
		}
		logLookup("gps", geo, clientIP(r))
	} else {
		// Address path: forward geocode (or coordinate paste detection)
		address := r.FormValue("address")
		if address == "" {
			h.renderError(w, "Bitte geben Sie eine Adresse ein.", "Please enter an address.")
			return
		}
		if len(address) > 100 {
			h.renderError(w, "Adresse zu lang (max. 100 Zeichen).", "Address too long (max. 100 characters).")
			return
		}

		var err error
		if lat, lon, ok := parseCoords(address); ok {
			// Looks like pasted coordinates — use reverse geocode
			geo, err = api.ReverseGeocode(lat, lon, h.hereAPIKey)
			if err != nil {
				log.Printf("[results] revgeocode error: %v", err)
				geo = &api.GeocodeResult{
					Lat:   lat,
					Lon:   lon,
					Title: fmt.Sprintf("%.5f, %.5f", lat, lon),
				}
			}
			logLookup("coords", geo, clientIP(r))
		} else {
			// Normal address — forward geocode
			geo, err = api.Geocode(address, h.hereAPIKey)
			if err != nil {
				log.Printf("[results] geocode error: %v", err)
				h.renderError(w, "Adresse nicht gefunden. Bitte prüfen Sie die Eingabe.", "Address not found. Please check your input.")
				return
			}
			logLookup("address", geo, clientIP(r))
		}
	}

	fetched := h.fetchAll(geo.Lat, geo.Lon, geo.City)

	labels := []string{"UTM", "OpenWeather", "Kp-Index", "DiPUL", "OpenSky"}
	for i, e := range fetched.errs {
		if e != nil {
			log.Printf("[results] %s error: %v", labels[i], e)
		}
	}

	if fetched.utm == nil {
		h.renderError(w, "Wetterdaten (UTM/DFS) nicht verfügbar. Bitte später erneut versuchen.", "Weather data (UTM/DFS) unavailable. Please try again later.")
		return
	}

	owFailed      := fetched.errs[1] != nil
	kpFailed      := fetched.errs[2] != nil
	dipulFailed   := fetched.errs[3] != nil
	trafficFailed := fetched.errs[4] != nil

	var hasRedZone, hasOrangeZone bool
	for _, z := range fetched.zones {
		switch z.TypeCode {
		case "FLIGHT_RESTRICTION":
			hasRedZone = true
		case "CONTROL_ZONE", "AIRPORT", "AIRFIELD_LAW", "MILITARY":
			hasOrangeZone = true
		}
	}

	a := assessment.Assess(fetched.utm, fetched.ow, fetched.kp)
	weatherFlyable := a.Flyable // capture before zone override

	// Restricted or controlled airspace overrides weather-only assessment.
	if hasRedZone || hasOrangeZone {
		a.Flyable = false
	}

	// Serialize zones to JSON for direct injection into the Leaflet map script.
	zonesJSON, err := json.Marshal(fetched.zones)
	if err != nil {
		log.Printf("[results] zones marshal error: %v", err)
		zonesJSON = []byte("[]")
	}

	trafficJSON, err := json.Marshal(fetched.traffic)
	if err != nil {
		log.Printf("[results] traffic marshal error: %v", err)
		trafficJSON = []byte("[]")
	}

	data := resultsData{
		Address:       geo.Title,
		Lat:           geo.Lat,
		Lon:           geo.Lon,
		Assessment:    a,
		Zones:         fetched.zones,
		ZonesGeoJSON:  template.JS(zonesJSON),
		CloudBase:     fetched.cloudBase,
		Traffic:       fetched.traffic,
		TrafficJSON:   template.JS(trafficJSON),
		OWFailed:      owFailed,
		KpFailed:      kpFailed,
		DiPULFailed:   dipulFailed,
		TrafficFailed: trafficFailed,
		HasRedZone:     hasRedZone,
		HasOrangeZone:  hasOrangeZone,
		WeatherFlyable: weatherFlyable,
	}

	if err := h.tmpl.ExecuteTemplate(w, "results.html", data); err != nil {
		log.Printf("[results] template error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *ResultsHandler) renderError(w http.ResponseWriter, de, en string) {
	if err := h.tmpl.ExecuteTemplate(w, "results.html", resultsData{ErrorDE: de, ErrorEN: en}); err != nil {
		http.Error(w, de, http.StatusInternalServerError)
	}
}
