package shortener

import "time"

type URL struct {
	ID            int64
	ShortCode     string
	LongURL       string
	UserID        string
	Title         string
	Description   string
	TrackClicks   bool
	TrackReferrer bool
	TrackDevice   bool
	TrackGeo      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ExpiresAt     *time.Time
	IsActive      bool
	Tags          []string
	ClicksCount   int64
}

type ShortenRequest struct {
	LongURL       string   `json:"long_url" validate:"required"`
	CustomSlug    string   `json:"custom_slug,omitempty"`
	Title         string   `json:"title,omitempty"`
	TrackClicks   *bool    `json:"track_clicks,omitempty"`
	TrackReferrer *bool    `json:"track_referrer,omitempty"`
	TrackDevice   *bool    `json:"track_device,omitempty"`
	TrackGeo      *bool    `json:"track_geo,omitempty"`
	ExpiresInDays int      `json:"expires_in_days,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	UserID        string   `json:"-"`
}

type ClickEvent struct {
	ShortCode      string
	IPAddress      string
	Country        string
	UserAgent      string
	DeviceType     string
	Browser        string
	OS             string
	Referrer       string
	ReferrerDomain string
	ClickedAt      time.Time
	ResponseTimeMs int
}
