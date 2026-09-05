package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
)

var ErrSlugAlreadyExists = errors.New("short code already exists")

const urlSelectColumns = `id, short_code, long_url, user_id, coalesce(title, ''), coalesce(description, ''),
	track_clicks, track_referrer, track_device, track_geo,
	created_at, updated_at, expires_at, is_active, tags`

func scanURL(row pgx.Row) (*shortener.URL, error) {
	var u shortener.URL
	var tagsRaw []byte
	err := row.Scan(
		&u.ID, &u.ShortCode, &u.LongURL, &u.UserID, &u.Title, &u.Description,
		&u.TrackClicks, &u.TrackReferrer, &u.TrackDevice, &u.TrackGeo,
		&u.CreatedAt, &u.UpdatedAt, &u.ExpiresAt, &u.IsActive, &tagsRaw,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(tagsRaw, &u.Tags)
	return &u, nil
}

// CreateURL inserts a new URL row. Returns ErrSlugAlreadyExists on a
// short_code unique-constraint violation — the atomic fallback the
// application must respect even after a Bloom-filter check, since the
// filter only prevents most collisions, not all (false positives from the
// filter side are handled earlier; this handles the true race).
func (s *Store) CreateURL(ctx context.Context, u *shortener.URL) error {
	tagsJSON, err := json.Marshal(u.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	return s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `
			INSERT INTO urls (short_code, long_url, user_id, title, description,
				track_clicks, track_referrer, track_device, track_geo, expires_at, tags)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, created_at, updated_at`,
			u.ShortCode, u.LongURL, u.UserID, u.Title, u.Description,
			u.TrackClicks, u.TrackReferrer, u.TrackDevice, u.TrackGeo, u.ExpiresAt, tagsJSON,
		)
		scanErr := row.Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
		if scanErr != nil && isUniqueViolation(scanErr) {
			return ErrSlugAlreadyExists
		}
		return scanErr
	})
}

func (s *Store) GetURLByShortCode(ctx context.Context, shortCode string) (*shortener.URL, error) {
	var u *shortener.URL
	err := s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `SELECT `+urlSelectColumns+` FROM urls WHERE short_code = $1`, shortCode)
		result, scanErr := scanURL(row)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if scanErr != nil {
			return scanErr
		}
		u = result
		return nil
	})
	return u, err
}

// GetURLClickCount returns the number of click events recorded for a short
// code (used in list/detail responses).
func (s *Store) GetURLClickCount(ctx context.Context, shortCode string) (int64, error) {
	var count int64
	err := s.withBreaker(ctx, func() error {
		return s.pool.QueryRow(ctx, `SELECT count(*) FROM click_events WHERE short_code = $1`, shortCode).Scan(&count)
	})
	return count, err
}

type UpdateURLFields struct {
	Title         *string
	TrackClicks   *bool
	TrackReferrer *bool
	TrackDevice   *bool
	TrackGeo      *bool
	ExpiresAt     *time.Time
}

// UpdateURL applies only the provided fields (nil = leave unchanged),
// scoped to a specific owner so one user cannot edit another's URL.
func (s *Store) UpdateURL(ctx context.Context, shortCode, ownerUserID string, fields UpdateURLFields) (*shortener.URL, error) {
	var u *shortener.URL
	err := s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `
			UPDATE urls SET
				title = COALESCE($3, title),
				track_clicks = COALESCE($4, track_clicks),
				track_referrer = COALESCE($5, track_referrer),
				track_device = COALESCE($6, track_device),
				track_geo = COALESCE($7, track_geo),
				expires_at = COALESCE($8, expires_at)
			WHERE short_code = $1 AND user_id = $2
			RETURNING `+urlSelectColumns,
			shortCode, ownerUserID, fields.Title, fields.TrackClicks, fields.TrackReferrer,
			fields.TrackDevice, fields.TrackGeo, fields.ExpiresAt,
		)
		result, scanErr := scanURL(row)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if scanErr != nil {
			return scanErr
		}
		u = result
		return nil
	})
	return u, err
}

type ListURLsParams struct {
	UserID string
	Page   int
	Limit  int
	Sort   string // created_at | clicks | title
	Order  string // asc | desc
}

func (s *Store) ListURLsByUser(ctx context.Context, p ListURLsParams) ([]*shortener.URL, int, error) {
	sortCol := map[string]string{"created_at": "created_at", "title": "title"}[p.Sort]
	if sortCol == "" {
		sortCol = "created_at"
	}
	order := "DESC"
	if p.Order == "asc" {
		order = "ASC"
	}

	var urls []*shortener.URL
	var total int
	err := s.withBreaker(ctx, func() error {
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM urls WHERE user_id = $1`, p.UserID).Scan(&total); err != nil {
			return err
		}

		offset := (p.Page - 1) * p.Limit
		query := fmt.Sprintf(`SELECT %s FROM urls WHERE user_id = $1 ORDER BY %s %s LIMIT $2 OFFSET $3`,
			urlSelectColumns, sortCol, order)
		rows, err := s.pool.Query(ctx, query, p.UserID, p.Limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			u, err := scanURL(rows)
			if err != nil {
				return err
			}
			urls = append(urls, u)
		}
		return rows.Err()
	})
	return urls, total, err
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
