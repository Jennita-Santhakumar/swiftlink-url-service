// Package ratelimit implements a flat per-API-key rate limit using Redis,
// following the PRD's own scheme (`ratelimit:{key}:{minute}` with a 1-minute
// TTL) — kept exactly as sketched since it's a sound, simple design.
//
// Scope note: with subscription tiers out of scope for this project (see
// README's Design Decisions), the limit is a single flat value
// (RATE_LIMIT_PER_MIN) applied to every API key, not tier-dependent.
package ratelimit

import (
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/KabileshRajaselvan/url-shortener/internal/auth"
	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
	"github.com/KabileshRajaselvan/url-shortener/pkg/apierror"
)

// Middleware must run after auth.RequireAPIKey so an api-key ID is present
// in the request context to key the limit on.
func Middleware(client *redis.Client, limitPerMin int, m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyID, err := auth.APIKeyIDFromContext(r.Context())
			if err != nil {
				// Auth middleware didn't run first or rejected the request
				// already; nothing to rate-limit.
				next.ServeHTTP(w, r)
				return
			}

			minute := time.Now().Unix() / 60
			redisKey := fmt.Sprintf("ratelimit:%s:%d", keyID, minute)

			count, err := client.Incr(r.Context(), redisKey).Result()
			if err != nil {
				// Fail open: a Redis hiccup must not block legitimate traffic.
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				client.Expire(r.Context(), redisKey, 60*time.Second)
			}

			if int(count) > limitPerMin {
				m.RateLimited.Inc()
				w.Header().Set("Retry-After", "60")
				apierror.Write(w, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
