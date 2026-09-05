//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/KabileshRajaselvan/url-shortener/internal/analytics"
	"github.com/KabileshRajaselvan/url-shortener/internal/api"
	"github.com/KabileshRajaselvan/url-shortener/internal/auth"
	"github.com/KabileshRajaselvan/url-shortener/internal/cache"
	"github.com/KabileshRajaselvan/url-shortener/internal/geoip"
	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
	"github.com/KabileshRajaselvan/url-shortener/internal/store"
	"github.com/KabileshRajaselvan/url-shortener/tests/integration/testutil"
)

type testServer struct {
	*httptest.Server
	store *store.Store
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	s := testutil.NewStore(t)
	redisClient := testutil.NewRedisClient(t)

	geo, err := geoip.New()
	require.NoError(t, err)

	m := metrics.New(prometheus.NewRegistry())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	urlCache := cache.NewRedisCache(redisClient)
	bloom := cache.NewBloomFilter(redisClient, 100_000, 7)
	idGen, err := shortener.NewSnowflake(1)
	require.NoError(t, err)
	service := shortener.NewURLService(s, urlCache, bloom, idGen, time.Hour)

	// Tiny flush thresholds so tests don't need to wait long for click
	// events to land in Postgres.
	tracker := analytics.NewTracker(s, 1000, 1, 50, logger, m)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go tracker.Run(ctx)

	h := api.NewHandler(service, s, tracker, geo, m, logger, "")
	router := api.NewRouter(h, s, redisClient, api.RouterConfig{RateLimitPerMin: 1000}, logger)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	// baseURL depends on the actual httptest listener address, so
	// short_url in responses uses it — rebuild the handler with it.
	h2 := api.NewHandler(service, s, tracker, geo, m, logger, srv.URL)
	router2 := api.NewRouter(h2, s, redisClient, api.RouterConfig{RateLimitPerMin: 1000}, logger)
	srv.Config.Handler = router2

	return &testServer{Server: srv, store: s}
}

func (ts *testServer) register(t *testing.T, email string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email})
	resp, err := http.Post(ts.URL+"/api/v1/register", "application/json", strings.NewReader(string(body)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out struct {
		APIKey string `json:"api_key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out.APIKey
}

func TestFullLifecycle_CreateRedirectAnalytics(t *testing.T) {
	ts := newTestServer(t)
	apiKey := ts.register(t, "user1@example.com")

	// Create
	createBody, _ := json.Marshal(map[string]any{
		"long_url":    "https://example.com/landing-page",
		"custom_slug": "mytestlink",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/shorten", strings.NewReader(string(createBody)))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created struct {
		ShortCode string `json:"short_code"`
		QRCode    string `json:"qr_code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()
	require.Equal(t, "mytestlink", created.ShortCode)
	require.True(t, strings.HasPrefix(created.QRCode, "data:image/png;base64,"), "QR code must be a real generated PNG data URI")

	// Redirect (no-follow, real HTTP client)
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	redirectResp, err := client.Get(ts.URL + "/" + created.ShortCode)
	require.NoError(t, err)
	require.Equal(t, http.StatusMovedPermanently, redirectResp.StatusCode)
	require.Equal(t, "https://example.com/landing-page", redirectResp.Header.Get("Location"))
	redirectResp.Body.Close()

	// Give the async batch writer time to flush (flushEvery=50ms in this test server).
	require.Eventually(t, func() bool {
		total, _, err := ts.store.GetTotalClicksAndUniqueVisitors(context.Background(), created.ShortCode,
			time.Now().Add(-time.Hour).Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339))
		return err == nil && total == 1
	}, 3*time.Second, 50*time.Millisecond, "click event must land in Postgres via the batch writer")

	// Analytics (owner-only)
	analyticsReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/urls/"+created.ShortCode+"/analytics", nil)
	analyticsReq.Header.Set("Authorization", "Bearer "+apiKey)
	analyticsResp, err := http.DefaultClient.Do(analyticsReq)
	require.NoError(t, err)
	defer analyticsResp.Body.Close()
	require.Equal(t, http.StatusOK, analyticsResp.StatusCode)

	var analyticsOut struct {
		TotalClicks int `json:"total_clicks"`
	}
	require.NoError(t, json.NewDecoder(analyticsResp.Body).Decode(&analyticsOut))
	require.Equal(t, 1, analyticsOut.TotalClicks)
}

func TestCustomSlugCollisionReturns409(t *testing.T) {
	ts := newTestServer(t)
	apiKey := ts.register(t, "user2@example.com")

	body, _ := json.Marshal(map[string]any{"long_url": "https://example.com/a", "custom_slug": "taken-slug"})
	post := func() *http.Response {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/shorten", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	first := post()
	require.Equal(t, http.StatusCreated, first.StatusCode)
	first.Body.Close()

	second := post()
	defer second.Body.Close()
	require.Equal(t, http.StatusConflict, second.StatusCode)
}

func TestExpiredURLReturns410(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()

	user, err := ts.store.GetOrCreateUserByEmail(ctx, "user3@example.com")
	require.NoError(t, err)

	past := time.Now().Add(-time.Hour)
	u := &shortener.URL{
		ShortCode: "expiredlink", LongURL: "https://example.com", UserID: user.ID,
		TrackClicks: true, TrackReferrer: true, TrackDevice: true, TrackGeo: true,
		IsActive: true, ExpiresAt: &past,
	}
	require.NoError(t, ts.store.CreateURL(ctx, u))

	resp, err := http.Get(ts.URL + "/expiredlink")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusGone, resp.StatusCode)
}

func TestMissingAuthReturns401(t *testing.T) {
	ts := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"long_url": "https://example.com"})
	resp, err := http.Post(ts.URL+"/api/v1/shorten", "application/json", strings.NewReader(string(body)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRateLimitExceededReturns429(t *testing.T) {
	s := testutil.NewStore(t)
	redisClient := testutil.NewRedisClient(t)
	geo, err := geoip.New()
	require.NoError(t, err)
	m := metrics.New(prometheus.NewRegistry())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	urlCache := cache.NewRedisCache(redisClient)
	bloom := cache.NewBloomFilter(redisClient, 100_000, 7)
	idGen, err := shortener.NewSnowflake(1)
	require.NoError(t, err)
	service := shortener.NewURLService(s, urlCache, bloom, idGen, time.Hour)
	tracker := analytics.NewTracker(s, 1000, 500, 1000, logger, m)

	h := api.NewHandler(service, s, tracker, geo, m, logger, "")
	// Very small limit so the test doesn't need many requests.
	router := api.NewRouter(h, s, redisClient, api.RouterConfig{RateLimitPerMin: 2}, logger)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx := context.Background()
	user, err := s.GetOrCreateUserByEmail(ctx, "ratelimit@example.com")
	require.NoError(t, err)
	plaintextKey, hash, err := auth.GenerateAPIKey()
	require.NoError(t, err)
	_, err = s.CreateAPIKey(ctx, user.ID, hash, "test")
	require.NoError(t, err)

	post := func(slug string) int {
		body, _ := json.Marshal(map[string]any{"long_url": "https://example.com", "custom_slug": slug})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/shorten", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+plaintextKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return resp.StatusCode
	}

	var lastCode int
	for i := 0; i < 5; i++ {
		lastCode = post(fmt.Sprintf("ratelimit-slug-%d", i))
	}
	require.Equal(t, http.StatusTooManyRequests, lastCode)
}
