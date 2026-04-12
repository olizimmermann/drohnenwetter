# 🛸 Drohnenwetter

Real-time weather and airspace safety assessment for drone operators in Germany.

🇩🇪 [Deutsche Version](README.md)

[![Deploy](https://github.com/olizimmermann/drohnenwetter/actions/workflows/deploy.yml/badge.svg)](https://github.com/olizimmermann/drohnenwetter/actions/workflows/deploy.yml)
[![Daily Health Check](https://github.com/olizimmermann/drohnenwetter/actions/workflows/daily-health.yml/badge.svg)](https://github.com/olizimmermann/drohnenwetter/actions/workflows/daily-health.yml)

![Drohnenwetter](drohnenwetter.png)

---

## Features

- **Safety assessment** — wind speed (per altitude), gusts, temperature, dew point (per altitude), and geomagnetic activity evaluated against operating limits
- **Per-altitude dew point** — computed from DFS temperature and relative humidity via the Magnus-Tetens formula, with two severity tiers (warning / critical) plus an absolute no-go for freezing rain
- **Airspace overlay** — DiPUL/DFS WMS with 32 layer types (control zones, nature reserves, military areas, …)
- **Affected zones** — API-sourced GeoJSON polygons for your exact location, rendered on the map with popups and click-to-inspect details
- **Cloud base** — nearest airport TAF parsed for cloud base altitude
- **Live air traffic** — nearby aircraft (✈️ fixed-wing, 🚁 helicopters, 🛩 gliders, 🛸 UAVs) from OpenSky Network, auto-refreshed every 15 seconds
- **Parallel API calls** — all data sources fetched concurrently (UTM, OpenWeatherMap, Kp-Index, DiPUL, OpenSky)
- **Bilingual** — German / English toggle, persisted in localStorage
- **Dark & light mode** — toggle, persisted in localStorage
- **Mobile-optimised** — responsive grid, touch-friendly controls, 16px inputs (no iOS auto-zoom)
- **Rate limiting** — 10 requests / minute per IP, token-bucket via `golang.org/x/time/rate`

---

## Data Sources

| Source | Data |
|--------|------|
| [DFS UTM](https://utm.dfs.de) | Wind, temperature & relative humidity at 2 / 10 / 50 / 100 / 150 m AGL (dew point derived locally) |
| [OpenWeatherMap](https://openweathermap.org) | Supplementary surface data (fallback dew point) |
| [GFZ Potsdam](https://www.gfz-potsdam.de) | Kp-Index (geomagnetic activity) |
| [DiPUL / DFS](https://uas-betrieb.de) | Airspace zones, WMS overlay & zone details |
| [DFS TAF](https://www.dfs.de) | Cloud base from nearest airport TAF |
| [OpenSky Network](https://opensky-network.org) | Live air traffic (state vectors, ~11 km radius) |
| [HERE Maps](https://www.here.com) | Address geocoding (Germany only) |
| [OpenStreetMap](https://www.openstreetmap.org) | Base map tiles |
| [Leaflet](https://leafletjs.com) | Interactive map |

---

## Safety Limits

| Parameter | Limit |
|-----------|-------|
| Wind speed | ≤ 12 m/s at any altitude |
| Gusts | ≤ 12 m/s |
| Temperature | −20 °C … +50 °C |
| Dew point proximity (warn) | T < 3 °C **and** (T − Td) < 2 °C → fog / clear-ice risk |
| Dew point proximity (critical) | −10 °C ≤ T ≤ 0 °C **and** (T − Td) < 1 °C → icing risk (supercooled droplets) |
| Freezing rain | precipitation > 0 mm with surface T ≤ 0 °C → **absolute no-go** |
| Rain (non-freezing) | > 0 mm → warning (weather-resistant IP-rated drone recommended, not a hard block) |
| Snowfall | > 0 cm → warning (weather-resistant IP-rated drone recommended, not a hard block) |
| Kp-Index | ≤ 4 (GPS / radio reliability) |

> The app is primarily aimed at German BOS users (police, fire, rescue, civil protection) operating IP-rated drones. Light rain or snow therefore surface as warnings but do **not** flip the flight status to "blocked".

---

## Stack

- **Language:** Go 1.23 — `net/http`, `html/template`, no web framework
- **Version:** `0.9` (see [VERSION](go/VERSION))
- **Dependencies:** [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) (rate limiting)
- **Map:** Leaflet.js + OpenStreetMap + DiPUL WMS
- **Deploy:** Docker Compose — 3 app replicas + Nginx reverse proxy
- **Image:** multi-stage build → `gcr.io/distroless/static-debian12:nonroot` (~8 MB)
- **Proxy:** Cloudflare → Nginx → Go app (real IP via `CF-Connecting-IP`)

---

## Project Structure

```
drohnenwetter/
├── go/                          # Go application
│   ├── cmd/drohnenwetter/
│   │   └── main.go              # Entry point, server, rate limiting, middleware
│   ├── internal/
│   │   ├── api/
│   │   │   ├── client.go        # Shared HTTP client (8 s timeout)
│   │   │   ├── geocode.go       # HERE Maps geocoding
│   │   │   ├── weather.go       # DFS UTM + OpenWeatherMap + Kp-Index
│   │   │   ├── dipul.go         # DiPUL token cache + zone fetching
│   │   │   ├── wms.go           # DiPUL WMS GetFeatureInfo (zone click details)
│   │   │   ├── metar.go         # Cloud base from nearest airport TAF
│   │   │   └── opensky.go       # OpenSky OAuth2 token cache + traffic fetch
│   │   ├── assessment/
│   │   │   └── assess.go        # Safety evaluation logic
│   │   └── handler/
│   │       ├── home.go          # GET /
│   │       ├── results.go       # POST /results — parallel fetch & render
│   │       ├── zone-info.go     # GET /zone-info — zone click details (GeoJSON)
│   │       ├── traffic.go       # GET /traffic — live aircraft JSON
│   │       ├── track.go         # GET /track — flight track proxy
│   │       └── util.go          # Shared handler utilities
│   ├── templates/
│   │   ├── base.html            # Shared CSS, dark/light mode, i18n, footer
│   │   ├── index.html           # Landing page
│   │   ├── results.html         # Dashboard: metrics, map, zones, live traffic
│   │   ├── impressum.html       # Legal notice
│   │   └── datenschutz.html     # Privacy policy
│   ├── static/                  # Favicons, manifest, robots.txt, sitemap
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── nginx/
│   └── drohnenwetter.de.conf
├── checks/
│   └── smoke.sh                 # Smoke tests against live site
├── .github/
│   └── workflows/
│       └── daily-health.yml     # Daily automated health check (07:00 UTC)
├── docker-compose.yml
└── .env                         # API keys (not committed)
```

---

## Setup

### Prerequisites

- Docker + Docker Compose
- API keys (see below)

### Environment variables

Create a `.env` file in the project root:

```env
HERE_API_KEY=your_here_maps_api_key
OPENWEATHER_TOKEN=your_openweathermap_api_key
OPENSKY_CLIENT_ID=your_opensky_client_id
OPENSKY_CLIENT_SECRET=your_opensky_client_secret
```

| Variable | Where to get it |
|----------|----------------|
| `HERE_API_KEY` | [developer.here.com](https://developer.here.com) — free tier available |
| `OPENWEATHER_TOKEN` | [openweathermap.org/api](https://openweathermap.org/api) — OneCall 3.0 required |
| `OPENSKY_CLIENT_ID` | [opensky-network.org](https://opensky-network.org) — create API client under your account |
| `OPENSKY_CLIENT_SECRET` | Same as above — client credentials OAuth2 flow |

> `OPENSKY_CLIENT_ID` and `OPENSKY_CLIENT_SECRET` are optional. Without them the app falls back to the anonymous OpenSky API (lower rate limits, no track data).

### Run

```bash
docker compose up --build
```

The app is then available at **http://localhost:8080**.

### Logs

```bash
docker compose logs -f drohnenwetter-app
```

---

## Development

Go is not required on the host — the Docker build handles compilation.
If you do have Go installed locally:

```bash
cd go
go build ./...                                    # compile check
HERE_API_KEY=... OPENWEATHER_TOKEN=... \
  OPENSKY_CLIENT_ID=... OPENSKY_CLIENT_SECRET=... \
  go run ./cmd/drohnenwetter                     # run locally on :8080
```

### Smoke tests

```bash
./checks/smoke.sh                          # test live site
./checks/smoke.sh http://localhost:8080    # test local instance
```

---

## Deployment

The included `docker-compose.yml` runs **3 app replicas** behind Nginx.
Designed to sit behind Cloudflare (real client IP extracted from `CF-Connecting-IP`).

Live at: [drohnenwetter.de](https://drohnenwetter.de)

> **Security note:** The app does not terminate TLS. Run it exclusively behind Nginx + Cloudflare (or another TLS-terminating proxy). Do **not** expose the Go app directly on port 8080 to the public internet.

---

## License

Personal project — no warranty. Not a substitute for official pre-flight checks.
Weather data © respective providers. Map data © OpenStreetMap contributors.

---

**Contact:** [public@ozimmermann.com](mailto:public@ozimmermann.com) · [github.com/olizimmermann](https://github.com/olizimmermann)
