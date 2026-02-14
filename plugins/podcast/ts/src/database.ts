/**
 * Podcast Database Operations
 * Schema initialization, CRUD operations, and statistics for podcast feeds and episodes
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type { FeedRecord, EpisodeRecord, PodcastStats, FeedStatus } from './types.js';

const logger = createLogger('podcast:db');

export class PodcastDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(sourceAccountId: string): PodcastDatabase {
    return new PodcastDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query(text: string, params?: unknown[]): Promise<{ rows: Record<string, unknown>[] }> {
    return this.db.query(text, params);
  }

  // =========================================================================
  // Schema Initialization
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing podcast database schema...');

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS np_pod_feeds (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
        url TEXT NOT NULL,
        title TEXT,
        description TEXT,
        author TEXT,
        image_url TEXT,
        language VARCHAR(10),
        categories TEXT[],
        last_fetched_at TIMESTAMPTZ,
        last_episode_at TIMESTAMPTZ,
        fetch_interval_minutes INTEGER DEFAULT 60,
        error_count INTEGER DEFAULT 0,
        last_error TEXT,
        status VARCHAR(20) NOT NULL DEFAULT 'active',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, url)
      )
    `);

    await this.db.query(`
      CREATE TABLE IF NOT EXISTS np_pod_episodes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
        feed_id UUID NOT NULL REFERENCES np_pod_feeds(id) ON DELETE CASCADE,
        guid TEXT NOT NULL,
        title TEXT NOT NULL,
        description TEXT,
        pub_date TIMESTAMPTZ,
        duration_seconds INTEGER,
        enclosure_url TEXT,
        enclosure_type VARCHAR(100),
        enclosure_length BIGINT,
        season_number INTEGER,
        episode_number INTEGER,
        episode_type VARCHAR(20) DEFAULT 'full',
        chapters_url TEXT,
        transcript_url TEXT,
        image_url TEXT,
        played BOOLEAN DEFAULT FALSE,
        play_position_seconds INTEGER DEFAULT 0,
        downloaded BOOLEAN DEFAULT FALSE,
        download_path TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(feed_id, guid)
      )
    `);

    await this.db.query(`
      CREATE INDEX IF NOT EXISTS idx_np_pod_feeds_status
        ON np_pod_feeds(source_account_id, status)
    `);

    await this.db.query(`
      CREATE INDEX IF NOT EXISTS idx_np_pod_episodes_feed
        ON np_pod_episodes(feed_id, pub_date DESC)
    `);

    await this.db.query(`
      CREATE INDEX IF NOT EXISTS idx_np_pod_episodes_unplayed
        ON np_pod_episodes(source_account_id, played) WHERE played = FALSE
    `);

    await this.db.query(`
      CREATE INDEX IF NOT EXISTS idx_np_pod_feeds_next_refresh
        ON np_pod_feeds(status, last_fetched_at, fetch_interval_minutes)
        WHERE status = 'active'
    `);

    logger.info('Podcast database schema initialized');
  }

  // =========================================================================
  // Feed CRUD
  // =========================================================================

  async insertFeed(url: string, title?: string): Promise<FeedRecord> {
    const result = await this.db.query<FeedRecord>(
      `INSERT INTO np_pod_feeds (source_account_id, url, title)
       VALUES ($1, $2, $3)
       ON CONFLICT (source_account_id, url) DO UPDATE SET
         title = COALESCE(EXCLUDED.title, np_pod_feeds.title),
         updated_at = NOW(),
         synced_at = NOW()
       RETURNING *`,
      [this.sourceAccountId, url, title ?? null]
    );
    return result.rows[0] as FeedRecord;
  }

  async updateFeedMetadata(
    feedId: string,
    metadata: {
      title?: string | null;
      description?: string | null;
      author?: string | null;
      imageUrl?: string | null;
      language?: string | null;
      categories?: string[];
      lastEpisodeAt?: Date | null;
    }
  ): Promise<void> {
    await this.db.execute(
      `UPDATE np_pod_feeds SET
        title = COALESCE($2, title),
        description = COALESCE($3, description),
        author = COALESCE($4, author),
        image_url = COALESCE($5, image_url),
        language = COALESCE($6, language),
        categories = COALESCE($7, categories),
        last_episode_at = COALESCE($8, last_episode_at),
        updated_at = NOW(),
        synced_at = NOW()
       WHERE id = $1 AND source_account_id = $9`,
      [
        feedId,
        metadata.title ?? null,
        metadata.description ?? null,
        metadata.author ?? null,
        metadata.imageUrl ?? null,
        metadata.language ?? null,
        metadata.categories ?? null,
        metadata.lastEpisodeAt ?? null,
        this.sourceAccountId,
      ]
    );
  }

  async markFeedFetched(feedId: string, intervalMinutes?: number): Promise<void> {
    const params: unknown[] = [feedId, this.sourceAccountId];
    let intervalClause = '';
    if (intervalMinutes !== undefined) {
      intervalClause = ', fetch_interval_minutes = $3';
      params.push(intervalMinutes);
    }
    await this.db.execute(
      `UPDATE np_pod_feeds SET
        last_fetched_at = NOW(),
        error_count = 0,
        last_error = NULL,
        status = 'active',
        updated_at = NOW()
        ${intervalClause}
       WHERE id = $1 AND source_account_id = $2`,
      params
    );
  }

  async markFeedError(feedId: string, errorMessage: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_pod_feeds SET
        error_count = error_count + 1,
        last_error = $2,
        status = CASE WHEN error_count + 1 >= 7 THEN 'error' ELSE status END,
        updated_at = NOW()
       WHERE id = $1 AND source_account_id = $3`,
      [feedId, errorMessage, this.sourceAccountId]
    );
  }

  async getFeed(feedId: string): Promise<FeedRecord | null> {
    return this.db.queryOne<FeedRecord>(
      `SELECT * FROM np_pod_feeds WHERE id = $1 AND source_account_id = $2`,
      [feedId, this.sourceAccountId]
    );
  }

  async getFeedByUrl(url: string): Promise<FeedRecord | null> {
    return this.db.queryOne<FeedRecord>(
      `SELECT * FROM np_pod_feeds WHERE url = $1 AND source_account_id = $2`,
      [url, this.sourceAccountId]
    );
  }

  async listFeeds(status?: FeedStatus): Promise<FeedRecord[]> {
    if (status) {
      const result = await this.db.query<FeedRecord>(
        `SELECT * FROM np_pod_feeds
         WHERE source_account_id = $1 AND status = $2
         ORDER BY title ASC NULLS LAST`,
        [this.sourceAccountId, status]
      );
      return result.rows as FeedRecord[];
    }
    const result = await this.db.query<FeedRecord>(
      `SELECT * FROM np_pod_feeds
       WHERE source_account_id = $1
       ORDER BY title ASC NULLS LAST`,
      [this.sourceAccountId]
    );
    return result.rows as FeedRecord[];
  }

  async deleteFeed(feedId: string): Promise<boolean> {
    const count = await this.db.execute(
      `DELETE FROM np_pod_feeds WHERE id = $1 AND source_account_id = $2`,
      [feedId, this.sourceAccountId]
    );
    return count > 0;
  }

  async countFeeds(): Promise<number> {
    return this.db.countScoped('np_pod_feeds', this.sourceAccountId);
  }

  async getFeedsNeedingRefresh(): Promise<FeedRecord[]> {
    const result = await this.db.query<FeedRecord>(
      `SELECT * FROM np_pod_feeds
       WHERE source_account_id = $1
         AND status = 'active'
         AND (
           last_fetched_at IS NULL
           OR last_fetched_at < NOW() - (fetch_interval_minutes || ' minutes')::INTERVAL
         )
       ORDER BY last_fetched_at ASC NULLS FIRST
       LIMIT 50`,
      [this.sourceAccountId]
    );
    return result.rows as FeedRecord[];
  }

  // =========================================================================
  // Episode CRUD
  // =========================================================================

  async upsertEpisode(feedId: string, episode: {
    guid: string;
    title: string;
    description?: string | null;
    pubDate?: Date | null;
    durationSeconds?: number | null;
    enclosureUrl?: string | null;
    enclosureType?: string | null;
    enclosureLength?: number | null;
    seasonNumber?: number | null;
    episodeNumber?: number | null;
    episodeType?: string;
    chaptersUrl?: string | null;
    transcriptUrl?: string | null;
    imageUrl?: string | null;
  }): Promise<EpisodeRecord> {
    const result = await this.db.query<EpisodeRecord>(
      `INSERT INTO np_pod_episodes (
        source_account_id, feed_id, guid, title, description,
        pub_date, duration_seconds, enclosure_url, enclosure_type, enclosure_length,
        season_number, episode_number, episode_type, chapters_url, transcript_url,
        image_url
      ) VALUES (
        $1, $2, $3, $4, $5,
        $6, $7, $8, $9, $10,
        $11, $12, $13, $14, $15,
        $16
      )
      ON CONFLICT (feed_id, guid) DO UPDATE SET
        title = EXCLUDED.title,
        description = EXCLUDED.description,
        pub_date = EXCLUDED.pub_date,
        duration_seconds = EXCLUDED.duration_seconds,
        enclosure_url = EXCLUDED.enclosure_url,
        enclosure_type = EXCLUDED.enclosure_type,
        enclosure_length = EXCLUDED.enclosure_length,
        season_number = EXCLUDED.season_number,
        episode_number = EXCLUDED.episode_number,
        episode_type = EXCLUDED.episode_type,
        chapters_url = EXCLUDED.chapters_url,
        transcript_url = EXCLUDED.transcript_url,
        image_url = EXCLUDED.image_url,
        updated_at = NOW(),
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        feedId,
        episode.guid,
        episode.title,
        episode.description ?? null,
        episode.pubDate ?? null,
        episode.durationSeconds ?? null,
        episode.enclosureUrl ?? null,
        episode.enclosureType ?? null,
        episode.enclosureLength ?? null,
        episode.seasonNumber ?? null,
        episode.episodeNumber ?? null,
        episode.episodeType ?? 'full',
        episode.chaptersUrl ?? null,
        episode.transcriptUrl ?? null,
        episode.imageUrl ?? null,
      ]
    );
    return result.rows[0] as EpisodeRecord;
  }

  async upsertEpisodes(feedId: string, episodes: Array<{
    guid: string;
    title: string;
    description?: string | null;
    pubDate?: Date | null;
    durationSeconds?: number | null;
    enclosureUrl?: string | null;
    enclosureType?: string | null;
    enclosureLength?: number | null;
    seasonNumber?: number | null;
    episodeNumber?: number | null;
    episodeType?: string;
    chaptersUrl?: string | null;
    transcriptUrl?: string | null;
    imageUrl?: string | null;
  }>): Promise<number> {
    let count = 0;
    for (const episode of episodes) {
      await this.upsertEpisode(feedId, episode);
      count++;
    }
    return count;
  }

  async getEpisode(episodeId: string): Promise<EpisodeRecord | null> {
    return this.db.queryOne<EpisodeRecord>(
      `SELECT * FROM np_pod_episodes WHERE id = $1 AND source_account_id = $2`,
      [episodeId, this.sourceAccountId]
    );
  }

  async listEpisodes(feedId: string, limit = 50, offset = 0): Promise<EpisodeRecord[]> {
    const result = await this.db.query<EpisodeRecord>(
      `SELECT * FROM np_pod_episodes
       WHERE feed_id = $1 AND source_account_id = $2
       ORDER BY pub_date DESC NULLS LAST
       LIMIT $3 OFFSET $4`,
      [feedId, this.sourceAccountId, limit, offset]
    );
    return result.rows as EpisodeRecord[];
  }

  async countEpisodes(feedId?: string): Promise<number> {
    if (feedId) {
      return this.db.countScoped(
        'np_pod_episodes',
        this.sourceAccountId,
        'feed_id = $1',
        [feedId]
      );
    }
    return this.db.countScoped('np_pod_episodes', this.sourceAccountId);
  }

  async getNewEpisodes(limit = 50): Promise<EpisodeRecord[]> {
    const result = await this.db.query<EpisodeRecord>(
      `SELECT e.* FROM np_pod_episodes e
       JOIN np_pod_feeds f ON e.feed_id = f.id
       WHERE e.source_account_id = $1
         AND e.played = FALSE
         AND f.status = 'active'
       ORDER BY e.pub_date DESC NULLS LAST
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows as EpisodeRecord[];
  }

  async markEpisodePlayed(episodeId: string, positionSeconds?: number): Promise<void> {
    await this.db.execute(
      `UPDATE np_pod_episodes SET
        played = TRUE,
        play_position_seconds = COALESCE($3, play_position_seconds),
        updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [episodeId, this.sourceAccountId, positionSeconds ?? null]
    );
  }

  async updatePlayPosition(episodeId: string, positionSeconds: number): Promise<void> {
    await this.db.execute(
      `UPDATE np_pod_episodes SET
        play_position_seconds = $3,
        updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [episodeId, this.sourceAccountId, positionSeconds]
    );
  }

  async markEpisodeDownloaded(episodeId: string, downloadPath: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_pod_episodes SET
        downloaded = TRUE,
        download_path = $3,
        updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [episodeId, this.sourceAccountId, downloadPath]
    );
  }

  async getExistingGuids(feedId: string): Promise<Set<string>> {
    const result = await this.db.query<{ guid: string }>(
      `SELECT guid FROM np_pod_episodes WHERE feed_id = $1`,
      [feedId]
    );
    return new Set(result.rows.map((r: { guid: string }) => r.guid));
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<PodcastStats> {
    const feedCount = await this.countFeeds();
    const episodeCount = await this.countEpisodes();

    const durationResult = await this.db.queryOne<{ total_hours: string }>(
      `SELECT COALESCE(SUM(duration_seconds) / 3600.0, 0) AS total_hours
       FROM np_pod_episodes
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const unplayedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_pod_episodes
       WHERE source_account_id = $1 AND played = FALSE`,
      [this.sourceAccountId]
    );

    const downloadedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_pod_episodes
       WHERE source_account_id = $1 AND downloaded = TRUE`,
      [this.sourceAccountId]
    );

    return {
      feed_count: feedCount,
      episode_count: episodeCount,
      total_duration_hours: parseFloat(
        (durationResult as { total_hours: string } | null)?.total_hours ?? '0'
      ),
      unplayed_count: parseInt(
        (unplayedResult as { count: string } | null)?.count ?? '0',
        10
      ),
      downloaded_count: parseInt(
        (downloadedResult as { count: string } | null)?.count ?? '0',
        10
      ),
    };
  }
}
