# drohnenwetter ↔ API gateway

This is how the drohnenwetter app talks to HERE Maps and OpenWeather. **You no
longer call those providers directly.** Instead you call our gateway, which:

- checks your internal API key (so only our app gets in),
- adds the real HERE / OpenWeather credential for you (you never see or handle it),
- enforces one shared usage budget across all app instances,
- forwards the call to the provider and streams the response straight back.

So from your side it's a normal HTTP call to our host, plus one header.

---

## Connection

| | Value |
|---|---|
| Base URL | `https://api.drohnenwetter.de` (HTTPS only, via Cloudflare) |
| Auth header | `X-API-Key: <key>` on **every** request |

You get two internal keys (shared separately — they are **not** in this repo):

| Key | Use it for |
|---|---|
| `<HERE_INTERNAL_KEY>` | both HERE routes (`/here/geocode/`, `/here/revgeocode/`) |
| `<OWM_INTERNAL_KEY>`  | the OpenWeather route (`/owm/`) |

The keys are **route-scoped**: the HERE key will not open the OpenWeather route
(and vice versa) — you get a `401`.

---

## The routes

For each provider call you used to make, swap the base URL for the gateway path
below and **drop the provider key** from the query string. The gateway adds it.

### 1. HERE — forward geocoding (address/place → coordinates)

```
GET /here/geocode/v1/geocode?q=<query>
Header: X-API-Key: <HERE_INTERNAL_KEY>
```

- Was: `https://geocode.search.hereapi.com/v1/geocode?q=...&apiKey=...`
- The `/here/geocode` prefix is stripped before forwarding, so everything after
  it (e.g. `/v1/geocode?q=Berlin`) goes to HERE exactly as-is.
- Any `apiKey` you send is ignored/overwritten — leave it out.

Example:

```sh
curl -H 'X-API-Key: <HERE_INTERNAL_KEY>' \
  'https://api.drohnenwetter.de/here/geocode/v1/geocode?q=Berlin'
```

### 2. HERE — reverse geocoding (coordinates → address)

```
GET /here/revgeocode/v1/revgeocode?at=<lat>,<lng>
Header: X-API-Key: <HERE_INTERNAL_KEY>
```

- Was: `https://revgeocode.search.hereapi.com/v1/revgeocode?at=...&apiKey=...`
- Note this is a **different HERE host** than forward geocoding, which is why
  it's a separate route. Same internal key, though.

Example:

```sh
curl -H 'X-API-Key: <HERE_INTERNAL_KEY>' \
  'https://api.drohnenwetter.de/here/revgeocode/v1/revgeocode?at=52.52,13.405'
```

### 3. OpenWeather — One Call 3.0 (current/forecast weather)

```
GET /owm/data/3.0/onecall?lat=<lat>&lon=<lon>
Header: X-API-Key: <OWM_INTERNAL_KEY>
```

- Was: `https://api.openweathermap.org/data/3.0/onecall?lat=...&lon=...&appid=...`
- Drop `appid` — the gateway injects it.
- Standard One Call params still work, e.g. `&units=metric&exclude=minutely,hourly`.

Example:

```sh
curl -H 'X-API-Key: <OWM_INTERNAL_KEY>' \
  'https://api.drohnenwetter.de/owm/data/3.0/onecall?lat=52.52&lon=13.405&units=metric'
```

---

## Rules to code against

- **Method:** `GET` only. Anything else → `405`.
- **Request body:** keep it tiny (max 4 KB); these are GETs anyway.
- **Don't send provider keys** (`apiKey`, `appid`) — they're ignored and the
  gateway's own credential is used.
- **Response** is the provider's response, passed through unchanged (same JSON,
  same status codes from HERE/OpenWeather).

## Responses & error handling

| Status | Meaning | What to do |
|---|---|---|
| `200` | Success | Use the body as you would the provider's. |
| `401` | Missing/wrong/wrong-route `X-API-Key` | Check the header and that you're using the right key for the route. |
| `405` | Non-GET method | Use GET. |
| `429` | Rate limit hit (shared budget) | Back off. Read the `Retry-After` header (seconds) and retry after that. |
| `502`/`504` | Upstream provider error/timeout | Treat like a provider outage; retry with backoff. |

### Rate limits (shared across ALL app instances)

These are global budgets — every drohnenwetter instance draws from the same
bucket, so the provider quota can't be blown by scaling out.

| Route | Sustained | Burst |
|---|---|---|
| `/here/geocode/`, `/here/revgeocode/` | ~3 req/s | up to 30 |
| `/owm/` | ~1000 req/day | up to 60 at once |

On every response you get standard headers to drive your client:
`RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`, and on a `429`,
`Retry-After`. **Please honor `Retry-After`** instead of hammering.

## Debugging / support

Every response includes an `X-Request-Id` header. If something looks wrong,
grab that value and send it to ops — it maps directly to one line in the
gateway log, which makes a problem trivial to trace. The log records the route,
status, client IP, and which key (`identity`) was used.

## Quick self-test

```sh
# should be 200 + JSON:
curl -H 'X-API-Key: <HERE_INTERNAL_KEY>' \
  'https://api.drohnenwetter.de/here/geocode/v1/geocode?q=Berlin'

# should be 401 (no key):
curl -o /dev/null -w '%{http_code}\n' \
  'https://api.drohnenwetter.de/here/geocode/v1/geocode?q=Berlin'
```
