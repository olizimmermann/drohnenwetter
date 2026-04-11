package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// openskyRadius is the bounding box half-extent in decimal degrees (~11 km).
const openskyRadius = 0.1

// ── OAuth2 token cache ────────────────────────────────────────────────────────

type openskyTokenCache struct {
	mu          sync.Mutex
	token       string
	expires     time.Time
	retryAfter  time.Time // backoff after auth failure
}

var openskyToken openskyTokenCache

const openskyTokenURL = "https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token"

func getOpenskyToken() (string, error) {
	openskyToken.mu.Lock()
	defer openskyToken.mu.Unlock()

	if openskyToken.token != "" && time.Now().Before(openskyToken.expires) {
		return openskyToken.token, nil
	}

	clientID := os.Getenv("OPENSKY_CLIENT_ID")
	clientSecret := os.Getenv("OPENSKY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return "", nil // no credentials — fall back to anonymous
	}

	// Don't hammer the auth server after a recent failure.
	if !openskyToken.retryAfter.IsZero() && time.Now().Before(openskyToken.retryAfter) {
		return "", nil // anonymous fallback during backoff
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	req, err := newFormPost(openskyTokenURL, form)
	if err != nil {
		return "", fmt.Errorf("OpenSky token request: %w", err)
	}

	body, err := doRequest(req)
	if err != nil {
		log.Printf("[opensky] token fetch failed (%v) — falling back to anonymous for 5 min", err)
		openskyToken.retryAfter = time.Now().Add(5 * time.Minute)
		return "", nil // graceful fallback, no error propagated
	}

	var resp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.AccessToken == "" {
		return "", fmt.Errorf("OpenSky token parse: %w", err)
	}

	// Cache the token, refreshing 30s before expiry.
	// If the server returns a very short-lived token, don't cache it at all
	// to avoid an immediate-expiry loop.
	ttl := time.Duration(resp.ExpiresIn-30)*time.Second - time.Duration(rand.Intn(30))*time.Second
	if ttl <= 0 {
		return resp.AccessToken, nil
	}
	openskyToken.token = resp.AccessToken
	openskyToken.expires = time.Now().Add(ttl)
	return openskyToken.token, nil
}

func openskyHeaders() (map[string]string, error) {
	token, err := getOpenskyToken()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil // anonymous — no headers needed
	}
	return map[string]string{"Authorization": "Bearer " + token}, nil
}

// ── Types ─────────────────────────────────────────────────────────────────────

// AircraftState holds the fields we care about from an OpenSky state vector.
type AircraftState struct {
	ICAO24   string  `json:"icao24"`
	Callsign string  `json:"callsign"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	BaroAlt  float64 `json:"baroAlt"`  // metres; 0 when null
	Velocity float64 `json:"velocity"` // m/s; 0 when null
	TruTrack float64 `json:"truTrack"` // degrees from north; 0 when null
	OnGround bool    `json:"onGround"`
	Category int     `json:"category"` // 8 = rotorcraft, 9 = glider, 14 = UAV
}

type openskyResponse struct {
	Time   int64           `json:"time"`
	States [][]interface{} `json:"states"`
}

// TrackPoint is one waypoint in a flight track.
type TrackPoint struct {
	Time     int64   `json:"time"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	BaroAlt  float64 `json:"baroAlt"`
	TruTrack float64 `json:"truTrack"`
	OnGround bool    `json:"onGround"`
}

// FlightTrack is the route of a single aircraft.
type FlightTrack struct {
	ICAO24    string       `json:"icao24"`
	Callsign  string       `json:"callsign"`
	StartTime int64        `json:"startTime"`
	EndTime   int64        `json:"endTime"`
	Path      []TrackPoint `json:"path"`
}

// ── API calls ─────────────────────────────────────────────────────────────────

// FetchTrack fetches the current flight track for the given ICAO24 address.
func FetchTrack(icao24 string) (*FlightTrack, error) {
	headers, err := openskyHeaders()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://opensky-network.org/api/tracks?icao24=%s&time=0", icao24)
	body, err := doGet(apiURL, headers)
	if err != nil {
		return nil, fmt.Errorf("OpenSky track: %w", err)
	}

	var raw struct {
		ICAO24    string          `json:"icao24"`
		Callsign  string          `json:"callsign"`
		StartTime int64           `json:"startTime"`
		EndTime   int64           `json:"endTime"`
		Path      [][]interface{} `json:"path"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("OpenSky track parse: %w", err)
	}

	track := &FlightTrack{
		ICAO24:    raw.ICAO24,
		Callsign:  strings.TrimSpace(raw.Callsign),
		StartTime: raw.StartTime,
		EndTime:   raw.EndTime,
		Path:      make([]TrackPoint, 0, len(raw.Path)),
	}

	for _, p := range raw.Path {
		if len(p) < 6 {
			continue
		}
		pt := TrackPoint{}
		if v, ok := p[0].(float64); ok {
			pt.Time = int64(v)
		}
		if v, ok := p[1].(float64); ok {
			pt.Lat = v
		}
		if v, ok := p[2].(float64); ok {
			pt.Lon = v
		}
		if v, ok := p[3].(float64); ok {
			pt.BaroAlt = v
		}
		if v, ok := p[4].(float64); ok {
			pt.TruTrack = v
		}
		if v, ok := p[5].(bool); ok {
			pt.OnGround = v
		}
		if pt.Lat == 0 && pt.Lon == 0 {
			continue
		}
		track.Path = append(track.Path, pt)
	}
	return track, nil
}

// FetchNearbyTraffic fetches current aircraft state vectors within ~11 km of lat/lon.
func FetchNearbyTraffic(lat, lon float64) ([]AircraftState, error) {
	headers, err := openskyHeaders()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf(
		"https://opensky-network.org/api/states/all?lamin=%.4f&lomin=%.4f&lamax=%.4f&lomax=%.4f&extended=1",
		lat-openskyRadius, lon-openskyRadius,
		lat+openskyRadius, lon+openskyRadius,
	)

	body, err := doGet(apiURL, headers)
	if err != nil {
		return nil, fmt.Errorf("OpenSky: %w", err)
	}

	var resp openskyResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("OpenSky parse: %w", err)
	}

	aircraft := make([]AircraftState, 0, len(resp.States))
	for _, s := range resp.States {
		if len(s) < 17 {
			continue
		}
		a := AircraftState{}
		if v, ok := s[0].(string); ok {
			a.ICAO24 = v
		}
		if v, ok := s[1].(string); ok {
			a.Callsign = strings.TrimSpace(v)
		}
		if v, ok := s[6].(float64); ok {
			a.Lat = v
		}
		if v, ok := s[5].(float64); ok {
			a.Lon = v
		}
		if v, ok := s[7].(float64); ok {
			a.BaroAlt = v
		}
		if v, ok := s[9].(float64); ok {
			a.Velocity = v
		}
		if v, ok := s[10].(float64); ok {
			a.TruTrack = v
		}
		if v, ok := s[8].(bool); ok {
			a.OnGround = v
		}
		if len(s) > 17 {
			if v, ok := s[17].(float64); ok {
				a.Category = int(v)
			}
		}
		if a.Lat == 0 && a.Lon == 0 {
			continue
		}
		aircraft = append(aircraft, a)
	}
	return aircraft, nil
}
