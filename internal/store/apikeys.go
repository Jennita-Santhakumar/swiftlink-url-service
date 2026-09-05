package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type APIKey struct {
	ID     string
	UserID string
	Name   string
}

func (s *Store) CreateAPIKey(ctx context.Context, userID, keyHash, name string) (string, error) {
	var id string
	err := s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `
			INSERT INTO api_keys (user_id, key_hash, name) VALUES ($1, $2, $3)
			RETURNING id`, userID, keyHash, name)
		return row.Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}
	return id, nil
}

// AuthenticateByKeyHash looks up an active API key by its hash and updates
// last_used_at. Returns ErrNotFound if no active key matches — the caller
// (auth middleware) maps that to 401, never leaking which part of the
// lookup failed.
func (s *Store) AuthenticateByKeyHash(ctx context.Context, keyHash string) (*APIKey, error) {
	var k APIKey
	err := s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `
			UPDATE api_keys SET last_used_at = now()
			WHERE key_hash = $1 AND is_active = true
			RETURNING id, user_id, coalesce(name, '')`, keyHash)
		err := row.Scan(&k.ID, &k.UserID, &k.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &k, nil
}
