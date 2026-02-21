/**
 * Content Acquisition Database
 */

import { Pool, PoolClient } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type {
  QualityProfile, Subscription, RSSFeed, RSSFeedItem, ReleaseCalendarItem,
  AcquisitionQueueItem, PipelineRunRecord, MovieMonitoring, Download,
  DownloadStateTransition, DownloadRule, DownloadQueueItem, DashboardSummary,
  AcquisitionHistoryItem,
} from './types.js';

const logger = createLogger('content-acquisition:database');

export class ContentAcquisitionDatabase {
  private pool: Pool;

  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString });
  }

  async initialize(): Promise<void> {
    const client = await this.pool.connect();
    try {
      await this.createSchema(client);
      logger.info('Database schema initialized');
    } finally {
      client.release();
    }
  }

  private async createSchema(client: PoolClient): Promise<void> {
    await client.query(`
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

      CREATE TABLE IF NOT EXISTS acquisition_queue (
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
        ON acquisition_queue(status, priority DESC, created_at);

      CREATE TABLE IF NOT EXISTS acquisition_history (
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
        upgrade_of UUID REFERENCES acquisition_history(id),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_history_account
        ON acquisition_history(source_account_id, created_at DESC);

      CREATE TABLE IF NOT EXISTS acquisition_rules (
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

      -- Pipeline extensions: encoding job reference and publishing stage
      ALTER TABLE np_contentacquisition_pipeline_runs ADD COLUMN IF NOT EXISTS encoding_job_id TEXT;
      ALTER TABLE np_contentacquisition_pipeline_runs ADD COLUMN IF NOT EXISTS publishing_status TEXT DEFAULT 'pending';
      ALTER TABLE np_contentacquisition_pipeline_runs ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

      -- =====================================================================
      -- nTV Downloads (state-machine driven)
      -- =====================================================================

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

      -- =====================================================================
      -- Download State History
      -- =====================================================================

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

      -- =====================================================================
      -- Movie Monitoring
      -- =====================================================================

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

      -- =====================================================================
      -- Download Rules
      -- =====================================================================

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

      -- =====================================================================
      -- Download Queue (priority queue for pending downloads)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_contentacquisition_download_queue (
        download_id UUID PRIMARY KEY REFERENCES np_contentacquisition_downloads(id) ON DELETE CASCADE,
        priority INT DEFAULT 10,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_contacq_queue_priority
        ON np_contentacquisition_download_queue(priority DESC, created_at ASC);
    `);
  }

  async createQualityProfile(profile: Partial<QualityProfile>): Promise<QualityProfile> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_quality_profiles (source_account_id, name, description, preferred_qualities,
        preferred_sources, excluded_sources, min_seeders)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING *`,
      [
        profile.source_account_id,
        profile.name,
        profile.description,
        profile.preferred_qualities,
        profile.preferred_sources,
        profile.excluded_sources,
        profile.min_seeders,
      ]
    );
    return result.rows[0];
  }

  async createSubscription(sub: Partial<Subscription>): Promise<Subscription> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_acquisition_subscriptions (source_account_id, subscription_type, content_id,
        content_name, content_metadata, quality_profile_id, enabled)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING *`,
      [
        sub.source_account_id,
        sub.subscription_type,
        sub.content_id,
        sub.content_name,
        sub.content_metadata || {},
        sub.quality_profile_id,
        sub.enabled !== false,
      ]
    );
    return result.rows[0];
  }

  async listSubscriptions(accountId: string): Promise<Subscription[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_acquisition_subscriptions WHERE source_account_id = $1 ORDER BY created_at DESC`,
      [accountId]
    );
    return result.rows;
  }

  async createRSSFeed(feed: Partial<RSSFeed>): Promise<RSSFeed> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_rss_feeds (source_account_id, name, url, feed_type, check_interval_minutes)
       VALUES ($1, $2, $3, $4, $5)
       RETURNING *`,
      [feed.source_account_id, feed.name, feed.url, feed.feed_type, feed.check_interval_minutes || 60]
    );
    return result.rows[0];
  }

  async listRSSFeeds(accountId: string): Promise<RSSFeed[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_rss_feeds WHERE source_account_id = $1 ORDER BY created_at DESC`,
      [accountId]
    );
    return result.rows;
  }

  async getQueue(accountId: string): Promise<AcquisitionQueueItem[]> {
    const result = await this.pool.query(
      `SELECT * FROM acquisition_queue
       WHERE source_account_id = $1 AND status IN ('pending', 'searching', 'matched', 'downloading')
       ORDER BY priority DESC, created_at ASC`,
      [accountId]
    );
    return result.rows;
  }

  async addToQueue(item: Partial<AcquisitionQueueItem>): Promise<AcquisitionQueueItem> {
    const result = await this.pool.query(
      `INSERT INTO acquisition_queue (source_account_id, content_type, content_name, year, season,
        episode, quality_profile_id, requested_by, request_source_id, priority)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
       RETURNING *`,
      [
        item.source_account_id,
        item.content_type,
        item.content_name,
        item.year,
        item.season,
        item.episode,
        item.quality_profile_id,
        item.requested_by,
        item.request_source_id,
        item.priority || 5,
      ]
    );
    return result.rows[0];
  }

  /**
   * Insert or update an RSS feed item (upsert on feed_id + title to avoid duplicates).
   * Returns the inserted/updated row and a boolean indicating whether the item is new.
   */
  async insertRSSFeedItem(item: Partial<RSSFeedItem>): Promise<{ feedItem: RSSFeedItem; isNew: boolean }> {
    // Check if an item with the same feed_id and title already exists
    const existing = await this.pool.query(
      `SELECT id FROM np_contentacquisition_rss_feed_items WHERE feed_id = $1 AND title = $2 LIMIT 1`,
      [item.feed_id, item.title]
    );

    if (existing.rows.length > 0) {
      // Item already processed; return existing without modification
      const row = await this.pool.query(`SELECT * FROM np_contentacquisition_rss_feed_items WHERE id = $1`, [existing.rows[0].id]);
      return { feedItem: row.rows[0], isNew: false };
    }

    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_rss_feed_items (
        feed_id, source_account_id, title, link, magnet_uri, info_hash,
        pub_date, parsed_title, parsed_year, parsed_season, parsed_episode,
        parsed_quality, parsed_source, parsed_group, size_bytes, seeders, leechers,
        status, matched_subscription_id, rejection_reason
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
      RETURNING *`,
      [
        item.feed_id,
        item.source_account_id,
        item.title,
        item.link ?? null,
        item.magnet_uri ?? null,
        item.info_hash ?? null,
        item.pub_date ?? null,
        item.parsed_title ?? null,
        item.parsed_year ?? null,
        item.parsed_season ?? null,
        item.parsed_episode ?? null,
        item.parsed_quality ?? null,
        item.parsed_source ?? null,
        item.parsed_group ?? null,
        item.size_bytes ?? null,
        item.seeders ?? null,
        item.leechers ?? null,
        item.status ?? 'pending',
        item.matched_subscription_id ?? null,
        item.rejection_reason ?? null,
      ]
    );
    return { feedItem: result.rows[0], isNew: true };
  }

  /**
   * Find active subscriptions whose content_name matches the parsed title.
   * Uses case-insensitive substring matching against the subscription's content_name.
   * Optionally filters by subscription_type based on the feed type.
   */
  async matchSubscriptions(
    accountId: string,
    parsedTitle: string,
    feedType?: string,
  ): Promise<Subscription[]> {
    let query = `
      SELECT * FROM np_contentacquisition_acquisition_subscriptions
      WHERE source_account_id = $1
        AND enabled = true
        AND LOWER($2) LIKE '%' || LOWER(content_name) || '%'
    `;
    const params: (string | undefined)[] = [accountId, parsedTitle];

    // Map feed_type to subscription_type for more precise matching
    if (feedType) {
      const subscriptionTypeMap: Record<string, string> = {
        'tv_shows': 'tv_show',
        'movies': 'movie_collection',
        'anime': 'tv_show',
        'music': 'artist',
      };
      const mappedType = subscriptionTypeMap[feedType];
      if (mappedType) {
        query += ` AND subscription_type = $3`;
        params.push(mappedType);
      }
    }

    query += ` ORDER BY created_at DESC`;

    const result = await this.pool.query(query, params);
    return result.rows;
  }

  /**
   * Update the last_check_at timestamp for a feed and reset/increment failure counters.
   */
  async updateFeedLastChecked(feedId: string, error?: string): Promise<void> {
    if (error) {
      await this.pool.query(
        `UPDATE np_contentacquisition_rss_feeds
         SET last_check_at = NOW(),
             last_error = $2,
             consecutive_failures = consecutive_failures + 1,
             updated_at = NOW()
         WHERE id = $1`,
        [feedId, error]
      );
    } else {
      await this.pool.query(
        `UPDATE np_contentacquisition_rss_feeds
         SET last_check_at = NOW(),
             last_success_at = NOW(),
             last_error = NULL,
             consecutive_failures = 0,
             updated_at = NOW()
         WHERE id = $1`,
        [feedId]
      );
    }
  }

  /**
   * Update an RSS feed item's status and optionally set the matched subscription.
   */
  async updateRSSFeedItemStatus(
    itemId: string,
    status: RSSFeedItem['status'],
    matchedSubscriptionId?: string,
    rejectionReason?: string,
  ): Promise<void> {
    await this.pool.query(
      `UPDATE np_contentacquisition_rss_feed_items
       SET status = $2,
           matched_subscription_id = COALESCE($3, matched_subscription_id),
           rejection_reason = $4,
           processed_at = NOW()
       WHERE id = $1`,
      [itemId, status, matchedSubscriptionId ?? null, rejectionReason ?? null]
    );
  }

  /**
   * Retrieve all enabled feeds across all accounts (for scheduled background checks).
   */
  async listAllEnabledFeeds(): Promise<RSSFeed[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_rss_feeds WHERE enabled = true ORDER BY next_check_at ASC NULLS FIRST`
    );
    return result.rows;
  }

  // ---------------------------------------------------------------------------
  // Pipeline Runs
  // ---------------------------------------------------------------------------

  async createPipelineRun(data: {
    source_account_id: string;
    trigger_type: string;
    trigger_source?: string;
    content_title: string;
    content_type?: string;
    metadata?: Record<string, unknown>;
  }): Promise<PipelineRunRecord> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_pipeline_runs (source_account_id, trigger_type, trigger_source,
        content_title, content_type, metadata)
       VALUES ($1, $2, $3, $4, $5, $6)
       RETURNING *`,
      [
        data.source_account_id,
        data.trigger_type,
        data.trigger_source ?? null,
        data.content_title,
        data.content_type ?? null,
        JSON.stringify(data.metadata ?? {}),
      ]
    );
    return result.rows[0];
  }

  async updatePipelineRun(id: number, updates: Partial<PipelineRunRecord>): Promise<PipelineRunRecord | null> {
    // Build dynamic SET clause from provided fields
    const allowedFields = [
      'status', 'vpn_check_status', 'torrent_status', 'torrent_download_id',
      'metadata_status', 'subtitle_status', 'encoding_status', 'encoding_job_id',
      'publishing_status',
      'vpn_checked_at', 'torrent_submitted_at', 'download_completed_at',
      'metadata_enriched_at', 'subtitles_fetched_at', 'encoding_completed_at',
      'published_at', 'pipeline_completed_at', 'error_message', 'metadata',
    ];

    const setClauses: string[] = ['updated_at = NOW()'];
    const values: unknown[] = [];
    let paramIndex = 1;

    for (const field of allowedFields) {
      const value = (updates as Record<string, unknown>)[field];
      if (value !== undefined) {
        setClauses.push(`${field} = $${paramIndex}`);
        values.push(field === 'metadata' ? JSON.stringify(value) : value);
        paramIndex++;
      }
    }

    values.push(id);
    const result = await this.pool.query(
      `UPDATE np_contentacquisition_pipeline_runs SET ${setClauses.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values
    );
    return result.rows[0] ?? null;
  }

  async getPipelineRun(id: number): Promise<PipelineRunRecord | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_pipeline_runs WHERE id = $1`,
      [id]
    );
    return result.rows[0] ?? null;
  }

  async listPipelineRuns(filters?: {
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<{ runs: PipelineRunRecord[]; total: number }> {
    const conditions: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (filters?.status) {
      conditions.push(`status = $${paramIndex}`);
      params.push(filters.status);
      paramIndex++;
    }

    const whereClause = conditions.length > 0 ? `WHERE ${conditions.join(' AND ')}` : '';

    const countResult = await this.pool.query(
      `SELECT COUNT(*)::int AS total FROM np_contentacquisition_pipeline_runs ${whereClause}`,
      params
    );
    const total: number = countResult.rows[0].total;

    const limit = filters?.limit ?? 50;
    const offset = filters?.offset ?? 0;
    const dataParams = [...params, limit, offset];

    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_pipeline_runs ${whereClause}
       ORDER BY created_at DESC
       LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      dataParams
    );

    return { runs: result.rows, total };
  }

  // ---------------------------------------------------------------------------
  // Pool accessor (used by DownloadStateMachine)
  // ---------------------------------------------------------------------------

  getPool(): Pool {
    return this.pool;
  }

  // ---------------------------------------------------------------------------
  // Subscriptions (individual CRUD)
  // ---------------------------------------------------------------------------

  async getSubscription(id: string): Promise<Subscription | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_acquisition_subscriptions WHERE id = $1`,
      [id],
    );
    return result.rows[0] ?? null;
  }

  async updateSubscription(id: string, updates: Partial<Subscription>): Promise<Subscription | null> {
    const setClauses: string[] = ['updated_at = NOW()'];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields: (keyof Subscription)[] = [
      'content_name', 'content_id', 'subscription_type', 'quality_profile_id',
      'enabled', 'auto_upgrade', 'monitor_future_seasons', 'monitor_existing_seasons',
      'season_folder',
    ];

    for (const field of allowedFields) {
      const value = (updates as Record<string, unknown>)[field];
      if (value !== undefined) {
        setClauses.push(`${field} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (values.length === 0) return this.getSubscription(id);

    values.push(id);
    const result = await this.pool.query(
      `UPDATE np_contentacquisition_acquisition_subscriptions SET ${setClauses.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values,
    );
    return result.rows[0] ?? null;
  }

  async deleteSubscription(id: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_contentacquisition_acquisition_subscriptions WHERE id = $1`,
      [id],
    );
    return (result.rowCount ?? 0) > 0;
  }

  // ---------------------------------------------------------------------------
  // RSS Feeds (individual CRUD)
  // ---------------------------------------------------------------------------

  async getRSSFeed(id: string): Promise<RSSFeed | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_rss_feeds WHERE id = $1`,
      [id],
    );
    return result.rows[0] ?? null;
  }

  async updateRSSFeed(id: string, updates: Partial<RSSFeed>): Promise<RSSFeed | null> {
    const setClauses: string[] = ['updated_at = NOW()'];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields: (keyof RSSFeed)[] = [
      'name', 'url', 'feed_type', 'enabled', 'check_interval_minutes', 'quality_profile_id',
    ];

    for (const field of allowedFields) {
      const value = (updates as Record<string, unknown>)[field];
      if (value !== undefined) {
        setClauses.push(`${field} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (values.length === 0) return this.getRSSFeed(id);

    values.push(id);
    const result = await this.pool.query(
      `UPDATE np_contentacquisition_rss_feeds SET ${setClauses.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values,
    );
    return result.rows[0] ?? null;
  }

  async deleteRSSFeed(id: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_contentacquisition_rss_feeds WHERE id = $1`,
      [id],
    );
    return (result.rowCount ?? 0) > 0;
  }

  // ---------------------------------------------------------------------------
  // Movie Monitoring
  // ---------------------------------------------------------------------------

  async createMovieMonitoring(movie: Partial<MovieMonitoring>): Promise<MovieMonitoring> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_movie_monitoring
        (source_account_id, user_id, movie_title, tmdb_id, release_date,
         digital_release_date, quality_profile, auto_download, auto_upgrade, status)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
       RETURNING *`,
      [
        movie.source_account_id,
        movie.user_id,
        movie.movie_title,
        movie.tmdb_id ?? null,
        movie.release_date ?? null,
        movie.digital_release_date ?? null,
        movie.quality_profile ?? 'balanced',
        movie.auto_download !== false,
        movie.auto_upgrade === true,
        movie.status ?? 'scheduled',
      ],
    );
    return result.rows[0];
  }

  async getMovieMonitoring(id: string): Promise<MovieMonitoring | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_movie_monitoring WHERE id = $1`,
      [id],
    );
    return result.rows[0] ?? null;
  }

  async listMovieMonitoring(accountId: string): Promise<MovieMonitoring[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_movie_monitoring
       WHERE source_account_id = $1
       ORDER BY created_at DESC`,
      [accountId],
    );
    return result.rows;
  }

  async updateMovieMonitoring(id: string, updates: Partial<MovieMonitoring>): Promise<MovieMonitoring | null> {
    const setClauses: string[] = ['updated_at = NOW()'];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields: (keyof MovieMonitoring)[] = [
      'movie_title', 'tmdb_id', 'release_date', 'digital_release_date',
      'quality_profile', 'auto_download', 'auto_upgrade', 'status', 'downloaded_quality',
    ];

    for (const field of allowedFields) {
      const value = (updates as Record<string, unknown>)[field];
      if (value !== undefined) {
        setClauses.push(`${field} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (values.length === 0) return this.getMovieMonitoring(id);

    values.push(id);
    const result = await this.pool.query(
      `UPDATE np_contentacquisition_movie_monitoring SET ${setClauses.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values,
    );
    return result.rows[0] ?? null;
  }

  async deleteMovieMonitoring(id: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_contentacquisition_movie_monitoring WHERE id = $1`,
      [id],
    );
    return (result.rowCount ?? 0) > 0;
  }

  // ---------------------------------------------------------------------------
  // Downloads (state-machine driven)
  // ---------------------------------------------------------------------------

  async createDownload(download: Partial<Download>): Promise<Download> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_downloads
        (source_account_id, user_id, content_type, title, state, magnet_uri,
         quality_profile, show_id, season_number, episode_number, tmdb_id)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
       RETURNING *`,
      [
        download.source_account_id,
        download.user_id,
        download.content_type ?? 'movie',
        download.title,
        download.state ?? 'created',
        download.magnet_uri ?? null,
        download.quality_profile ?? 'balanced',
        download.show_id ?? null,
        download.season_number ?? null,
        download.episode_number ?? null,
        download.tmdb_id ?? null,
      ],
    );

    // Record the initial state in history
    const dl = result.rows[0] as Download;
    await this.pool.query(
      `INSERT INTO np_contentacquisition_download_state_history
         (download_id, from_state, to_state, metadata)
       VALUES ($1, NULL, $2, $3)`,
      [dl.id, dl.state, JSON.stringify({ source: 'creation' })],
    );

    return dl;
  }

  async getDownload(id: string): Promise<Download | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_downloads WHERE id = $1`,
      [id],
    );
    return result.rows[0] ?? null;
  }

  async listDownloads(accountId: string, statusFilter?: string): Promise<Download[]> {
    let query = `SELECT * FROM np_contentacquisition_downloads WHERE source_account_id = $1`;
    const params: unknown[] = [accountId];

    if (statusFilter) {
      query += ` AND state = $2`;
      params.push(statusFilter);
    }

    query += ` ORDER BY created_at DESC`;
    const result = await this.pool.query(query, params);
    return result.rows;
  }

  async updateDownloadProgress(id: string, progress: number): Promise<void> {
    await this.pool.query(
      `UPDATE np_contentacquisition_downloads SET progress = $2, updated_at = NOW() WHERE id = $1`,
      [id, progress],
    );
  }

  async updateDownloadFields(id: string, fields: Partial<Download>): Promise<Download | null> {
    const setClauses: string[] = ['updated_at = NOW()'];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields: (keyof Download)[] = [
      'torrent_id', 'encoding_job_id', 'error_message', 'retry_count', 'progress',
      'quality_profile', 'magnet_uri',
    ];

    for (const field of allowedFields) {
      const value = (fields as Record<string, unknown>)[field];
      if (value !== undefined) {
        setClauses.push(`${field} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (values.length === 0) return this.getDownload(id);

    values.push(id);
    const result = await this.pool.query(
      `UPDATE np_contentacquisition_downloads SET ${setClauses.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values,
    );
    return result.rows[0] ?? null;
  }

  async deleteDownload(id: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_contentacquisition_downloads WHERE id = $1`,
      [id],
    );
    return (result.rowCount ?? 0) > 0;
  }

  async getDownloadStateHistory(downloadId: string): Promise<DownloadStateTransition[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_download_state_history
       WHERE download_id = $1
       ORDER BY created_at ASC`,
      [downloadId],
    );
    return result.rows;
  }

  // ---------------------------------------------------------------------------
  // Download Queue
  // ---------------------------------------------------------------------------

  async addToDownloadQueue(downloadId: string, priority?: number): Promise<DownloadQueueItem> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_download_queue (download_id, priority)
       VALUES ($1, $2)
       ON CONFLICT (download_id) DO UPDATE SET priority = $2
       RETURNING *`,
      [downloadId, priority ?? 10],
    );
    return result.rows[0];
  }

  async removeFromDownloadQueue(downloadId: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_contentacquisition_download_queue WHERE download_id = $1`,
      [downloadId],
    );
    return (result.rowCount ?? 0) > 0;
  }

  async getDownloadQueueDepth(accountId: string): Promise<number> {
    const result = await this.pool.query(
      `SELECT COUNT(*)::int AS depth
       FROM np_contentacquisition_download_queue q
       JOIN np_contentacquisition_downloads d ON d.id = q.download_id
       WHERE d.source_account_id = $1`,
      [accountId],
    );
    return result.rows[0]?.depth ?? 0;
  }

  // ---------------------------------------------------------------------------
  // Download Rules
  // ---------------------------------------------------------------------------

  async createDownloadRule(rule: Partial<DownloadRule>): Promise<DownloadRule> {
    const result = await this.pool.query(
      `INSERT INTO np_contentacquisition_download_rules
        (source_account_id, user_id, name, conditions, action, priority, enabled)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING *`,
      [
        rule.source_account_id,
        rule.user_id,
        rule.name,
        JSON.stringify(rule.conditions ?? {}),
        rule.action ?? 'auto-download',
        rule.priority ?? 0,
        rule.enabled !== false,
      ],
    );
    return result.rows[0];
  }

  async getDownloadRule(id: string): Promise<DownloadRule | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_download_rules WHERE id = $1`,
      [id],
    );
    return result.rows[0] ?? null;
  }

  async listDownloadRules(accountId: string): Promise<DownloadRule[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contentacquisition_download_rules
       WHERE source_account_id = $1
       ORDER BY priority DESC, created_at DESC`,
      [accountId],
    );
    return result.rows;
  }

  async updateDownloadRule(id: string, updates: Partial<DownloadRule>): Promise<DownloadRule | null> {
    const setClauses: string[] = ['updated_at = NOW()'];
    const values: unknown[] = [];
    let paramIndex = 1;

    if (updates.name !== undefined) {
      setClauses.push(`name = $${paramIndex}`);
      values.push(updates.name);
      paramIndex++;
    }
    if (updates.conditions !== undefined) {
      setClauses.push(`conditions = $${paramIndex}`);
      values.push(JSON.stringify(updates.conditions));
      paramIndex++;
    }
    if (updates.action !== undefined) {
      setClauses.push(`action = $${paramIndex}`);
      values.push(updates.action);
      paramIndex++;
    }
    if (updates.priority !== undefined) {
      setClauses.push(`priority = $${paramIndex}`);
      values.push(updates.priority);
      paramIndex++;
    }
    if (updates.enabled !== undefined) {
      setClauses.push(`enabled = $${paramIndex}`);
      values.push(updates.enabled);
      paramIndex++;
    }

    if (values.length === 0) return this.getDownloadRule(id);

    values.push(id);
    const result = await this.pool.query(
      `UPDATE np_contentacquisition_download_rules SET ${setClauses.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values,
    );
    return result.rows[0] ?? null;
  }

  async deleteDownloadRule(id: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_contentacquisition_download_rules WHERE id = $1`,
      [id],
    );
    return (result.rowCount ?? 0) > 0;
  }

  // ---------------------------------------------------------------------------
  // Acquisition History (last N days)
  // ---------------------------------------------------------------------------

  async listAcquisitionHistory(accountId: string, days: number = 90): Promise<AcquisitionHistoryItem[]> {
    const result = await this.pool.query(
      `SELECT * FROM acquisition_history
       WHERE source_account_id = $1
         AND created_at >= NOW() - ($2 || ' days')::INTERVAL
       ORDER BY created_at DESC`,
      [accountId, days],
    );
    return result.rows;
  }

  // ---------------------------------------------------------------------------
  // Dashboard Summary
  // ---------------------------------------------------------------------------

  async getDashboardSummary(accountId: string): Promise<DashboardSummary> {
    const [downloads, subs, movies, feeds, rules, queue] = await Promise.all([
      this.pool.query(
        `SELECT
           COUNT(*) FILTER (WHERE state NOT IN ('completed', 'failed', 'cancelled'))::int AS active,
           COUNT(*) FILTER (WHERE state = 'completed' AND updated_at >= CURRENT_DATE)::int AS completed_today,
           COUNT(*) FILTER (WHERE state = 'failed' AND updated_at >= CURRENT_DATE)::int AS failed_today
         FROM np_contentacquisition_downloads WHERE source_account_id = $1`,
        [accountId],
      ),
      this.pool.query(
        `SELECT COUNT(*)::int AS cnt FROM np_contentacquisition_acquisition_subscriptions WHERE source_account_id = $1 AND enabled = true`,
        [accountId],
      ),
      this.pool.query(
        `SELECT COUNT(*)::int AS cnt FROM np_contentacquisition_movie_monitoring WHERE source_account_id = $1 AND status != 'downloaded'`,
        [accountId],
      ),
      this.pool.query(
        `SELECT COUNT(*)::int AS cnt FROM np_contentacquisition_rss_feeds WHERE source_account_id = $1 AND enabled = true`,
        [accountId],
      ),
      this.pool.query(
        `SELECT COUNT(*)::int AS cnt FROM np_contentacquisition_download_rules WHERE source_account_id = $1 AND enabled = true`,
        [accountId],
      ),
      this.getDownloadQueueDepth(accountId),
    ]);

    const dlRow = downloads.rows[0];
    return {
      active_downloads: dlRow?.active ?? 0,
      completed_today: dlRow?.completed_today ?? 0,
      failed_today: dlRow?.failed_today ?? 0,
      active_subscriptions: subs.rows[0]?.cnt ?? 0,
      monitored_movies: movies.rows[0]?.cnt ?? 0,
      enabled_feeds: feeds.rows[0]?.cnt ?? 0,
      enabled_rules: rules.rows[0]?.cnt ?? 0,
      queue_depth: queue,
    };
  }

  async close(): Promise<void> {
    await this.pool.end();
  }
}
