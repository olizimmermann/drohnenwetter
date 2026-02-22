package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type GeocodeResult struct {
	Lat   float64
	Lon   float64
	Title string
	City  string // from HERE address.city — used for METAR airport lookup
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

func Geocode(address, hereAPIKey string) (*GeocodeResult, error) {
	params := url.Values{}
	params.Set("q", address)
	params.Set("in", "countryCode:DEU")
	params.Set("limit", "1")
	params.Set("lang", "de")
	params.Set("apiKey", hereAPIKey)

	rawURL := "https://geocode.search.hereapi.com/v1/geocode?" + params.Encode()

	headers := map[string]string{
		"User-Agent":      "safeflight/1.0",
		"Accept":          "application/json",
		"Accept-Language": "de-DE,de;q=0.9",
	}

	body, err := doGet(rawURL, headers)
	if err != nil {
		return nil, fmt.Errorf("geocode request: %w", err)
	}

	var resp hereResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("geocode parse: %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("address not found")
	}

	item := resp.Items[0]
	return &GeocodeResult{
		Lat:   item.Position.Lat,
		Lon:   item.Position.Lng,
		Title: item.Title,
		City:  item.Address.City,
	}, nil
}
