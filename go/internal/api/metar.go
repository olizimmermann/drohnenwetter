package api

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CloudBaseResult holds the nearest airport METAR cloud base for a city.
type CloudBaseResult struct {
	ICAO        string
	AirportName string
	CloudBaseFt int     // feet; -1 = not parseable
	CloudBaseM  float64 // metres
	Available   bool
}

// ── Airport list cache ────────────────────────────────────────────────────────

type airportListCache struct {
	mu      sync.Mutex
	data    map[string]string // display name → ICAO code
	expires time.Time
}

var airportCache airportListCache

var (
	optionRe  = regexp.MustCompile(`<option[^>]+value="([^"]*)"[^>]*>\s*([^<]+?)\s*</option>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
	tafSecRe  = regexp.MustCompile(`(?s)<b>TAF:</b>(.*?)(?:<b>|</div>|</p>|$)`)
	cloudRe   = regexp.MustCompile(`(?:BKN|OVC)(\d{3})`)
)

const allmetBase = "https://de.allmetsat.com/metar-taf/deutschland.php"

var allmetHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	"Accept-Language": "en-US,en;q=0.9",
	"Referer":         allmetBase,
}

func isICAO(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

func getAirportList() (map[string]string, error) {
	airportCache.mu.Lock()
	defer airportCache.mu.Unlock()

	if airportCache.data != nil && time.Now().Before(airportCache.expires) {
		return airportCache.data, nil
	}

	body, err := doGet(allmetBase, allmetHeaders)
	if err != nil {
		return nil, fmt.Errorf("airport list: %w", err)
	}

	result := make(map[string]string)
	for _, m := range optionRe.FindAllSubmatch(body, -1) {
		val := string(m[1])
		name := strings.TrimSpace(string(m[2]))
		if len(val) >= 4 {
			icao := strings.ToUpper(val[len(val)-4:])
			if isICAO(icao) {
				result[name] = icao
			}
		}
	}

	airportCache.data = result
	airportCache.expires = time.Now().Add(4 * time.Hour)
	return result, nil
}

// findAirport does a case-insensitive substring match: city name in airport name.
// Returns (ICAO, display name) or ("", "").
func findAirport(city string) (string, string) {
	if city == "" {
		return "", ""
	}
	list, err := getAirportList()
	if err != nil || len(list) == 0 {
		return "", ""
	}
	city = strings.ToLower(city)
	for name, icao := range list {
		if strings.Contains(strings.ToLower(name), city) {
			return icao, name
		}
	}
	return "", ""
}

// ── TAF fetch + parse ─────────────────────────────────────────────────────────

func fetchTAFPage(icao string) (string, error) {
	body, err := doGet(allmetBase+"?icao="+icao, allmetHeaders)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// parseCloudBase finds the lowest BKN or OVC layer in a TAF/METAR string.
// Returns feet, or -1 if none found.
func parseCloudBase(text string) int {
	matches := cloudRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return -1
	}
	min := -1
	for _, m := range matches {
		val := 0
		for _, c := range m[1] {
			val = val*10 + int(c-'0')
		}
		if min == -1 || val < min {
			min = val
		}
	}
	return min * 100 // hundreds of feet → feet
}

// FetchCloudBase looks up the nearest airport for city and returns the TAF cloud base.
// Always returns a non-nil result; check Available field.
func FetchCloudBase(city string) *CloudBaseResult {
	icao, airportName := findAirport(city)
	if icao == "" {
		return &CloudBaseResult{Available: false}
	}

	page, err := fetchTAFPage(icao)
	if err != nil {
		return &CloudBaseResult{ICAO: icao, AirportName: airportName, Available: false}
	}

	// Extract the TAF text block (strip inner HTML tags, e.g. <br/>)
	tafMatch := tafSecRe.FindStringSubmatch(page)
	if tafMatch == nil {
		return &CloudBaseResult{ICAO: icao, AirportName: airportName, Available: false}
	}
	tafText := strings.TrimSpace(tagRe.ReplaceAllString(tafMatch[1], " "))

	cloudFt := parseCloudBase(tafText)
	if cloudFt < 0 {
		return &CloudBaseResult{ICAO: icao, AirportName: airportName, Available: false}
	}

	return &CloudBaseResult{
		ICAO:        icao,
		AirportName: airportName,
		CloudBaseFt: cloudFt,
		CloudBaseM:  math.Round(float64(cloudFt) * 0.3048),
		Available:   true,
	}
}
