package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// Token cache — one anonymous token is valid for ~30min.
type tokenCache struct {
	mu      sync.Mutex
	token   string
	expires time.Time
}

var dipulToken tokenCache

func getToken() (string, error) {
	dipulToken.mu.Lock()
	defer dipulToken.mu.Unlock()

	if dipulToken.token != "" && time.Now().Before(dipulToken.expires) {
		return dipulToken.token, nil
	}

	headers := map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "en-US,en;q=0.9",
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36",
		"Origin":          "https://maptool-dipul.dfs.de",
		"Referer":         "https://maptool-dipul.dfs.de/",
	}

	body, err := doGet("https://uas-betrieb.dfs.de/api/token/v1/anonymous/bmdv/token", headers)
	if err != nil {
		return "", fmt.Errorf("DiPUL token: %w", err)
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Token == "" {
		return "", fmt.Errorf("DiPUL token parse: %w", err)
	}

	dipulToken.token = resp.Token
	jitter := time.Duration(rand.Intn(60)) * time.Second
	dipulToken.expires = time.Now().Add(28*time.Minute - jitter)
	return dipulToken.token, nil
}

// geoFeatureBody builds the circle GeoJSON POST body used by DiPUL APIs.
func geoFeatureBody(lat, lon float64) ([]byte, error) {
	payload := map[string]interface{}{
		"type": "Feature",
		"properties": map[string]interface{}{
			"radius":  100,
			"subType": "Circle",
			"altitude": map[string]interface{}{
				"value":             0,
				"altitudeReference": "AGL",
				"unit":              "m",
			},
		},
		"geometry": map[string]interface{}{
			"type":        "Point",
			"coordinates": []float64{lon, lat},
		},
	}
	return json.Marshal(payload)
}

type ZoneSummary struct {
	TypeCode string `json:"typeCode"`
	Count    int    `json:"count"`
}

// AffectedArea is what the detailed area endpoint returns per typeCode.
type AffectedArea struct {
	TypeCode     string                   `json:"typeCode"`
	TotalRecords int                      `json:"totalRecords"`
	Areas        []map[string]interface{} `json:"affectedAreas"`
}

func dipulHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization":   "Bearer " + token,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "en-US,en;q=0.9",
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.70 Safari/537.36",
		"Origin":          "https://maptool-dipul.dfs.de",
		"Referer":         "https://maptool-dipul.dfs.de/",
	}
}

func fetchAffectedAreaCodes(lat, lon float64, token string) ([]string, error) {
	body, err := geoFeatureBody(lat, lon)
	if err != nil {
		return nil, err
	}

	respBody, err := doPost(
		"https://dipul-service.dfs.de/api/geoapi/dipul/v2/affectedAreas/typeCode/count",
		body,
		dipulHeaders(token),
	)
	if err != nil {
		return nil, fmt.Errorf("DiPUL areas: %w", err)
	}

	// Response is an object keyed by typeCode.
	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("DiPUL areas parse: %w", err)
	}

	codes := make([]string, 0, len(raw))
	for k := range raw {
		codes = append(codes, k)
	}
	return codes, nil
}

func fetchZoneDetail(typeCode string, lat, lon float64, token string) (*AffectedArea, error) {
	body, err := geoFeatureBody(lat, lon)
	if err != nil {
		return nil, err
	}

	rawURL := fmt.Sprintf(
		"https://dipul-service.dfs.de/api/geoapi/dipul/v2/affectedAreas?typeCode=%s",
		typeCode,
	)

	respBody, err := doPost(rawURL, body, dipulHeaders(token))
	if err != nil {
		return nil, fmt.Errorf("DiPUL zone %s: %w", typeCode, err)
	}

	var resp struct {
		TotalRecords  int                      `json:"totalRecords"`
		AffectedAreas []map[string]interface{} `json:"affectedAreas"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("DiPUL zone %s parse: %w", typeCode, err)
	}

	return &AffectedArea{
		TypeCode:     typeCode,
		TotalRecords: resp.TotalRecords,
		Areas:        resp.AffectedAreas,
	}, nil
}

func isUnauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), "HTTP 401 ")
}

func invalidateDipulToken() {
	dipulToken.mu.Lock()
	dipulToken.token = ""
	dipulToken.mu.Unlock()
}

// FetchAllZoneDetails fetches affected area codes then parallel-fetches each zone's detail.
// On a 401 it invalidates the cached token and retries once with a fresh one.
func FetchAllZoneDetails(lat, lon float64) ([]AffectedArea, error) {
	token, err := getToken()
	if err != nil {
		return nil, err
	}

	codes, err := fetchAffectedAreaCodes(lat, lon, token)
	if err != nil {
		if isUnauthorized(err) {
			invalidateDipulToken()
			if token, err = getToken(); err != nil {
				return nil, err
			}
			codes, err = fetchAffectedAreaCodes(lat, lon, token)
		}
		if err != nil {
			return nil, err
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}

	type result struct {
		area *AffectedArea
		err  error
	}

	ch := make(chan result, len(codes))
	for _, code := range codes {
		code := code
		go func() {
			a, e := fetchZoneDetail(code, lat, lon, token)
			ch <- result{a, e}
		}()
	}

	var areas []AffectedArea
	for range codes {
		r := <-ch
		if r.err != nil {
			if isUnauthorized(r.err) {
				invalidateDipulToken() // next request will fetch a fresh token
			}
			continue
		}
		if r.area != nil && len(r.area.Areas) > 0 {
			areas = append(areas, *r.area)
		}
	}
	return areas, nil
}
