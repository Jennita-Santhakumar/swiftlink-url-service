package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/KabileshRajaselvan/url-shortener/internal/store"
	"github.com/KabileshRajaselvan/url-shortener/pkg/apierror"
)

// WithTestUserID and WithTestAPIKeyID exist so other packages' tests
// (e.g. internal/ratelimit) can construct a request as if it already
// passed RequireAPIKey, without depending on this package's unexported
// context key type.
func WithTestUserID(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDContextKey, userID))
}

func WithTestAPIKeyID(r *http.Request, apiKeyID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), apiKeyContextKey, apiKeyID))
}

type contextKey string

const (
	userIDContextKey contextKey = "user_id"
	apiKeyContextKey contextKey = "api_key_id"
)

// Authenticator is the subset of *store.Store the middleware needs,
// letting handler tests substitute a fake.
type Authenticator interface {
	AuthenticateByKeyHash(ctx context.Context, keyHash string) (*store.APIKey, error)
}

// RequireAPIKey enforces `Authorization: Bearer <key>` on protected routes.
func RequireAPIKey(store Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) {
				apierror.Write(w, http.StatusUnauthorized, "unauthorized", "missing or malformed Authorization header")
				return
			}
			plaintext := strings.TrimPrefix(header, prefix)

			key, err := store.AuthenticateByKeyHash(r.Context(), HashAPIKey(plaintext))
			if err != nil {
				apierror.Write(w, http.StatusUnauthorized, "unauthorized", "invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, key.UserID)
			ctx = context.WithValue(ctx, apiKeyContextKey, key.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var ErrNoUserInContext = errors.New("no authenticated user in request context")

func UserIDFromContext(ctx context.Context) (string, error) {
	v, ok := ctx.Value(userIDContextKey).(string)
	if !ok || v == "" {
		return "", ErrNoUserInContext
	}
	return v, nil
}

func APIKeyIDFromContext(ctx context.Context) (string, error) {
	v, ok := ctx.Value(apiKeyContextKey).(string)
	if !ok || v == "" {
		return "", ErrNoUserInContext
	}
	return v, nil
}
