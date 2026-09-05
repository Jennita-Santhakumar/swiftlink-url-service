package shortener

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBase62RoundTrip(t *testing.T) {
	cases := []int64{0, 1, 61, 62, 63, 12345, 9223372036854775807}
	for _, n := range cases {
		encoded := EncodeToBase62(n)
		decoded, err := DecodeFromBase62(encoded)
		require.NoError(t, err)
		assert.Equal(t, n, decoded, "round trip failed for %d", n)
	}
}

func TestBase62KnownValues(t *testing.T) {
	assert.Equal(t, "0", EncodeToBase62(0))
	assert.Equal(t, "1", EncodeToBase62(1))
	assert.Equal(t, "Z", EncodeToBase62(35))
	assert.Equal(t, "10", EncodeToBase62(62))
}

func TestBase62DecodeInvalidCharacter(t *testing.T) {
	_, err := DecodeFromBase62("abc!def")
	require.Error(t, err, "invalid characters must error, not silently corrupt the result")
}

func TestBase62DecodeEmptyString(t *testing.T) {
	_, err := DecodeFromBase62("")
	require.Error(t, err)
}

func TestSnowflakeUniqueUnderConcurrency(t *testing.T) {
	sf, err := NewSnowflake(1)
	require.NoError(t, err)

	const n = 20000
	ids := make([]int64, n)
	var wg sync.WaitGroup
	workers := 8
	perWorker := n / workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				ids[offset+i] = sf.Generate()
			}
		}(w * perWorker)
	}
	wg.Wait()

	seen := make(map[int64]bool, n)
	for _, id := range ids {
		require.False(t, seen[id], "duplicate snowflake ID generated: %d", id)
		seen[id] = true
	}
}

func TestSnowflakeMonotonicWithinSingleGoroutine(t *testing.T) {
	sf, err := NewSnowflake(2)
	require.NoError(t, err)

	prev := sf.Generate()
	for i := 0; i < 1000; i++ {
		next := sf.Generate()
		assert.Greater(t, next, prev, "snowflake IDs must be strictly increasing when generated sequentially")
		prev = next
	}
}

func TestNewSnowflakeRejectsInvalidWorkerID(t *testing.T) {
	_, err := NewSnowflake(-1)
	require.Error(t, err)
	_, err = NewSnowflake(maxWorkerID + 1)
	require.Error(t, err)
}
