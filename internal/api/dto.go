package api

import (
	"time"

	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
)

type RegisterRequest struct {
	Email   string `json:"email" validate:"required,email"`
	KeyName string `json:"key_name,omitempty"`
}

type RegisterResponse struct {
	UserID  string `json:"user_id"`
	Email   string `json:"email"`
	APIKey  string `json:"api_key"`
	Warning string `json:"warning"`
}

type ShortenResponse struct {
	ShortCode   string     `json:"short_code"`
	ShortURL    string     `json:"short_url"`
	LongURL     string     `json:"long_url"`
	Title       string     `json:"title,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ClicksCount int64      `json:"clicks_count"`
	QRCode      string     `json:"qr_code"`
}

type UpdateURLRequest struct {
	Title         *string    `json:"title,omitempty"`
	TrackClicks   *bool      `json:"track_clicks,omitempty"`
	TrackReferrer *bool      `json:"track_referrer,omitempty"`
	TrackDevice   *bool      `json:"track_device,omitempty"`
	TrackGeo      *bool      `json:"track_geo,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type URLListItem struct {
	ShortCode string    `json:"short_code"`
	ShortURL  string    `json:"short_url"`
	Title     string    `json:"title,omitempty"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

type ListURLsResponse struct {
	URLs       []URLListItem `json:"urls"`
	Pagination Pagination    `json:"pagination"`
}

type BreakdownItem struct {
	Key        string  `json:"key"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

type TimelinePoint struct {
	Date           string `json:"date"`
	Clicks         int    `json:"clicks"`
	UniqueVisitors int    `json:"unique_visitors"`
}

type AnalyticsResponse struct {
	ShortCode      string          `json:"short_code"`
	TotalClicks    int             `json:"total_clicks"`
	UniqueVisitors int             `json:"unique_visitors"`
	Timeline       []TimelinePoint `json:"timeline"`
	ByCountry      []BreakdownItem `json:"by_country"`
	ByDevice       []BreakdownItem `json:"by_device"`
	ByReferrer     []BreakdownItem `json:"by_referrer"`
	Browsers       []BreakdownItem `json:"browsers"`
}

func toURLListItem(u *shortener.URL, baseURL string, clicks int64) URLListItem {
	return URLListItem{
		ShortCode: u.ShortCode,
		ShortURL:  baseURL + "/" + u.ShortCode,
		Title:     u.Title,
		Clicks:    clicks,
		CreatedAt: u.CreatedAt,
	}
}
