package cache

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestBloom(t *testing.T, size uint64, numHashes int) *BloomFilter {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return NewBloomFilter(client, size, numHashes)
}

func TestBloomFilterNoFalseNegatives(t *testing.T) {
	bf := newTestBloom(t, 100_000, 7)
	ctx := context.Background()

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("short-code-%d", i)
		require.NoError(t, bf.Add(ctx, keys[i]))
	}

	for _, k := range keys {
		present, err := bf.MightContain(ctx, k)
		require.NoError(t, err)
		require.True(t, present, "bloom filter must never false-negative on an inserted key: %s", k)
	}
}

func TestBloomFilterFalsePositiveRateWithinExpectedBound(t *testing.T) {
	// Sized for ~100K entries at ~1% target FP rate (matches config.go's
	// default BloomFilterSize/BloomFilterHashes).
	const size = 958_506
	const numHashes = 7
	bf := newTestBloom(t, size, numHashes)
	ctx := context.Background()

	const inserted = 10_000
	for i := 0; i < inserted; i++ {
		require.NoError(t, bf.Add(ctx, fmt.Sprintf("inserted-%d", i)))
	}

	falsePositives := 0
	const trials = 5000
	for i := 0; i < trials; i++ {
		present, err := bf.MightContain(ctx, fmt.Sprintf("never-inserted-%d", i))
		require.NoError(t, err)
		if present {
			falsePositives++
		}
	}

	fpRate := float64(falsePositives) / float64(trials)
	// At 10K/958K fill with k=7, expected FP rate is well under 1%; allow
	// generous headroom (5%) so this stays stable across hash-seed luck.
	require.Less(t, fpRate, 0.05, "false positive rate %.4f exceeds expected bound", fpRate)
}

func TestBloomFilterDefinitelyAbsent(t *testing.T) {
	bf := newTestBloom(t, 100_000, 7)
	ctx := context.Background()

	present, err := bf.MightContain(ctx, "never-added")
	require.NoError(t, err)
	require.False(t, present)
}
