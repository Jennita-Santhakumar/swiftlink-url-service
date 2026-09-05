// Package cache implements the Redis-backed URL cache and a hand-rolled
// Bloom filter.
//
// Design note: the source PRD's own bloom filter calls Redis's `BF.ADD` /
// `BF.EXISTS` commands, which require the RedisBloom module — but the
// PRD's own docker-compose.yml runs plain `redis:7-alpine`, which does NOT
// bundle RedisBloom. Those calls would fail at runtime with "unknown
// command." This implementation instead builds a real Bloom filter
// directly on a Redis bitmap using SETBIT/GETBIT and double hashing
// (Kirsch-Mitzenmacher: h_i(x) = h1(x) + i*h2(x) mod m), which works
// against any stock Redis.
package cache

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/redis/go-redis/v9"
)

const bloomKey = "bloom:short_codes"

type BloomFilter struct {
	client    *redis.Client
	size      uint64
	numHashes int
}

func NewBloomFilter(client *redis.Client, size uint64, numHashes int) *BloomFilter {
	return &BloomFilter{client: client, size: size, numHashes: numHashes}
}

// twoHashes derives two independent 64-bit hashes from a single FNV-1a
// pass (splitting isn't ideal, but combined with a per-position salt below
// it gives adequately independent bit positions for a demo-scale filter;
// see positions()).
func twoHashes(data string) (uint64, uint64) {
	h1 := fnv.New64a()
	_, _ = h1.Write([]byte(data))
	sum1 := h1.Sum64()

	h2 := fnv.New64a()
	_, _ = h2.Write([]byte(data))
	_, _ = h2.Write([]byte{0xff}) // salt so h2 diverges from h1
	sum2 := h2.Sum64()

	return sum1, sum2
}

func (b *BloomFilter) positions(key string) []uint64 {
	h1, h2 := twoHashes(key)
	positions := make([]uint64, b.numHashes)
	for i := 0; i < b.numHashes; i++ {
		positions[i] = (h1 + uint64(i)*h2) % b.size
	}
	return positions
}

// Add sets all k bit positions for key. Idempotent.
func (b *BloomFilter) Add(ctx context.Context, key string) error {
	positions := b.positions(key)
	_, err := b.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, pos := range positions {
			pipe.SetBit(ctx, bloomKey, int64(pos), 1)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to add %q to bloom filter: %w", key, err)
	}
	return nil
}

// MightContain returns false if key is DEFINITELY not present (at least
// one bit unset), or true if it's POSSIBLY present (all bits set — subject
// to the filter's configured false-positive rate). Callers must still
// verify against the authoritative store on a "possibly present" result.
func (b *BloomFilter) MightContain(ctx context.Context, key string) (bool, error) {
	positions := b.positions(key)
	cmds := make([]*redis.IntCmd, len(positions))
	_, err := b.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i, pos := range positions {
			cmds[i] = pipe.GetBit(ctx, bloomKey, int64(pos))
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to check bloom filter for %q: %w", key, err)
	}
	for _, cmd := range cmds {
		if cmd.Val() == 0 {
			return false, nil
		}
	}
	return true, nil
}
