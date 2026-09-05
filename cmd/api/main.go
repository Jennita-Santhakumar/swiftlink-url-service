// Command api runs the URL shortener's REST API, including the fast
// redirect path and the in-process click-tracking batch writer.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/KabileshRajaselvan/url-shortener/internal/analytics"
	"github.com/KabileshRajaselvan/url-shortener/internal/api"
	"github.com/KabileshRajaselvan/url-shortener/internal/cache"
	"github.com/KabileshRajaselvan/url-shortener/internal/config"
	"github.com/KabileshRajaselvan/url-shortener/internal/geoip"
	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
	"github.com/KabileshRajaselvan/url-shortener/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	logger := config.NewLogger(cfg.LogFormat, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer s.Close()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid REDIS_URL", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	geo, err := geoip.New()
	if err != nil {
		logger.Error("failed to load geoip dataset", "error", err)
		os.Exit(1)
	}

	m := metrics.New(prometheus.DefaultRegisterer)

	urlCache := cache.NewRedisCache(redisClient)
	bloom := cache.NewBloomFilter(redisClient, cfg.BloomFilterSize, cfg.BloomFilterHashes)
	idGen, err := shortener.NewSnowflake(cfg.WorkerID)
	if err != nil {
		logger.Error("invalid snowflake worker ID", "error", err)
		os.Exit(1)
	}
	service := shortener.NewURLService(s, urlCache, bloom, idGen, time.Duration(cfg.CacheTTLHours)*time.Hour)

	// Tracker gets its own cancellation, stopped only after the HTTP server
	// has finished draining in-flight requests — otherwise a redirect still
	// being served during shutdown could push an event after the tracker
	// has already flushed and exited, silently losing it.
	trackerCtx, stopTracker := context.WithCancel(context.Background())
	defer stopTracker()
	tracker := analytics.NewTracker(s, cfg.BatchChannelBuffer, cfg.BatchFlushSize, cfg.BatchFlushMillis, logger, m)
	go tracker.Run(trackerCtx)

	h := api.NewHandler(service, s, tracker, geo, m, logger, cfg.BaseURL)
	router := api.NewRouter(h, s, redisClient, api.RouterConfig{
		RateLimitPerMin: cfg.RateLimitPerMin,
		CORSOrigins:     corsOrigins(),
	}, logger)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api server listening", "port", cfg.APIPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down api server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownGraceSeconds)*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	stopTracker() // now safe: no more in-flight requests can call tracker.Track
}

func corsOrigins() []string {
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		return []string{v}
	}
	return nil
}
