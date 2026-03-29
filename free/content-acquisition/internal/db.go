package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

// =========================================================================
// Quality Profiles
// =========================================================================

// CreateQualityProfile inserts a new quality profile.
func (d *DB) CreateQualityProfile(accountID, name string, preferredQualities []string, minSeeders int) (*QualityProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if preferredQualities == nil {
		preferredQualities = []string{"1080p", "720p"}
	}

	row := d.pool.QueryRow(ctx,
		`INSERT INTO np_contentacquisition_quality_profiles
		   (source_account_id, name, preferred_qualities, min_seeders)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, source_account_id, name, description, preferred_qualities,
		   max_size_gb, min_size_gb, preferred_sources, excluded_sources,
		   preferred_groups, excluded_groups, preferred_languages,
		   require_subtitles, min_seeders, wait_for_better_quality, wait_hours,
		   created_at, updated_at`,
		accountID, name, preferredQualities, minSeeders,
	)
	return scanQualityProfile(row)
}

func scanQualityProfile(row pgx.Row) (*QualityProfile, error) {
	var p QualityProfile
	err := row.Scan(
		&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
		&p.PreferredQualities, &p.MaxSizeGB, &p.MinSizeGB,
		&p.PreferredSources, &p.ExcludedSources,
		&p.PreferredGroups, &p.ExcludedGroups, &p.PreferredLanguages,
		&p.RequireSubtitles, &p.MinSeeders, &p.WaitForBetter, &p.WaitHours,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProfiles lists all quality profiles for an account.
func (d *DB) ListProfiles(accountID string) ([]QualityProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, name, description, preferred_qualities,
		   max_size_gb, min_size_gb, preferred_sources, excluded_sources,
		   preferred_groups, excluded_groups, preferred_languages,
		   require_subtitles, min_seeders, wait_for_better_quality, wait_hours,
		   created_at, updated_at
		 FROM np_contentacquisition_quality_profiles
		 WHERE source_account_id = $1
		 ORDER BY created_at DESC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []QualityProfile
	for rows.Next() {
		var p QualityProfile
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
			&p.PreferredQualities, &p.MaxSizeGB, &p.MinSizeGB,
			&p.PreferredSources, &p.ExcludedSources,
			&p.PreferredGroups, &p.ExcludedGroups, &p.PreferredLanguages,
			&p.RequireSubtitles, &p.MinSeeders, &p.WaitForBetter, &p.WaitHours,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// =========================================================================
// Subscriptions
// =========================================================================

func scanSubscription(row pgx.Row) (*Subscription, error) {
	var s Subscription
	err := row.Scan(
		&s.ID, &s.SourceAccountID, &s.SubscriptionType, &s.ContentID,
		&s.ContentName, &s.ContentMetadata, &s.QualityProfileID,
		&s.Enabled, &s.AutoUpgrade, &s.MonitorFutureSeasons,
		&s.MonitorExistingSeasons, &s.SeasonFolder,
		&s.LastCheckAt, &s.LastDownloadAt, &s.NextCheckAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateSubscription inserts a new subscription.
func (d *DB) CreateSubscription(accountID, subType string, contentID *string, contentName string, qualityProfileID *string) (*Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		`INSERT INTO np_contentacquisition_acquisition_subscriptions
		   (source_account_id, subscription_type, content_id, content_name,
		    content_metadata, quality_profile_id, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, true)
		 RETURNING id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at`,
		accountID, subType, contentID, contentName, json.RawMessage("{}"), qualityProfileID,
	)
	return scanSubscription(row)
}

// GetSubscription returns a single subscription by ID.
func (d *DB) GetSubscription(id string) (*Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at
		 FROM np_contentacquisition_acquisition_subscriptions WHERE id = $1`, id)
	s, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ListSubscriptions returns all subscriptions for an account.
func (d *DB) ListSubscriptions(accountID string) ([]Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at
		 FROM np_contentacquisition_acquisition_subscriptions
		 WHERE source_account_id = $1
		 ORDER BY created_at DESC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(
			&s.ID, &s.SourceAccountID, &s.SubscriptionType, &s.ContentID,
			&s.ContentName, &s.ContentMetadata, &s.QualityProfileID,
			&s.Enabled, &s.AutoUpgrade, &s.MonitorFutureSeasons,
			&s.MonitorExistingSeasons, &s.SeasonFolder,
			&s.LastCheckAt, &s.LastDownloadAt, &s.NextCheckAt,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// UpdateSubscription updates allowed fields on a subscription.
func (d *DB) UpdateSubscription(id string, req UpdateSubscriptionRequest) (*Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.ContentType != nil {
		setClauses = append(setClauses, fmt.Sprintf("subscription_type = $%d", idx))
		args = append(args, *req.ContentType)
		idx++
	}
	if req.ContentID != nil {
		setClauses = append(setClauses, fmt.Sprintf("content_id = $%d", idx))
		args = append(args, *req.ContentID)
		idx++
	}
	if req.ContentName != nil {
		setClauses = append(setClauses, fmt.Sprintf("content_name = $%d", idx))
		args = append(args, *req.ContentName)
		idx++
	}
	if req.QualityProfileID != nil {
		setClauses = append(setClauses, fmt.Sprintf("quality_profile_id = $%d", idx))
		args = append(args, *req.QualityProfileID)
		idx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}
	if req.AutoUpgrade != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_upgrade = $%d", idx))
		args = append(args, *req.AutoUpgrade)
		idx++
	}

	if len(args) == 0 {
		return d.GetSubscription(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_acquisition_subscriptions SET %s WHERE id = $%d
		 RETURNING id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at`,
		strings.Join(setClauses, ", "), idx,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	s, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// DeleteSubscription deletes a subscription. Returns true if a row was deleted.
func (d *DB) DeleteSubscription(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_acquisition_subscriptions WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// =========================================================================
// RSS Feeds
// =========================================================================

func scanRSSFeed(row pgx.Row) (*RSSFeed, error) {
	var f RSSFeed
	err := row.Scan(
		&f.ID, &f.SourceAccountID, &f.Name, &f.URL, &f.FeedType,
		&f.Enabled, &f.CheckIntervalMinutes, &f.QualityProfileID,
		&f.LastCheckAt, &f.LastSuccessAt, &f.LastError, &f.ConsecutiveFailures,
		&f.NextCheckAt, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

const rssFeedColumns = `id, source_account_id, name, url, feed_type,
  enabled, check_interval_minutes, quality_profile_id,
  last_check_at, last_success_at, last_error, consecutive_failures,
  next_check_at, created_at, updated_at`

// CreateRSSFeed inserts a new RSS feed.
func (d *DB) CreateRSSFeed(accountID, name, feedURL, feedType string) (*RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_rss_feeds
			   (source_account_id, name, url, feed_type, check_interval_minutes)
			 VALUES ($1, $2, $3, $4, 60)
			 RETURNING %s`, rssFeedColumns),
		accountID, name, feedURL, feedType,
	)
	return scanRSSFeed(row)
}

// ListRSSFeeds returns all feeds for an account.
func (d *DB) ListRSSFeeds(accountID string) ([]RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT %s FROM np_contentacquisition_rss_feeds
			 WHERE source_account_id = $1
			 ORDER BY created_at DESC`, rssFeedColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []RSSFeed
	for rows.Next() {
		var f RSSFeed
		if err := rows.Scan(
			&f.ID, &f.SourceAccountID, &f.Name, &f.URL, &f.FeedType,
			&f.Enabled, &f.CheckIntervalMinutes, &f.QualityProfileID,
			&f.LastCheckAt, &f.LastSuccessAt, &f.LastError, &f.ConsecutiveFailures,
			&f.NextCheckAt, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

// UpdateRSSFeed updates allowed fields on a feed.
func (d *DB) UpdateRSSFeed(id string, req UpdateFeedRequest) (*RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.URL != nil {
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", idx))
		args = append(args, *req.URL)
		idx++
	}
	if req.FeedType != nil {
		setClauses = append(setClauses, fmt.Sprintf("feed_type = $%d", idx))
		args = append(args, *req.FeedType)
		idx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}
	if req.CheckIntervalMinutes != nil {
		setClauses = append(setClauses, fmt.Sprintf("check_interval_minutes = $%d", idx))
		args = append(args, *req.CheckIntervalMinutes)
		idx++
	}

	if len(args) == 0 {
		return d.GetRSSFeed(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_rss_feeds SET %s WHERE id = $%d
		 RETURNING %s`,
		strings.Join(setClauses, ", "), idx, rssFeedColumns,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	f, err := scanRSSFeed(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return f, err
}

// GetRSSFeed returns a single feed by ID.
func (d *DB) GetRSSFeed(id string) (*RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_rss_feeds WHERE id = $1`, rssFeedColumns), id)
	f, err := scanRSSFeed(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return f, err
}

// DeleteRSSFeed deletes a feed by ID.
func (d *DB) DeleteRSSFeed(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_rss_feeds WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// =========================================================================
// Acquisition Queue
// =========================================================================

func scanQueueItem(row pgx.Row) (*AcquisitionQueueItem, error) {
	var q AcquisitionQueueItem
	err := row.Scan(
		&q.ID, &q.SourceAccountID, &q.ContentType, &q.ContentName,
		&q.Year, &q.Season, &q.Episode, &q.QualityProfileID,
		&q.RequestedBy, &q.RequestSourceID, &q.Status, &q.Priority,
		&q.Attempts, &q.MaxAttempts, &q.MatchedTorrent, &q.DownloadID,
		&q.ErrorMessage, &q.CreatedAt, &q.StartedAt, &q.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

const queueColumns = `id, source_account_id, content_type, content_name,
  year, season, episode, quality_profile_id,
  requested_by, request_source_id, status, priority,
  attempts, max_attempts, matched_torrent, download_id,
  error_message, created_at, started_at, completed_at`

// GetQueue returns active queue items for an account.
func (d *DB) GetQueue(accountID string) ([]AcquisitionQueueItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT %s FROM np_contentacquisition_acquisition_queue
			 WHERE source_account_id = $1
			   AND status IN ('pending', 'searching', 'matched', 'downloading')
			 ORDER BY priority DESC, created_at ASC`, queueColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AcquisitionQueueItem
	for rows.Next() {
		var q AcquisitionQueueItem
		if err := rows.Scan(
			&q.ID, &q.SourceAccountID, &q.ContentType, &q.ContentName,
			&q.Year, &q.Season, &q.Episode, &q.QualityProfileID,
			&q.RequestedBy, &q.RequestSourceID, &q.Status, &q.Priority,
			&q.Attempts, &q.MaxAttempts, &q.MatchedTorrent, &q.DownloadID,
			&q.ErrorMessage, &q.CreatedAt, &q.StartedAt, &q.CompletedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, q)
	}
	return items, rows.Err()
}

// AddToQueue inserts a new item into the acquisition queue.
func (d *DB) AddToQueue(accountID, contentType, contentName string, year, season, episode *int, requestedBy string) (*AcquisitionQueueItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_acquisition_queue
			   (source_account_id, content_type, content_name, year, season, episode,
			    requested_by, priority)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 5)
			 RETURNING %s`, queueColumns),
		accountID, contentType, contentName, year, season, episode, requestedBy,
	)
	return scanQueueItem(row)
}

// =========================================================================
// Acquisition History
// =========================================================================

// ListAcquisitionHistory returns history items within the last N days.
func (d *DB) ListAcquisitionHistory(accountID string, days int) ([]AcquisitionHistoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, content_type, content_name,
		   year, season, episode, torrent_title, torrent_source,
		   quality, size_bytes, download_id, status, acquired_from,
		   upgrade_of, created_at
		 FROM np_contentacquisition_acquisition_history
		 WHERE source_account_id = $1
		   AND created_at >= NOW() - ($2 || ' days')::INTERVAL
		 ORDER BY created_at DESC`,
		accountID, strconv.Itoa(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AcquisitionHistoryItem
	for rows.Next() {
		var h AcquisitionHistoryItem
		if err := rows.Scan(
			&h.ID, &h.SourceAccountID, &h.ContentType, &h.ContentName,
			&h.Year, &h.Season, &h.Episode, &h.TorrentTitle, &h.TorrentSource,
			&h.Quality, &h.SizeBytes, &h.DownloadID, &h.Status, &h.AcquiredFrom,
			&h.UpgradeOf, &h.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

// =========================================================================
// Pipeline Runs
// =========================================================================

const pipelineColumns = `id, source_account_id, trigger_type, trigger_source,
  content_title, content_type, status,
  vpn_check_status, torrent_status, torrent_download_id,
  metadata_status, subtitle_status, encoding_status, encoding_job_id,
  publishing_status,
  detected_at, vpn_checked_at, torrent_submitted_at,
  download_completed_at, metadata_enriched_at, subtitles_fetched_at,
  encoding_completed_at, published_at, pipeline_completed_at,
  error_message, metadata, created_at, updated_at`

func scanPipelineRun(row pgx.Row) (*PipelineRun, error) {
	var p PipelineRun
	err := row.Scan(
		&p.ID, &p.SourceAccountID, &p.TriggerType, &p.TriggerSource,
		&p.ContentTitle, &p.ContentType, &p.Status,
		&p.VPNCheckStatus, &p.TorrentStatus, &p.TorrentDownloadID,
		&p.MetadataStatus, &p.SubtitleStatus, &p.EncodingStatus, &p.EncodingJobID,
		&p.PublishingStatus,
		&p.DetectedAt, &p.VPNCheckedAt, &p.TorrentSubmittedAt,
		&p.DownloadCompletedAt, &p.MetadataEnrichedAt, &p.SubtitlesFetchedAt,
		&p.EncodingCompletedAt, &p.PublishedAt, &p.PipelineCompletedAt,
		&p.ErrorMessage, &p.Metadata, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePipelineRun inserts a new pipeline run.
func (d *DB) CreatePipelineRun(accountID, triggerType string, triggerSource *string, contentTitle string, contentType *string, metadata json.RawMessage) (*PipelineRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_pipeline_runs
			   (source_account_id, trigger_type, trigger_source,
			    content_title, content_type, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING %s`, pipelineColumns),
		accountID, triggerType, triggerSource, contentTitle, contentType, metadata,
	)
	return scanPipelineRun(row)
}

// GetPipelineRun returns a single pipeline run by ID.
func (d *DB) GetPipelineRun(id int) (*PipelineRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_pipeline_runs WHERE id = $1`, pipelineColumns), id)
	p, err := scanPipelineRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListPipelineRuns returns paginated pipeline runs with optional status filter.
func (d *DB) ListPipelineRuns(status *string, limit, offset int) ([]PipelineRun, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	// Count
	countQuery := `SELECT COUNT(*)::int FROM np_contentacquisition_pipeline_runs`
	countArgs := []interface{}{}
	if status != nil {
		countQuery += ` WHERE status = $1`
		countArgs = append(countArgs, *status)
	}

	var total int
	if err := d.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data
	dataQuery := fmt.Sprintf(`SELECT %s FROM np_contentacquisition_pipeline_runs`, pipelineColumns)
	dataArgs := []interface{}{}
	idx := 1

	if status != nil {
		dataQuery += fmt.Sprintf(` WHERE status = $%d`, idx)
		dataArgs = append(dataArgs, *status)
		idx++
	}

	dataQuery += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	dataArgs = append(dataArgs, limit, offset)

	rows, err := d.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []PipelineRun
	for rows.Next() {
		var p PipelineRun
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.TriggerType, &p.TriggerSource,
			&p.ContentTitle, &p.ContentType, &p.Status,
			&p.VPNCheckStatus, &p.TorrentStatus, &p.TorrentDownloadID,
			&p.MetadataStatus, &p.SubtitleStatus, &p.EncodingStatus, &p.EncodingJobID,
			&p.PublishingStatus,
			&p.DetectedAt, &p.VPNCheckedAt, &p.TorrentSubmittedAt,
			&p.DownloadCompletedAt, &p.MetadataEnrichedAt, &p.SubtitlesFetchedAt,
			&p.EncodingCompletedAt, &p.PublishedAt, &p.PipelineCompletedAt,
			&p.ErrorMessage, &p.Metadata, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		runs = append(runs, p)
	}
	return runs, total, rows.Err()
}

// =========================================================================
// Movie Monitoring
// =========================================================================

const movieColumns = `id, source_account_id, user_id, movie_title, tmdb_id,
  release_date, digital_release_date, quality_profile,
  auto_download, auto_upgrade, status, downloaded_quality,
  created_at, updated_at`

func scanMovie(row pgx.Row) (*MovieMonitoring, error) {
	var m MovieMonitoring
	err := row.Scan(
		&m.ID, &m.SourceAccountID, &m.UserID, &m.MovieTitle, &m.TmdbID,
		&m.ReleaseDate, &m.DigitalReleaseDate, &m.QualityProfile,
		&m.AutoDownload, &m.AutoUpgrade, &m.Status, &m.DownloadedQuality,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateMovieMonitoring adds a movie to the monitoring list.
func (d *DB) CreateMovieMonitoring(accountID, title string, tmdbID *int, qualityProfile string, autoDownload, autoUpgrade bool) (*MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_movie_monitoring
			   (source_account_id, user_id, movie_title, tmdb_id,
			    quality_profile, auto_download, auto_upgrade, status)
			 VALUES ($1, $1, $2, $3, $4, $5, $6, 'scheduled')
			 RETURNING %s`, movieColumns),
		accountID, title, tmdbID, qualityProfile, autoDownload, autoUpgrade,
	)
	return scanMovie(row)
}

// ListMovieMonitoring returns all monitored movies for an account.
func (d *DB) ListMovieMonitoring(accountID string) ([]MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_movie_monitoring
		 WHERE source_account_id = $1 ORDER BY created_at DESC`, movieColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []MovieMonitoring
	for rows.Next() {
		var m MovieMonitoring
		if err := rows.Scan(
			&m.ID, &m.SourceAccountID, &m.UserID, &m.MovieTitle, &m.TmdbID,
			&m.ReleaseDate, &m.DigitalReleaseDate, &m.QualityProfile,
			&m.AutoDownload, &m.AutoUpgrade, &m.Status, &m.DownloadedQuality,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		movies = append(movies, m)
	}
	return movies, rows.Err()
}

// UpdateMovieMonitoring updates allowed fields on a monitored movie.
func (d *DB) UpdateMovieMonitoring(id string, req UpdateMovieRequest) (*MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("movie_title = $%d", idx))
		args = append(args, *req.Title)
		idx++
	}
	if req.TmdbID != nil {
		setClauses = append(setClauses, fmt.Sprintf("tmdb_id = $%d", idx))
		args = append(args, *req.TmdbID)
		idx++
	}
	if req.QualityProfile != nil {
		setClauses = append(setClauses, fmt.Sprintf("quality_profile = $%d", idx))
		args = append(args, *req.QualityProfile)
		idx++
	}
	if req.AutoDownload != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_download = $%d", idx))
		args = append(args, *req.AutoDownload)
		idx++
	}
	if req.AutoUpgrade != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_upgrade = $%d", idx))
		args = append(args, *req.AutoUpgrade)
		idx++
	}
	if req.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, *req.Status)
		idx++
	}

	if len(args) == 0 {
		return d.GetMovieMonitoring(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_movie_monitoring SET %s WHERE id = $%d
		 RETURNING %s`,
		strings.Join(setClauses, ", "), idx, movieColumns,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	m, err := scanMovie(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// GetMovieMonitoring returns a single monitored movie by ID.
func (d *DB) GetMovieMonitoring(id string) (*MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_movie_monitoring WHERE id = $1`, movieColumns), id)
	m, err := scanMovie(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// DeleteMovieMonitoring removes a movie from monitoring.
func (d *DB) DeleteMovieMonitoring(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_movie_monitoring WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// =========================================================================
// Downloads
// =========================================================================

const downloadColumns = `id, source_account_id, user_id, content_type, title,
  state, progress, magnet_uri, torrent_id, encoding_job_id,
  quality_profile, retry_count, error_message,
  show_id, season_number, episode_number, tmdb_id,
  created_at, updated_at`

func scanDownload(row pgx.Row) (*Download, error) {
	var dl Download
	err := row.Scan(
		&dl.ID, &dl.SourceAccountID, &dl.UserID, &dl.ContentType, &dl.Title,
		&dl.State, &dl.Progress, &dl.MagnetURI, &dl.TorrentID, &dl.EncodingJobID,
		&dl.QualityProfile, &dl.RetryCount, &dl.ErrorMessage,
		&dl.ShowID, &dl.SeasonNumber, &dl.EpisodeNumber, &dl.TmdbID,
		&dl.CreatedAt, &dl.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &dl, nil
}

// CreateDownload inserts a new download record and its initial state history entry.
func (d *DB) CreateDownload(accountID, contentType, title string, magnetURI *string, qualityProfile string, showID *string, seasonNumber, episodeNumber, tmdbID *int) (*Download, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_downloads
			   (source_account_id, user_id, content_type, title, state,
			    magnet_uri, quality_profile, show_id, season_number, episode_number, tmdb_id)
			 VALUES ($1, $1, $2, $3, 'created', $4, $5, $6, $7, $8, $9)
			 RETURNING %s`, downloadColumns),
		accountID, contentType, title, magnetURI, qualityProfile,
		showID, seasonNumber, episodeNumber, tmdbID,
	)
	dl, err := scanDownload(row)
	if err != nil {
		return nil, err
	}

	// Record initial state
	_, err = tx.Exec(ctx,
		`INSERT INTO np_contentacquisition_download_state_history
		   (download_id, from_state, to_state, metadata)
		 VALUES ($1, NULL, $2, $3)`,
		dl.ID, dl.State, json.RawMessage(`{"source":"creation"}`),
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return dl, nil
}

// GetDownload returns a single download by ID.
func (d *DB) GetDownload(id string) (*Download, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_downloads WHERE id = $1`, downloadColumns), id)
	dl, err := scanDownload(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return dl, err
}

// ListDownloads returns downloads for an account, optionally filtered by state.
func (d *DB) ListDownloads(accountID string, stateFilter *string) ([]Download, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	query := fmt.Sprintf(`SELECT %s FROM np_contentacquisition_downloads WHERE source_account_id = $1`, downloadColumns)
	args := []interface{}{accountID}

	if stateFilter != nil {
		query += ` AND state = $2`
		args = append(args, *stateFilter)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var downloads []Download
	for rows.Next() {
		var dl Download
		if err := rows.Scan(
			&dl.ID, &dl.SourceAccountID, &dl.UserID, &dl.ContentType, &dl.Title,
			&dl.State, &dl.Progress, &dl.MagnetURI, &dl.TorrentID, &dl.EncodingJobID,
			&dl.QualityProfile, &dl.RetryCount, &dl.ErrorMessage,
			&dl.ShowID, &dl.SeasonNumber, &dl.EpisodeNumber, &dl.TmdbID,
			&dl.CreatedAt, &dl.UpdatedAt,
		); err != nil {
			return nil, err
		}
		downloads = append(downloads, dl)
	}
	return downloads, rows.Err()
}

// UpdateDownloadState transitions a download to a new state and records history.
func (d *DB) UpdateDownloadState(id, toState string, meta json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get current state
	var fromState *string
	err = tx.QueryRow(ctx,
		`SELECT state FROM np_contentacquisition_downloads WHERE id = $1`, id).Scan(&fromState)
	if err != nil {
		return err
	}

	// Update state
	_, err = tx.Exec(ctx,
		`UPDATE np_contentacquisition_downloads SET state = $2, updated_at = NOW() WHERE id = $1`,
		id, toState)
	if err != nil {
		return err
	}

	if meta == nil {
		meta = json.RawMessage("{}")
	}

	// Record transition
	_, err = tx.Exec(ctx,
		`INSERT INTO np_contentacquisition_download_state_history
		   (download_id, from_state, to_state, metadata)
		 VALUES ($1, $2, $3, $4)`,
		id, fromState, toState, meta)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpdateDownloadFields updates specific mutable fields on a download.
func (d *DB) UpdateDownloadFields(id string, retryCount *int, errorMessage *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if retryCount != nil {
		setClauses = append(setClauses, fmt.Sprintf("retry_count = $%d", idx))
		args = append(args, *retryCount)
		idx++
	}
	if errorMessage != nil {
		setClauses = append(setClauses, fmt.Sprintf("error_message = $%d", idx))
		args = append(args, *errorMessage)
		idx++
	}

	if len(args) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_downloads SET %s WHERE id = $%d`,
		strings.Join(setClauses, ", "), idx,
	)
	_, err := d.pool.Exec(ctx, query, args...)
	return err
}

// GetDownloadStateHistory returns all state transitions for a download.
func (d *DB) GetDownloadStateHistory(downloadID string) ([]DownloadStateTransition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, download_id, from_state, to_state, metadata, created_at
		 FROM np_contentacquisition_download_state_history
		 WHERE download_id = $1
		 ORDER BY created_at ASC`,
		downloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []DownloadStateTransition
	for rows.Next() {
		var t DownloadStateTransition
		if err := rows.Scan(&t.ID, &t.DownloadID, &t.FromState, &t.ToState, &t.Metadata, &t.CreatedAt); err != nil {
			return nil, err
		}
		transitions = append(transitions, t)
	}
	return transitions, rows.Err()
}

// AddToDownloadQueue adds or updates a download in the priority queue.
func (d *DB) AddToDownloadQueue(downloadID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_contentacquisition_download_queue (download_id, priority)
		 VALUES ($1, 10)
		 ON CONFLICT (download_id) DO UPDATE SET priority = 10`,
		downloadID)
	return err
}

// RemoveFromDownloadQueue removes a download from the queue.
func (d *DB) RemoveFromDownloadQueue(downloadID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_download_queue WHERE download_id = $1`, downloadID)
	return err
}

// =========================================================================
// Download Rules
// =========================================================================

const ruleColumns = `id, source_account_id, user_id, name, conditions, action,
  priority, enabled, created_at, updated_at`

func scanRule(row pgx.Row) (*DownloadRule, error) {
	var r DownloadRule
	err := row.Scan(
		&r.ID, &r.SourceAccountID, &r.UserID, &r.Name, &r.Conditions,
		&r.Action, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateDownloadRule inserts a new download rule.
func (d *DB) CreateDownloadRule(accountID, name string, conditions json.RawMessage, action string, priority int, enabled bool) (*DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_download_rules
			   (source_account_id, user_id, name, conditions, action, priority, enabled)
			 VALUES ($1, $1, $2, $3, $4, $5, $6)
			 RETURNING %s`, ruleColumns),
		accountID, name, conditions, action, priority, enabled,
	)
	return scanRule(row)
}

// GetDownloadRule returns a single rule by ID.
func (d *DB) GetDownloadRule(id string) (*DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_download_rules WHERE id = $1`, ruleColumns), id)
	r, err := scanRule(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// ListDownloadRules returns all rules for an account.
func (d *DB) ListDownloadRules(accountID string) ([]DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT %s FROM np_contentacquisition_download_rules
			 WHERE source_account_id = $1
			 ORDER BY priority DESC, created_at DESC`, ruleColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []DownloadRule
	for rows.Next() {
		var r DownloadRule
		if err := rows.Scan(
			&r.ID, &r.SourceAccountID, &r.UserID, &r.Name, &r.Conditions,
			&r.Action, &r.Priority, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// UpdateDownloadRule updates allowed fields on a download rule.
func (d *DB) UpdateDownloadRule(id string, req UpdateRuleRequest) (*DownloadRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.Conditions != nil {
		setClauses = append(setClauses, fmt.Sprintf("conditions = $%d", idx))
		args = append(args, *req.Conditions)
		idx++
	}
	if req.Action != nil {
		setClauses = append(setClauses, fmt.Sprintf("action = $%d", idx))
		args = append(args, *req.Action)
		idx++
	}
	if req.Priority != nil {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *req.Priority)
		idx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}

	if len(args) == 0 {
		return d.GetDownloadRule(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_download_rules SET %s WHERE id = $%d
		 RETURNING %s`,
		strings.Join(setClauses, ", "), idx, ruleColumns,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	r, err := scanRule(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// DeleteDownloadRule deletes a rule by ID.
func (d *DB) DeleteDownloadRule(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_download_rules WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// =========================================================================
// Dashboard Summary
// =========================================================================

// GetDashboardSummary returns aggregate counts for the dashboard.
func (d *DB) GetDashboardSummary(accountID string) (*DashboardSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var s DashboardSummary

	// Downloads stats
	err := d.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE state NOT IN ('completed', 'failed', 'cancelled'))::int,
		   COUNT(*) FILTER (WHERE state = 'completed' AND updated_at >= CURRENT_DATE)::int,
		   COUNT(*) FILTER (WHERE state = 'failed' AND updated_at >= CURRENT_DATE)::int
		 FROM np_contentacquisition_downloads WHERE source_account_id = $1`,
		accountID,
	).Scan(&s.ActiveDownloads, &s.CompletedToday, &s.FailedToday)
	if err != nil {
		return nil, err
	}

	// Active subscriptions
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_acquisition_subscriptions
		 WHERE source_account_id = $1 AND enabled = true`, accountID,
	).Scan(&s.ActiveSubscriptions)
	if err != nil {
		return nil, err
	}

	// Monitored movies
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_movie_monitoring
		 WHERE source_account_id = $1 AND status != 'downloaded'`, accountID,
	).Scan(&s.MonitoredMovies)
	if err != nil {
		return nil, err
	}

	// Enabled feeds
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_rss_feeds
		 WHERE source_account_id = $1 AND enabled = true`, accountID,
	).Scan(&s.EnabledFeeds)
	if err != nil {
		return nil, err
	}

	// Enabled rules
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM np_contentacquisition_download_rules
		 WHERE source_account_id = $1 AND enabled = true`, accountID,
	).Scan(&s.EnabledRules)
	if err != nil {
		return nil, err
	}

	// Queue depth
	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int
		 FROM np_contentacquisition_download_queue q
		 JOIN np_contentacquisition_downloads d ON d.id = q.download_id
		 WHERE d.source_account_id = $1`, accountID,
	).Scan(&s.QueueDepth)
	if err != nil {
		return nil, err
	}

	return &s, nil
}
