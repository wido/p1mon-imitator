// Package api serves the subset of the P1 Monitor HTTP API that the Home
// Assistant p1_monitor integration uses.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/wido/p1mon-imitator/internal/p1mon"
)

// Handler builds the HTTP routes. configuration is the pre-rendered
// /api/v1/configuration document.
func Handler(latest *p1mon.Latest, configuration []byte, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Query parameters (json=object, limit=1, round=on) are accepted and
	// ignored: this server always answers as if json=object&limit=1 was given,
	// which is the only form the Home Assistant client sends.
	mux.HandleFunc("GET /api/v1/smartmeter", func(w http.ResponseWriter, r *http.Request) {
		s := latest.Get()
		if s == nil {
			noData(w)
			return
		}
		writeJSON(w, s.SmartMeter)
	})
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		s := latest.Get()
		if s == nil {
			noData(w)
			return
		}
		writeJSON(w, s.Status)
	})
	mux.HandleFunc("GET /api/v1/configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, configuration)
	})

	// No water meter: an empty list makes the client raise P1MonitorNoDataError,
	// after which Home Assistant stops asking.
	empty := []byte("[]")
	mux.HandleFunc("GET /api/v2/watermeter/day", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, empty)
	})
	mux.HandleFunc("GET /api/v1/watermeter/day", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, empty)
	})

	return logRequests(mux, log)
}

// NewServer wraps the handler in an http.Server with conservative limits so a
// misbehaving client cannot make the process grow.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    4 << 10,
	}
}

func writeJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// noData tells the client to retry: the client library turns a non-2xx status
// into a connection error and Home Assistant polls again 5 seconds later.
func noData(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "5")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"error":"no telegram received from meter yet"}`))
}

func logRequests(next http.Handler, log *slog.Logger) http.Handler {
	if log == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug("http", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
