package assessment

import (
	"testing"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

// makeUTM builds a minimal UTMResponse with one forecast.
// temps: values at heights 2, 50, 100, 150 m
// winds: vComponent values at heights 10, 50, 100, 150 m
// gust:  windSpeedGust on the first wind entry
func makeUTM(temps []float64, winds []float64, gust float64) *api.UTMResponse {
	tempHeights := []float64{2, 50, 100, 150}
	windHeights := []float64{10, 50, 100, 150}

	var temperature []api.UTMTemperature
	for i, v := range temps {
		h := tempHeights[0]
		if i < len(tempHeights) {
			h = tempHeights[i]
		}
		temperature = append(temperature, api.UTMTemperature{
			Height: api.UTMHeight{Value: h, Unit: "m", Reference: "AGL"},
			Unit:   "°C",
			Value:  v,
		})
	}

	var wind []api.UTMWind
	for i, v := range winds {
		h := windHeights[0]
		if i < len(windHeights) {
			h = windHeights[i]
		}
		g := 0.0
		if i == 0 {
			g = gust
		}
		wind = append(wind, api.UTMWind{
			Height:        api.UTMHeight{Value: h, Unit: "m", Reference: "AGL"},
			Unit:          "m/s",
			VComponent:    v,
			WindSpeedGust: g,
		})
	}

	return &api.UTMResponse{
		Positions: []api.UTMPosition{
			{
				Forecasts: []api.UTMForecast{
					{
						Temperature:       temperature,
						Wind:              wind,
						RainPrecipitation: api.UTMPrecip{Unit: "mm", Value: 0},
						SnowPrecipitation: api.UTMPrecip{Unit: "cm", Value: 0},
						TotalCloudCover:   api.UTMPrecip{Unit: "%", Value: 0},
					},
				},
			},
		},
	}
}

func makeOW(dewPoint float64) *api.OWResponse {
	return &api.OWResponse{
		Current: api.OWCurrent{DewPoint: dewPoint},
	}
}

// normal returns a UTM response with all values safely within limits.
func normal() *api.UTMResponse {
	return makeUTM([]float64{20, 18, 16, 14}, []float64{5, 5, 5, 5}, 5)
}

func TestAssess(t *testing.T) {
	tests := []struct {
		name string
		utm  *api.UTMResponse
		ow   *api.OWResponse
		kp   float64
		// expected
		flyable    bool
		dewPointOK bool
		kpOK       bool
	}{
		// ── Empty / missing data ──
		{
			name:       "nil positions",
			utm:        &api.UTMResponse{Positions: nil},
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name: "empty forecasts",
			utm: &api.UTMResponse{
				Positions: []api.UTMPosition{{Forecasts: nil}},
			},
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},

		// ── All within limits ──
		{
			name:       "all ok",
			utm:        normal(),
			ow:         makeOW(10),
			kp:         2,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "ow nil no dew warn",
			utm:        normal(),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},

		// ── Temperature boundaries ──
		{
			name:       "temp at upper bound 50",
			utm:        makeUTM([]float64{50}, []float64{5}, 5),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "temp above upper 50.1",
			utm:        makeUTM([]float64{50.1}, []float64{5}, 5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "temp at lower bound -20",
			utm:        makeUTM([]float64{-20}, []float64{5}, 5),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "temp below lower -20.1",
			utm:        makeUTM([]float64{-20.1}, []float64{5}, 5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},

		// ── Wind boundaries ──
		{
			name:       "wind at limit 12",
			utm:        makeUTM([]float64{20}, []float64{12}, 5),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "wind over limit 12.1",
			utm:        makeUTM([]float64{20}, []float64{12.1}, 5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "wind negative vcomp -11",
			utm:        makeUTM([]float64{20}, []float64{-11}, 5),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "wind negative vcomp over limit -12.1",
			utm:        makeUTM([]float64{20}, []float64{-12.1}, 5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "one altitude over limit others ok",
			utm:        makeUTM([]float64{20, 20, 20, 20}, []float64{5, 5, 12.1, 5}, 5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},

		// ── Gust boundaries ──
		{
			name:       "gust at limit 12",
			utm:        makeUTM([]float64{20}, []float64{5}, 12),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "gust over limit 12.1",
			utm:        makeUTM([]float64{20}, []float64{5}, 12.1),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: true,
			kpOK:       true,
		},

		// ── Kp-Index ──
		{
			name:       "kp at 4",
			utm:        normal(),
			ow:         nil,
			kp:         4,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "kp at 4.1",
			utm:        normal(),
			ow:         nil,
			kp:         4.1,
			flyable:    false,
			dewPointOK: true,
			kpOK:       false,
		},

		// ── Dew point / fog risk ──
		{
			name:       "dew point proximity fog risk",
			utm:        makeUTM([]float64{2}, []float64{5}, 5),
			ow:         makeOW(1),
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			kpOK:       true,
		},
		{
			name:       "dew point safe gap 3 degrees",
			utm:        makeUTM([]float64{2}, []float64{5}, 5),
			ow:         makeOW(-1),
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "dew point near but warm temp 8",
			utm:        makeUTM([]float64{8}, []float64{5}, 5),
			ow:         makeOW(7.5),
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "dew point exactly 2 degrees gap",
			utm:        makeUTM([]float64{2}, []float64{5}, 5),
			ow:         makeOW(0),
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "dew point 1.9 degrees gap triggers",
			utm:        makeUTM([]float64{2}, []float64{5}, 5),
			ow:         makeOW(0.1),
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			kpOK:       true,
		},
		{
			name:       "dew point proximity at exactly temp 3 no warn",
			utm:        makeUTM([]float64{3}, []float64{5}, 5),
			ow:         makeOW(2),
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "dew point proximity at temp 2.9 warns",
			utm:        makeUTM([]float64{2.9}, []float64{5}, 5),
			ow:         makeOW(2),
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			kpOK:       true,
		},

		// ── Combined failures ──
		{
			name:       "wind and kp both fail",
			utm:        makeUTM([]float64{20}, []float64{15}, 5),
			ow:         nil,
			kp:         5,
			flyable:    false,
			dewPointOK: true,
			kpOK:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := Assess(tc.utm, tc.ow, tc.kp)

			if a.Flyable != tc.flyable {
				t.Errorf("Flyable = %v, want %v", a.Flyable, tc.flyable)
			}
			if a.DewPointOK != tc.dewPointOK {
				t.Errorf("DewPointOK = %v, want %v", a.DewPointOK, tc.dewPointOK)
			}
			if a.KpOK != tc.kpOK {
				t.Errorf("KpOK = %v, want %v", a.KpOK, tc.kpOK)
			}
		})
	}
}

func TestAssessWindWarnZone(t *testing.T) {
	// 8.1 m/s: within limit but should trigger warn
	a := Assess(makeUTM([]float64{20}, []float64{8.1}, 5), nil, 0)
	if !a.Flyable {
		t.Error("8.1 m/s should be flyable")
	}
	if len(a.WindSpeed) == 0 {
		t.Fatal("expected wind speed entries")
	}
	if !a.WindSpeed[0].OK {
		t.Error("8.1 m/s should be OK")
	}
	if !a.WindSpeed[0].Warn {
		t.Error("8.1 m/s should have Warn=true")
	}

	// 8.0 m/s: at boundary, should NOT warn
	a = Assess(makeUTM([]float64{20}, []float64{8}, 5), nil, 0)
	if len(a.WindSpeed) == 0 {
		t.Fatal("expected wind speed entries")
	}
	if a.WindSpeed[0].Warn {
		t.Error("8.0 m/s should not have Warn=true")
	}
}

func TestAssessSortsAltitudes(t *testing.T) {
	// Pass altitudes out of order — verify output is sorted
	utm := makeUTM(
		[]float64{20, 18, 16, 14},
		[]float64{5, 6, 7, 8},
		5,
	)
	a := Assess(utm, nil, 0)

	if len(a.Temperature) < 4 {
		t.Fatalf("expected 4 temp entries, got %d", len(a.Temperature))
	}
	for i := 1; i < len(a.Temperature); i++ {
		if a.Temperature[i].Height < a.Temperature[i-1].Height {
			t.Errorf("temperature not sorted: %v at index %d < %v at index %d",
				a.Temperature[i].Height, i, a.Temperature[i-1].Height, i-1)
		}
	}
	for i := 1; i < len(a.WindSpeed); i++ {
		if a.WindSpeed[i].Height < a.WindSpeed[i-1].Height {
			t.Errorf("wind not sorted: %v at index %d < %v at index %d",
				a.WindSpeed[i].Height, i, a.WindSpeed[i-1].Height, i-1)
		}
	}
}
