// Package shortener implements short-code generation: a Snowflake-style
// distributed ID generator and Base62 encoding, per the PRD's own design —
// hardened with input validation the PRD's sketch omitted.
package shortener

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// EncodeToBase62 encodes a non-negative int64 as a Base62 string.
func EncodeToBase62(num int64) string {
	if num == 0 {
		return "0"
	}
	if num < 0 {
		num = -num
	}

	var result []byte
	for num > 0 {
		result = append([]byte{base62Chars[num%62]}, result...)
		num /= 62
	}
	return string(result)
}

// DecodeFromBase62 decodes a Base62 string back to an int64. Unlike the
// PRD's sketch — which calls strings.IndexByte and never checks for -1 (an
// invalid character silently corrupts the decoded value instead of
// erroring) — this validates every character.
func DecodeFromBase62(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty base62 string")
	}
	var result int64
	for _, ch := range s {
		idx := strings.IndexByte(base62Chars, byte(ch))
		if idx < 0 {
			return 0, fmt.Errorf("invalid base62 character %q", ch)
		}
		result = result*62 + int64(idx)
	}
	return result, nil
}

// Snowflake generates distributed, roughly-time-sortable 63-bit IDs:
// 41 bits timestamp (ms since epoch) | 10 bits worker ID | 12 bits sequence.
type Snowflake struct {
	epoch    int64
	workerID int64
	mu       sync.Mutex
	lastTS   int64
	sequence int64
}

const (
	workerIDBits = 10
	sequenceBits = 12
	maxWorkerID  = (1 << workerIDBits) - 1
	maxSequence  = (1 << sequenceBits) - 1
)

// snowflakeEpoch is a fixed reference point (2025-01-01 UTC) so generated
// IDs stay well within int64 range for decades.
var snowflakeEpoch = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

func NewSnowflake(workerID int64) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("worker ID must be between 0 and %d, got %d", maxWorkerID, workerID)
	}
	return &Snowflake{epoch: snowflakeEpoch, workerID: workerID, lastTS: -1}, nil
}

func timeInMs() int64 {
	return time.Now().UnixMilli()
}

// Generate returns a new unique ID. Safe for concurrent use.
func (s *Snowflake) Generate() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := timeInMs()
	if ts < s.lastTS {
		// Clock moved backwards (NTP adjustment, etc). Clamp to lastTS
		// rather than risk emitting a duplicate/out-of-order ID — the
		// PRD's own sketch has no handling for this at all.
		ts = s.lastTS
	}

	if ts == s.lastTS {
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			// Sequence exhausted within this millisecond; spin until the
			// clock advances rather than risk a duplicate ID.
			for ts <= s.lastTS {
				ts = timeInMs()
			}
		}
	} else {
		s.sequence = 0
	}

	s.lastTS = ts
	return ((ts - s.epoch) << (workerIDBits + sequenceBits)) | (s.workerID << sequenceBits) | s.sequence
}
