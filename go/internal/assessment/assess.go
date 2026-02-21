package assessment

import (
	"fmt"
	"math"
	"sort"

	"github.com/olizimmermann/drone-weather/internal/api"
)

// AltEntry holds a measurement at one altitude, pre-sorted by Height.
type AltEntry struct {
	Key    string  // display label e.g. "50m"
	Height float64 // numeric metres, used for sorting
	Value  float64
	OK     bool // hard limit not exceeded
	Warn   bool // soft warning (dew-point proximity, wind approaching limit)
}

type SpeedEntry struct {
	Value float64
	OK    bool
}

type Assessment struct {
	Flyable bool

	Temperature  []AltEntry // sorted 2m → 150m
	TempWarnings []string

	WindSpeed    []AltEntry // sorted 10m → 150m
	WindGust     SpeedEntry
	WindWarnings []string

	DewPoint        float64
	DewPointOK      bool
	DewPointWarning string

	RainMM     float64
	SnowCM     float64
	CloudCover float64

	KpIndex   float64
	KpOK      bool
	KpWarning string
}

func Assess(utm *api.UTMResponse, ow *api.OWResponse, kp float64) *Assessment {
	a := &Assessment{
		Flyable:    true,
		DewPoint:   ow.Current.DewPoint,
		DewPointOK: true,
		KpIndex:    kp,
		KpOK:       true,
	}

	if len(utm.Positions) == 0 || len(utm.Positions[0].Forecasts) == 0 {
		a.Flyable = false
		return a
	}

	fc := utm.Positions[0].Forecasts[0]

	// ── Temperature ─────────────────────────────────────────────────────────
	for _, t := range fc.Temperature {
		key := fmt.Sprintf("%gm", t.Height.Value)
		ok := t.Value <= 50 && t.Value >= -20
		dewWarn := math.Abs(t.Value-ow.Current.DewPoint) < 2 && t.Value < 7
		a.Temperature = append(a.Temperature, AltEntry{
			Key:    key,
			Height: t.Height.Value,
			Value:  round2(t.Value),
			OK:     ok,
			Warn:   dewWarn,
		})
		if !ok {
			a.Flyable = false
			a.TempWarnings = append(a.TempWarnings, fmt.Sprintf("%.1f°C [%s]", t.Value, key))
		}
		// Dew-point fog risk: record first affected altitude in the global warning
		if dewWarn && a.DewPointOK {
			a.DewPointOK = false
			a.Flyable = false
			a.DewPointWarning = fmt.Sprintf("Taupunkt %.1f°C nahe Temperatur %.1f°C [%s] – Nebelgefahr",
				ow.Current.DewPoint, t.Value, key)
		}
	}
	sort.Slice(a.Temperature, func(i, j int) bool {
		return a.Temperature[i].Height < a.Temperature[j].Height
	})

	// ── Wind speed (each height independently) ───────────────────────────────
	for _, w := range fc.Wind {
		key := fmt.Sprintf("%gm", w.Height.Value)
		speed := math.Abs(w.VComponent)
		ok := speed <= 12
		a.WindSpeed = append(a.WindSpeed, AltEntry{
			Key:    key,
			Height: w.Height.Value,
			Value:  round2(speed),
			OK:     ok,
			Warn:   ok && speed > 8, // 8–12 m/s: approaching limit
		})
		if !ok {
			a.Flyable = false
			a.WindWarnings = append(a.WindWarnings, fmt.Sprintf("%.1f m/s [%s]", speed, key))
		}
	}
	sort.Slice(a.WindSpeed, func(i, j int) bool {
		return a.WindSpeed[i].Height < a.WindSpeed[j].Height
	})

	// ── Gust ────────────────────────────────────────────────────────────────
	if len(fc.Wind) > 0 {
		gust := math.Abs(fc.Wind[0].WindSpeedGust)
		ok := gust <= 12
		a.WindGust = SpeedEntry{Value: round2(gust), OK: ok}
		if !ok {
			a.Flyable = false
			a.WindWarnings = append(a.WindWarnings, fmt.Sprintf("Böen %.1f m/s", gust))
		}
	}

	// ── Precipitation / cloud cover ──────────────────────────────────────────
	a.RainMM = round2(fc.RainPrecipitation.Value)
	a.SnowCM = round2(fc.SnowPrecipitation.Value)
	a.CloudCover = round2(fc.TotalCloudCover.Value)

	// ── KP-Index ────────────────────────────────────────────────────────────
	if kp > 4 {
		a.KpOK = false
		a.Flyable = false
		a.KpWarning = fmt.Sprintf("Kp-Index %.1f – GPS/Funk-Zuverlässigkeit beeinträchtigt", kp)
	}

	return a
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
