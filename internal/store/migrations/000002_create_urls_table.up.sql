-- short_code widened from the PRD's VARCHAR(12) to VARCHAR(64): Base62 of a
-- 63-bit Snowflake ID needs at most 11 characters, but a reasonable custom
-- slug ("spring-sale-2026-campaign") can exceed 12. Validated at the API
-- layer with ^[A-Za-z0-9_-]{1,64}$.
CREATE TABLE urls (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(64) NOT NULL UNIQUE,
    long_url TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255),
    description TEXT,

    track_clicks BOOLEAN NOT NULL DEFAULT true,
    track_referrer BOOLEAN NOT NULL DEFAULT true,
    track_device BOOLEAN NOT NULL DEFAULT true,
    track_geo BOOLEAN NOT NULL DEFAULT true,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,

    tags JSONB NOT NULL DEFAULT '[]',

    CONSTRAINT valid_url CHECK (long_url ~ '^https?://')
);

CREATE INDEX idx_urls_short_code ON urls(short_code);
CREATE INDEX idx_urls_user_id ON urls(user_id);
CREATE INDEX idx_urls_created_at ON urls(created_at DESC);
CREATE INDEX idx_urls_active ON urls(is_active) WHERE is_active = true;
CREATE INDEX idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_urls_set_updated_at
    BEFORE UPDATE ON urls
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
