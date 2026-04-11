package main

import (
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/olizimmermann/drohnenwetter/internal/assessment"
	"github.com/olizimmermann/drohnenwetter/internal/handler"
)

// version is injected at build time via -ldflags="-X main.version=x.y"
var version = "dev"

// ── Rate limiting ─────────────────────────────────────────────────────────────

type ipLimiter struct {
	limiter  *rate.Limiter
	mu       sync.Mutex
	lastSeen time.Time
}

var limiters sync.Map // map[string]*ipLimiter

func getLimiter(ip string) *rate.Limiter {
	now := time.Now()
	v, _ := limiters.LoadOrStore(ip, &ipLimiter{
		limiter:  rate.NewLimiter(rate.Every(time.Minute/10), 10),
		lastSeen: now,
	})
	il := v.(*ipLimiter)
	il.mu.Lock()
	il.lastSeen = now
	il.mu.Unlock()
	return il.limiter
}

func cleanLimiters() {
	for range time.Tick(5 * time.Minute) {
		cutoff := time.Now().Add(-10 * time.Minute)
		limiters.Range(func(k, v interface{}) bool {
			il := v.(*ipLimiter)
			il.mu.Lock()
			expired := il.lastSeen.Before(cutoff)
			il.mu.Unlock()
			if expired {
				limiters.Delete(k)
			}
			return true
		})
	}
}

// clientIP extracts the real client IP, preferring Cloudflare's header.
// NOTE: duplicated in internal/handler/util.go — keep both in sync.
func clientIP(r *http.Request) string {
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !getLimiter(clientIP(r)).Allow() {
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", clientIP(r), r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://unpkg.com; style-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https://*.tile.openstreetmap.org https://uas-betrieb.de; connect-src 'self' https://uas-betrieb.de https://unpkg.com https://*.tile.openstreetmap.org")
		next.ServeHTTP(w, r)
	})
}

// ── Template loading ──────────────────────────────────────────────────────────

func loadTemplates(dir string) *template.Template {
	funcMap := template.FuncMap{
		"appVersion": func() string { return version },
		"findAlt": func(entries []assessment.AltEntry, key string) assessment.AltEntry {
			for _, e := range entries {
				if e.Key == key {
					return e
				}
			}
			return assessment.AltEntry{}
		},
		"add":   func(a, b int) int { return a + b },
		"msToKmh": func(ms float64) float64 { return math.Round(ms*3.6*10) / 10 },
	}
	tmpl := template.New("").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(dir + "/*.html"))
	return tmpl
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	tmplDir := os.Getenv("TEMPLATE_DIR")
	if tmplDir == "" {
		tmplDir = "templates"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "static"
	}

	tmpl := loadTemplates(tmplDir)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, staticDir+"/favicon.ico")
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, staticDir+"/robots.txt")
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, staticDir+"/sitemap.xml")
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Service-Worker-Allowed", "/")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		http.ServeFile(w, r, staticDir+"/sw.js")
	})

	mux.Handle("/impressum", rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.ExecuteTemplate(w, "impressum.html", struct{ Address string }{}); err != nil {
			log.Printf("[impressum] template error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})))
	mux.Handle("/datenschutz", rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.ExecuteTemplate(w, "datenschutz.html", struct{ Address string }{}); err != nil {
			log.Printf("[datenschutz] template error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})))
	mux.Handle("/zone-info", rateLimitMiddleware(http.HandlerFunc(handler.ZoneInfoHandler)))
	mux.Handle("/track", rateLimitMiddleware(http.HandlerFunc(handler.TrackHandler)))
	mux.Handle("/traffic", rateLimitMiddleware(http.HandlerFunc(handler.TrafficHandler)))
	mux.Handle("/", rateLimitMiddleware(handler.NewHomeHandler(tmpl)))
	mux.Handle("/results", rateLimitMiddleware(handler.NewResultsHandler(
		tmpl,
		os.Getenv("HERE_API_KEY"),
		os.Getenv("OPENWEATHER_TOKEN"),
	)))

	go cleanLimiters()

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      loggingMiddleware(securityHeadersMiddleware(mux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Drohnenwetter server listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
