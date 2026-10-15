package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
)

func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(instrument)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Route("/api", func(r chi.Router) {
		r.Get("/players/search", h.SearchPlayers)
		r.Get("/players/{id}/today", h.TodayResult)
		r.Get("/players/{id}/today/jobs/{jobId}", h.JobStatus)
		r.Get("/images/proxy", h.ProxyImage)
		r.Get("/stats/rgx-jobs", h.RgxJobStats)
		r.Post("/feedback", h.SubmitFeedback)
	})

	// Simple health check.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// instrument records request metrics and writes one JSON log line per request
// (except /health, which is probed constantly). Labels use the chi route
// pattern, never the raw path, so player and job IDs can't blow up cardinality
// — the pattern is only known once chi has routed, hence read after next.
func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		elapsed := time.Since(start)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK // nothing written: net/http sends an implicit 200
		}
		method := methodLabel(r.Method)
		metrics.HTTPRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
		metrics.HTTPDuration.WithLabelValues(route, method).Observe(elapsed.Seconds())

		if route == "/health" {
			return
		}
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "http request", "method", method, "route", route,
			"status", status, "duration_seconds", elapsed.Seconds(), "bytes", ww.BytesWritten())
	})
}

// methodLabel caps the method label to the standard verbs, since clients can
// send arbitrary method strings.
func methodLabel(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "other"
}
