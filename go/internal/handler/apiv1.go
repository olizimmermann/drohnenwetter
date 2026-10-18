package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/olizimmermann/drohnenwetter/internal/api"
	"github.com/olizimmermann/drohnenwetter/internal/assessment"
)

// ── Dispatcher API v1 ─────────────────────────────────────────────────────────
//
// GET /api/v1/assessment?lat=..&lon=..  or  ?address=..   [&lang=de|en] [&geometry=1]
//
// Same data and safety logic as the results page, as JSON. OpenSky traffic is
// only fetched for callers whose gateway key carries the "opensky" feature.

const apiVersion = "1"

// apiLog emits one JSON line per API request (msg=api_request) for Wazuh.
// The web UI keeps its plain-text log lines; only API events are JSON.
var apiLog = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// APIHandler serves the dispatcher API. It shares the ResultsHandler's cache,
// data-source hooks and evaluation so web and API verdicts never diverge.
type APIHandler struct {
	rh *ResultsHandler
}

func NewAPIHandler(rh *ResultsHandler) *APIHandler {
	return &APIHandler{rh: rh}
}

// ── Response DTOs ─────────────────────────────────────────────────────────────

type apiResponse struct {
	APIVersion  string        `json:"api_version"`
	RequestID   string        `json:"request_id"`
	GeneratedAt string        `json:"generated_at"`
	Cached      bool          `json:"cached"`
	Location    apiLocation   `json:"location"`
	Verdict     apiVerdict    `json:"verdict"`
	Weather     apiWeather    `json:"weather"`
	Kp          apiKp         `json:"kp"`
	CloudBase   *apiCloudBase `json:"cloud_base"`
	Zones       []apiZone     `json:"zones"`
	Sources     apiSources    `json:"sources"`
	Traffic     []apiAircraft `json:"traffic,omitempty"`
	Disclaimer  string        `json:"disclaimer"`
}

type apiLocation struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Address string  `json:"address"`
	// Terrain height at the location (DiPUL elevation model); nil if unavailable.
	Terrain *apiTerrain `json:"terrain"`
}

type apiTerrain struct {
	ElevationMSLM float64 `json:"elevation_msl_m"`
	AccuracyM     float64 `json:"accuracy_m"`
}

type apiVerdict struct {
	Flyable        bool     `json:"flyable"`
	WeatherFlyable bool     `json:"weather_flyable"`
	Airspace       string   `json:"airspace"` // "red" | "orange" | "none" | "unknown"
	Reasons        []string `json:"reasons"`  // why flyable is false
	Notices        []string `json:"notices"`  // informational, does not affect flyable
}

type apiAltValue struct {
	HeightM float64 `json:"height_m"`
	Value   float64 `json:"value"`
	OK      bool    `json:"ok"`
	Warn    bool    `json:"warn"`
	Crit    bool    `json:"crit"`
}

type apiWeather struct {
	TemperatureC  []apiAltValue  `json:"temperature_c"`
	WindMS        []apiAltValue  `json:"wind_ms"`
	GustMS        apiGust        `json:"gust_ms"`
	DewPoint      apiDewPoint    `json:"dew_point"`
	Freezing      apiFlagMessage `json:"freezing_hazard"`
	Precipitation apiPrecip      `json:"precipitation"`
	CloudCoverPct float64        `json:"cloud_cover_pct"`
}

type apiGust struct {
	Value float64 `json:"value"`
	OK    bool    `json:"ok"`
}

type apiDewPoint struct {
	SurfaceC   float64       `json:"surface_c"`
	ByAltitude []apiAltValue `json:"by_altitude"`
	OK         bool          `json:"ok"`
	Critical   bool          `json:"critical"`
	Warning    string        `json:"warning,omitempty"`
}

type apiFlagMessage struct {
	Active  bool   `json:"active"`
	Message string `json:"message,omitempty"`
}

type apiPrecip struct {
	Warning bool    `json:"warning"`
	Message string  `json:"message,omitempty"`
	RainMM  float64 `json:"rain_mm"`
	SnowCM  float64 `json:"snow_cm"`
}

type apiKp struct {
	Available bool    `json:"available"`
	Value     float64 `json:"value"`
	OK        bool    `json:"ok"`
	Warning   string  `json:"warning,omitempty"`
}

type apiCloudBase struct {
	ICAO    string      `json:"icao"`
	Airport string      `json:"airport"`
	Metar   *apiCBMetar `json:"metar"`
	Taf     *apiCBTaf   `json:"taf"`
}

type apiCBMetar struct {
	CloudBaseFt int     `json:"cloud_base_ft"`
	CloudBaseM  float64 `json:"cloud_base_m"`
	VisibilityM int     `json:"visibility_m"` // 9999 = ≥10 km
}

type apiCBTaf struct {
	CloudBaseFt    int     `json:"cloud_base_ft"`
	CloudBaseM     float64 `json:"cloud_base_m"`
	MinVisibilityM int     `json:"min_visibility_m"`
}

type apiZone struct {
	TypeCode string        `json:"type_code"`
	Severity string        `json:"severity"` // "red" | "orange" | "info"
	Areas    []apiZoneArea `json:"areas"`
}

type apiZoneArea struct {
	Name           string          `json:"name,omitempty"`
	LegalReference string          `json:"legal_reference,omitempty"`
	Lower          *apiLimit       `json:"lower,omitempty"`
	Upper          *apiLimit       `json:"upper,omitempty"`
	Geometry       json.RawMessage `json:"geometry,omitempty"`
	// Plain-language rule behind the area and the DiPUL legal-basis link.
	Rule          *apiZoneRule `json:"rule,omitempty"`
	LegalBasisURL string       `json:"legal_basis_url,omitempty"`
	// DFS control zones 2–4: general-clearance height limit at this location.
	CTRHeightLimit *apiCTRHeightLimit `json:"ctr_height_limit,omitempty"`
}

type apiZoneRule struct {
	Summary    string `json:"summary"`
	CTRZone    int    `json:"ctr_zone,omitempty"`
	HeightRule string `json:"height_rule,omitempty"`
	Conditions string `json:"conditions,omitempty"`
	AtLocation string `json:"at_location,omitempty"`
}

type apiCTRHeightLimit struct {
	// MaxAGLM is the conservative limit above ground (terrain accuracy
	// included, rounded down). 0 = no general clearance at this location.
	MaxAGLM          float64 `json:"max_agl_m"`
	GeneralClearance bool    `json:"general_clearance"`
	TerrainMSLM      float64 `json:"terrain_msl_m"`
	AerodromeMSLM    float64 `json:"aerodrome_msl_m"`
}

type apiLimit struct {
	Altitude  string `json:"altitude"`
	Unit      string `json:"unit,omitempty"`
	Reference string `json:"reference,omitempty"`
}

type apiSources struct {
	UTM         string `json:"utm"`
	OpenWeather string `json:"openweather"`
	Kp          string `json:"kp"`
	DiPUL       string `json:"dipul"`
	OpenSky     string `json:"opensky"`
}

type apiAircraft struct {
	ICAO24    string  `json:"icao24"`
	Callsign  string  `json:"callsign"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	BaroAltM  float64 `json:"baro_alt_m"`
	VelocityM float64 `json:"velocity_ms"`
	TrackDeg  float64 `json:"track_deg"`
	OnGround  bool    `json:"on_ground"`
	Category  int     `json:"category"`
}

type apiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// ── Handler ───────────────────────────────────────────────────────────────────

// apiEvent accumulates the fields of the single per-request log line.
type apiEvent struct {
	caller        apiCaller
	mode          string
	lat, lon      float64
	haveCoords    bool
	status        int
	errCode       string
	cacheHit      bool
	flyable       *bool
	airspace      string
	opensky       bool
	sourcesFailed []string
}

func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ev := &apiEvent{caller: callerFrom(r.Context()), status: http.StatusOK, sourcesFailed: []string{}}
	defer func() { logAPIEvent(ev, time.Since(start)) }()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set(headerRequestID, ev.caller.requestID)

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		h.fail(w, ev, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported.")
		return
	}

	q := r.URL.Query()
	en := q.Get("lang") == "en"
	msg := func(de, enMsg string) string {
		if en {
			return enMsg
		}
		return de
	}

	var geo *api.GeocodeResult
	latStr, lonStr, address := q.Get("lat"), q.Get("lon"), q.Get("address")
	switch {
	case latStr != "" || lonStr != "":
		ev.mode = "coords"
		lat, errLat := strconv.ParseFloat(latStr, 64)
		lon, errLon := strconv.ParseFloat(lonStr, 64)
		if errLat != nil || errLon != nil || !validCoords(lat, lon) {
			h.fail(w, ev, http.StatusBadRequest, "invalid_coordinates",
				msg("Ungültige Koordinaten (lat/lon in Dezimalgrad).", "Invalid coordinates (lat/lon in decimal degrees)."))
			return
		}
		geo = h.rh.resolveCoords(lat, lon)
	case address != "":
		var lerr *locError
		geo, ev.mode, lerr = h.rh.resolveAddress(address)
		if lerr != nil {
			if lerr.code == errLocRateLimited.code {
				secs := int(math.Ceil(lerr.retryAfter.Seconds()))
				if secs < 1 {
					secs = 5
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
			}
			h.fail(w, ev, lerr.status, lerr.code, msg(lerr.de, lerr.en))
			return
		}
	default:
		h.fail(w, ev, http.StatusBadRequest, "location_missing",
			msg("Parameter lat+lon oder address erforderlich.", "Parameters lat+lon or address required."))
		return
	}
	ev.lat, ev.lon, ev.haveCoords = geo.Lat, geo.Lon, true

	withTraffic := ev.caller.features[FeatureOpenSky]
	ev.opensky = withTraffic
	fetched, hit := h.rh.fetchFn(geo.Lat, geo.Lon, geo.City, withTraffic)
	ev.cacheHit = hit

	labels := []string{"utm", "openweather", "kp", "dipul", "opensky"}
	for i, e := range fetched.errs {
		if e != nil && (i != 4 || withTraffic) {
			log.Printf("[api] %s error: %v", labels[i], e)
			ev.sourcesFailed = append(ev.sourcesFailed, labels[i])
		}
	}

	if fetched.utm == nil {
		if fetched.errs[0] == nil {
			ev.sourcesFailed = append(ev.sourcesFailed, "utm")
		}
		h.fail(w, ev, http.StatusServiceUnavailable, "weather_unavailable",
			msg("Wetterdaten (UTM/DFS) nicht verfügbar. Bitte später erneut versuchen.",
				"Weather data (UTM/DFS) unavailable. Please try again later."))
		return
	}

	evl := evaluate(fetched)
	resp := buildAPIResponse(geo, fetched, evl, ev.caller, hit, withTraffic, en, q.Get("geometry") == "1")
	ev.flyable = &resp.Verdict.Flyable
	ev.airspace = resp.Verdict.Airspace

	writeJSON(w, http.StatusOK, resp)
}

func (h *APIHandler) fail(w http.ResponseWriter, ev *apiEvent, status int, code, message string) {
	ev.status, ev.errCode = status, code
	var body apiErrorBody
	body.Error.Code = code
	body.Error.Message = message
	body.RequestID = ev.caller.requestID
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[api] encode error: %v", err)
	}
}

func logAPIEvent(ev *apiEvent, d time.Duration) {
	attrs := []any{
		"event", "dw.api.assessment",
		"api_version", apiVersion,
		"request_id", ev.caller.requestID,
		"identity", ev.caller.identity,
		"mode", ev.mode,
		"status", ev.status,
		"duration_ms", d.Milliseconds(),
		"cache_hit", ev.cacheHit,
		"opensky", ev.opensky,
		"sources_failed", ev.sourcesFailed,
	}
	if ev.haveCoords {
		// Rounded to the cache-key grid (~111 m); raw addresses are never logged.
		attrs = append(attrs, "lat", round3(ev.lat), "lon", round3(ev.lon))
	}
	if ev.errCode != "" {
		attrs = append(attrs, "error_code", ev.errCode)
	}
	if ev.flyable != nil {
		attrs = append(attrs, "flyable", *ev.flyable, "airspace", ev.airspace)
	}
	apiLog.Info("api_request", attrs...)
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// ── Mapping ───────────────────────────────────────────────────────────────────

const (
	disclaimerDE = "Entscheidungshilfe ohne Gewähr. Die Verantwortung für die Flugdurchführung liegt beim Fernpiloten bzw. der einsatzführenden Stelle."
	disclaimerEN = "Decision aid without warranty. Responsibility for conducting the flight remains with the remote pilot / operating organisation."
)

func zoneSeverity(typeCode string) string {
	switch typeCode {
	case "FLIGHT_RESTRICTION", "U_NFZ":
		return "red"
	case "CONTROL_ZONE", "U_CTR", "AIRPORT", "AIRFIELD_LAW", "MILITARY":
		return "orange"
	}
	return "info"
}

func buildAPIResponse(geo *api.GeocodeResult, f *allFetched, evl evaluation, c apiCaller,
	cached, withTraffic, en, withGeometry bool) apiResponse {

	a := evl.assessment
	pick := func(de, enMsg string) string {
		if en && enMsg != "" {
			return enMsg
		}
		return de
	}

	resp := apiResponse{
		APIVersion:  apiVersion,
		RequestID:   c.requestID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Cached:      cached,
		Location:    apiLocation{Lat: geo.Lat, Lon: geo.Lon, Address: geo.Title, Terrain: terrain(f.ground)},
		Disclaimer:  pick(disclaimerDE, disclaimerEN),
		Zones:       []apiZone{},
	}

	// ── Verdict ──
	v := apiVerdict{
		Flyable:        a.Flyable,
		WeatherFlyable: evl.weatherFlyable,
		Airspace:       "none",
		Reasons:        []string{},
		Notices:        []string{},
	}
	switch {
	case f.errs[3] != nil:
		v.Airspace = "unknown"
	case evl.hasRedZone:
		v.Airspace = "red"
	case evl.hasOrangeZone:
		v.Airspace = "orange"
	}
	if evl.hasRedZone {
		v.Reasons = append(v.Reasons, pick("Flugbeschränkungsgebiet / Flugverbotszone am Standort",
			"Flight restriction / no-fly zone at this location"))
	}
	if evl.hasOrangeZone {
		v.Reasons = append(v.Reasons, pick("Kontrollierter Luftraum (CTR/Flugplatz/Militär) – Freigabe erforderlich",
			"Controlled airspace (CTR/aerodrome/military) – clearance required"))
	}
	for _, t := range a.TempWarnings {
		v.Reasons = append(v.Reasons, pick("Temperatur außerhalb des Bereichs: ", "Temperature out of range: ")+t)
	}
	if en {
		v.Reasons = append(v.Reasons, a.WindWarningsEN...)
	} else {
		v.Reasons = append(v.Reasons, a.WindWarnings...)
	}
	if !a.DewPointOK && a.DewPointWarning != "" {
		v.Reasons = append(v.Reasons, pick(a.DewPointWarning, a.DewPointWarningEN))
	}
	if a.FreezingHazard {
		v.Reasons = append(v.Reasons, pick(a.FreezingHazardDE, a.FreezingHazardEN))
	}
	if !a.KpOK && a.KpWarning != "" {
		v.Reasons = append(v.Reasons, pick(a.KpWarning, a.KpWarningEN))
	}
	if a.PrecipWarning {
		v.Notices = append(v.Notices, pick(a.PrecipWarningDE, a.PrecipWarningEN))
	}
	if f.errs[3] != nil {
		v.Notices = append(v.Notices, pick("Luftraumdaten (DiPUL) nicht verfügbar – Luftraum selbst prüfen",
			"Airspace data (DiPUL) unavailable – check airspace manually"))
	}
	resp.Verdict = v

	// ── Weather ──
	resp.Weather = apiWeather{
		TemperatureC: altValues(a.Temperature),
		WindMS:       altValues(a.WindSpeed),
		GustMS:       apiGust{Value: a.WindGust.Value, OK: a.WindGust.OK},
		DewPoint: apiDewPoint{
			SurfaceC:   a.DewPoint,
			ByAltitude: altValues(a.DewPointByAltitude),
			OK:         a.DewPointOK,
			Critical:   a.DewPointCritical,
		},
		Freezing:      apiFlagMessage{Active: a.FreezingHazard},
		Precipitation: apiPrecip{Warning: a.PrecipWarning, RainMM: a.RainMM, SnowCM: a.SnowCM},
		CloudCoverPct: a.CloudCover,
	}
	if !a.DewPointOK {
		resp.Weather.DewPoint.Warning = pick(a.DewPointWarning, a.DewPointWarningEN)
	}
	if a.FreezingHazard {
		resp.Weather.Freezing.Message = pick(a.FreezingHazardDE, a.FreezingHazardEN)
	}
	if a.PrecipWarning {
		resp.Weather.Precipitation.Message = pick(a.PrecipWarningDE, a.PrecipWarningEN)
	}

	// ── Kp ──
	resp.Kp = apiKp{Available: f.errs[2] == nil, Value: a.KpIndex, OK: a.KpOK}
	if !a.KpOK {
		resp.Kp.Warning = pick(a.KpWarning, a.KpWarningEN)
	}

	// ── Cloud base ──
	if cb := f.cloudBase; cb != nil && (cb.Available || cb.MetarAvailable) {
		out := &apiCloudBase{ICAO: cb.ICAO, Airport: cb.AirportName}
		if cb.MetarAvailable {
			out.Metar = &apiCBMetar{CloudBaseFt: cb.MetarCloudBaseFt, CloudBaseM: cb.MetarCloudBaseM, VisibilityM: cb.MetarVisibilityM}
		}
		if cb.Available {
			out.Taf = &apiCBTaf{CloudBaseFt: cb.CloudBaseFt, CloudBaseM: cb.CloudBaseM, MinVisibilityM: cb.TafMinVisibilityM}
		}
		resp.CloudBase = out
	}

	// ── Zones ──
	for _, z := range f.zones {
		zone := apiZone{TypeCode: z.TypeCode, Severity: zoneSeverity(z.TypeCode), Areas: []apiZoneArea{}}
		for _, area := range z.Areas {
			za := apiZoneArea{
				Name:           firstString(area, "name", "designator"),
				LegalReference: firstString(area, "legalReference"),
				Lower:          limit(area, "lowerLimit"),
				Upper:          limit(area, "upperLimit"),
			}
			if rule := ZoneRule(area); rule.HasRule {
				za.Rule = &apiZoneRule{
					Summary:    pick(rule.DE, rule.EN),
					CTRZone:    rule.CTRZone,
					HeightRule: pick(rule.HeightDE, rule.HeightEN),
					Conditions: pick(rule.ConditionsDE, rule.ConditionsEN),
					AtLocation: pick(rule.LocalDE, rule.LocalEN),
				}
				za.LegalBasisURL = pick(rule.LinkDE, rule.LinkEN)
			} else if link, _ := area[keyLinkDE].(string); link != "" {
				za.LegalBasisURL = pick(link, firstString(area, keyLinkEN))
			}
			za.CTRHeightLimit = ctrHeightLimit(area)
			if withGeometry {
				if g, ok := area["geometry"]; ok && g != nil {
					if raw, err := json.Marshal(g); err == nil {
						za.Geometry = raw
					}
				}
			}
			zone.Areas = append(zone.Areas, za)
		}
		resp.Zones = append(resp.Zones, zone)
	}

	// ── Sources ──
	status := func(err error) string {
		if err != nil {
			return "failed"
		}
		return "ok"
	}
	resp.Sources = apiSources{
		UTM:         "ok",
		OpenWeather: status(f.errs[1]),
		Kp:          status(f.errs[2]),
		DiPUL:       status(f.errs[3]),
		OpenSky:     "disabled",
	}

	// ── Traffic (opensky feature only) ──
	if withTraffic {
		resp.Sources.OpenSky = status(f.errs[4])
		resp.Traffic = make([]apiAircraft, 0, len(f.traffic))
		for _, s := range f.traffic {
			resp.Traffic = append(resp.Traffic, apiAircraft{
				ICAO24: s.ICAO24, Callsign: s.Callsign, Lat: s.Lat, Lon: s.Lon,
				BaroAltM: s.BaroAlt, VelocityM: s.Velocity, TrackDeg: s.TruTrack,
				OnGround: s.OnGround, Category: s.Category,
			})
		}
	}

	return resp
}

func terrain(e *api.Elevation) *apiTerrain {
	if e == nil {
		return nil
	}
	return &apiTerrain{ElevationMSLM: round1(e.MSL), AccuracyM: e.RMSE}
}

// ctrHeightLimit returns the limit computed by enrichZones, or nil.
func ctrHeightLimit(area map[string]interface{}) *apiCTRHeightLimit {
	maxAGL, ok1 := area[keyCTRMaxAGL].(float64)
	ground, ok2 := area[keyGroundMSL].(float64)
	aerodrome, ok3 := area[keyAerodromeMSL].(float64)
	if !ok1 || !ok2 || !ok3 {
		return nil
	}
	return &apiCTRHeightLimit{
		MaxAGLM:          math.Max(maxAGL, 0),
		GeneralClearance: maxAGL > 0,
		TerrainMSLM:      round1(ground),
		AerodromeMSLM:    round1(aerodrome),
	}
}

func altValues(in []assessment.AltEntry) []apiAltValue {
	out := make([]apiAltValue, 0, len(in))
	for _, e := range in {
		out = append(out, apiAltValue{HeightM: e.Height, Value: e.Value, OK: e.OK, Warn: e.Warn, Crit: e.Crit})
	}
	return out
}

// firstString returns the first non-empty value among keys, stringified.
func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s := fmt.Sprint(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// limit reads DiPUL's <prefix>Altitude / Unit / Reference triple.
func limit(m map[string]interface{}, prefix string) *apiLimit {
	alt := firstString(m, prefix+"Altitude")
	if alt == "" {
		return nil
	}
	return &apiLimit{
		Altitude:  alt,
		Unit:      firstString(m, prefix+"Unit"),
		Reference: firstString(m, prefix+"Reference"),
	}
}
