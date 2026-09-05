// Package config loads process configuration from environment variables,
// shared by cmd/api, cmd/worker, and cmd/migrate.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL,required"`

	APIPort  int    `env:"API_PORT" envDefault:"8080"`
	BaseURL  string `env:"BASE_URL" envDefault:"http://localhost:8040"`
	WorkerID int64  `env:"SNOWFLAKE_WORKER_ID" envDefault:"1"`

	RateLimitPerMin int `env:"RATE_LIMIT_PER_MIN" envDefault:"60"`

	CacheTTLHours int `env:"CACHE_TTL_HOURS" envDefault:"24"`

	// Bloom filter sizing: ~100K expected slugs at ~1% false-positive rate.
	BloomFilterSize   uint64 `env:"BLOOM_FILTER_SIZE" envDefault:"958506"`
	BloomFilterHashes int    `env:"BLOOM_FILTER_HASHES" envDefault:"7"`

	WorkerMetricsPort      int `env:"WORKER_METRICS_PORT" envDefault:"9104"`
	RollupIntervalSeconds  int `env:"ROLLUP_INTERVAL_SECONDS" envDefault:"60"`
	PartitionCheckInterval int `env:"PARTITION_CHECK_INTERVAL_SECONDS" envDefault:"3600"`

	BatchFlushSize     int `env:"BATCH_FLUSH_SIZE" envDefault:"500"`
	BatchFlushMillis   int `env:"BATCH_FLUSH_MILLIS" envDefault:"1000"`
	BatchChannelBuffer int `env:"BATCH_CHANNEL_BUFFER" envDefault:"10000"`

	ShutdownGraceSeconds int `env:"SHUTDOWN_GRACE_SECONDS" envDefault:"30"`

	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`
}

// Load loads a .env file if present (local dev convenience only), then
// parses process environment into Config.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, nil
}
