// Package auth implements API-key generation/verification and the HTTP
// middleware that enforces it — the project's simplified auth model (no
// JWT login, no subscription tiers; see README's Design Decisions).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const keyPrefix = "usk_" // "url shortener key" — makes leaked keys grep-able

// GenerateAPIKey returns a new plaintext API key (shown to the caller
// exactly once — the server only ever stores its hash) and its SHA-256
// hash for persistence.
func GenerateAPIKey() (plaintext, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("failed to generate random key: %w", err)
	}
	plaintext = keyPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashAPIKey(plaintext), nil
}

// HashAPIKey hashes a plaintext key for lookup/comparison. API keys are
// high-entropy random tokens (not low-entropy passwords), so a fast hash
// (SHA-256) is appropriate — unlike passwords, there's no offline
// brute-force risk worth paying bcrypt's cost for.
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
