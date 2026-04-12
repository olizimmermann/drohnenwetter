package assessment

import (
	"math"
	"strings"
	"testing"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

// makeUTM builds a minimal UTMResponse with one forecast.
// temps: values at heights 2, 50, 100, 150 m
// winds: vComponent values at heights 10, 50, 100, 150 m
// gust:  windSpeedGust on the first wind entry
func makeUTM(temps []float64, winds []float64, gust float64) *api.UTMResponse {
	return makeUTMFull(temps, winds, gust, nil, 0)
}

// makeUTMFull is the fully-parameterised builder. humidities aligns 1:1 with
// temps (same altitudes); pass nil to omit humidity. rain sets mm of rain.
func makeUTMFull(temps []float64, winds []float64, gust float64, humidities []float64, rain float64) *api.UTMResponse {
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

	var humidity []api.UTMAirHumidity
	for i, rh := range humidities {
		h := tempHeights[0]
		if i < len(tempHeights) {
			h = tempHeights[i]
		}
		humidity = append(humidity, api.UTMAirHumidity{
			Height: api.UTMHeight{Value: h, Unit: "m", Reference: "AGL"},
			Unit:   "%",
			Value:  rh,
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
						AirHumidity:       humidity,
						RainPrecipitation: api.UTMPrecip{Unit: "mm", Value: rain},
						SnowPrecipitation: api.UTMPrecip{Unit: "cm", Value: 0},
						TotalCloudCover:   api.UTMPrecip{Unit: "%", Value: 0},
					},
				},
			},
		},
	}
}

// humidityFor returns the relative humidity (%) that yields the given target
// dew-point at tempC — inverse of the Magnus-Tetens dewPoint() function.
func humidityFor(tempC, targetDew float64) float64 {
	const a, b = 17.625, 243.04
	return 100 * math.Exp((a*targetDew)/(b+targetDew)-(a*tempC)/(b+tempC))
}

// dewUTM is a convenience builder for dew-point test cases: one altitude
// with the supplied temp and humidity chosen to yield targetDew.
func dewUTM(temp, targetDew float64) *api.UTMResponse {
	return makeUTMFull([]float64{temp}, []float64{5}, 5, []float64{humidityFor(temp, targetDew)}, 0)
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
		dewCrit    bool
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
			ow:         nil,
			kp:         2,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "no humidity no dew warn",
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

		// ── Dew point warn tier (T<3 AND ΔT<2) ────────────────────────────────
		{
			name:       "warm temp 8 with 0.5 gap no warn",
			utm:        dewUTM(8, 7.5),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "warn T=2 delta=1",
			utm:        dewUTM(2, 1),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			kpOK:       true,
		},
		{
			name:       "safe T=2 delta=3",
			utm:        dewUTM(2, -1),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "boundary T=3 no warn",
			utm:        dewUTM(3, 2),
			ow:         nil,
			kp:         0,
			flyable:    true,
			dewPointOK: true,
			kpOK:       true,
		},
		{
			name:       "T=2.9 delta=0.9 warns",
			utm:        dewUTM(2.9, 2),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			kpOK:       true,
		},

		// ── Dew point critical tier (-10≤T≤0 AND ΔT<1) ─────────────────────────
		{
			name:       "critical T=0 delta=0.5",
			utm:        dewUTM(0, -0.5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			dewCrit:    true,
			kpOK:       true,
		},
		{
			name:       "critical T=-10 delta=0.5",
			utm:        dewUTM(-10, -10.5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			dewCrit:    true,
			kpOK:       true,
		},
		{
			name:       "boundary T=0 delta=1 not critical but warn",
			utm:        dewUTM(0, -1),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			dewCrit:    false,
			kpOK:       true,
		},
		{
			name:       "just below crit band T=-10.1 delta=0.4 warn only",
			utm:        dewUTM(-10.1, -10.5),
			ow:         nil,
			kp:         0,
			flyable:    false,
			dewPointOK: false,
			dewCrit:    false,
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
			if a.DewPointCritical != tc.dewCrit {
				t.Errorf("DewPointCritical = %v, want %v", a.DewPointCritical, tc.dewCrit)
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

func TestAssessFreezingRainHazard(t *testing.T) {
	// Freezing rain: surface T ≤ 0 AND rain > 0 → absolute no-go.
	utm := makeUTMFull([]float64{-2, -3, -4, -5}, []float64{5, 5, 5, 5}, 5, nil, 0.5)
	a := Assess(utm, nil, 0)
	if !a.FreezingHazard {
		t.Error("expected FreezingHazard=true for rain at -2°C")
	}
	if a.Flyable {
		t.Error("freezing rain must flip Flyable=false")
	}
	// Freezing rain is an absolute no-go, not a downgraded precip warning.
	if a.PrecipWarning {
		t.Error("freezing rain should use FreezingHazard only, not PrecipWarning")
	}

	// Sub-zero but no rain: no hazard.
	utm = makeUTMFull([]float64{-5, -6, -7, -8}, []float64{5, 5, 5, 5}, 5, nil, 0)
	a = Assess(utm, nil, 0)
	if a.FreezingHazard {
		t.Error("expected no hazard when rain=0")
	}
}

func TestAssessPrecipWarning(t *testing.T) {
	// Non-freezing rain: warn only, Flyable stays true (IP-rated drones OK).
	utm := makeUTMFull([]float64{5, 4, 3, 2}, []float64{5, 5, 5, 5}, 5, nil, 0.5)
	a := Assess(utm, nil, 0)
	if !a.PrecipWarning {
		t.Error("expected PrecipWarning=true for rain at +5°C")
	}
	if !a.Flyable {
		t.Error("non-freezing rain must NOT flip Flyable — IP-rated drones are certified for light rain")
	}
	if a.FreezingHazard {
		t.Error("non-freezing rain should not trip FreezingHazard")
	}

	// Snow at any temperature: warn only.
	utm = makeUTMFull([]float64{-3, -4, -5, -6}, []float64{5, 5, 5, 5}, 5, nil, 0)
	utm.Positions[0].Forecasts[0].SnowPrecipitation = api.UTMPrecip{Unit: "cm", Value: 2}
	a = Assess(utm, nil, 0)
	if !a.PrecipWarning {
		t.Error("expected PrecipWarning=true for snow")
	}
	if !a.Flyable {
		t.Error("snow must NOT flip Flyable")
	}

	// Dry conditions: no precip warning.
	utm = makeUTMFull([]float64{10, 9, 8, 7}, []float64{5, 5, 5, 5}, 5, nil, 0)
	a = Assess(utm, nil, 0)
	if a.PrecipWarning {
		t.Error("expected no PrecipWarning when dry")
	}
}

// Gap #1: the switch branch for rain + snow simultaneously was never exercised.
func TestAssessPrecipWarningRainAndSnow(t *testing.T) {
	utm := makeUTMFull([]float64{4, 3, 2, 1}, []float64{5, 5, 5, 5}, 5, nil, 0.3)
	utm.Positions[0].Forecasts[0].SnowPrecipitation = api.UTMPrecip{Unit: "cm", Value: 1.5}
	a := Assess(utm, nil, 0)
	if !a.PrecipWarning {
		t.Fatal("expected PrecipWarning=true for combined rain+snow")
	}
	if !a.Flyable {
		t.Error("combined non-freezing rain+snow must not flip Flyable")
	}
	// Joint-case message should mention both quantities.
	if !strings.Contains(a.PrecipWarningDE, "Regen") || !strings.Contains(a.PrecipWarningDE, "Schnee") {
		t.Errorf("DE joint message missing rain or snow mention: %q", a.PrecipWarningDE)
	}
	if !strings.Contains(a.PrecipWarningEN, "Rain") || !strings.Contains(a.PrecipWarningEN, "snow") {
		t.Errorf("EN joint message missing rain or snow mention: %q", a.PrecipWarningEN)
	}
}

// Gap #2: boundary — rain exactly at surface T=0 must count as freezing rain.
func TestAssessFreezingRainBoundaryAtZero(t *testing.T) {
	utm := makeUTMFull([]float64{0, -1, -2, -3}, []float64{5, 5, 5, 5}, 5, nil, 0.2)
	a := Assess(utm, nil, 0)
	if !a.FreezingHazard {
		t.Error("T=0°C with rain must trigger FreezingHazard (boundary is <=0)")
	}
	if a.Flyable {
		t.Error("freezing rain at T=0 must flip Flyable")
	}

	// Just above zero: not freezing.
	utm = makeUTMFull([]float64{0.1, -0.5, -1, -2}, []float64{5, 5, 5, 5}, 5, nil, 0.2)
	a = Assess(utm, nil, 0)
	if a.FreezingHazard {
		t.Error("T=0.1°C with rain must NOT trigger FreezingHazard")
	}
	if !a.PrecipWarning {
		t.Error("T=0.1°C with rain should fall through to non-freezing precip warning")
	}
}

// Gap #3: freezing rain and a dew-point warn in the same forecast must both
// fire — no short-circuit between precipitation and dew-point evaluation.
func TestAssessFreezingRainPlusDewWarn(t *testing.T) {
	// T=-5°C at surface AND lowest altitude → critical dew band.
	// Humidity chosen to land squarely in the warn tier (delta < 2, T < 3).
	temps := []float64{-5, -5, -5, -5}
	hums := []float64{
		humidityFor(-5, -6),
		humidityFor(-5, -6),
		humidityFor(-5, -6),
		humidityFor(-5, -6),
	}
	utm := makeUTMFull(temps, []float64{5, 5, 5, 5}, 5, hums, 0.4)
	a := Assess(utm, nil, 0)
	if !a.FreezingHazard {
		t.Error("expected FreezingHazard=true")
	}
	if a.DewPointOK {
		t.Error("expected DewPointOK=false — dew-point evaluation must not be skipped by freezing rain")
	}
	if a.Flyable {
		t.Error("Flyable must be false (both hazards active)")
	}
	if len(a.DewPointByAltitude) == 0 {
		t.Error("expected per-altitude dew-point entries even when freezing hazard active")
	}
}

// Gap #4: humidity at an altitude without a matching temperature is skipped.
func TestAssessHumidityWithoutMatchingTemp(t *testing.T) {
	// Build a UTM response where humidity is reported at 200 m but temperature
	// is only available at 2/50/100/150 m. The 200 m humidity entry has no
	// matching temp and must be silently dropped (not panic, not produce an
	// entry with bogus temperature).
	utm := makeUTMFull([]float64{10, 10, 10, 10}, []float64{5, 5, 5, 5}, 5,
		[]float64{humidityFor(10, 5), humidityFor(10, 5), humidityFor(10, 5), humidityFor(10, 5)}, 0)
	// Append an orphan humidity entry at 200 m with no matching temp.
	fc := &utm.Positions[0].Forecasts[0]
	fc.AirHumidity = append(fc.AirHumidity, api.UTMAirHumidity{
		Height: api.UTMHeight{Value: 200, Unit: "m", Reference: "AGL"},
		Unit:   "%",
		Value:  90,
	})
	a := Assess(utm, nil, 0)
	if len(a.DewPointByAltitude) != 4 {
		t.Errorf("expected 4 dew-point entries (orphan skipped), got %d", len(a.DewPointByAltitude))
	}
	for _, e := range a.DewPointByAltitude {
		if e.Height == 200 {
			t.Error("orphan 200m humidity entry must not produce a dew-point row")
		}
	}
}

func TestDewPointByAltitudePopulated(t *testing.T) {
	// Humidity at each temp altitude → entry per altitude, sorted by height.
	utm := makeUTMFull(
		[]float64{2, 3, 4, 5},
		[]float64{5, 5, 5, 5}, 5,
		[]float64{humidityFor(2, -5), humidityFor(3, -5), humidityFor(4, -5), humidityFor(5, -5)},
		0,
	)
	a := Assess(utm, nil, 0)
	if len(a.DewPointByAltitude) != 4 {
		t.Fatalf("expected 4 dew-point entries, got %d", len(a.DewPointByAltitude))
	}
	for i := 1; i < len(a.DewPointByAltitude); i++ {
		if a.DewPointByAltitude[i].Height < a.DewPointByAltitude[i-1].Height {
			t.Errorf("dew point not sorted at index %d", i)
		}
	}
	// All safe: delta≈7, no warn/crit
	if !a.DewPointOK {
		t.Error("expected DewPointOK=true for large spread")
	}
}

func TestDewPointMagnusRoundtrip(t *testing.T) {
	cases := []struct{ t, td float64 }{
		{20, 10}, {5, 2}, {0, -3}, {-10, -12},
	}
	for _, c := range cases {
		rh := humidityFor(c.t, c.td)
		got := dewPoint(c.t, rh)
		if math.Abs(got-c.td) > 0.05 {
			t.Errorf("roundtrip T=%v Td=%v: got %v", c.t, c.td, got)
		}
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
