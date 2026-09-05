package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
)

// InsertClickEventsBatch writes a batch of click events in a single
// multi-row INSERT (via pgx's CopyFrom-free batch builder) — this is the
// async, batched write path that replaces the PRD's Kafka pipeline.
func (s *Store) InsertClickEventsBatch(ctx context.Context, events []shortener.ClickEvent) error {
	if len(events) == 0 {
		return nil
	}
	return s.withBreaker(ctx, func() error {
		rows := make([][]any, len(events))
		for i, e := range events {
			var ip any
			if e.IPAddress != "" {
				ip = e.IPAddress
			}
			rows[i] = []any{
				e.ShortCode, ip, nullIfEmpty(e.Country), e.UserAgent,
				nullIfEmpty(e.DeviceType), nullIfEmpty(e.Browser), nullIfEmpty(e.OS),
				nullIfEmpty(e.Referrer), nullIfEmpty(e.ReferrerDomain), e.ClickedAt, e.ResponseTimeMs,
			}
		}
		_, err := s.pool.CopyFrom(ctx,
			pgx.Identifier{"click_events"},
			[]string{"short_code", "ip_address", "country", "user_agent", "device_type",
				"browser", "os", "referrer", "referrer_domain", "clicked_at", "response_time_ms"},
			pgx.CopyFromRows(rows),
		)
		if err != nil {
			return fmt.Errorf("failed to batch-insert click events: %w", err)
		}
		return nil
	})
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// EnsurePartitionForMonth creates a monthly click_events partition if it
// doesn't already exist. Called ahead of time by the worker so writes
// never fall through to the default partition in normal operation.
func (s *Store) EnsurePartitionForMonth(ctx context.Context, monthsFromNow int) error {
	return s.withBreaker(ctx, func() error {
		_, err := s.pool.Exec(ctx, `
			DO $$
			DECLARE
				month_start date := (date_trunc('month', CURRENT_DATE) + ($1 || ' months')::interval)::date;
				month_end date := (date_trunc('month', CURRENT_DATE) + (($1 + 1) || ' months')::interval)::date;
				partition_name text := 'click_events_' || to_char(month_start, 'YYYY_MM');
			BEGIN
				EXECUTE format(
					'CREATE TABLE IF NOT EXISTS %I PARTITION OF click_events FOR VALUES FROM (%L) TO (%L)',
					partition_name, month_start, month_end
				);
			END $$;
		`, monthsFromNow)
		return err
	})
}

type DailyStat struct {
	Day            string `json:"date"`
	Clicks         int    `json:"clicks"`
	UniqueVisitors int    `json:"unique_visitors"`
}

// RefreshDailyRollup recomputes daily_click_stats from raw click_events for
// the given lookback window, called periodically by the worker.
func (s *Store) RefreshDailyRollup(ctx context.Context, lookbackDays int) error {
	return s.withBreaker(ctx, func() error {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO daily_click_stats (short_code, day, clicks, unique_visitors, updated_at)
			SELECT short_code, clicked_at::date AS day, count(*), count(DISTINCT ip_address), now()
			FROM click_events
			WHERE clicked_at >= now() - ($1 || ' days')::interval
			GROUP BY short_code, clicked_at::date
			ON CONFLICT (short_code, day) DO UPDATE SET
				clicks = EXCLUDED.clicks,
				unique_visitors = EXCLUDED.unique_visitors,
				updated_at = EXCLUDED.updated_at
		`, lookbackDays)
		return err
	})
}

func (s *Store) GetDailyStats(ctx context.Context, shortCode, startDate, endDate string) ([]DailyStat, error) {
	var stats []DailyStat
	err := s.withBreaker(ctx, func() error {
		rows, err := s.pool.Query(ctx, `
			SELECT day::text, clicks, unique_visitors FROM daily_click_stats
			WHERE short_code = $1 AND day BETWEEN $2::date AND $3::date
			ORDER BY day ASC`, shortCode, startDate, endDate)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d DailyStat
			if err := rows.Scan(&d.Day, &d.Clicks, &d.UniqueVisitors); err != nil {
				return err
			}
			stats = append(stats, d)
		}
		return rows.Err()
	})
	return stats, err
}

type BreakdownRow struct {
	Key   string
	Count int
}

// GetBreakdown computes a real GROUP BY over raw click_events for the
// requested dimension. Unlike the timeline (served from the rollup table),
// these are computed directly — a documented scope choice, fine at demo
// data volumes (see README's Design Decisions).
func (s *Store) GetBreakdown(ctx context.Context, shortCode, column, startDate, endDate string) ([]BreakdownRow, error) {
	allowed := map[string]bool{"country": true, "device_type": true, "referrer_domain": true, "browser": true}
	if !allowed[column] {
		return nil, fmt.Errorf("invalid breakdown column %q", column)
	}

	var rows []BreakdownRow
	err := s.withBreaker(ctx, func() error {
		query := fmt.Sprintf(`
			SELECT coalesce(%s, 'unknown'), count(*) FROM click_events
			WHERE short_code = $1 AND clicked_at BETWEEN $2::timestamptz AND $3::timestamptz
			GROUP BY %s ORDER BY count(*) DESC LIMIT 20`, column, column)
		result, err := s.pool.Query(ctx, query, shortCode, startDate, endDate)
		if err != nil {
			return err
		}
		defer result.Close()
		for result.Next() {
			var r BreakdownRow
			if err := result.Scan(&r.Key, &r.Count); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return result.Err()
	})
	return rows, err
}

func (s *Store) GetTotalClicksAndUniqueVisitors(ctx context.Context, shortCode, startDate, endDate string) (total int, unique int, err error) {
	err = s.withBreaker(ctx, func() error {
		return s.pool.QueryRow(ctx, `
			SELECT count(*), count(DISTINCT ip_address) FROM click_events
			WHERE short_code = $1 AND clicked_at BETWEEN $2::timestamptz AND $3::timestamptz`,
			shortCode, startDate, endDate,
		).Scan(&total, &unique)
	})
	return total, unique, err
}
