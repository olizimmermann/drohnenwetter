package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// UTM (DFS) types

type UTMHeight struct {
	Reference string  `json:"reference"`
	Unit      string  `json:"unit"`
	Value     float64 `json:"value"`
}

type UTMTemperature struct {
	Height UTMHeight `json:"height"`
	Unit   string    `json:"unit"`
	Value  float64   `json:"value"`
}

type UTMWind struct {
	Height        UTMHeight `json:"height"`
	Unit          string    `json:"unit"`
	VComponent    float64   `json:"vComponent"`
	UComponent    float64   `json:"uComponent"`
	WindSpeedGust float64   `json:"windSpeedGust"`
}

type UTMPrecip struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

type UTMForecast struct {
	Temperature      []UTMTemperature `json:"temperature"`
	Wind             []UTMWind        `json:"wind"`
	RainPrecipitation UTMPrecip       `json:"rainPrecipitation"`
	SnowPrecipitation UTMPrecip       `json:"snowPrecipitation"`
	TotalCloudCover  UTMPrecip        `json:"totalCloudCover"`
}

type UTMPosition struct {
	Forecasts []UTMForecast `json:"forecasts"`
}

type UTMResponse struct {
	Positions []UTMPosition `json:"positions"`
}

// OpenWeatherMap types

type OWCurrent struct {
	Temp     float64 `json:"temp"`
	DewPoint float64 `json:"dew_point"`
	Humidity int     `json:"humidity"`
	WindSpeed float64 `json:"wind_speed"`
	WindGust  float64 `json:"wind_gust"`
}

type OWResponse struct {
	Current OWCurrent `json:"current"`
}

func FetchUTMForecast(lat, lon float64) (*UTMResponse, error) {
	berlin := time.FixedZone("CET", 1*60*60)
	now := time.Now().In(berlin)
	forecasts := []string{
		now.Format("2006-01-02T15:04:05") + "Z",
		now.Add(1 * time.Hour).Format("2006-01-02T15:04:05") + "Z",
		now.Add(2 * time.Hour).Format("2006-01-02T15:04:05") + "Z",
	}

	payload := map[string]interface{}{
		"positions": []map[string]interface{}{
			{
				"longitude": lon,
				"latitude":  lat,
				"forecasts": forecasts,
				"temperature": map[string]interface{}{
					"unit": "C",
					"heights": []map[string]interface{}{
						{"reference": "AGL", "unit": "m", "value": 2},
						{"reference": "AGL", "unit": "m", "value": 50},
						{"reference": "AGL", "unit": "m", "value": 100},
						{"reference": "AGL", "unit": "m", "value": 150},
					},
				},
				"wind": map[string]interface{}{
					"unit": "m/s",
					"heights": []map[string]interface{}{
						{"reference": "AGL", "unit": "m", "value": 10},
						{"reference": "AGL", "unit": "m", "value": 50},
						{"reference": "AGL", "unit": "m", "value": 100},
						{"reference": "AGL", "unit": "m", "value": 150},
					},
				},
				"rainPrecipitation": map[string]string{"unit": "mm"},
				"snowPrecipitation": map[string]string{"unit": "cm"},
				"totalCloudCover":   map[string]string{"unit": "%"},
				"cloudCover": map[string]interface{}{
					"unit": "%",
					"heights": []map[string]interface{}{
						{"reference": "AGL", "unit": "m", "value": 50},
						{"reference": "AGL", "unit": "m", "value": 100},
						{"reference": "AGL", "unit": "m", "value": 150},
					},
				},
				"airHumidity": map[string]interface{}{
					"unit": "%",
					"heights": []map[string]interface{}{
						{"reference": "AGL", "unit": "m", "value": 50},
						{"reference": "AGL", "unit": "m", "value": 100},
						{"reference": "AGL", "unit": "m", "value": 150},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "en-US,en;q=0.9",
		"Origin":          "https://utm.dfs.de",
		"Referer":         "https://utm.dfs.de/",
	}

	respBody, err := doPost("https://utm-service.dfs.de/api/weather/v1/weather", body, headers)
	if err != nil {
		return nil, fmt.Errorf("UTM forecast: %w", err)
	}

	var result UTMResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("UTM parse: %w", err)
	}
	return &result, nil
}

func FetchOpenWeather(lat, lon float64, token string) (*OWResponse, error) {
	params := url.Values{}
	params.Set("lat", fmt.Sprintf("%f", lat))
	params.Set("lon", fmt.Sprintf("%f", lon))
	params.Set("exclude", "minutely,hourly,daily,alerts")
	params.Set("appid", token)
	params.Set("units", "metric")

	rawURL := "https://api.openweathermap.org/data/3.0/onecall?" + params.Encode()

	body, err := doGet(rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("OpenWeather: %w", err)
	}

	var result OWResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("OpenWeather parse: %w", err)
	}
	return &result, nil
}

func FetchKpIndex() (float64, error) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z")
	end := now.Format("2006-01-02T15:04:05Z")

	// No &status=def — definitive values have a multi-day delay; use all available.
	rawURL := fmt.Sprintf(
		"https://kp.gfz-potsdam.de/app/json/?start=%s&end=%s&index=Kp",
		start, end,
	)

	body, err := doGet(rawURL, map[string]string{"Accept": "application/json"})
	if err != nil {
		return 0, fmt.Errorf("Kp fetch: %w", err)
	}

	var resp struct {
		Kp []float64 `json:"Kp"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("Kp parse: %w", err)
	}
	if len(resp.Kp) == 0 {
		return 0, fmt.Errorf("Kp: empty response")
	}
	return resp.Kp[len(resp.Kp)-1], nil
}
