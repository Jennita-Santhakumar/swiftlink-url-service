-- Not in the PRD, but necessary: without it, the analytics timeline query
-- would have to scan the (potentially large) partitioned click_events
-- table on every request. Refreshed periodically by the worker
-- (internal/analytics/rollup.go).
CREATE TABLE daily_click_stats (
    short_code VARCHAR(64) NOT NULL,
    day DATE NOT NULL,
    clicks INT NOT NULL DEFAULT 0,
    unique_visitors INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (short_code, day)
);

CREATE INDEX idx_daily_click_stats_day ON daily_click_stats(day);
