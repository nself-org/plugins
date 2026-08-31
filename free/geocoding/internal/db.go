package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// NewDB creates and connects a pgxpool using DATABASE_URL.
func NewDB(cfg *Config) (*DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close shuts down the pool.
func (d *DB) Close() {
	d.Pool.Close()
}

// InitSchema creates all required tables if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	ddl := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Geocode cache
CREATE TABLE IF NOT EXISTS geo_cache (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  query_type        VARCHAR(16)  NOT NULL,
  query_hash        VARCHAR(64)  NOT NULL,
  query_text        TEXT         NOT NULL,
  provider          VARCHAR(64)  NOT NULL,
  lat               DOUBLE PRECISION,
  lng               DOUBLE PRECISION,
  formatted_address TEXT,
  street_number     VARCHAR(32),
  street_name       VARCHAR(255),
  city              VARCHAR(255),
  state             VARCHAR(128),
  state_code        VARCHAR(8),
  country           VARCHAR(128),
  country_code      VARCHAR(4),
  postal_code       VARCHAR(32),
  place_id          VARCHAR(255),
  place_type        VARCHAR(64),
  accuracy          VARCHAR(32),
  bounds            JSONB,
  raw_response      JSONB,
  hit_count         INTEGER      NOT NULL DEFAULT 0,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  expires_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS geo_cache_lookup
  ON geo_cache (source_account_id, query_type, query_hash, provider);
CREATE INDEX IF NOT EXISTS geo_cache_expires ON geo_cache (expires_at)
  WHERE expires_at IS NOT NULL;

-- Geofences
CREATE TABLE IF NOT EXISTS geo_geofences (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name              VARCHAR(255) NOT NULL,
  description       TEXT,
  fence_type        VARCHAR(16)  NOT NULL DEFAULT 'circle',
  center_lat        DOUBLE PRECISION NOT NULL,
  center_lng        DOUBLE PRECISION NOT NULL,
  radius_meters     DOUBLE PRECISION,
  polygon           JSONB,
  active            BOOLEAN      NOT NULL DEFAULT TRUE,
  notify_on_enter   BOOLEAN      NOT NULL DEFAULT FALSE,
  notify_on_exit    BOOLEAN      NOT NULL DEFAULT FALSE,
  notify_url        TEXT,
  metadata          JSONB        NOT NULL DEFAULT '{}',
  created_by        VARCHAR(255),
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  deleted_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS geo_geofences_account
  ON geo_geofences (source_account_id) WHERE deleted_at IS NULL;

-- Geofence events
CREATE TABLE IF NOT EXISTS geo_geofence_events (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  geofence_id       UUID         NOT NULL REFERENCES geo_geofences(id),
  event_type        VARCHAR(8)   NOT NULL,
  entity_id         VARCHAR(255) NOT NULL,
  entity_type       VARCHAR(64)  NOT NULL DEFAULT 'user',
  lat               DOUBLE PRECISION NOT NULL,
  lng               DOUBLE PRECISION NOT NULL,
  notified          BOOLEAN      NOT NULL DEFAULT FALSE,
  notified_at       TIMESTAMPTZ,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS geo_geofence_events_fence
  ON geo_geofence_events (geofence_id, created_at DESC);

-- Places
CREATE TABLE IF NOT EXISTS geo_places (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider          VARCHAR(64)  NOT NULL,
  provider_place_id VARCHAR(255) NOT NULL,
  name              VARCHAR(255) NOT NULL,
  category          VARCHAR(128),
  lat               DOUBLE PRECISION NOT NULL,
  lng               DOUBLE PRECISION NOT NULL,
  formatted_address TEXT,
  phone             VARCHAR(64),
  website           TEXT,
  rating            DOUBLE PRECISION,
  review_count      INTEGER,
  hours             JSONB,
  photos            JSONB        NOT NULL DEFAULT '[]',
  metadata          JSONB        NOT NULL DEFAULT '{}',
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS geo_places_lookup
  ON geo_places (source_account_id, provider, provider_place_id);

-- API quota tracking
CREATE TABLE IF NOT EXISTS geo_api_quotas (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  quota_type        VARCHAR(16)  NOT NULL DEFAULT 'daily',
  period_start      DATE         NOT NULL DEFAULT CURRENT_DATE,
  api_calls         INTEGER      NOT NULL DEFAULT 0,
  geocode_calls     INTEGER      NOT NULL DEFAULT 0,
  cache_hits        INTEGER      NOT NULL DEFAULT 0,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS geo_api_quotas_period
  ON geo_api_quotas (source_account_id, quota_type, period_start);
`
	_, err := d.Pool.Exec(ctx, ddl)
	return err
}
