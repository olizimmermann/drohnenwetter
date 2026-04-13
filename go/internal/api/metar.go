package api

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CloudBaseResult holds nearest-airport METAR observation + TAF forecast data.
// All int fields use -1 as "not parseable / absent".
type CloudBaseResult struct {
	ICAO        string
	AirportName string

	// TAF forecast (lowest BKN/OVC across forecast period)
	CloudBaseFt int     // feet
	CloudBaseM  float64 // metres
	Available   bool    // TAF cloud-base parsed successfully

	// METAR current observation
	MetarAvailable   bool
	MetarCloudBaseFt int
	MetarCloudBaseM  float64
	MetarVisibilityM  int     // metres; 9999 means ≥10 km
	MetarVisibilityKm float64 // derived for display

	// TAF worst-case visibility across forecast period
	TafMinVisibilityM  int
	TafMinVisibilityKm float64
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
	// Each METAR/TAF block lives on a single line in the source HTML, so
	// terminating at a newline keeps footer prose ("mehr als 4000 Flughäfen")
	// out of the captured section.
	metarSecRe = regexp.MustCompile(`(?s)<b>METAR:</b>(.*?)(?:\n|<b>|</div>|</p>|$)`)
	tafSecRe   = regexp.MustCompile(`(?s)<b>TAF:</b>(.*?)(?:\n|<b>|</div>|</p>|$)`)
	cloudRe    = regexp.MustCompile(`(?:BKN|OVC)(\d{3})`)
	// Date/validity groups like 1318/1418 — stripped before visibility parsing.
	dateGroupRe = regexp.MustCompile(`\d{4}/\d{4}`)
	// Standalone 4-digit tokens: visibility in metres (0000–9999). \b boundaries
	// allow back-to-back tokens (e.g. "9999 4000 5000") without consuming the
	// separating whitespace, which a (^|\s)(\s|$) regex would.
	vis4Re = regexp.MustCompile(`\b\d{4}\b`)
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
	body, err := doGet(allmetBase+"?icao="+url.QueryEscape(icao), allmetHeaders)
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

// parseVisibility finds standalone 4-digit metre tokens (e.g. "9999", "4000")
// in a METAR/TAF string and returns the MINIMUM (worst-case). Date/validity
// groups like "1318/1418" are stripped first so they don't poison the match.
// Returns -1 if none found.
func parseVisibility(text string) int {
	cleaned := dateGroupRe.ReplaceAllString(text, " ")
	matches := vis4Re.FindAllString(cleaned, -1)
	if len(matches) == 0 {
		return -1
	}
	min := -1
	for _, tok := range matches {
		val := 0
		for _, c := range tok {
			val = val*10 + int(c-'0')
		}
		// Plausibility: visibility in METAR/TAF fits 0000–9999.
		if val < 0 || val > 9999 {
			continue
		}
		if min == -1 || val < min {
			min = val
		}
	}
	return min
}

// extractSection pulls the text from a <b>LABEL:</b> … block and strips HTML tags.
func extractSection(page string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(tagRe.ReplaceAllString(m[1], " "))
}

// FetchCloudBase looks up the nearest airport for city and returns the
// METAR observation + TAF forecast cloud base and visibility. Always returns
// a non-nil result; check Available / MetarAvailable fields.
func FetchCloudBase(city string) *CloudBaseResult {
	res := &CloudBaseResult{
		CloudBaseFt:       -1,
		MetarCloudBaseFt:  -1,
		MetarVisibilityM:  -1,
		TafMinVisibilityM: -1,
	}
	icao, airportName := findAirport(city)
	if icao == "" {
		return res
	}
	res.ICAO = icao
	res.AirportName = airportName

	page, err := fetchTAFPage(icao)
	if err != nil {
		return res
	}

	// ── METAR section (current observation) ──────────────────────────────
	if metarText := extractSection(page, metarSecRe); metarText != "" {
		res.MetarVisibilityM = parseVisibility(metarText)
		if res.MetarVisibilityM >= 0 {
			res.MetarVisibilityKm = float64(res.MetarVisibilityM) / 1000
		}
		if ft := parseCloudBase(metarText); ft >= 0 {
			res.MetarCloudBaseFt = ft
			res.MetarCloudBaseM = math.Round(float64(ft) * 0.3048)
			res.MetarAvailable = true
		} else if res.MetarVisibilityM >= 0 {
			// No cloud layer reported (clear) but we still have a valid METAR.
			res.MetarAvailable = true
		}
	}

	// ── TAF section (forecast) ────────────────────────────────────────────
	if tafText := extractSection(page, tafSecRe); tafText != "" {
		res.TafMinVisibilityM = parseVisibility(tafText)
		if res.TafMinVisibilityM >= 0 {
			res.TafMinVisibilityKm = float64(res.TafMinVisibilityM) / 1000
		}
		if ft := parseCloudBase(tafText); ft >= 0 {
			res.CloudBaseFt = ft
			res.CloudBaseM = math.Round(float64(ft) * 0.3048)
			res.Available = true
		}
	}

	return res
}
