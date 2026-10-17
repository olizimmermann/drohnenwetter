# drohnenwetter.de API v1 – für Leitstellen und Einsatzzentralen

Die API liefert dieselbe Flugwetter- und Luftraumbewertung wie
[drohnenwetter.de](https://drohnenwetter.de) als JSON – zur direkten Einbindung
in Einsatzleitsysteme.

> **Entscheidungshilfe ohne Gewähr.** Die Verantwortung für die Flugdurchführung
> liegt beim Fernpiloten bzw. der einsatzführenden Stelle.

## Zugang

| | |
|---|---|
| Basis-URL | `https://api.drohnenwetter.de` *(vorläufig, wird mitgeteilt)* |
| Authentifizierung | Header `X-API-Key: <Ihr Schlüssel>` bei **jeder** Anfrage |
| Methode | nur `GET` |
| Rate-Limit | 30 Anfragen/Minute pro Schlüssel (Burst 10) |

API-Schlüssel werden individuell vergeben. Bitte vertraulich behandeln; bei
Verdacht auf Missbrauch sperren wir den Schlüssel und stellen einen neuen aus.

## Endpunkt

```
GET /api/v1/assessment?lat=<Breite>&lon=<Länge>
GET /api/v1/assessment?address=<Adresse>
```

| Parameter | Pflicht | Beschreibung |
|---|---|---|
| `lat`, `lon` | eins von beiden | Dezimalgrad (WGS84), z. B. `lat=52.52&lon=13.405` |
| `address` | eins von beiden | Freitext-Adresse in Deutschland, max. 100 Zeichen |
| `lang` | nein | `de` (Standard) oder `en` – Sprache der Texte |
| `geometry` | nein | `1` = GeoJSON-Geometrie der Luftraumzonen mitliefern |

Beispiel:

```sh
curl -H 'X-API-Key: <Ihr Schlüssel>' \
  'https://api.drohnenwetter.de/api/v1/assessment?lat=48.137&lon=11.575'
```

## Antwort (200)

```json
{
  "api_version": "1",
  "request_id": "c9c45cc6d3901764",
  "generated_at": "2026-10-09T21:25:52Z",
  "cached": false,
  "location": { "lat": 48.137, "lon": 11.575, "address": "Marienplatz, 80331 München" },
  "verdict": {
    "flyable": false,
    "weather_flyable": true,
    "airspace": "orange",
    "reasons": ["Kontrollierter Luftraum (CTR/Flugplatz/Militär) – Freigabe erforderlich"],
    "notices": []
  },
  "weather": {
    "temperature_c": [{ "height_m": 2, "value": 11.2, "ok": true, "warn": false, "crit": false }],
    "wind_ms":       [{ "height_m": 10, "value": 4.4, "ok": true, "warn": false, "crit": false }],
    "gust_ms": { "value": 7.1, "ok": true },
    "dew_point": { "surface_c": 4.2, "by_altitude": [], "ok": true, "critical": false },
    "freezing_hazard": { "active": false },
    "precipitation": { "warning": false, "rain_mm": 0, "snow_cm": 0 },
    "cloud_cover_pct": 40
  },
  "kp": { "available": true, "value": 1.3, "ok": true },
  "cloud_base": {
    "icao": "EDDM", "airport": "München",
    "metar": { "cloud_base_ft": 3500, "cloud_base_m": 1067, "visibility_m": 9999 },
    "taf":   { "cloud_base_ft": 2500, "cloud_base_m": 762, "min_visibility_m": 8000 }
  },
  "zones": [
    { "type_code": "CONTROL_ZONE", "severity": "orange",
      "areas": [{ "name": "CTR München", "lower": { "altitude": "0", "unit": "FT", "reference": "AGL" } }] }
  ],
  "sources": { "utm": "ok", "openweather": "ok", "kp": "ok", "dipul": "ok", "opensky": "disabled" },
  "disclaimer": "Entscheidungshilfe ohne Gewähr. …"
}
```

### Felder

| Feld | Bedeutung |
|---|---|
| `verdict.flyable` | **Gesamtbewertung**: `true` = Wetter und Luftraum unkritisch |
| `verdict.weather_flyable` | Bewertung nur nach Wetter (ohne Luftraum) |
| `verdict.airspace` | `red` = Flugbeschränkung/Flugverbot, `orange` = kontrollierter Luftraum (Freigabe nötig), `none`, `unknown` = DiPUL nicht erreichbar |
| `verdict.reasons` | Gründe, warum `flyable` = `false` |
| `verdict.notices` | Hinweise ohne Einfluss auf `flyable` (z. B. Niederschlag – für IP-geschützte Drohnen oft unkritisch) |
| `weather.*` | Werte je Höhe über Grund (m). `ok`/`warn`/`crit` = Grenzwertstatus |
| `kp` | Geomagnetische Aktivität (Warnung bei Kp > 4: GPS-/Funkstörungen möglich) |
| `cloud_base` | Wolkenuntergrenze und Sicht vom nächstgelegenen Flughafen (METAR = aktuell, TAF = Vorhersage, ungünstigster Wert); `null`, wenn nicht verfügbar |
| `zones` | DiPUL-Luftraumzonen am Standort. `severity`: `red`, `orange`, `info` |
| `sources` | Status je Datenquelle: `ok`, `failed`, `disabled` |
| `traffic` | Live-Luftverkehr im Umkreis von ca. 11 km – **nur bei freigeschalteten Schlüsseln** |
| `cached` | Daten stammen aus dem 5-Minuten-Cache (gleicher Standort ±~110 m) |

Fällt eine Nebenquelle aus (`openweather`, `kp`, `dipul`), wird trotzdem
geantwortet; der Ausfall steht in `sources`. Bei `dipul: "failed"` ist der
Luftraum **selbst zu prüfen** (`airspace: "unknown"`).

## Fehler

Fehler kommen immer als JSON:

```json
{ "error": { "code": "invalid_coordinates", "message": "…" }, "request_id": "…" }
```

| Status | `code` | Bedeutung |
|---|---|---|
| 400 | `location_missing`, `invalid_coordinates`, `address_missing`, `address_too_long` | Parameter prüfen |
| 401 | – | Schlüssel fehlt oder ungültig |
| 404 | `address_not_found` | Adresse nicht gefunden |
| 405 | `method_not_allowed` | nur `GET` |
| 429 | – / `rate_limited` | Limit erreicht: `Retry-After` (Sekunden) beachten |
| 502/504 | – | Upstream-Fehler/Timeout, später erneut versuchen |
| 503 | `weather_unavailable`, `geocoder_unavailable` | Pflichtquelle derzeit nicht erreichbar |

Jede Antwort enthält einen `X-Request-Id`-Header. Bitte bei Supportanfragen
angeben.

## Hinweise für die Integration

- Rate-Limit-Header: `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`.
- Antworten nicht länger als 5 Minuten zwischenspeichern; Wetter und Luftverkehr ändern sich.
- Neue Felder können jederzeit hinzukommen (abwärtskompatibel). Unbekannte Felder bitte ignorieren.
