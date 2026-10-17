package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// captured holds what the mock gateway saw on the last request.
type captured struct {
	method string
	path   string
	query  url.Values
	apiKey string
}

// newMockGateway starts a fake gateway that records the incoming request and
// replies with body. It points GATEWAY_BASE_URL at itself for the test.
func newMockGateway(t *testing.T, body string) *captured {
	t.Helper()
	c := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.method = r.Method
		c.path = r.URL.Path
		c.query = r.URL.Query()
		c.apiKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GATEWAY_BASE_URL", srv.URL)
	// gatewayHTTPClient is memoised; reset so each test gets a fresh client
	// that honours the current GATEWAY_INSECURE_TLS value.
	gatewayClientOnce = sync.Once{}
	gatewayCooldownUntil.Store(0) // don't let a prior 429 test short-circuit this one
	return c
}

func assertNoProviderKeys(t *testing.T, c *captured) {
	t.Helper()
	for _, leaked := range []string{"apiKey", "appid"} {
		if v := c.query.Get(leaked); v != "" {
			t.Errorf("provider key %q leaked in query string: %q", leaked, v)
		}
	}
}

func TestGeocodeRoutesThroughGateway(t *testing.T) {
	c := newMockGateway(t, `{"items":[{"title":"Berlin","position":{"lat":52.52,"lng":13.405},"address":{"city":"Berlin"}}]}`)

	res, err := Geocode("Berlin", "here-test-key")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if c.method != http.MethodGet {
		t.Errorf("method = %s, want GET", c.method)
	}
	if c.path != "/here/geocode/v1/geocode" {
		t.Errorf("path = %s, want /here/geocode/v1/geocode", c.path)
	}
	if c.apiKey != "here-test-key" {
		t.Errorf("X-API-Key = %q, want here-test-key", c.apiKey)
	}
	if got := c.query.Get("q"); got != "Berlin" {
		t.Errorf("q = %q, want Berlin", got)
	}
	assertNoProviderKeys(t, c)
	if res.City != "Berlin" {
		t.Errorf("City = %q, want Berlin", res.City)
	}
}

func TestReverseGeocodeRoutesThroughGateway(t *testing.T) {
	c := newMockGateway(t, `{"items":[{"title":"Somewhere","position":{"lat":52.52,"lng":13.405},"address":{"city":"Berlin"}}]}`)

	if _, err := ReverseGeocode(52.52, 13.405, "here-test-key"); err != nil {
		t.Fatalf("ReverseGeocode: %v", err)
	}
	if c.path != "/here/revgeocode/v1/revgeocode" {
		t.Errorf("path = %s, want /here/revgeocode/v1/revgeocode", c.path)
	}
	if c.apiKey != "here-test-key" {
		t.Errorf("X-API-Key = %q, want here-test-key", c.apiKey)
	}
	if got := c.query.Get("at"); got != "52.520000,13.405000" {
		t.Errorf("at = %q", got)
	}
	assertNoProviderKeys(t, c)
}

func TestFetchOpenWeatherRoutesThroughGateway(t *testing.T) {
	c := newMockGateway(t, `{"current":{"temp":20}}`)

	if _, err := FetchOpenWeather(52.52, 13.405, "owm-test-key"); err != nil {
		t.Fatalf("FetchOpenWeather: %v", err)
	}
	if c.path != "/owm/data/3.0/onecall" {
		t.Errorf("path = %s, want /owm/data/3.0/onecall", c.path)
	}
	if c.apiKey != "owm-test-key" {
		t.Errorf("X-API-Key = %q, want owm-test-key", c.apiKey)
	}
	assertNoProviderKeys(t, c)
}

func TestGatewayBaseURLMissing(t *testing.T) {
	t.Setenv("GATEWAY_BASE_URL", "")
	if _, err := Geocode("Berlin", "k"); err == nil {
		t.Fatal("expected error when GATEWAY_BASE_URL is unset")
	}
}

// A genuine "no such address" must be reported as ErrNotFound so the handler
// can tell the user to check their input.
func TestGeocodeNoMatchIsNotFound(t *testing.T) {
	newMockGateway(t, `{"items":[]}`)

	_, err := Geocode("Nowhere-XYZ", "here-test-key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A gateway/upstream failure must NOT look like ErrNotFound — the handler shows
// a "temporarily unavailable" message for these instead.
func TestGeocodeUpstreamErrorIsNotErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GATEWAY_BASE_URL", srv.URL)
	gatewayClientOnce = sync.Once{}

	_, err := Geocode("Berlin", "here-test-key")
	if err == nil {
		t.Fatal("expected error on 502")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("502 was misclassified as ErrNotFound: %v", err)
	}
}

// A 429 is reported as ErrRateLimited, and the subsequent call short-circuits
// via the process-wide cooldown instead of hitting the gateway again.
func TestGeocodeRateLimitedTriggersCooldown(t *testing.T) {
	gatewayCooldownUntil.Store(0)
	t.Cleanup(func() { gatewayCooldownUntil.Store(0) })

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "30")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GATEWAY_BASE_URL", srv.URL)
	gatewayClientOnce = sync.Once{}

	if _, err := Geocode("Berlin", "k"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("first call err = %v, want ErrRateLimited", err)
	}
	if _, err := Geocode("Berlin", "k"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second call err = %v, want ErrRateLimited", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("gateway hit %d times, want 1 (cooldown should short-circuit the rest)", got)
	}
	if d := gatewayCooldownRemaining(); d <= 0 || d > 30*time.Second {
		t.Fatalf("cooldown remaining = %s, want (0, 30s]", d)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{
		"5":     5 * time.Second,
		" 12 ":  12 * time.Second,
		"":      0,
		"0":     0,
		"-3":    0,
		"later": 0, // HTTP-date form not supported; treated as 0
	}
	for in, want := range cases {
		if got := parseRetryAfter(in); got != want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", in, got, want)
		}
	}
}

// When the gateway is unreachable, the call fails (not ErrNotFound).
func TestGeocodeGatewayDownIsNotErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now
	t.Setenv("GATEWAY_BASE_URL", url)
	gatewayClientOnce = sync.Once{}

	_, err := Geocode("Berlin", "here-test-key")
	if err == nil {
		t.Fatal("expected error when gateway is down")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("connection failure misclassified as ErrNotFound: %v", err)
	}
}
