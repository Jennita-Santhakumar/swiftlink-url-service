package analytics

import (
	"context"
	"log/slog"
	"time"

	"github.com/KabileshRajaselvan/url-shortener/internal/store"
)

// Maintainer runs the worker's periodic responsibilities: with Kafka/Spark
// out of scope (see README's Design Decisions), the worker's real job
// becomes keeping click_events partitioned ahead of need and keeping
// daily_click_stats fresh for cheap analytics timeline queries.
type Maintainer struct {
	store                  *store.Store
	logger                 *slog.Logger
	rollupInterval         time.Duration
	partitionCheckInterval time.Duration
}

func NewMaintainer(s *store.Store, rollupInterval, partitionCheckInterval time.Duration, logger *slog.Logger) *Maintainer {
	return &Maintainer{store: s, logger: logger, rollupInterval: rollupInterval, partitionCheckInterval: partitionCheckInterval}
}

func (m *Maintainer) Run(ctx context.Context) {
	m.ensurePartitions(ctx)
	m.refreshRollup(ctx)

	rollupTicker := time.NewTicker(m.rollupInterval)
	defer rollupTicker.Stop()
	partitionTicker := time.NewTicker(m.partitionCheckInterval)
	defer partitionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rollupTicker.C:
			m.refreshRollup(ctx)
		case <-partitionTicker.C:
			m.ensurePartitions(ctx)
		}
	}
}

func (m *Maintainer) ensurePartitions(ctx context.Context) {
	for _, months := range []int{0, 1, 2} {
		if err := m.store.EnsurePartitionForMonth(ctx, months); err != nil {
			m.logger.Error("failed to ensure click_events partition", "months_from_now", months, "error", err)
		}
	}
}

func (m *Maintainer) refreshRollup(ctx context.Context) {
	const lookbackDays = 90
	if err := m.store.RefreshDailyRollup(ctx, lookbackDays); err != nil {
		m.logger.Error("failed to refresh daily click rollup", "error", err)
		return
	}
	m.logger.Info("daily click rollup refreshed", "lookback_days", lookbackDays)
}
