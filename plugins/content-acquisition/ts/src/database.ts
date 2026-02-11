/**
 * Content Acquisition Database
 */

import { Pool, PoolClient } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type { QualityProfile, Subscription, RSSFeed, RSSFeedItem, ReleaseCalendarItem, AcquisitionQueueItem } from './types.js';

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
      CREATE TABLE IF NOT EXISTS quality_profiles (
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

      CREATE TABLE IF NOT EXISTS acquisition_subscriptions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id UUID NOT NULL,
        subscription_type VARCHAR(50) NOT NULL,
        content_id VARCHAR(255),
        content_name VARCHAR(255) NOT NULL,
        content_metadata JSONB,
        quality_profile_id UUID REFERENCES quality_profiles(id),
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
        ON acquisition_subscriptions(source_account_id, enabled);

      CREATE TABLE IF NOT EXISTS rss_feeds (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id UUID NOT NULL,
        name VARCHAR(255) NOT NULL,
        url TEXT NOT NULL,
        feed_type VARCHAR(50) NOT NULL,
        enabled BOOLEAN DEFAULT true,
        check_interval_minutes INT DEFAULT 60,
        quality_profile_id UUID REFERENCES quality_profiles(id),
        last_check_at TIMESTAMPTZ,
        last_success_at TIMESTAMPTZ,
        last_error TEXT,
        consecutive_failures INT DEFAULT 0,
        next_check_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_rss_feeds_next_check
        ON rss_feeds(next_check_at) WHERE enabled = true;

      CREATE TABLE IF NOT EXISTS rss_feed_items (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        feed_id UUID NOT NULL REFERENCES rss_feeds(id) ON DELETE CASCADE,
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
        matched_subscription_id UUID REFERENCES acquisition_subscriptions(id),
        rejection_reason TEXT,
        download_id UUID,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        processed_at TIMESTAMPTZ
      );

      CREATE INDEX IF NOT EXISTS idx_rss_items_feed
        ON rss_feed_items(feed_id, created_at DESC);

      CREATE TABLE IF NOT EXISTS release_calendar (
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
        subscription_id UUID REFERENCES acquisition_subscriptions(id),
        quality_profile_id UUID REFERENCES quality_profiles(id),
        monitoring_enabled BOOLEAN DEFAULT true,
        status VARCHAR(50) DEFAULT 'awaiting',
        first_search_at TIMESTAMPTZ,
        found_at TIMESTAMPTZ,
        download_id UUID,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_release_date
        ON release_calendar(release_date, monitoring_enabled);

      CREATE TABLE IF NOT EXISTS acquisition_queue (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id UUID NOT NULL,
        content_type VARCHAR(50) NOT NULL,
        content_name VARCHAR(255) NOT NULL,
        year INT,
        season INT,
        episode INT,
        quality_profile_id UUID REFERENCES quality_profiles(id),
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
    `);
  }

  async createQualityProfile(profile: Partial<QualityProfile>): Promise<QualityProfile> {
    const result = await this.pool.query(
      `INSERT INTO quality_profiles (source_account_id, name, description, preferred_qualities,
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
      `INSERT INTO acquisition_subscriptions (source_account_id, subscription_type, content_id,
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
      `SELECT * FROM acquisition_subscriptions WHERE source_account_id = $1 ORDER BY created_at DESC`,
      [accountId]
    );
    return result.rows;
  }

  async createRSSFeed(feed: Partial<RSSFeed>): Promise<RSSFeed> {
    const result = await this.pool.query(
      `INSERT INTO rss_feeds (source_account_id, name, url, feed_type, check_interval_minutes)
       VALUES ($1, $2, $3, $4, $5)
       RETURNING *`,
      [feed.source_account_id, feed.name, feed.url, feed.feed_type, feed.check_interval_minutes || 60]
    );
    return result.rows[0];
  }

  async listRSSFeeds(accountId: string): Promise<RSSFeed[]> {
    const result = await this.pool.query(
      `SELECT * FROM rss_feeds WHERE source_account_id = $1 ORDER BY created_at DESC`,
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

  async close(): Promise<void> {
    await this.pool.end();
  }
}
