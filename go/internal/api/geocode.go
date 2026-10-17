package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// ErrNotFound means the geocoder reached the provider successfully but found no
// match for the query. It is distinct from a transport/gateway failure, so
// callers can tell "no such address" apart from "service unreachable".
var ErrNotFound = errors.New("no match found")

type GeocodeResult struct {
	Lat         float64
	Lon         float64
	Title       string
	City        string // from HERE address.city — used for METAR airport lookup
	Street      string
	HouseNumber string
	PostalCode  string
}

type hereResponse struct {
	Items []struct {
		Title    string `json:"title"`
		Position struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"position"`
		Address struct {
			City        string `json:"city"`
			Street      string `json:"street"`
			PostalCode  string `json:"postalCode"`
			HouseNumber string `json:"houseNumber"`
		} `json:"address"`
	} `json:"items"`
}

func ReverseGeocode(lat, lon float64, hereAPIKey string) (*GeocodeResult, error) {
	params := url.Values{}
	params.Set("at", fmt.Sprintf("%f,%f", lat, lon))
	params.Set("lang", "de")

	headers := map[string]string{
		"User-Agent":      "drohnenwetter/1.0",
		"Accept":          "application/json",
		"Accept-Language": "de-DE,de;q=0.9",
	}

	body, err := gatewayGet("/here/revgeocode/v1/revgeocode?"+params.Encode(), hereAPIKey, headers)
	if err != nil {
		return nil, fmt.Errorf("revgeocode request: %w", err)
	}

	var resp hereResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("revgeocode parse: %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("revgeocode: %w", ErrNotFound)
	}

	item := resp.Items[0]
	return &GeocodeResult{
		Lat:         lat,
		Lon:         lon,
		Title:       item.Title,
		City:        item.Address.City,
		Street:      item.Address.Street,
		HouseNumber: item.Address.HouseNumber,
		PostalCode:  item.Address.PostalCode,
	}, nil
}

func Geocode(address, hereAPIKey string) (*GeocodeResult, error) {
	params := url.Values{}
	params.Set("q", address)
	params.Set("in", "countryCode:DEU")
	params.Set("limit", "1")
	params.Set("lang", "de")

	headers := map[string]string{
		"User-Agent":      "drohnenwetter/1.0",
		"Accept":          "application/json",
		"Accept-Language": "de-DE,de;q=0.9",
	}

	body, err := gatewayGet("/here/geocode/v1/geocode?"+params.Encode(), hereAPIKey, headers)
	if err != nil {
		return nil, fmt.Errorf("geocode request: %w", err)
	}

	var resp hereResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("geocode parse: %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("geocode: %w", ErrNotFound)
	}

	item := resp.Items[0]
	return &GeocodeResult{
		Lat:         item.Position.Lat,
		Lon:         item.Position.Lng,
		Title:       item.Title,
		City:        item.Address.City,
		Street:      item.Address.Street,
		HouseNumber: item.Address.HouseNumber,
		PostalCode:  item.Address.PostalCode,
	}, nil
}
