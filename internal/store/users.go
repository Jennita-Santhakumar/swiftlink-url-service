package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID        string
	Email     string
	CreatedAt string
}

// GetOrCreateUserByEmail returns the existing user for email, or creates
// one. Simplified per this project's auth scope: registration has no
// password — a user is just an identity to own URLs and API keys under.
func (s *Store) GetOrCreateUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `
			INSERT INTO users (email) VALUES ($1)
			ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
			RETURNING id, email, created_at::text`, email)
		return row.Scan(&u.ID, &u.Email, &u.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get or create user: %w", err)
	}
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.withBreaker(ctx, func() error {
		row := s.pool.QueryRow(ctx, `SELECT id, email, created_at::text FROM users WHERE id = $1`, id)
		err := row.Scan(&u.ID, &u.Email, &u.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}
