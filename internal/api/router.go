package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/KabileshRajaselvan/url-shortener/internal/auth"
	"github.com/KabileshRajaselvan/url-shortener/internal/ratelimit"
	"github.com/KabileshRajaselvan/url-shortener/internal/store"
)

type RouterConfig struct {
	RateLimitPerMin int
	CORSOrigins     []string
}

func NewRouter(h *Handler, s *store.Store, redisClient *redis.Client, cfg RouterConfig, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(RequestLogger(logger))

	r.Get("/health", h.Health)
	r.Handle("/metrics", promhttp.Handler())

	// The redirect route lives at the root, matching the PRD's
	// GET /{short_code} — unauthenticated, no rate limit (it's the public,
	// performance-critical read path; only write/analytics endpoints that
	// require auth are rate-limited).
	r.Get("/{short_code}", h.Redirect)

	authMW := auth.RequireAPIKey(s)
	rateMW := ratelimit.Middleware(redisClient, cfg.RateLimitPerMin, h.metrics)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/register", h.Register)

		r.Group(func(r chi.Router) {
			r.Use(authMW)
			r.With(rateMW).Post("/shorten", h.CreateURL)
			r.Get("/urls", h.ListURLs)
			r.Patch("/urls/{short_code}", h.UpdateURL)
			r.Get("/urls/{short_code}/analytics", h.GetAnalytics)
		})
	})

	return withCORS(r, cfg.CORSOrigins)
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
