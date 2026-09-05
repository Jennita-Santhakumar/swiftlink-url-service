# High-Performance URL Shortener with Redis Caching and Analytics Dashboard

A production-shaped URL shortener in Go: a Redis-cached redirect path targeting sub-50ms
latency, Snowflake-based distributed short-code generation, a Redis-bitmap Bloom filter for
collision pre-checks, a buffered click-tracking pipeline with per-day rollups, offline GeoIP,
and a React analytics dashboard, all backed by Postgres with time-partitioned click events.

Built from a PRD sketching a Bitly-style service with Kafka + Spark + a separate analytics DB
behind it. Several parts of the PRD's own reference design don't hold up as written — a
RedisBloom-only Bloom filter against a plain `redis:7-alpine` compose file, a Base62 decoder
that silently corrupts input instead of erroring, a Snowflake generator with no clock-skew
handling — fixed here and covered in [Design Decisions & Trade-offs](#design-decisions--trade-offs)
below, along with the deliberate scope cuts (no Kafka/Spark, no JWT/subscription tiers,
country-level GeoIP only).

## Architecture

```
                     ┌────────────────────┐
  POST /shorten  ──▶ │      API (Go)       │──── writes ───▶ ┌──────────────┐
  GET  /{code}       │      chi :8080      │◀──── reads ─────│  PostgreSQL  │
  GET  /urls          │                    │                  │  urls        │
  GET  /.../analytics │                    │                  │  click_events│
                       └──────┬──────┬──────┘                  │ (partitioned)│
                              │      │                          │ daily_click_ │
                       cache  │      │ buffered                 │   stats      │
                       lookup │      │ click events             └──────▲───────┘
                              ▼      ▼                                 │ rollup /
                     ┌────────────────┐                                │ partition
                     │  Redis          │                       ┌───────┴────────┐
                     │  url: cache     │                       │  Worker (Go)    │
                     │  bloom: bitmap  │                       │  batched flush  │
                     │  ratelimit:*    │                       │  :9114 /metrics │
                     └────────────────┘                        └────────┬────────┘
                                                                          │ scrape
                     ┌─────────────┐        scrape          ┌────────────▼────────┐
                     │   Grafana   │◀────────────────────────│     Prometheus       │
                     │   :3006     │                          │       :9098          │
                     └─────────────┘                          └──────────────────────┘

                     ┌──────────────────────┐
                     │  React Dashboard       │──── HTTP ───▶ API
                     │  :5223                 │
                     └──────────────────────┘
```

**Write path** (`POST /api/v1/shorten`): validate URL → check Bloom filter for a custom-slug
collision → generate a Snowflake ID + Base62 code if no custom slug → insert into Postgres →
cache in Redis → add to Bloom filter → return the short URL + QR code.

**Redirect path** (`GET /{short_code}`, the perf-critical path, unauthenticated): Redis cache
lookup first; on a miss, fall back to Postgres and re-populate the cache. A click event is
pushed onto a buffered channel (non-blocking) and flushed to Postgres in batches by a
background goroutine, so the redirect itself never waits on a database write.

**Analytics path**: the worker rolls up `click_events` into a `daily_click_stats` table every
`ROLLUP_INTERVAL_SECONDS` so the dashboard's timeline is a cheap read; breakdowns
(country/device/referrer/browser) are computed with a live `GROUP BY` over raw events, which is
fine at this project's data volumes. The worker also keeps `click_events`' monthly partitions
created ahead of need.

## API

| Method | Path | Auth | Notes |
|---|---|---|---|
| `POST` | `/api/v1/register` | none | Creates a user + returns a plaintext API key (shown once, only the SHA-256 hash is stored) |
| `POST` | `/api/v1/shorten` | API key | Rate-limited (`RATE_LIMIT_PER_MIN`); returns the short URL + base64 QR code |
| `GET` | `/{short_code}` | none | 301 redirect, the hot path |
| `GET` | `/api/v1/urls` | API key | Paginated list of the caller's URLs |
| `PATCH` | `/api/v1/urls/{short_code}` | API key | Update title/tracking flags/expiry |
| `GET` | `/api/v1/urls/{short_code}/analytics` | API key | Timeline + country/device/referrer/browser breakdowns |
| `GET` | `/health` | none | Liveness probe |
| `GET` | `/metrics` | none | Prometheus exposition |

## Running it

```bash
cp .env.example .env      # defaults already match docker-compose.yml
docker compose up --build
```

This starts Postgres, Redis, a one-shot migration runner, the API (`:8050`), the analytics
worker (`:9114`), the React dashboard (`:5223`), Prometheus (`:9098`), and Grafana (`:3006`,
`admin`/`admin`, dashboards auto-provisioned).

Try the full flow against the running stack:

```bash
curl -s -X POST http://localhost:8050/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","key_name":"cli"}'
# -> {"api_key": "usk_...", ...}

curl -s -X POST http://localhost:8050/api/v1/shorten \
  -H "Authorization: Bearer usk_..." -H "Content-Type: application/json" \
  -d '{"long_url":"https://example.com","title":"Example"}'
# -> {"short_code": "...", "short_url": "http://localhost:8050/...", "qr_code": "data:image/png;base64,..."}

curl -sI http://localhost:8050/<short_code>          # 301 redirect
curl -s http://localhost:8050/api/v1/urls/<short_code>/analytics -H "Authorization: Bearer usk_..."
```

### Tests

```bash
make test              # unit tests, no external services
docker compose up -d postgres redis
make test-integration  # exercises the real API + Postgres + Redis end-to-end
```

## Design Decisions & Trade-offs

**Bloom filter reimplemented on plain Redis, not RedisBloom.** The PRD's own sketch calls
`BF.ADD`/`BF.EXISTS`, which require the RedisBloom module — but the PRD's own
`docker-compose.yml` runs stock `redis:7-alpine`, which doesn't bundle it; those calls would
fail at runtime with "unknown command." This project instead builds a real Bloom filter on a
Redis bitmap with `SETBIT`/`GETBIT` and Kirsch–Mitzenmacher double hashing, which works against
any stock Redis.

**No Kafka/Spark analytics pipeline.** The PRD's architecture diagram routes click events
through Kafka into a Spark analytics pipeline into a separate ClickHouse/Timescale database.
That's real operational weight — three more stateful services — that this project's scale
doesn't justify. Click events are instead pushed onto an in-process buffered channel and
flushed to Postgres in configurable batches (`BATCH_FLUSH_SIZE`/`BATCH_FLUSH_MILLIS`), with a
worker doing periodic rollups into `daily_click_stats`. The redirect path never blocks on the
write.

**Country-level GeoIP only, fully offline.** Precise city/lat-long geolocation needs a paid or
account-gated database (MaxMind GeoLite2 requires a free account + license key just to
download). This project embeds DB-IP's IP-to-Country Lite dataset (via
[sapics/ip-location-db](https://github.com/sapics/ip-location-db), CC BY 4.0) directly in the
binary, so it works with zero external accounts or network calls. The schema's
`city`/`latitude`/`longitude` columns are a documented gap, not a silent one.

**API-key auth, not JWT + subscription tiers.** The PRD sketches a fuller auth/billing surface
(login, subscription tiers changing rate limits). This project keeps a single flat
`RATE_LIMIT_PER_MIN` per API key and skips JWT login entirely — API keys (`usk_...`, SHA-256
hashed at rest, shown once) are enough to demonstrate the pattern without building out a second
project's worth of billing logic.

**Base62 decode now validates input.** The PRD's own decoder calls
`strings.IndexByte(base62Chars, ch)` and never checks for `-1`; an invalid character silently
corrupts the decoded value instead of failing. Fixed to return an error on any non-Base62
character.

**Snowflake ID generator hardened against clock skew.** The PRD's sketch has no handling for
the system clock moving backwards (NTP adjustment) — a naive implementation can then emit a
duplicate or out-of-order ID. This implementation clamps to the last-seen timestamp and spins
until the clock catches up if the per-millisecond sequence is exhausted.

## Stack

Go 1.25 (chi router), PostgreSQL 16 (time-partitioned `click_events`, `golang-migrate`
migrations), Redis 7 (cache, Bloom-filter bitmap, rate limiting), React + TypeScript + Vite
(dashboard), Prometheus + Grafana (metrics), Docker Compose.

## Performance targets (from the PRD)

| Metric | Target |
|---|---|
| Redirect latency (P99) | <50ms |
| Create URL latency (P99) | <200ms |
| Cache hit rate | >99% |
