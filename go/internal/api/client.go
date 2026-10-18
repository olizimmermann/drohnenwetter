package api

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrUnauthorized wraps any 4xx/5xx response with status 401, so callers
// (e.g. OpenSky) can detect a revoked/expired token and refresh.
var ErrUnauthorized = errors.New("unauthorized")

// ErrRateLimited indicates the gateway returned HTTP 429. Callers can surface a
// "too many requests" message; the gateway client also enters a short
// process-wide cooldown so concurrent requests stop hammering it.
var ErrRateLimited = errors.New("rate limited")

var httpClient = &http.Client{
	Timeout: 8 * time.Second,
}

// ── API gateway ────────────────────────────────────────────────────────────
//
// HERE and OpenWeather are reached through our own gateway, which injects the
// real provider credential server-side. The app only ever holds a route-scoped
// X-API-Key, supplied via the environment (.env) — never hard-coded here.

// gatewayBaseURL is the gateway root, e.g. "https://api.drohnenwetter.de",
// taken from GATEWAY_BASE_URL. Trailing slashes are trimmed so callers can
// always prefix an absolute path.
func gatewayBaseURL() string {
	return strings.TrimRight(os.Getenv("GATEWAY_BASE_URL"), "/")
}

var (
	gatewayClientOnce sync.Once
	gatewayClient     *http.Client
)

// gatewayTimeout is shorter than the default provider timeout: gateway calls
// sit on the user's critical path, so a dead/hung gateway should fail fast
// rather than block the request for the full 8s.
const gatewayTimeout = 5 * time.Second

// gatewayHTTPClient returns the client used for gateway calls. When
// GATEWAY_INSECURE_TLS=true it skips TLS verification, which exists solely for
// a local gateway's self-signed dev cert. All other provider
// traffic keeps full certificate verification via the default httpClient.
func gatewayHTTPClient() *http.Client {
	gatewayClientOnce.Do(func() {
		gatewayClient = &http.Client{Timeout: gatewayTimeout}
		if strings.EqualFold(os.Getenv("GATEWAY_INSECURE_TLS"), "true") {
			gatewayClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		}
	})
	return gatewayClient
}

// ── Rate-limit backoff ──────────────────────────────────────────────────────
//
// The gateway enforces one shared budget across all app instances and asks
// clients to honor Retry-After on a 429. Rather than block a user request by
// sleeping-and-retrying, we record a process-wide cooldown: while it is active,
// every gateway call short-circuits immediately, so all concurrent requests
// back off together instead of hammering an already-rate-limited gateway.

// rateLimitError is returned by doRequestWith on a 429 and carries the parsed
// Retry-After. It reports errors.Is(err, ErrRateLimited) == true.
type rateLimitError struct {
	retryAfter time.Duration
}

func (e *rateLimitError) Error() string {
	if e.retryAfter > 0 {
		return fmt.Sprintf("HTTP 429 rate limited, retry after %s", e.retryAfter)
	}
	return "HTTP 429 rate limited"
}

func (e *rateLimitError) Is(target error) bool { return target == ErrRateLimited }

// RetryAfter returns the gateway's Retry-After hint carried by a rate-limit
// error, or 0 if err is not one or the gateway sent none.
func RetryAfter(err error) time.Duration {
	var rle *rateLimitError
	if errors.As(err, &rle) {
		return rle.retryAfter
	}
	return 0
}

// gatewayCooldownUntil is the backoff deadline as unix-nanos (0 = no cooldown).
var gatewayCooldownUntil atomic.Int64

const (
	defaultGatewayCooldown = 5 * time.Second   // when Retry-After is missing/unparseable
	maxGatewayCooldown     = 120 * time.Second // clamp absurd Retry-After values
)

func gatewayCooldownRemaining() time.Duration {
	until := gatewayCooldownUntil.Load()
	if until == 0 {
		return 0
	}
	if d := time.Until(time.Unix(0, until)); d > 0 {
		return d
	}
	return 0
}

func setGatewayCooldown(d time.Duration) {
	if d <= 0 {
		d = defaultGatewayCooldown
	}
	if d > maxGatewayCooldown {
		d = maxGatewayCooldown
	}
	gatewayCooldownUntil.Store(time.Now().Add(d).UnixNano())
}

// parseRetryAfter reads the delta-seconds form of Retry-After (the form the
// gateway documents). Returns 0 if absent or not an integer ≥ 1.
func parseRetryAfter(h string) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// gatewayGet performs a GET against the gateway. path must be an absolute,
// already-encoded path (incl. query), e.g. "/here/geocode/v1/geocode?q=Berlin".
// apiKey is sent as X-API-Key; provider keys must NOT be in the query.
func gatewayGet(path, apiKey string, headers map[string]string) ([]byte, error) {
	base := gatewayBaseURL()
	if base == "" {
		return nil, fmt.Errorf("gateway: GATEWAY_BASE_URL not configured")
	}
	if d := gatewayCooldownRemaining(); d > 0 {
		return nil, fmt.Errorf("gateway: in rate-limit cooldown for %s: %w", d.Round(time.Second), ErrRateLimited)
	}
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	body, err := doRequestWith(gatewayHTTPClient(), req)
	var rle *rateLimitError
	if errors.As(err, &rle) {
		setGatewayCooldown(rle.retryAfter)
	}
	return body, err
}

// doGet executes a GET request and returns the response body.
func doGet(url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

// doPost executes a POST request with a JSON body and returns the response body.
func doPost(url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

// newFormPost creates a POST request with an application/x-www-form-urlencoded body.
func newFormPost(apiURL string, form url.Values) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

// maxResponseSize limits how much data we read from upstream APIs (10 MB).
const maxResponseSize = 10 * 1024 * 1024

func doRequest(req *http.Request) ([]byte, error) {
	return doRequestWith(httpClient, req)
}

func doRequestWith(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("HTTP %d from %s: %w", resp.StatusCode, redactURL(req.URL), ErrUnauthorized)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &rateLimitError{retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, redactURL(req.URL))
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
}

// redactURL returns the scheme://host/path of u without the query string, so
// secrets carried as query params (e.g. HERE apiKey, OpenWeather appid) never
// reach logs. The query is replaced with a "?<redacted>" marker when present.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	s := u.Scheme + "://" + u.Host + u.EscapedPath()
	if u.RawQuery != "" {
		s += "?<redacted>"
	}
	return s
}
