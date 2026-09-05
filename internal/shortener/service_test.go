package shortener

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	mu   sync.Mutex
	urls map[string]*URL
}

func newFakeStore() *fakeStore { return &fakeStore{urls: map[string]*URL{}} }

func (f *fakeStore) CreateURL(ctx context.Context, u *URL) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.urls[u.ShortCode]; exists {
		return ErrSlugAlreadyExists
	}
	u.CreatedAt = time.Now()
	u.UpdatedAt = u.CreatedAt
	f.urls[u.ShortCode] = u
	return nil
}

func (f *fakeStore) GetURLByShortCode(ctx context.Context, shortCode string) (*URL, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.urls[shortCode]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

type fakeCache struct {
	mu    sync.Mutex
	items map[string]string
}

func newFakeCache() *fakeCache { return &fakeCache{items: map[string]string{}} }

func (f *fakeCache) Get(ctx context.Context, shortCode string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.items[shortCode]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fakeCache) Set(ctx context.Context, shortCode, longURL string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[shortCode] = longURL
	return nil
}

type fakeBloom struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newFakeBloom() *fakeBloom { return &fakeBloom{seen: map[string]bool{}} }

func (f *fakeBloom) Add(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen[key] = true
	return nil
}

func (f *fakeBloom) MightContain(ctx context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[key], nil
}

type fakeIDGen struct {
	mu  sync.Mutex
	cur int64
}

func (g *fakeIDGen) Generate() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cur++
	return g.cur
}

func newTestService() *URLService {
	return NewURLService(newFakeStore(), newFakeCache(), newFakeBloom(), &fakeIDGen{}, time.Hour)
}

func TestShortenURLWithCustomSlug(t *testing.T) {
	svc := newTestService()
	res, err := svc.ShortenURL(context.Background(), &ShortenRequest{
		LongURL: "https://example.com/very/long/path", CustomSlug: "mylink", UserID: "user123",
	})
	require.NoError(t, err)
	require.Equal(t, "mylink", res.ShortCode)
}

func TestShortenURLGeneratesCodeWithoutSlug(t *testing.T) {
	svc := newTestService()
	res, err := svc.ShortenURL(context.Background(), &ShortenRequest{
		LongURL: "https://example.com", UserID: "user123",
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.ShortCode)
}

func TestShortenURLRejectsInvalidURL(t *testing.T) {
	svc := newTestService()
	_, err := svc.ShortenURL(context.Background(), &ShortenRequest{LongURL: "not-a-url", UserID: "u"})
	require.ErrorIs(t, err, ErrInvalidURL)
}

func TestShortenURLRejectsInvalidSlug(t *testing.T) {
	svc := newTestService()
	_, err := svc.ShortenURL(context.Background(), &ShortenRequest{
		LongURL: "https://example.com", CustomSlug: "has space", UserID: "u",
	})
	require.ErrorIs(t, err, ErrInvalidSlug)
}

func TestShortenURLDuplicateSlugRejected(t *testing.T) {
	svc := newTestService()
	req := &ShortenRequest{LongURL: "https://example.com", CustomSlug: "taken", UserID: "u"}
	_, err := svc.ShortenURL(context.Background(), req)
	require.NoError(t, err)

	_, err = svc.ShortenURL(context.Background(), req)
	require.ErrorIs(t, err, ErrSlugAlreadyExists)
}

func TestResolveURLCacheThenDB(t *testing.T) {
	svc := newTestService()
	req := &ShortenRequest{LongURL: "https://example.com/path", CustomSlug: "resolvetest", UserID: "u"}
	_, err := svc.ShortenURL(context.Background(), req)
	require.NoError(t, err)

	resolved, hit, err := svc.ResolveURL(context.Background(), "resolvetest")
	require.NoError(t, err)
	require.True(t, hit, "should be served from cache since ShortenURL populates it")
	require.Equal(t, req.LongURL, resolved.LongURL)
}

func TestResolveURLNotFound(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.ResolveURL(context.Background(), "does-not-exist")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestResolveURLExpired(t *testing.T) {
	store := newFakeStore()
	svc := NewURLService(store, newFakeCache(), newFakeBloom(), &fakeIDGen{}, time.Hour)

	past := time.Now().Add(-time.Hour)
	store.urls["expired"] = &URL{ShortCode: "expired", LongURL: "https://example.com", IsActive: true, ExpiresAt: &past}

	_, _, err := svc.ResolveURL(context.Background(), "expired")
	require.ErrorIs(t, err, ErrURLExpired)
}
