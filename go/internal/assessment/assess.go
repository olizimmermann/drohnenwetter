package assessment

import (
	"fmt"
	"math"
	"sort"

	"github.com/olizimmermann/drohnenwetter/internal/api"
)

// AltEntry holds a measurement at one altitude, pre-sorted by Height.
type AltEntry struct {
	Key    string  // display label e.g. "50m"
	Height float64 // numeric metres, used for sorting
	Value  float64
	OK     bool // hard limit not exceeded
	Warn   bool // soft warning (dew-point proximity, wind approaching limit)
	Crit   bool // critical tier (hard fail, stronger than Warn)
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

	DewPoint           float64    // surface / lowest-altitude value
	DewPointByAltitude []AltEntry // sorted low → high
	DewPointOK         bool
	DewPointCritical   bool
	DewPointWarning    string
	DewPointWarningEN  string

	FreezingHazard   bool
	FreezingHazardDE string
	FreezingHazardEN string

	// PrecipWarning: non-freezing rain or any snow. Informational only —
	// does NOT flip Flyable, since the audience (BOS) primarily uses
	// IP-rated airframes certified for light precipitation.
	PrecipWarning   bool
	PrecipWarningDE string
	PrecipWarningEN string

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
		DewPointOK: true,
		KpIndex:    kp,
		KpOK:       true,
	}
	if ow != nil {
		a.DewPoint = ow.Current.DewPoint
	}

	if len(utm.Positions) == 0 || len(utm.Positions[0].Forecasts) == 0 {
		a.Flyable = false
		return a
	}

	fc := utm.Positions[0].Forecasts[0]

	// ── Temperature ─────────────────────────────────────────────────────────
	tempAt := make(map[float64]float64, len(fc.Temperature))
	for _, t := range fc.Temperature {
		key := fmt.Sprintf("%gm", t.Height.Value)
		ok := t.Value <= 50 && t.Value >= -20
		a.Temperature = append(a.Temperature, AltEntry{
			Key:    key,
			Height: t.Height.Value,
			Value:  round2(t.Value),
			OK:     ok,
		})
		if !ok {
			a.Flyable = false
			a.TempWarnings = append(a.TempWarnings, fmt.Sprintf("%.1f°C [%s]", t.Value, key))
		}
		tempAt[t.Height.Value] = t.Value
	}
	sort.Slice(a.Temperature, func(i, j int) bool {
		return a.Temperature[i].Height < a.Temperature[j].Height
	})

	// ── Dew point per altitude (derived from DIPUL humidity + temp) ─────────
	var worstT, worstTd, worstHeight float64
	worstCrit := false
	haveWorst := false
	for _, h := range fc.AirHumidity {
		t, haveTemp := tempAt[h.Height.Value]
		if !haveTemp {
			continue
		}
		td := dewPoint(t, h.Value)
		// Round delta to 2 decimals to match display precision and avoid
		// floating-point boundary flicker on tier comparisons.
		delta := round2(t - td)
		warn := t < 3 && delta < 2
		crit := t >= -10 && t <= 0 && delta < 1
		key := fmt.Sprintf("%gm", h.Height.Value)
		a.DewPointByAltitude = append(a.DewPointByAltitude, AltEntry{
			Key:    key,
			Height: h.Height.Value,
			Value:  round2(td),
			OK:     !warn && !crit,
			Warn:   warn && !crit,
			Crit:   crit,
		})
		if warn || crit {
			a.Flyable = false
			a.DewPointOK = false
			// Track the worst-severity altitude for the displayed message:
			// crit beats warn; within the same tier, the smallest delta wins.
			promote := !haveWorst ||
				(crit && !worstCrit) ||
				(crit == worstCrit && delta < (worstT-worstTd))
			if promote {
				worstT, worstTd, worstHeight, worstCrit = t, td, h.Height.Value, crit
				haveWorst = true
			}
			if crit {
				a.DewPointCritical = true
			}
		}
	}
	if haveWorst {
		key := fmt.Sprintf("%gm", worstHeight)
		label := "Nebel- und Klareisbildungsgefahr"
		labelEN := "fog and clear ice risk"
		if worstCrit {
			label = "kritische Vereisungsgefahr"
			labelEN = "critical icing risk (supercooled droplets)"
		}
		a.DewPointWarning = fmt.Sprintf("Taupunkt %.1f°C nahe Temperatur %.1f°C [%s] – %s", worstTd, worstT, key, label)
		a.DewPointWarningEN = fmt.Sprintf("Dew point %.1f°C near temperature %.1f°C [%s] – %s", worstTd, worstT, key, labelEN)
	}
	sort.Slice(a.DewPointByAltitude, func(i, j int) bool {
		return a.DewPointByAltitude[i].Height < a.DewPointByAltitude[j].Height
	})
	if len(a.DewPointByAltitude) > 0 {
		a.DewPoint = a.DewPointByAltitude[0].Value
	} else if ow != nil {
		a.DewPoint = ow.Current.DewPoint
	}

	// ── Wind speed (each height independently) ───────────────────────────────
	for _, w := range fc.Wind {
		key := fmt.Sprintf("%gm", w.Height.Value)
		speed := math.Sqrt(w.UComponent*w.UComponent + w.VComponent*w.VComponent)
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

	// ── Gust (only reported at 10 m AGL per DFS spec) ───────────────────────
	var gustRaw float64
	for _, w := range fc.Wind {
		if w.Height.Value == 10 && w.WindSpeedGust != 0 {
			gustRaw = w.WindSpeedGust
			break
		}
	}
	if gustRaw != 0 {
		gust := math.Abs(gustRaw)
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

	// ── Precipitation hazards ────────────────────────────────────────────────
	// Surface temp = lowest available altitude (typically 2 m AGL).
	if len(a.Temperature) > 0 {
		tSurface := a.Temperature[0].Value
		rain := fc.RainPrecipitation.Value
		snow := fc.SnowPrecipitation.Value

		// Freezing rain: absolute no-go. IP rating does not protect against
		// icing from supercooled droplets.
		if rain > 0 && tSurface <= 0 {
			a.FreezingHazard = true
			a.Flyable = false
			a.FreezingHazardDE = fmt.Sprintf("Gefrierender Regen (%.2f mm bei %.1f°C) – absolutes Flugverbot",
				rain, tSurface)
			a.FreezingHazardEN = fmt.Sprintf("Freezing rain (%.2f mm at %.1f°C) – absolute no-go",
				rain, tSurface)
		} else if rain > 0 || snow > 0 {
			// Non-freezing precipitation: amber warning only. IP-rated drones
			// (primary BOS fleet) handle light rain/snow; flyable status
			// stays unchanged so pilots aren't forced to override on drizzle.
			a.PrecipWarning = true
			switch {
			case rain > 0 && snow > 0:
				a.PrecipWarningDE = fmt.Sprintf("Regen %.2f mm und Schnee %.2f cm – nur mit wetterfester Drohne (IP-Schutzklasse) empfohlen",
					rain, snow)
				a.PrecipWarningEN = fmt.Sprintf("Rain %.2f mm and snow %.2f cm – only recommended for weather-resistant drones (IP-rated)",
					rain, snow)
			case snow > 0:
				a.PrecipWarningDE = fmt.Sprintf("Schneefall %.2f cm – nur mit wetterfester Drohne (IP-Schutzklasse) empfohlen; Sicht und Propeller-Vereisung beachten",
					snow)
				a.PrecipWarningEN = fmt.Sprintf("Snowfall %.2f cm – only recommended for weather-resistant drones (IP-rated); mind visibility and prop icing",
					snow)
			default:
				a.PrecipWarningDE = fmt.Sprintf("Regen %.2f mm – nur mit wetterfester Drohne (IP-Schutzklasse) empfohlen",
					rain)
				a.PrecipWarningEN = fmt.Sprintf("Rain %.2f mm – only recommended for weather-resistant drones (IP-rated)",
					rain)
			}
		}
	}

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

// dewPoint returns the dew-point temperature in °C for the given air
// temperature (°C) and relative humidity (%) using the Magnus-Tetens formula
// (Alduchov & Eskridge 1996 coefficients). Accurate to ~0.4°C for 0–60°C.
func dewPoint(tempC, rhPct float64) float64 {
	const a, b = 17.625, 243.04
	if rhPct < 0.01 {
		rhPct = 0.01
	}
	alpha := math.Log(rhPct/100) + (a*tempC)/(b+tempC)
	return (b * alpha) / (a - alpha)
}
