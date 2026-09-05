-- The PRD's own schema places `PARTITION BY RANGE (DATE_TRUNC('month', clicked_at))`
-- *after* the closing paren of CREATE TABLE, which is not valid PostgreSQL
-- syntax for declarative partitioning (PARTITION BY must be part of the
-- CREATE TABLE statement itself, and child partitions must be created
-- explicitly — Postgres does not auto-create them). Fixed below: proper
-- declarative partitioning by RANGE on clicked_at directly (not wrapped in
-- DATE_TRUNC, which partitioned tables require to be an exact column or
-- immutable expression matching partition bounds cleanly), a DEFAULT
-- partition as a safety net, and explicit monthly partitions covering the
-- current month plus the next two — the worker (internal/analytics/rollup.go)
-- keeps creating future partitions ahead of time so writes never fail.
CREATE TABLE click_events (
    id BIGSERIAL,
    short_code VARCHAR(64) NOT NULL,

    ip_address INET,
    country VARCHAR(2),
    -- city/latitude/longitude are part of the PRD's schema but are left
    -- unpopulated in this build: precise city-level geolocation requires a
    -- paid or account-gated database (e.g. MaxMind GeoLite2 needs a free
    -- account + license key to download). Country-level lookup uses a
    -- bundled, fully offline, no-signup-required dataset instead. See
    -- README's Design Decisions.
    city VARCHAR(100),
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),

    user_agent TEXT,
    device_type VARCHAR(50),
    browser VARCHAR(50),
    os VARCHAR(50),

    referrer TEXT,
    referrer_domain VARCHAR(255),

    clicked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    response_time_ms INT,

    PRIMARY KEY (id, clicked_at)
) PARTITION BY RANGE (clicked_at);

CREATE INDEX idx_click_events_short_code ON click_events(short_code);
CREATE INDEX idx_click_events_clicked_at ON click_events(clicked_at DESC);
CREATE INDEX idx_click_events_country ON click_events(country);
CREATE INDEX idx_click_events_device_type ON click_events(device_type);

-- Safety-net partition: catches any row outside the explicit monthly
-- ranges below (e.g. if the worker's ahead-of-time partition creation ever
-- falls behind), so inserts never fail outright.
CREATE TABLE click_events_default PARTITION OF click_events DEFAULT;

-- Explicit monthly partitions for the current month and the next two,
-- computed relative to the migration run date rather than hardcoded.
DO $$
DECLARE
    month_start date := date_trunc('month', CURRENT_DATE)::date;
    i int;
BEGIN
    FOR i IN 0..2 LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS click_events_%s PARTITION OF click_events FOR VALUES FROM (%L) TO (%L)',
            to_char(month_start + (i || ' months')::interval, 'YYYY_MM'),
            month_start + (i || ' months')::interval,
            month_start + ((i + 1) || ' months')::interval
        );
    END LOOP;
END $$;
