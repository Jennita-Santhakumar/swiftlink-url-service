package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/KabileshRajaselvan/url-shortener/internal/auth"
	"github.com/KabileshRajaselvan/url-shortener/internal/geoip"
	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
	"github.com/KabileshRajaselvan/url-shortener/internal/store"
	"github.com/KabileshRajaselvan/url-shortener/internal/useragent"
	"github.com/KabileshRajaselvan/url-shortener/pkg/apierror"
	"github.com/KabileshRajaselvan/url-shortener/pkg/qrcode"
)

// Tracker is the subset of *analytics.Tracker the handler needs.
type Tracker interface {
	Track(evt shortener.ClickEvent)
}

type Handler struct {
	service   *shortener.URLService
	store     *store.Store
	tracker   Tracker
	geo       *geoip.Lookup
	validator *validator.Validate
	logger    *slog.Logger
	metrics   *metrics.Metrics
	baseURL   string
}

func NewHandler(service *shortener.URLService, s *store.Store, tracker Tracker, geo *geoip.Lookup, m *metrics.Metrics, logger *slog.Logger, baseURL string) *Handler {
	return &Handler{
		service:   service,
		store:     s,
		tracker:   tracker,
		geo:       geo,
		validator: validator.New(),
		logger:    logger,
		metrics:   m,
		baseURL:   baseURL,
	}
}

func timeoutCtx(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// Register handles POST /api/v1/register — no password/JWT, per this
// project's simplified auth scope: an email identifies the user, and a
// fresh API key is issued and returned exactly once.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 5*time.Second)
	defer cancel()

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}
	if err := h.validator.Struct(req); err != nil {
		apierror.Write(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	user, err := h.store.GetOrCreateUserByEmail(ctx, req.Email)
	if err != nil {
		h.logger.Error("failed to create user", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to register")
		return
	}

	plaintext, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("failed to generate API key", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to issue API key")
		return
	}
	if _, err := h.store.CreateAPIKey(ctx, user.ID, hash, req.KeyName); err != nil {
		h.logger.Error("failed to persist API key", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to issue API key")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(RegisterResponse{
		UserID:  user.ID,
		Email:   user.Email,
		APIKey:  plaintext,
		Warning: "store this key now — it cannot be retrieved again",
	})
}

// CreateURL handles POST /api/v1/shorten
func (h *Handler) CreateURL(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 5*time.Second)
	defer cancel()

	userID, err := auth.UserIDFromContext(ctx)
	if err != nil {
		apierror.Write(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req shortener.ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}
	req.UserID = userID

	start := time.Now()
	created, err := h.service.ShortenURL(ctx, &req)
	h.metrics.CreateLatency.Observe(time.Since(start).Seconds())
	if err != nil {
		h.handleShortenError(w, err)
		return
	}
	h.metrics.URLsCreated.Inc()

	shortURL := h.baseURL + "/" + created.ShortCode
	qr, err := qrcode.GenerateDataURI(shortURL)
	if err != nil {
		h.logger.Warn("failed to generate QR code", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ShortenResponse{
		ShortCode: created.ShortCode,
		ShortURL:  shortURL,
		LongURL:   created.LongURL,
		Title:     created.Title,
		CreatedAt: created.CreatedAt,
		ExpiresAt: created.ExpiresAt,
		QRCode:    qr,
	})
}

func (h *Handler) handleShortenError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shortener.ErrInvalidURL):
		apierror.Write(w, http.StatusBadRequest, "invalid_url", "invalid long_url")
	case errors.Is(err, shortener.ErrInvalidSlug):
		apierror.Write(w, http.StatusBadRequest, "invalid_slug", "custom_slug must match ^[A-Za-z0-9_-]{1,64}$")
	case errors.Is(err, shortener.ErrSlugAlreadyExists), errors.Is(err, store.ErrSlugAlreadyExists):
		apierror.Write(w, http.StatusConflict, "slug_exists", "custom slug already exists")
	default:
		h.logger.Error("failed to shorten URL", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to shorten URL")
	}
}

// Redirect handles GET /{short_code} — the critical, unauthenticated path.
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 2*time.Second)
	defer cancel()

	shortCode := chi.URLParam(r, "short_code")
	start := time.Now()

	resolved, cacheHit, err := h.service.ResolveURL(ctx, shortCode)
	h.metrics.RedirectLatency.Observe(time.Since(start).Seconds())

	if err != nil {
		switch {
		case errors.Is(err, shortener.ErrURLExpired):
			h.metrics.Redirects.WithLabelValues("expired").Inc()
			apierror.Write(w, http.StatusGone, "expired", "short URL has expired")
		default:
			h.metrics.Redirects.WithLabelValues("not_found").Inc()
			apierror.Write(w, http.StatusNotFound, "not_found", "short URL not found")
		}
		return
	}

	resultLabel := "miss"
	if cacheHit {
		resultLabel = "hit"
	}
	h.metrics.Redirects.WithLabelValues(resultLabel).Inc()

	h.trackClick(r, shortCode, time.Since(start))

	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Header().Set("X-Short-Code-Original", shortCode)
	http.Redirect(w, r, resolved.LongURL, http.StatusMovedPermanently)
}

func (h *Handler) trackClick(r *http.Request, shortCode string, latency time.Duration) {
	ua := useragent.Parse(r.Header.Get("User-Agent"))
	referrer := r.Header.Get("Referer")
	ip := clientIP(r)

	evt := shortener.ClickEvent{
		ShortCode:      shortCode,
		IPAddress:      ip,
		UserAgent:      r.Header.Get("User-Agent"),
		DeviceType:     ua.DeviceType,
		Browser:        ua.Browser,
		OS:             ua.OS,
		Referrer:       referrer,
		ReferrerDomain: referrerDomain(referrer),
		ClickedAt:      time.Now(),
		ResponseTimeMs: int(latency.Milliseconds()),
	}
	if ip != "" {
		evt.Country = h.geo.Country(ip)
	}
	h.tracker.Track(evt)
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func referrerDomain(referrer string) string {
	if referrer == "" {
		return ""
	}
	u, err := url.Parse(referrer)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// GetAnalytics handles GET /api/v1/urls/{short_code}/analytics
func (h *Handler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 10*time.Second)
	defer cancel()

	userID, err := auth.UserIDFromContext(ctx)
	if err != nil {
		apierror.Write(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	shortCode := chi.URLParam(r, "short_code")
	owned, err := h.store.GetURLByShortCode(ctx, shortCode)
	if errors.Is(err, store.ErrNotFound) {
		apierror.Write(w, http.StatusNotFound, "not_found", "short URL not found")
		return
	}
	if err != nil {
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to fetch URL")
		return
	}
	if owned.UserID != userID {
		apierror.Write(w, http.StatusNotFound, "not_found", "short URL not found")
		return
	}

	startDate, endDate := parseDateRange(r)

	total, unique, err := h.store.GetTotalClicksAndUniqueVisitors(ctx, shortCode, startDate, endDate)
	if err != nil {
		h.logger.Error("failed to fetch click totals", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to fetch analytics")
		return
	}

	timelineRows, err := h.store.GetDailyStats(ctx, shortCode, startDate[:10], endDate[:10])
	if err != nil {
		h.logger.Error("failed to fetch timeline", "error", err)
	}
	timeline := make([]TimelinePoint, 0, len(timelineRows))
	for _, t := range timelineRows {
		timeline = append(timeline, TimelinePoint{Date: t.Day, Clicks: t.Clicks, UniqueVisitors: t.UniqueVisitors})
	}

	byCountry := h.breakdown(ctx, shortCode, "country", startDate, endDate, total)
	byDevice := h.breakdown(ctx, shortCode, "device_type", startDate, endDate, total)
	byReferrer := h.breakdown(ctx, shortCode, "referrer_domain", startDate, endDate, total)
	browsers := h.breakdown(ctx, shortCode, "browser", startDate, endDate, total)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(AnalyticsResponse{
		ShortCode:      shortCode,
		TotalClicks:    total,
		UniqueVisitors: unique,
		Timeline:       timeline,
		ByCountry:      byCountry,
		ByDevice:       byDevice,
		ByReferrer:     byReferrer,
		Browsers:       browsers,
	})
}

func (h *Handler) breakdown(ctx context.Context, shortCode, column, startDate, endDate string, total int) []BreakdownItem {
	rows, err := h.store.GetBreakdown(ctx, shortCode, column, startDate, endDate)
	if err != nil {
		h.logger.Error("failed to fetch breakdown", "column", column, "error", err)
		return nil
	}
	items := make([]BreakdownItem, 0, len(rows))
	for _, row := range rows {
		pct := 0.0
		if total > 0 {
			pct = float64(row.Count) / float64(total) * 100
		}
		items = append(items, BreakdownItem{Key: row.Key, Count: row.Count, Percentage: pct})
	}
	return items
}

func parseDateRange(r *http.Request) (string, string) {
	q := r.URL.Query()
	end := time.Now()
	start := end.AddDate(0, 0, -30)
	if v := q.Get("start_date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		} else if t, err := time.Parse("2006-01-02", v); err == nil {
			start = t
		}
	}
	if v := q.Get("end_date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		} else if t, err := time.Parse("2006-01-02", v); err == nil {
			end = t
		}
	}
	// RFC3339Nano (not RFC3339, which truncates to whole seconds) — a
	// whole-second boundary can fall between a click's sub-second
	// timestamp and the analytics request's own time.Now(), excluding a
	// click that happened earlier in the very same second.
	return start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)
}

// UpdateURL handles PATCH /api/v1/urls/{short_code}
func (h *Handler) UpdateURL(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 5*time.Second)
	defer cancel()

	userID, err := auth.UserIDFromContext(ctx)
	if err != nil {
		apierror.Write(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req UpdateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	shortCode := chi.URLParam(r, "short_code")
	updated, err := h.store.UpdateURL(ctx, shortCode, userID, store.UpdateURLFields{
		Title: req.Title, TrackClicks: req.TrackClicks, TrackReferrer: req.TrackReferrer,
		TrackDevice: req.TrackDevice, TrackGeo: req.TrackGeo, ExpiresAt: req.ExpiresAt,
	})
	if errors.Is(err, store.ErrNotFound) {
		apierror.Write(w, http.StatusNotFound, "not_found", "short URL not found")
		return
	}
	if err != nil {
		h.logger.Error("failed to update URL", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to update URL")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"short_code": updated.ShortCode,
		"updated_at": updated.UpdatedAt,
		"title":      updated.Title,
	})
}

// ListURLs handles GET /api/v1/urls
func (h *Handler) ListURLs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 5*time.Second)
	defer cancel()

	userID, err := auth.UserIDFromContext(ctx)
	if err != nil {
		apierror.Write(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	q := r.URL.Query()
	page := 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	limit := 20
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}

	urls, total, err := h.store.ListURLsByUser(ctx, store.ListURLsParams{
		UserID: userID, Page: page, Limit: limit, Sort: q.Get("sort"), Order: q.Get("order"),
	})
	if err != nil {
		h.logger.Error("failed to list URLs", "error", err)
		apierror.Write(w, http.StatusInternalServerError, "internal_error", "failed to list URLs")
		return
	}

	items := make([]URLListItem, 0, len(urls))
	for _, u := range urls {
		clicks, _ := h.store.GetURLClickCount(ctx, u.ShortCode)
		items = append(items, toURLListItem(u, h.baseURL, clicks))
	}

	pages := (total + limit - 1) / limit
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ListURLsResponse{
		URLs:       items,
		Pagination: Pagination{Page: page, Limit: limit, Total: total, Pages: pages},
	})
}

// Health handles GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeoutCtx(r, 3*time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		apierror.Write(w, http.StatusServiceUnavailable, "unhealthy", "database unreachable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
