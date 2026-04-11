package handler

import (
	"net"
	"net/http"
	"strings"
)

// clientIP extracts the real client IP, preferring Cloudflare's header.
// NOTE: duplicated in cmd/drohnenwetter/main.go — keep both in sync.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.SplitN(ip, ",", 2)[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
