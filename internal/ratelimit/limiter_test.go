package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KabileshRajaselvan/url-shortener/internal/auth"
	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
)

func testMetrics() *metrics.Metrics {
	return metrics.New(prometheus.NewRegistry())
}

func TestRateLimitAllowsUnderLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	called := 0
	handler := Middleware(client, 5, testMetrics())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := auth.WithTestAPIKeyID(httptest.NewRequest(http.MethodPost, "/api/v1/shorten", nil), "key-1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	require.Equal(t, 5, called)
}

func TestRateLimitRejectsOverLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	handler := Middleware(client, 3, testMetrics())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var lastCode int
	for i := 0; i < 5; i++ {
		req := auth.WithTestAPIKeyID(httptest.NewRequest(http.MethodPost, "/api/v1/shorten", nil), "key-2")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	require.Equal(t, http.StatusTooManyRequests, lastCode)
}

func TestRateLimitIsolatedPerKey(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	handler := Middleware(client, 1, testMetrics())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := auth.WithTestAPIKeyID(httptest.NewRequest(http.MethodPost, "/api/v1/shorten", nil), "key-a")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)

	req2 := auth.WithTestAPIKeyID(httptest.NewRequest(http.MethodPost, "/api/v1/shorten", nil), "key-b")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code, "a different API key must have its own limit bucket")
}
