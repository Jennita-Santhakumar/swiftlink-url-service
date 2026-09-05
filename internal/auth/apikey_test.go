package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateAPIKeyShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		plaintext, hash, err := GenerateAPIKey()
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(plaintext, keyPrefix))
		require.False(t, seen[plaintext], "generated duplicate API key")
		seen[plaintext] = true
		require.Equal(t, HashAPIKey(plaintext), hash)
	}
}

func TestHashAPIKeyDeterministic(t *testing.T) {
	h1 := HashAPIKey("usk_sometoken")
	h2 := HashAPIKey("usk_sometoken")
	require.Equal(t, h1, h2)

	h3 := HashAPIKey("usk_differenttoken")
	require.NotEqual(t, h1, h3)
}

func TestHashAPIKeyNeverEqualsPlaintext(t *testing.T) {
	plaintext, hash, err := GenerateAPIKey()
	require.NoError(t, err)
	require.NotEqual(t, plaintext, hash, "the hash must never equal the plaintext key")
}
