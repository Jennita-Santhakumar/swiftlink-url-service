// Command worker runs periodic maintenance: ensuring future click_events
// partitions exist ahead of need, and refreshing the daily_click_stats
// rollup used by the analytics timeline query. With Kafka/Spark out of
// scope (see README's Design Decisions), this — not stream consumption —
// is the worker's real job.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/KabileshRajaselvan/url-shortener/internal/analytics"
	"github.com/KabileshRajaselvan/url-shortener/internal/config"
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

	metricsSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WorkerMetricsPort),
		Handler:           promhttp.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("worker metrics server listening", "port", cfg.WorkerMetricsPort)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server failed", "error", err)
		}
	}()

	maintainer := analytics.NewMaintainer(
		s,
		time.Duration(cfg.RollupIntervalSeconds)*time.Second,
		time.Duration(cfg.PartitionCheckInterval)*time.Second,
		logger,
	)

	logger.Info("worker started")
	go maintainer.Run(ctx)

	<-ctx.Done()
	logger.Info("shutting down worker")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = metricsSrv.Shutdown(shutdownCtx)
}
