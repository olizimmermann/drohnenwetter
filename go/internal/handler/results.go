package handler

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/olizimmermann/drone-weather/internal/api"
	"github.com/olizimmermann/drone-weather/internal/assessment"
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
	Address      string
	Lat          float64
	Lon          float64
	Assessment   *assessment.Assessment
	Zones        []api.AffectedArea
	ZonesGeoJSON template.JS // safe JS — injected directly into <script>
	DataWarnings []string    // non-fatal service degradation notices
	Error        string
}

type allFetched struct {
	utm   *api.UTMResponse
	ow    *api.OWResponse
	kp    float64
	zones []api.AffectedArea
	errs  [4]error
}

func (h *ResultsHandler) fetchAll(lat, lon float64) *allFetched {
	var wg sync.WaitGroup
	out := &allFetched{}

	wg.Add(4)
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
	wg.Wait()
	return out
}

func (h *ResultsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	address := r.FormValue("address")
	if address == "" {
		h.renderError(w, "Bitte geben Sie eine Adresse ein.")
		return
	}
	if len(address) > 100 {
		h.renderError(w, "Adresse zu lang (max. 100 Zeichen).")
		return
	}

	geo, err := api.Geocode(address, h.hereAPIKey)
	if err != nil {
		log.Printf("[results] geocode error: %v", err)
		h.renderError(w, "Adresse nicht gefunden.")
		return
	}

	log.Printf("[results] resolved %q → %.6f, %.6f", geo.Title, geo.Lat, geo.Lon)

	fetched := h.fetchAll(geo.Lat, geo.Lon)

	labels := []string{"UTM", "OpenWeather", "Kp-Index", "DiPUL"}
	for i, e := range fetched.errs {
		if e != nil {
			log.Printf("[results] %s error: %v", labels[i], e)
		}
	}

	if fetched.utm == nil {
		h.renderError(w, "Wetterdaten (UTM/DFS) konnten nicht abgerufen werden. Bitte später erneut versuchen.")
		return
	}

	var dataWarnings []string
	if fetched.errs[1] != nil {
		dataWarnings = append(dataWarnings, "Taupunkt-Daten nicht verfügbar (OpenWeatherMap). Nebelgefahr kann nicht berechnet werden.")
	}
	if fetched.errs[2] != nil {
		dataWarnings = append(dataWarnings, "Kp-Index nicht verfügbar (GFZ Potsdam). GPS-Zuverlässigkeit kann nicht bewertet werden.")
	}
	if fetched.errs[3] != nil {
		dataWarnings = append(dataWarnings, "Luftraumdaten nicht verfügbar (DiPUL). Bitte Luftraum manuell prüfen.")
	}

	a := assessment.Assess(fetched.utm, fetched.ow, fetched.kp)

	// Serialize zones to JSON for direct injection into the Leaflet map script.
	zonesJSON, err := json.Marshal(fetched.zones)
	if err != nil {
		zonesJSON = []byte("[]")
	}

	data := resultsData{
		Address:      geo.Title,
		Lat:          geo.Lat,
		Lon:          geo.Lon,
		Assessment:   a,
		Zones:        fetched.zones,
		ZonesGeoJSON: template.JS(zonesJSON),
		DataWarnings: dataWarnings,
	}

	if err := h.tmpl.ExecuteTemplate(w, "results.html", data); err != nil {
		log.Printf("[results] template error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *ResultsHandler) renderError(w http.ResponseWriter, msg string) {
	if err := h.tmpl.ExecuteTemplate(w, "results.html", resultsData{Error: msg}); err != nil {
		http.Error(w, msg, http.StatusInternalServerError)
	}
}
