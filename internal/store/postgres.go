// Package store implements PostgreSQL persistence for users, API keys,
// URLs, click events, and daily rollups.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"
)

type Store struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	settings := gobreaker.Settings{
		Name:        "postgres",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}

	return &Store{pool: pool, breaker: gobreaker.NewCircuitBreaker[any](settings)}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Exec runs an arbitrary statement, bypassing the circuit breaker. Used
// for test setup/teardown only.
func (s *Store) Exec(ctx context.Context, sql string) error {
	_, err := s.pool.Exec(ctx, sql)
	return err
}

func (s *Store) withBreaker(_ context.Context, fn func() error) error {
	_, err := s.breaker.Execute(func() (any, error) {
		return nil, fn()
	})
	return err
}
