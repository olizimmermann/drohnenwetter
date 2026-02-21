# 🛸 Drone Weather

Real-time weather and airspace safety assessment for **DJI Matrice 30T (M30T)** drone operators in Germany.

![Drone Weather](SafeFlight.png)

---

## Features

- **Safety assessment** — evaluates wind speed (per altitude), gusts, temperature, dew point, and geomagnetic activity against M30T operating limits
- **Airspace overlay** — DiPUL/DFS WMS with 32 layer types (control zones, nature reserves, military areas, …)
- **Affected zones** — API-sourced GeoJSON polygons for your exact location, rendered on the map with popups
- **Parallel API calls** — all 4 data sources fetched concurrently (UTM, OpenWeatherMap, Kp-Index, DiPUL)
- **Bilingual** — German / English toggle, persisted in localStorage
- **Dark & light mode** — toggle, persisted in localStorage
- **Mobile-optimised** — responsive grid, touch-friendly controls, 16px inputs (no iOS auto-zoom)
- **Rate limiting** — 5 requests / minute per IP, token-bucket via `golang.org/x/time/rate`

---

## Data Sources

| Source | Data |
|--------|------|
| [DFS UTM](https://utm.dfs.de) | Wind & temperature forecast at 10 / 50 / 100 / 150 m AGL |
| [OpenWeatherMap](https://openweathermap.org) | Current dew point |
| [GFZ Potsdam](https://www.gfz-potsdam.de) | Kp-Index (geomagnetic activity) |
| [DiPUL / DFS](https://uas-betrieb.de) | Airspace zones & WMS overlay |
| [HERE Maps](https://www.here.com) | Address geocoding (Germany only) |
| [OpenStreetMap](https://www.openstreetmap.org) | Base map tiles |
| [Leaflet](https://leafletjs.com) | Interactive map |

---

## Safety Limits (DJI M30T)

| Parameter | Limit |
|-----------|-------|
| Wind speed | ≤ 12 m/s at any altitude |
| Gusts | ≤ 12 m/s |
| Temperature | −20 °C … +50 °C |
| Dew point proximity | > 2 °C margin (fog risk below 7 °C) |
| Kp-Index | ≤ 4 (GPS / radio reliability) |

---

## Stack

- **Language:** Go 1.23 — `net/http`, `html/template`, no web framework
- **Dependencies:** [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) (rate limiting)
- **Map:** Leaflet.js + OpenStreetMap + DiPUL WMS
- **Deploy:** Docker Compose — 3 app replicas + Nginx reverse proxy
- **Image:** multi-stage build → `gcr.io/distroless/static-debian12:nonroot` (~8 MB)
- **Proxy:** Cloudflare → Nginx → Go app (real IP via `CF-Connecting-IP`)

---

## Project Structure

```
drone-weather/
├── go/                          # Go application (primary)
│   ├── cmd/drone-weather/
│   │   └── main.go              # Entry point, server, rate limiting
│   ├── internal/
│   │   ├── api/
│   │   │   ├── client.go        # Shared HTTP client (8 s timeout)
│   │   │   ├── geocode.go       # HERE Maps geocoding
│   │   │   ├── weather.go       # DFS UTM + OpenWeatherMap + Kp-Index
│   │   │   └── dipul.go         # DiPUL token cache + zone fetching
│   │   ├── assessment/
│   │   │   └── assess.go        # Safety evaluation logic
│   │   └── handler/
│   │       ├── home.go          # GET /
│   │       └── results.go       # POST /results — parallel fetch & render
│   ├── templates/
│   │   ├── base.html            # Shared CSS, dark/light mode, i18n, footer
│   │   ├── index.html           # Landing page
│   │   └── results.html         # Dashboard: metrics, map, zones
│   ├── static/                  # Favicons, manifest
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── nginx/
│   └── mt30.drone-weather.com.conf
├── safeflight/                  # Original Python/FastAPI app (reference)
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
```

| Variable | Where to get it |
|----------|----------------|
| `HERE_API_KEY` | [developer.here.com](https://developer.here.com) — free tier available |
| `OPENWEATHER_TOKEN` | [openweathermap.org/api](https://openweathermap.org/api) — OneCall 3.0 required |

### Run

```bash
docker compose up --build
```

The app is then available at **http://localhost**.

### Logs

```bash
docker compose logs -f safeflight-app
```

---

## Development

Go is not required on the host — the Docker build handles compilation.
If you do have Go installed locally:

```bash
cd go
go build ./...                                    # compile check
HERE_API_KEY=... OPENWEATHER_TOKEN=... \
  go run ./cmd/drone-weather                      # run locally on :8080
```

---

## Deployment

The included `docker-compose.yml` runs **3 app replicas** behind Nginx.
Designed to sit behind Cloudflare (real client IP extracted from `CF-Connecting-IP`).

Domains in use: `mt30.drone-weather.com`, `m30t.drone-weather.com`

> **Security note:** The app itself does not terminate TLS. Run it exclusively behind Nginx + Cloudflare (or another TLS-terminating proxy). Do **not** expose the Go app directly on port 8080 to the public internet — API keys are forwarded server-side but the address form data travels in plain HTTP between client and proxy if TLS is not enforced at the edge.

---

## License

Personal project — no warranty. Not a substitute for official pre-flight checks.
Weather data © respective providers. Map data © OpenStreetMap contributors.

---

**Contact:** [drone-weather@oz-security.io](mailto:drone-weather@oz-security.io) · [github.com/olizimmermann](https://github.com/olizimmermann)
