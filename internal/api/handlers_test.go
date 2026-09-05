package api

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/KabileshRajaselvan/url-shortener/internal/auth"
	"github.com/KabileshRajaselvan/url-shortener/internal/geoip"
	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
)

// This suite covers validation logic that runs before the store is ever
// touched (CreateURL/Register request-shape validation). Store-dependent
// paths (create->redirect->analytics, ownership checks, 409/404/410) are
// exercised against real Postgres+Redis in tests/integration/.

type fakeTracker struct{ tracked []shortener.ClickEvent }

func (f *fakeTracker) Track(evt shortener.ClickEvent) { f.tracked = append(f.tracked, evt) }

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	geo, err := geoip.New()
	require.NoError(t, err)

	m := metrics.New(prometheus.NewRegistry())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return &Handler{
		tracker:   &fakeTracker{},
		geo:       geo,
		validator: validator.New(),
		logger:    logger,
		metrics:   m,
		baseURL:   "https://short.test",
	}
}

func TestRegister_InvalidBody(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRegister_InvalidEmail(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewBufferString(`{"email":"not-an-email"}`))
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateURL_Unauthorized(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", bytes.NewBufferString(`{"long_url":"https://example.com"}`))
	rec := httptest.NewRecorder()
	h.CreateURL(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code, "no user_id in context must be rejected before touching the store")
}

func TestCreateURL_InvalidBody(t *testing.T) {
	h := newTestHandler(t)
	req := auth.WithTestUserID(httptest.NewRequest(http.MethodPost, "/api/v1/shorten", bytes.NewBufferString("not json")), "user-1")
	rec := httptest.NewRecorder()
	h.CreateURL(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListURLs_Unauthorized(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	rec := httptest.NewRecorder()
	h.ListURLs(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetAnalytics_Unauthorized(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/urls/abc/analytics", nil)
	rec := httptest.NewRecorder()
	h.GetAnalytics(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestParseDateRangeDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/urls/abc/analytics", nil)
	start, end := parseDateRange(req)
	startT, err := time.Parse(time.RFC3339, start)
	require.NoError(t, err)
	endT, err := time.Parse(time.RFC3339, end)
	require.NoError(t, err)
	require.True(t, endT.Sub(startT) >= 29*24*time.Hour, "default range should be about 30 days")
}

func TestReferrerDomain(t *testing.T) {
	require.Equal(t, "twitter.com", referrerDomain("https://twitter.com/some/path?x=1"))
	require.Equal(t, "", referrerDomain(""))
	require.Equal(t, "", referrerDomain("not a url"))
}
