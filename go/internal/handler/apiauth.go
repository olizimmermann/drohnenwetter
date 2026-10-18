package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
)

// The public API is never reached directly: zateway authenticates the
// customer's X-API-Key, applies the per-key rate limit and forwards the request
// with a shared internal secret plus the caller's identity and features.
const (
	headerInternalSecret  = "X-Internal-Secret"
	headerGatewayIdentity = "X-Gateway-Identity"
	headerGatewayFeatures = "X-Gateway-Features"
	headerRequestID       = "X-Request-Id"
)

// FeatureOpenSky enables live air traffic in API responses for a key.
const FeatureOpenSky = "opensky"

// apiCaller is what the gateway told us about the authenticated customer.
type apiCaller struct {
	identity  string
	features  map[string]bool
	requestID string
}

type apiCallerKey struct{}

func callerFrom(ctx context.Context) apiCaller {
	c, _ := ctx.Value(apiCallerKey{}).(apiCaller)
	return c
}

// tokenRe bounds identity/feature/request-id values so they are safe to log.
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// APIAuth guards the dispatcher API. Requests without the correct internal
// secret get a plain 404 so the endpoint is not discoverable. An empty secret
// disables the API entirely.
func APIAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(headerInternalSecret)
		if secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			http.NotFound(w, r)
			return
		}

		c := apiCaller{identity: "unknown", features: map[string]bool{}}
		if id := r.Header.Get(headerGatewayIdentity); tokenRe.MatchString(id) {
			c.identity = id
		}
		for _, f := range strings.Split(r.Header.Get(headerGatewayFeatures), ",") {
			if f = strings.ToLower(strings.TrimSpace(f)); tokenRe.MatchString(f) {
				c.features[f] = true
			}
		}
		if rid := r.Header.Get(headerRequestID); tokenRe.MatchString(rid) {
			c.requestID = rid
		} else {
			c.requestID = newRequestID()
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), apiCallerKey{}, c)))
	})
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "dw-norand"
	}
	return hex.EncodeToString(b[:])
}
