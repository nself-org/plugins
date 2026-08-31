package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal subset of *pgxpool.Pool used by this plugin's
// handlers. Extracting it as an interface lets tests substitute a mock
// (e.g. pgxmock) without a live database, while production code continues
// to pass a real *pgxpool.Pool unchanged.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool Querier

	// pool holds the concrete pgxpool.Pool when constructed via NewDB, so
	// callers can release real connections. Nil when Pool is a test double.
	pool *pgxpool.Pool
}

// NewDB creates a new database connection pool.
func NewDB(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}

	return &DB{Pool: pool, pool: pool}, nil
}

// Close releases the underlying connection pool, if this DB was constructed
// via NewDB. No-op for test doubles (pool is nil).
func (d *DB) Close() {
	if d.pool != nil {
		d.pool.Close()
	}
}

// InitSchema creates all plugin tables if they do not exist.
func (d *DB) InitSchema(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, schema)
	return err
}

const schema = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ROM Metadata
CREATE TABLE IF NOT EXISTS np_romdisc_metadata (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  rom_title VARCHAR(1000) NOT NULL,
  rom_title_normalized VARCHAR(1000) NOT NULL,
  platform VARCHAR(100) NOT NULL,
  region VARCHAR(50),
  file_name VARCHAR(500) NOT NULL,
  file_size_bytes BIGINT,
  file_hash_md5 VARCHAR(32),
  file_hash_sha256 VARCHAR(64),
  file_hash_crc32 VARCHAR(8),
  download_url TEXT,
  download_source VARCHAR(200),
  download_url_verified_at TIMESTAMPTZ,
  download_url_dead BOOLEAN DEFAULT false,
  release_year INTEGER,
  release_month INTEGER,
  release_day INTEGER,
  version VARCHAR(50),
  quality_score INTEGER DEFAULT 0,
  popularity_score INTEGER DEFAULT 0,
  release_group VARCHAR(200),
  is_verified_dump BOOLEAN DEFAULT false,
  is_hack BOOLEAN DEFAULT false,
  is_translation BOOLEAN DEFAULT false,
  is_homebrew BOOLEAN DEFAULT false,
  is_public_domain BOOLEAN DEFAULT false,
  game_title VARCHAR(1000),
  genre VARCHAR(200),
  publisher VARCHAR(500),
  developer VARCHAR(500),
  description TEXT,
  igdb_id INTEGER,
  mobygames_id INTEGER,
  box_art_url TEXT,
  screenshot_urls TEXT[] DEFAULT '{}',
  is_community_rom BOOLEAN DEFAULT false,
  community_source_url TEXT,
  community_update_year INTEGER,
  scraped_from VARCHAR(200),
  scraped_at TIMESTAMPTZ,
  search_vector tsvector,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, file_hash_sha256)
);

CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_source_app ON np_romdisc_metadata(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_platform ON np_romdisc_metadata(source_account_id, platform);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_region ON np_romdisc_metadata(source_account_id, region);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_quality ON np_romdisc_metadata(source_account_id, quality_score DESC);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_popularity ON np_romdisc_metadata(source_account_id, popularity_score DESC);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_title_norm ON np_romdisc_metadata(source_account_id, rom_title_normalized);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_hash_sha256 ON np_romdisc_metadata(file_hash_sha256);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_hash_md5 ON np_romdisc_metadata(file_hash_md5);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_hash_crc32 ON np_romdisc_metadata(file_hash_crc32);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_search ON np_romdisc_metadata USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_verified ON np_romdisc_metadata(source_account_id, is_verified_dump) WHERE is_verified_dump = true;
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_homebrew ON np_romdisc_metadata(source_account_id, is_homebrew) WHERE is_homebrew = true;
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_community ON np_romdisc_metadata(source_account_id, is_community_rom) WHERE is_community_rom = true;
CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_trgm_title ON np_romdisc_metadata USING GIN(rom_title_normalized gin_trgm_ops);

CREATE OR REPLACE FUNCTION np_romdisc_metadata_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english',
    COALESCE(NEW.rom_title, '') || ' ' ||
    COALESCE(NEW.game_title, '') || ' ' ||
    COALESCE(NEW.platform, '') || ' ' ||
    COALESCE(NEW.publisher, '') || ' ' ||
    COALESCE(NEW.developer, '') || ' ' ||
    COALESCE(NEW.genre, '') || ' ' ||
    COALESCE(NEW.description, '')
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS np_romdisc_metadata_search_update ON np_romdisc_metadata;
CREATE TRIGGER np_romdisc_metadata_search_update
  BEFORE INSERT OR UPDATE ON np_romdisc_metadata
  FOR EACH ROW EXECUTE FUNCTION np_romdisc_metadata_search_trigger();

-- Download Queue
CREATE TABLE IF NOT EXISTS np_romdisc_download_queue (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255),
  rom_metadata_id UUID NOT NULL REFERENCES np_romdisc_metadata(id) ON DELETE CASCADE,
  job_id VARCHAR(255),
  status VARCHAR(20) NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'downloading', 'verifying', 'completed', 'failed', 'cancelled')),
  download_started_at TIMESTAMPTZ,
  download_completed_at TIMESTAMPTZ,
  download_progress_percent INTEGER DEFAULT 0,
  downloaded_bytes BIGINT DEFAULT 0,
  total_bytes BIGINT DEFAULT 0,
  object_storage_path TEXT,
  checksum_verified BOOLEAN DEFAULT false,
  error_message TEXT,
  retry_count INTEGER DEFAULT 0,
  max_retries INTEGER DEFAULT 3,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_source_app ON np_romdisc_download_queue(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_status ON np_romdisc_download_queue(source_account_id, status);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_rom ON np_romdisc_download_queue(rom_metadata_id);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_created ON np_romdisc_download_queue(created_at DESC);

-- Scraper Jobs
CREATE TABLE IF NOT EXISTS np_romdisc_scraper_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scraper_name VARCHAR(100) NOT NULL UNIQUE,
  scraper_type VARCHAR(50) NOT NULL,
  scraper_source_url TEXT NOT NULL,
  enabled BOOLEAN DEFAULT true,
  last_run_at TIMESTAMPTZ,
  last_run_status VARCHAR(20),
  last_run_duration_seconds INTEGER,
  roms_found INTEGER DEFAULT 0,
  roms_added INTEGER DEFAULT 0,
  roms_updated INTEGER DEFAULT 0,
  roms_removed INTEGER DEFAULT 0,
  errors TEXT[] DEFAULT '{}',
  cron_schedule VARCHAR(100) NOT NULL DEFAULT '0 3 * * *',
  next_run_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_romdisc_scraper_jobs_name ON np_romdisc_scraper_jobs(scraper_name);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_scraper_jobs_enabled ON np_romdisc_scraper_jobs(enabled) WHERE enabled = true;

-- Popularity Tracking
CREATE TABLE IF NOT EXISTS np_romdisc_popularity (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  rom_metadata_id UUID NOT NULL REFERENCES np_romdisc_metadata(id) ON DELETE CASCADE,
  download_count INTEGER DEFAULT 0,
  search_count INTEGER DEFAULT 0,
  play_count INTEGER DEFAULT 0,
  archive_org_downloads INTEGER DEFAULT 0,
  computed_popularity_score INTEGER DEFAULT 0,
  last_score_update_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(rom_metadata_id)
);

CREATE INDEX IF NOT EXISTS idx_np_romdisc_popularity_rom ON np_romdisc_popularity(rom_metadata_id);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_popularity_score ON np_romdisc_popularity(computed_popularity_score DESC);

-- Audit Log
CREATE TABLE IF NOT EXISTS np_romdisc_audit_log (
  id SERIAL PRIMARY KEY,
  source_account_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  action TEXT NOT NULL,
  rom_metadata_id UUID REFERENCES np_romdisc_metadata(id),
  rom_name TEXT,
  rom_platform TEXT,
  rom_source TEXT,
  ip_address TEXT,
  user_agent TEXT,
  details JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_account ON np_romdisc_audit_log(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_user ON np_romdisc_audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_action ON np_romdisc_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_created ON np_romdisc_audit_log(created_at);

-- Legal Acceptance
CREATE TABLE IF NOT EXISTS np_romdisc_legal_acceptance (
  id SERIAL PRIMARY KEY,
  source_account_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  disclaimer_version TEXT NOT NULL DEFAULT '1.0',
  accepted_at TIMESTAMPTZ DEFAULT NOW(),
  ip_address TEXT,
  user_agent TEXT,
  UNIQUE(source_account_id, user_id, disclaimer_version)
);
`
