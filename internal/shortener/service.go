package shortener

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidURL        = errors.New("invalid long_url")
	ErrInvalidSlug       = errors.New("invalid custom_slug")
	ErrSlugAlreadyExists = errors.New("custom_slug already exists")
	ErrNotFound          = errors.New("short URL not found")
	ErrURLExpired        = errors.New("short URL has expired")
)

// Store, Cache, and BloomFilter are the narrow interfaces the service
// depends on, so handler/service tests can substitute fakes without a
// real Postgres/Redis.
type Store interface {
	CreateURL(ctx context.Context, u *URL) error
	GetURLByShortCode(ctx context.Context, shortCode string) (*URL, error)
}

type Cache interface {
	Get(ctx context.Context, shortCode string) (string, error)
	Set(ctx context.Context, shortCode, longURL string, ttl time.Duration) error
}

type BloomFilter interface {
	Add(ctx context.Context, key string) error
	MightContain(ctx context.Context, key string) (bool, error)
}

type IDGenerator interface {
	Generate() int64
}

type URLService struct {
	store    Store
	cache    Cache
	bloom    BloomFilter
	idGen    IDGenerator
	cacheTTL time.Duration
}

func NewURLService(store Store, cache Cache, bloom BloomFilter, idGen IDGenerator, cacheTTL time.Duration) *URLService {
	return &URLService{store: store, cache: cache, bloom: bloom, idGen: idGen, cacheTTL: cacheTTL}
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func (s *URLService) ShortenURL(ctx context.Context, req *ShortenRequest) (*URL, error) {
	if !IsValidLongURL(req.LongURL) {
		return nil, ErrInvalidURL
	}

	shortCode := req.CustomSlug
	if shortCode != "" {
		if !IsValidCustomSlug(shortCode) {
			return nil, ErrInvalidSlug
		}
		// Bloom filter check first (cheap, no DB round trip for the common
		// case of a fresh slug); a positive result still requires a real
		// DB check since bloom filters can false-positive, and the actual
		// collision is caught atomically by the DB's unique constraint
		// regardless (see store.CreateURL).
		mightExist, err := s.bloom.MightContain(ctx, shortCode)
		if err != nil {
			return nil, fmt.Errorf("bloom filter check failed: %w", err)
		}
		if mightExist {
			if _, err := s.store.GetURLByShortCode(ctx, shortCode); err == nil {
				return nil, ErrSlugAlreadyExists
			}
		}
	} else {
		shortCode = EncodeToBase62(s.idGen.Generate())
	}

	url := &URL{
		ShortCode:     shortCode,
		LongURL:       req.LongURL,
		UserID:        req.UserID,
		Title:         req.Title,
		TrackClicks:   boolOr(req.TrackClicks, true),
		TrackReferrer: boolOr(req.TrackReferrer, true),
		TrackDevice:   boolOr(req.TrackDevice, true),
		TrackGeo:      boolOr(req.TrackGeo, true),
		IsActive:      true,
		Tags:          req.Tags,
	}
	if req.ExpiresInDays > 0 {
		expires := time.Now().AddDate(0, 0, req.ExpiresInDays)
		url.ExpiresAt = &expires
	}

	if err := s.store.CreateURL(ctx, url); err != nil {
		return nil, err
	}

	// Best-effort cache/bloom updates: don't fail the request if these
	// fail, matching the PRD's own "don't fail the request if caching
	// fails" note — the cache will simply be populated on the next read.
	_ = s.cache.Set(ctx, shortCode, url.LongURL, s.cacheTTL)
	_ = s.bloom.Add(ctx, shortCode)

	return url, nil
}

// ResolveURL implements the critical redirect path: cache first, DB on
// miss, re-cache for next time.
func (s *URLService) ResolveURL(ctx context.Context, shortCode string) (*URL, bool, error) {
	if longURL, err := s.cache.Get(ctx, shortCode); err == nil {
		return &URL{ShortCode: shortCode, LongURL: longURL, IsActive: true}, true, nil
	}

	url, err := s.store.GetURLByShortCode(ctx, shortCode)
	if err != nil {
		return nil, false, ErrNotFound
	}
	if !url.IsActive {
		return nil, false, ErrNotFound
	}
	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		return nil, false, ErrURLExpired
	}

	_ = s.cache.Set(ctx, shortCode, url.LongURL, s.cacheTTL)
	return url, false, nil
}
