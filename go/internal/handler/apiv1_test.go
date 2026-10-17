package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

const testSecret = "s3cret-for-tests"

// calmUTM is a benign forecast: 10 °C, 3 m/s wind, dry air at 2/10/50 m.
func calmUTM() *api.UTMResponse {
	var fc api.UTMForecast
	for _, h := range []float64{2, 10, 50} {
		height := api.UTMHeight{Unit: "m", Value: h}
		fc.Temperature = append(fc.Temperature, api.UTMTemperature{Height: height, Value: 10})
		fc.Wind = append(fc.Wind, api.UTMWind{Height: height, UComponent: 3, WindSpeedGust: 4})
		fc.AirHumidity = append(fc.AirHumidity, api.UTMAirHumidity{Height: height, Value: 50})
	}
	return &api.UTMResponse{Positions: []api.UTMPosition{{Forecasts: []api.UTMForecast{fc}}}}
}

type fetchCall struct{ withTraffic bool }

// newTestAPI builds an API handler with stubbed data sources and captures the
// JSON log output. fetched may be modified by the test before requests.
func newTestAPI(t *testing.T, fetched *allFetched) (http.Handler, *[]fetchCall, *bytes.Buffer) {
	t.Helper()
	rh := &ResultsHandler{
		cache: newResultCache(10, 0),
		reverseGeocodeFn: func(lat, lon float64, _ string) (*api.GeocodeResult, error) {
			return &api.GeocodeResult{Lat: lat, Lon: lon, Title: "Teststraße 1, Berlin", City: "Berlin"}, nil
		},
		geocodeFn: func(address, _ string) (*api.GeocodeResult, error) {
			if address == "ratelimited" {
				return nil, errors.Join(errors.New("geocode"), api.ErrRateLimited)
			}
			if address == "nirgendwo" {
				return nil, errors.Join(errors.New("geocode"), api.ErrNotFound)
			}
			return &api.GeocodeResult{Lat: 52.52, Lon: 13.405, Title: address, City: "Berlin"}, nil
		},
	}
	var calls []fetchCall
	rh.fetchFn = func(lat, lon float64, city string, withTraffic bool) (*allFetched, bool) {
		calls = append(calls, fetchCall{withTraffic})
		cp := *fetched
		cp.trafficFetched = withTraffic
		if !withTraffic {
			cp.traffic, cp.errs[4] = nil, nil
		}
		return &cp, false
	}

	var logBuf bytes.Buffer
	prev := apiLog
	apiLog = slog.New(slog.NewJSONHandler(&logBuf, nil))
	t.Cleanup(func() { apiLog = prev })

	return APIAuth(testSecret, NewAPIHandler(rh)), &calls, &logBuf
}

func defaultFetched() *allFetched {
	return &allFetched{
		utm:     calmUTM(),
		kp:      2,
		traffic: []api.AircraftState{{ICAO24: "abc123", Callsign: "CHX31", Lat: 52.5, Lon: 13.4}},
	}
}

func doAPI(h http.Handler, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func authed(extra map[string]string) map[string]string {
	h := map[string]string{headerInternalSecret: testSecret, headerGatewayIdentity: "ils-test"}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func decodeLog(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 log line, got %d: %q", len(lines), buf.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("log line is not JSON: %v", err)
	}
	return m
}

func TestAPIRejectsMissingOrWrongSecret(t *testing.T) {
	h, calls, _ := newTestAPI(t, defaultFetched())
	for name, hdr := range map[string]map[string]string{
		"none":  nil,
		"wrong": {headerInternalSecret: "nope"},
	} {
		rec := doAPI(h, "/api/v1/assessment?lat=52.52&lon=13.405", hdr)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, rec.Code)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("data fetched for unauthenticated request")
	}
}

func TestAPIDisabledWithoutSecret(t *testing.T) {
	h := APIAuth("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler reached with empty secret")
	}))
	rec := doAPI(h, "/api/v1/assessment", map[string]string{headerInternalSecret: ""})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func TestAPIBadParams(t *testing.T) {
	h, _, logBuf := newTestAPI(t, defaultFetched())
	for _, target := range []string{
		"/api/v1/assessment",
		"/api/v1/assessment?lat=abc&lon=13",
		"/api/v1/assessment?lat=95&lon=13",
		"/api/v1/assessment?lat=52.5",
	} {
		logBuf.Reset()
		rec := doAPI(h, target, authed(nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", target, rec.Code)
		}
		var body apiErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code == "" {
			t.Errorf("%s: bad error body %q", target, rec.Body.String())
		}
		if m := decodeLog(t, logBuf); m["status"] != float64(400) || m["error_code"] == nil || m["sources_failed"] == nil {
			t.Errorf("%s: log %v", target, m)
		}
	}
}

func TestAPIRateLimitedPassesRetryAfter(t *testing.T) {
	h, _, _ := newTestAPI(t, defaultFetched())
	rec := doAPI(h, "/api/v1/assessment?address=ratelimited", authed(nil))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "5" {
		t.Errorf("status %d Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestAPIAddressNotFound(t *testing.T) {
	h, _, _ := newTestAPI(t, defaultFetched())
	rec := doAPI(h, "/api/v1/assessment?address=nirgendwo", authed(nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "address_not_found") {
		t.Errorf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAPITrafficOnlyWithOpenSkyFeature(t *testing.T) {
	h, calls, _ := newTestAPI(t, defaultFetched())

	rec := doAPI(h, "/api/v1/assessment?lat=52.52&lon=13.405", authed(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Traffic != nil || resp.Sources.OpenSky != "disabled" || (*calls)[0].withTraffic {
		t.Errorf("traffic leaked without feature: %+v / %+v", resp.Traffic, resp.Sources)
	}
	if strings.Contains(rec.Body.String(), `"traffic"`) {
		t.Errorf("traffic key present without feature")
	}

	rec = doAPI(h, "/api/v1/assessment?lat=52.52&lon=13.405",
		authed(map[string]string{headerGatewayFeatures: "OpenSky"}))
	resp = apiResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !(*calls)[1].withTraffic || len(resp.Traffic) != 1 || resp.Sources.OpenSky != "ok" {
		t.Errorf("traffic missing with feature: %+v / %+v", resp.Traffic, resp.Sources)
	}
}

func TestAPISuccessResponseAndLog(t *testing.T) {
	h, _, logBuf := newTestAPI(t, defaultFetched())
	rec := doAPI(h, "/api/v1/assessment?lat=52.520071&lon=13.404954",
		authed(map[string]string{headerRequestID: "abcdef0123456789"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(headerRequestID); got != "abcdef0123456789" {
		t.Errorf("X-Request-Id %q", got)
	}
	var resp apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Verdict.Flyable || resp.Verdict.Airspace != "none" || len(resp.Weather.TemperatureC) != 3 {
		t.Errorf("unexpected verdict/weather: %+v", resp.Verdict)
	}

	m := decodeLog(t, logBuf)
	want := map[string]any{
		"msg": "api_request", "event": "dw.api.assessment", "identity": "ils-test",
		"request_id": "abcdef0123456789", "mode": "coords", "status": float64(200),
		"flyable": true, "airspace": "none", "opensky": false, "lat": 52.52, "lon": 13.405,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("log[%s] = %v, want %v", k, m[k], v)
		}
	}
	if _, ok := m["address"]; ok {
		t.Error("address must not be logged")
	}
}

func TestAPIAirspaceOverridesWeather(t *testing.T) {
	f := defaultFetched()
	f.zones = []api.AffectedArea{{TypeCode: "CONTROL_ZONE", Areas: []map[string]interface{}{
		{"name": "CTR Berlin", "lowerLimitAltitude": 0, "lowerLimitUnit": "FT", "geometry": map[string]any{"type": "Point"}},
	}}}
	h, _, _ := newTestAPI(t, f)
	rec := doAPI(h, "/api/v1/assessment?lat=52.52&lon=13.405", authed(nil))
	var resp apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	v := resp.Verdict
	if v.Flyable || !v.WeatherFlyable || v.Airspace != "orange" || len(v.Reasons) == 0 {
		t.Errorf("verdict %+v", v)
	}
	if len(resp.Zones) != 1 || resp.Zones[0].Areas[0].Name != "CTR Berlin" || resp.Zones[0].Areas[0].Geometry != nil {
		t.Errorf("zones %+v", resp.Zones)
	}
}

// DiPUL returns control zones as U_CTR (e.g. Hamburg city centre, inside the
// Hamburg CTR but away from the AIRPORT zone); it must override the weather
// verdict just like CONTROL_ZONE.
func TestEvaluateUCTRIsControlledAirspace(t *testing.T) {
	f := defaultFetched()
	f.zones = []api.AffectedArea{{TypeCode: "U_CTR"}, {TypeCode: "POLICE"}}
	ev := evaluate(f)
	if !ev.hasOrangeZone || ev.assessment.Flyable || !ev.weatherFlyable {
		t.Errorf("U_CTR: orange=%v flyable=%v weatherFlyable=%v", ev.hasOrangeZone, ev.assessment.Flyable, ev.weatherFlyable)
	}
	if got := zoneSeverity("U_CTR"); got != "orange" {
		t.Errorf("zoneSeverity(U_CTR) = %q", got)
	}
}

func TestAPIUTMUnavailable(t *testing.T) {
	f := defaultFetched()
	f.utm = nil
	f.errs[0] = errors.New("boom")
	h, _, _ := newTestAPI(t, f)
	rec := doAPI(h, "/api/v1/assessment?lat=52.52&lon=13.405", authed(nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", rec.Code)
	}
}

func TestAPISpoofedIdentitySanitized(t *testing.T) {
	h, _, logBuf := newTestAPI(t, defaultFetched())
	doAPI(h, "/api/v1/assessment?lat=52.52&lon=13.405",
		authed(map[string]string{headerGatewayIdentity: "evil\nidentity"}))
	if m := decodeLog(t, logBuf); m["identity"] != "unknown" {
		t.Errorf("identity %v", m["identity"])
	}
}

func TestFetchAllCacheHitWithoutTraffic(t *testing.T) {
	rh := &ResultsHandler{cache: newResultCache(10, 1<<62)}
	cached := &allFetched{utm: calmUTM()} // cached by a traffic-less API call
	rh.cache.set(cacheKey(52.52, 13.405), cached)

	got, hit := rh.fetchAll(52.52, 13.405, "", false)
	if !hit || got != cached {
		t.Fatal("expected the cached pointer for a traffic-less request")
	}
	// The withTraffic=true top-up path calls OpenSky over the network and is
	// not exercised here; it works on a copy, so the entry must stay as cached.
	if cached.trafficFetched {
		t.Error("cache entry mutated")
	}
}
