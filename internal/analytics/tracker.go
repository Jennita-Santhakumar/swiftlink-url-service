// Package analytics implements the click-tracking pipeline: a buffered,
// batched writer that replaces the PRD's Kafka-based event stream (see
// README's Design Decisions for why — Kafka+Spark+a separate analytics DB
// is real operational weight this project's demo scale doesn't justify).
package analytics

import (
	"context"
	"log/slog"
	"time"

	"github.com/KabileshRajaselvan/url-shortener/internal/metrics"
	"github.com/KabileshRajaselvan/url-shortener/internal/shortener"
	"github.com/KabileshRajaselvan/url-shortener/internal/store"
)

type Tracker struct {
	store      *store.Store
	events     chan shortener.ClickEvent
	flushSize  int
	flushEvery time.Duration
	logger     *slog.Logger
	metrics    *metrics.Metrics
}

func NewTracker(s *store.Store, bufferSize, flushSize, flushMillis int, logger *slog.Logger, m *metrics.Metrics) *Tracker {
	return &Tracker{
		store:      s,
		events:     make(chan shortener.ClickEvent, bufferSize),
		flushSize:  flushSize,
		flushEvery: time.Duration(flushMillis) * time.Millisecond,
		logger:     logger,
		metrics:    m,
	}
}

// Track enqueues a click event without blocking the caller (the redirect's
// critical path). If the buffer is full, the event is dropped and counted
// — tracking must never slow down or fail a redirect, matching the PRD's
// own "don't fail the request if caching fails" philosophy applied to
// analytics too.
func (t *Tracker) Track(evt shortener.ClickEvent) {
	select {
	case t.events <- evt:
	default:
		t.metrics.DroppedEvents.Inc()
		t.logger.Warn("click event dropped: batch channel full", "short_code", evt.ShortCode)
	}
}

// Run drains the channel into Postgres in batches, flushing on whichever
// comes first: flushSize events buffered, or flushEvery elapsed. Blocks
// until ctx is cancelled, then flushes whatever remains.
func (t *Tracker) Run(ctx context.Context) {
	batch := make([]shortener.ClickEvent, 0, t.flushSize)
	ticker := time.NewTicker(t.flushEvery)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := t.store.InsertClickEventsBatch(context.Background(), batch); err != nil {
			t.logger.Error("failed to flush click event batch", "size", len(batch), "error", err)
		} else {
			t.metrics.ClickEventsBatch.Observe(float64(len(batch)))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case evt := <-t.events:
			batch = append(batch, evt)
			if len(batch) >= t.flushSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
