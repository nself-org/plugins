package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)


const queryTimeout = 5 * time.Second

// DB wraps the connection pool and provides all database operations.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB wrapper around the given pool.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// InitSchema creates all required tables and indexes.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := d.pool.Exec(ctx, schemaSQL)
	return err
}

// schemaSQL contains all CREATE TABLE / INDEX statements.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS np_contentacquisition_quality_profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  preferred_qualities VARCHAR(10)[] DEFAULT ARRAY['1080p', '720p'],
  max_size_gb DECIMAL(10,2),
  min_size_gb DECIMAL(10,2),
  preferred_sources VARCHAR(20)[] DEFAULT ARRAY['BluRay', 'WEB-DL'],
  excluded_sources VARCHAR(20)[] DEFAULT ARRAY['CAM', 'TS', 'TC'],
  preferred_groups VARCHAR(50)[],
  excluded_groups VARCHAR(50)[],
  preferred_languages VARCHAR(10)[] DEFAULT ARRAY['English'],
  require_subtitles BOOLEAN DEFAULT false,
  min_seeders INT DEFAULT 1,
  wait_for_better_quality BOOLEAN DEFAULT true,
  wait_hours INT DEFAULT 24,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_contentacquisition_acquisition_subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  subscription_type VARCHAR(50) NOT NULL,
  content_id VARCHAR(255),
  content_name VARCHAR(255) NOT NULL,
  content_metadata JSONB,
  quality_profile_id UUID REFERENCES np_contentacquisition_quality_profiles(id),
  enabled BOOLEAN DEFAULT true,
  auto_upgrade BOOLEAN DEFAULT false,
  monitor_future_seasons BOOLEAN DEFAULT true,
  monitor_existing_seasons BOOLEAN DEFAULT false,
  season_folder BOOLEAN DEFAULT true,
  last_check_at TIMESTAMPTZ,
  last_download_at TIMESTAMPTZ,
  next_check_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_account
  ON np_contentacquisition_acquisition_subscriptions(source_account_id, enabled);

CREATE TABLE IF NOT EXISTS np_contentacquisition_rss_feeds (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  name VARCHAR(255) NOT NULL,
  url TEXT NOT NULL,
  feed_type VARCHAR(50) NOT NULL,
  enabled BOOLEAN DEFAULT true,
  check_interval_minutes INT DEFAULT 60,
  quality_profile_id UUID REFERENCES np_contentacquisition_quality_profiles(id),
  last_check_at TIMESTAMPTZ,
  last_success_at TIMESTAMPTZ,
  last_error TEXT,
  consecutive_failures INT DEFAULT 0,
  next_check_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rss_feeds_next_check
  ON np_contentacquisition_rss_feeds(next_check_at) WHERE enabled = true;

CREATE TABLE IF NOT EXISTS np_contentacquisition_rss_feed_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  feed_id UUID NOT NULL REFERENCES np_contentacquisition_rss_feeds(id) ON DELETE CASCADE,
  source_account_id UUID NOT NULL,
  title VARCHAR(500) NOT NULL,
  link TEXT,
  magnet_uri TEXT,
  info_hash VARCHAR(40),
  pub_date TIMESTAMPTZ,
  parsed_title VARCHAR(255),
  parsed_year INT,
  parsed_season INT,
  parsed_episode INT,
  parsed_quality VARCHAR(20),
  parsed_source VARCHAR(50),
  parsed_group VARCHAR(100),
  size_bytes BIGINT,
  seeders INT,
  leechers INT,
  status VARCHAR(50) DEFAULT 'pending',
  matched_subscription_id UUID REFERENCES np_contentacquisition_acquisition_subscriptions(id),
  rejection_reason TEXT,
  download_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rss_items_feed
  ON np_contentacquisition_rss_feed_items(feed_id, created_at DESC);

CREATE TABLE IF NOT EXISTS np_contentacquisition_release_calendar (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  content_type VARCHAR(50) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_name VARCHAR(255) NOT NULL,
  season INT,
  episode INT,
  release_date DATE NOT NULL,
  digital_release_date DATE,
  physical_release_date DATE,
  subscription_id UUID REFERENCES np_contentacquisition_acquisition_subscriptions(id),
  quality_profile_id UUID REFERENCES np_contentacquisition_quality_profiles(id),
  monitoring_enabled BOOLEAN DEFAULT true,
  status VARCHAR(50) DEFAULT 'awaiting',
  first_search_at TIMESTAMPTZ,
  found_at TIMESTAMPTZ,
  download_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_calendar_release_date
  ON np_contentacquisition_release_calendar(release_date, monitoring_enabled);

CREATE TABLE IF NOT EXISTS np_contentacquisition_acquisition_queue (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  content_type VARCHAR(50) NOT NULL,
  content_name VARCHAR(255) NOT NULL,
  year INT,
  season INT,
  episode INT,
  quality_profile_id UUID REFERENCES np_contentacquisition_quality_profiles(id),
  requested_by VARCHAR(100),
  request_source_id UUID,
  status VARCHAR(50) DEFAULT 'pending',
  priority INT DEFAULT 5,
  attempts INT DEFAULT 0,
  max_attempts INT DEFAULT 3,
  matched_torrent JSONB,
  download_id UUID,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_queue_status
  ON np_contentacquisition_acquisition_queue(status, priority DESC, created_at);

CREATE TABLE IF NOT EXISTS np_contentacquisition_acquisition_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  content_type VARCHAR(50) NOT NULL,
  content_name VARCHAR(255) NOT NULL,
  year INT,
  season INT,
  episode INT,
  torrent_title VARCHAR(500),
  torrent_source VARCHAR(50),
  quality VARCHAR(20),
  size_bytes BIGINT,
  download_id UUID,
  status VARCHAR(50) NOT NULL,
  acquired_from VARCHAR(100),
  upgrade_of UUID REFERENCES np_contentacquisition_acquisition_history(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_history_account
  ON np_contentacquisition_acquisition_history(source_account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS np_contentacquisition_acquisition_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id UUID NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  conditions JSONB NOT NULL,
  actions JSONB NOT NULL,
  enabled BOOLEAN DEFAULT true,
  priority INT DEFAULT 5,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_contentacquisition_pipeline_runs (
  id SERIAL PRIMARY KEY,
  source_account_id TEXT NOT NULL,
  trigger_type TEXT NOT NULL,
  trigger_source TEXT,
  content_title TEXT NOT NULL,
  content_type TEXT,
  status TEXT NOT NULL DEFAULT 'detected',
  vpn_check_status TEXT DEFAULT 'pending',
  torrent_status TEXT DEFAULT 'pending',
  torrent_download_id TEXT,
  metadata_status TEXT DEFAULT 'pending',
  subtitle_status TEXT DEFAULT 'pending',
  encoding_status TEXT DEFAULT 'pending',
  detected_at TIMESTAMPTZ DEFAULT NOW(),
  vpn_checked_at TIMESTAMPTZ,
  torrent_submitted_at TIMESTAMPTZ,
  download_completed_at TIMESTAMPTZ,
  metadata_enriched_at TIMESTAMPTZ,
  subtitles_fetched_at TIMESTAMPTZ,
  encoding_completed_at TIMESTAMPTZ,
  pipeline_completed_at TIMESTAMPTZ,
  error_message TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_ca_pipeline_source ON np_contentacquisition_pipeline_runs(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_ca_pipeline_status ON np_contentacquisition_pipeline_runs(status);
CREATE INDEX IF NOT EXISTS idx_np_ca_pipeline_created ON np_contentacquisition_pipeline_runs(created_at DESC);

ALTER TABLE np_contentacquisition_pipeline_runs ADD COLUMN IF NOT EXISTS encoding_job_id TEXT;
ALTER TABLE np_contentacquisition_pipeline_runs ADD COLUMN IF NOT EXISTS publishing_status TEXT DEFAULT 'pending';
ALTER TABLE np_contentacquisition_pipeline_runs ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS np_contentacquisition_downloads (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  content_type TEXT NOT NULL,
  title TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'created',
  progress REAL DEFAULT 0,
  magnet_uri TEXT,
  torrent_id TEXT,
  encoding_job_id TEXT,
  quality_profile TEXT DEFAULT 'balanced',
  retry_count INT DEFAULT 0,
  error_message TEXT,
  show_id UUID,
  season_number INT,
  episode_number INT,
  tmdb_id INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_contentacquisition_downloads_account
  ON np_contentacquisition_downloads(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_contentacquisition_downloads_state
  ON np_contentacquisition_downloads(state);
CREATE INDEX IF NOT EXISTS idx_np_contentacquisition_downloads_user
  ON np_contentacquisition_downloads(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS np_contentacquisition_download_state_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  download_id UUID NOT NULL REFERENCES np_contentacquisition_downloads(id) ON DELETE CASCADE,
  from_state TEXT,
  to_state TEXT NOT NULL,
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_contacq_state_history_download
  ON np_contentacquisition_download_state_history(download_id, created_at ASC);

CREATE TABLE IF NOT EXISTS np_contentacquisition_movie_monitoring (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  movie_title TEXT NOT NULL,
  tmdb_id INT,
  release_date DATE,
  digital_release_date DATE,
  quality_profile TEXT DEFAULT 'balanced',
  auto_download BOOLEAN DEFAULT true,
  auto_upgrade BOOLEAN DEFAULT false,
  status TEXT DEFAULT 'scheduled',
  downloaded_quality TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_contacq_movies_account
  ON np_contentacquisition_movie_monitoring(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_contacq_movies_status
  ON np_contentacquisition_movie_monitoring(status);
CREATE INDEX IF NOT EXISTS idx_np_contacq_movies_tmdb
  ON np_contentacquisition_movie_monitoring(tmdb_id) WHERE tmdb_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS np_contentacquisition_download_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  name TEXT NOT NULL,
  conditions JSONB NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('auto-download', 'notify', 'skip')),
  priority INT DEFAULT 0,
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_contacq_rules_account
  ON np_contentacquisition_download_rules(source_account_id, enabled);

CREATE TABLE IF NOT EXISTS np_contentacquisition_download_queue (
  download_id UUID PRIMARY KEY REFERENCES np_contentacquisition_downloads(id) ON DELETE CASCADE,
  priority INT DEFAULT 10,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_contacq_queue_priority
  ON np_contentacquisition_download_queue(priority DESC, created_at ASC);
`

