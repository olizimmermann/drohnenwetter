package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Two further DiPUL services used by the official maptool, authorised with
// the same anonymous token as the zone API:
//   - the elevation service: ground height above mean sea level (DGM, ±3 m);
//   - the ED-318 zone export: per-zone links to the legal basis and, for
//     control zones, the zone floor in AMSL, which DiPUL sets to
//     "aerodrome elevation + 25 / 45 / 100 m" for UAS zones 2 / 3 / 4
//     (NfL 2026-1-3960). Together they give the exact height limit.

const (
	dipulAltitudeURL = "https://uas-betrieb.dfs.de/api/altitude/v2/height"
	dipulUASZonesURL = "https://dipul-service.dfs.de/api/geofencing/v2/uas-zones"
)

// Elevation is the ground height at a point.
type Elevation struct {
	MSL  float64 // metres above mean sea level
	RMSE float64 // stated accuracy in metres (DiPUL reports "3m")
}

// UASZone is the part of an ED-318 zone the app uses.
type UASZone struct {
	ID       string // equals externalReference in the zone API (not for CTRs)
	Name     string
	TypeCode string
	LinkDE   string
	LinkEN   string
	// Vertical layer, converted to metres.
	LowerM   float64
	LowerRef string // "AGL" or "AMSL"
}

func pointBody(lat, lon float64) map[string]interface{} {
	return map[string]interface{}{"type": "Point", "coordinates": []float64{lon, lat}}
}

// withDipulToken runs call with the cached token and retries once with a
// fresh token on HTTP 401, like FetchAllZoneDetails.
func withDipulToken(call func(token string) ([]byte, error)) ([]byte, error) {
	token, err := getToken()
	if err != nil {
		return nil, err
	}
	body, err := call(token)
	if isUnauthorized(err) {
		invalidateDipulToken()
		if token, err = getToken(); err != nil {
			return nil, err
		}
		body, err = call(token)
	}
	return body, err
}

// FetchGroundElevation returns the terrain height at lat/lon.
func FetchGroundElevation(lat, lon float64) (*Elevation, error) {
	req, err := json.Marshal(map[string]interface{}{
		"aggregationType": "max",
		"dest":            []map[string]string{{"Ref": "MSL", "Unit": "m"}},
		"geometry":        pointBody(lat, lon),
	})
	if err != nil {
		return nil, err
	}
	body, err := withDipulToken(func(token string) ([]byte, error) {
		return doPost(dipulAltitudeURL, req, dipulHeaders(token))
	})
	if err != nil {
		return nil, fmt.Errorf("DiPUL elevation: %w", err)
	}

	var resp struct {
		Features []struct {
			Properties struct {
				Val  *float64 `json:"Val"`
				Ref  string   `json:"Ref"`
				Unit string   `json:"Unit"`
				RMSE string   `json:"RMSE"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("DiPUL elevation parse: %w", err)
	}
	if len(resp.Features) == 0 || resp.Features[0].Properties.Val == nil {
		return nil, fmt.Errorf("DiPUL elevation: no value")
	}
	p := resp.Features[0].Properties
	if p.Ref != "MSL" || p.Unit != "m" {
		return nil, fmt.Errorf("DiPUL elevation: unexpected reference %s/%s", p.Ref, p.Unit)
	}
	e := &Elevation{MSL: *p.Val, RMSE: 3}
	if v, err := strconv.ParseFloat(strings.TrimSuffix(p.RMSE, "m"), 64); err == nil && v > 0 {
		e.RMSE = v
	}
	return e, nil
}

// FetchUASZones returns the ED-318 zones at lat/lon.
func FetchUASZones(lat, lon float64) ([]UASZone, error) {
	req, err := json.Marshal(map[string]interface{}{"geometry": pointBody(lat, lon)})
	if err != nil {
		return nil, err
	}
	body, err := withDipulToken(func(token string) ([]byte, error) {
		return doPost(dipulUASZonesURL, req, dipulHeaders(token))
	})
	if err != nil {
		return nil, fmt.Errorf("DiPUL UAS zones: %w", err)
	}
	return parseUASZones(body)
}

type edText struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
}

func parseUASZones(body []byte) ([]UASZone, error) {
	var resp struct {
		UASZones struct {
			Features []struct {
				Geometry struct {
					Layer *struct {
						Lower          float64 `json:"lower"`
						LowerReference string  `json:"lowerReference"`
						UOM            string  `json:"uom"`
					} `json:"layer"`
				} `json:"geometry"`
				Properties struct {
					Identifier         string   `json:"identifier"`
					Name               []edText `json:"name"`
					ExtendedProperties struct {
						TypeCode string `json:"typeCode"`
						Links    []struct {
							Lang string `json:"lang"`
							URL  string `json:"url"`
						} `json:"links"`
					} `json:"extendedProperties"`
				} `json:"properties"`
			} `json:"features"`
		} `json:"uasZones"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("DiPUL UAS zones parse: %w", err)
	}

	zones := make([]UASZone, 0, len(resp.UASZones.Features))
	for _, f := range resp.UASZones.Features {
		p := f.Properties
		z := UASZone{ID: p.Identifier, TypeCode: p.ExtendedProperties.TypeCode}
		for _, n := range p.Name {
			if z.Name == "" || strings.HasPrefix(n.Lang, "de") {
				z.Name = n.Text
			}
		}
		for _, l := range p.ExtendedProperties.Links {
			switch {
			case strings.HasPrefix(l.Lang, "de") && strings.HasPrefix(l.URL, "https://"):
				z.LinkDE = l.URL
			case strings.HasPrefix(l.Lang, "en") && strings.HasPrefix(l.URL, "https://"):
				z.LinkEN = l.URL
			}
		}
		if l := f.Geometry.Layer; l != nil {
			z.LowerRef = l.LowerReference
			z.LowerM = l.Lower
			if strings.EqualFold(l.UOM, "ft") {
				z.LowerM = l.Lower * 0.3048
			}
		}
		zones = append(zones, z)
	}
	return zones, nil
}
