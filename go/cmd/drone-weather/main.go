package main

import (
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/olizimmermann/drone-weather/internal/assessment"
	"github.com/olizimmermann/drone-weather/internal/handler"
)

// version is injected at build time via -ldflags="-X main.version=x.y"
var version = "dev"

// ── Rate limiting ─────────────────────────────────────────────────────────────

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var limiters sync.Map // map[string]*ipLimiter

func getLimiter(ip string) *rate.Limiter {
	now := time.Now()
	v, _ := limiters.LoadOrStore(ip, &ipLimiter{
		limiter:  rate.NewLimiter(rate.Every(time.Minute/5), 5),
		lastSeen: now,
	})
	il := v.(*ipLimiter)
	il.lastSeen = now
	return il.limiter
}

func cleanLimiters() {
	for range time.Tick(5 * time.Minute) {
		cutoff := time.Now().Add(-10 * time.Minute)
		limiters.Range(func(k, v interface{}) bool {
			if v.(*ipLimiter).lastSeen.Before(cutoff) {
				limiters.Delete(k)
			}
			return true
		})
	}
}

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
		"add": func(a, b int) int { return a + b },
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

	mux.HandleFunc("/impressum", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.ExecuteTemplate(w, "impressum.html", struct{ Address string }{}); err != nil {
			log.Printf("[impressum] template error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})
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
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Drohnenwetter server listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
