/**
 * Database client for streaming operations
 * Multi-app aware: all queries are scoped by source_account_id
 */

import { Pool, PoolClient } from 'pg';
import { config } from './config.js';
import {
  Stream, CreateStreamInput, UpdateStreamInput,
  StreamKey,
  Viewer,
  Recording,
  Clip, CreateClipInput,
  StreamAnalytics,
  Moderator, ModeratorPermissionsInput,
  ChatMessage,
  Report, CreateReportInput,
  ScheduledStream, CreateScheduleInput,
  ListStreamsQuery,
} from './types.js';

function generateStreamKey(): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let key = 'live_';
  for (let i = 0; i < 24; i++) {
    key += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return key;
}

export class DatabaseClient {
  private pool: Pool;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.pool = new Pool({
      host: config.database.host,
      port: config.database.port,
      database: config.database.database,
      user: config.database.user,
      password: config.database.password,
      ssl: config.database.ssl ? { rejectUnauthorized: false } : false,
    });
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(accountId: string): DatabaseClient {
    const scoped = Object.create(DatabaseClient.prototype) as DatabaseClient;
    scoped.pool = this.pool;
    scoped.sourceAccountId = accountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async getClient(): Promise<PoolClient> {
    return this.pool.connect();
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // =============================================================================
  // Schema Initialization
  // =============================================================================

  async initializeSchema(): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_streams (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          title TEXT NOT NULL,
          description TEXT,
          category TEXT,
          tags JSONB DEFAULT '[]',
          broadcaster_id TEXT NOT NULL,
          status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'live', 'ended', 'offline')),
          started_at TIMESTAMPTZ,
          ended_at TIMESTAMPTZ,
          quality_preset TEXT NOT NULL DEFAULT 'hd' CHECK (quality_preset IN ('low', 'medium', 'hd', 'full_hd', 'ultra_hd')),
          enable_chat BOOLEAN DEFAULT TRUE,
          enable_recording BOOLEAN DEFAULT FALSE,
          enable_dvr BOOLEAN DEFAULT TRUE,
          dvr_duration_seconds INT DEFAULT 7200,
          visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'unlisted', 'private', 'subscriber_only')),
          requires_password BOOLEAN DEFAULT FALSE,
          password_hash TEXT,
          allowed_users JSONB DEFAULT '[]',
          blocked_users JSONB DEFAULT '[]',
          allowed_countries JSONB DEFAULT '[]',
          blocked_countries JSONB DEFAULT '[]',
          rtmp_url TEXT,
          hls_url TEXT,
          webrtc_url TEXT,
          thumbnail_url TEXT,
          peak_viewers INT DEFAULT 0,
          total_views INT DEFAULT 0,
          duration_seconds INT DEFAULT 0,
          is_flagged BOOLEAN DEFAULT FALSE,
          flag_reason TEXT,
          flagged_at TIMESTAMPTZ,
          is_taken_down BOOLEAN DEFAULT FALSE,
          takedown_reason TEXT,
          taken_down_at TIMESTAMPTZ,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_streams_broadcaster ON streaming_streams(broadcaster_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_streams_status ON streaming_streams(status);
        CREATE INDEX IF NOT EXISTS idx_streaming_streams_started_at ON streaming_streams(started_at);
        CREATE INDEX IF NOT EXISTS idx_streaming_streams_category ON streaming_streams(category);
        CREATE INDEX IF NOT EXISTS idx_streaming_streams_visibility ON streaming_streams(visibility);
        CREATE INDEX IF NOT EXISTS idx_streaming_streams_source ON streaming_streams(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_keys (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          key TEXT NOT NULL UNIQUE,
          name TEXT NOT NULL,
          is_active BOOLEAN DEFAULT TRUE,
          last_used_at TIMESTAMPTZ,
          ip_whitelist JSONB DEFAULT '[]',
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_keys_stream ON streaming_keys(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_keys_key ON streaming_keys(key);
        CREATE INDEX IF NOT EXISTS idx_streaming_keys_active ON streaming_keys(is_active);
        CREATE INDEX IF NOT EXISTS idx_streaming_keys_source ON streaming_keys(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_viewers (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          user_id TEXT,
          anonymous_id TEXT,
          ip_address INET,
          user_agent TEXT,
          country TEXT,
          city TEXT,
          joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          left_at TIMESTAMPTZ,
          watch_duration_seconds INT DEFAULT 0,
          current_quality TEXT,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_viewers_stream ON streaming_viewers(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_viewers_user ON streaming_viewers(user_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_viewers_source ON streaming_viewers(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_recordings (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          title TEXT NOT NULL,
          description TEXT,
          video_url TEXT NOT NULL,
          thumbnail_url TEXT,
          duration_seconds INT NOT NULL,
          file_size_bytes BIGINT,
          recorded_at TIMESTAMPTZ NOT NULL,
          status TEXT NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'ready', 'failed')),
          transcoding_progress INT DEFAULT 0,
          visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'unlisted', 'private')),
          views INT DEFAULT 0,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_recordings_stream ON streaming_recordings(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_recordings_status ON streaming_recordings(status);
        CREATE INDEX IF NOT EXISTS idx_streaming_recordings_source ON streaming_recordings(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_clips (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID REFERENCES streaming_streams(id) ON DELETE SET NULL,
          recording_id UUID REFERENCES streaming_recordings(id) ON DELETE SET NULL,
          title TEXT NOT NULL,
          description TEXT,
          created_by TEXT NOT NULL,
          start_offset_seconds INT NOT NULL,
          duration_seconds INT NOT NULL,
          video_url TEXT NOT NULL,
          thumbnail_url TEXT,
          status TEXT NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'ready', 'failed')),
          views INT DEFAULT 0,
          shares INT DEFAULT 0,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_clips_stream ON streaming_clips(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_clips_recording ON streaming_clips(recording_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_clips_creator ON streaming_clips(created_by);
        CREATE INDEX IF NOT EXISTS idx_streaming_clips_source ON streaming_clips(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_analytics (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          bucket_start TIMESTAMPTZ NOT NULL,
          bucket_end TIMESTAMPTZ NOT NULL,
          concurrent_viewers INT NOT NULL,
          unique_viewers INT NOT NULL,
          new_viewers INT NOT NULL,
          returning_viewers INT NOT NULL,
          chat_messages INT DEFAULT 0,
          reactions INT DEFAULT 0,
          shares INT DEFAULT 0,
          avg_bitrate BIGINT,
          avg_framerate DECIMAL,
          buffering_events INT DEFAULT 0,
          viewer_countries JSONB DEFAULT '{}',
          metadata JSONB DEFAULT '{}',
          UNIQUE(stream_id, bucket_start)
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_analytics_stream ON streaming_analytics(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_analytics_bucket ON streaming_analytics(bucket_start);
        CREATE INDEX IF NOT EXISTS idx_streaming_analytics_source ON streaming_analytics(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_moderators (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          user_id TEXT NOT NULL,
          can_delete_messages BOOLEAN DEFAULT TRUE,
          can_timeout_users BOOLEAN DEFAULT TRUE,
          can_ban_users BOOLEAN DEFAULT TRUE,
          can_manage_moderators BOOLEAN DEFAULT FALSE,
          UNIQUE(stream_id, user_id)
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_moderators_stream ON streaming_moderators(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_moderators_user ON streaming_moderators(user_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_moderators_source ON streaming_moderators(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_chat_messages (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          user_id TEXT NOT NULL,
          content TEXT NOT NULL,
          is_deleted BOOLEAN DEFAULT FALSE,
          deleted_by TEXT,
          deleted_at TIMESTAMPTZ,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_chat_messages_stream ON streaming_chat_messages(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_chat_messages_user ON streaming_chat_messages(user_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_chat_messages_created ON streaming_chat_messages(created_at);
        CREATE INDEX IF NOT EXISTS idx_streaming_chat_messages_source ON streaming_chat_messages(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_reports (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
          reported_by TEXT NOT NULL,
          reason TEXT NOT NULL CHECK (reason IN ('spam', 'harassment', 'violence', 'sexual_content', 'hate_speech', 'copyright', 'other')),
          description TEXT,
          status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reviewing', 'resolved', 'dismissed')),
          reviewed_by TEXT,
          reviewed_at TIMESTAMPTZ,
          resolution TEXT
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_reports_stream ON streaming_reports(stream_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_reports_status ON streaming_reports(status);
        CREATE INDEX IF NOT EXISTS idx_streaming_reports_source ON streaming_reports(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS streaming_schedule (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          stream_id UUID REFERENCES streaming_streams(id) ON DELETE CASCADE,
          broadcaster_id TEXT NOT NULL,
          title TEXT NOT NULL,
          description TEXT,
          scheduled_start TIMESTAMPTZ NOT NULL,
          estimated_duration_minutes INT,
          notify_followers BOOLEAN DEFAULT TRUE,
          notified BOOLEAN DEFAULT FALSE,
          status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'live', 'completed', 'cancelled')),
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_streaming_schedule_broadcaster ON streaming_schedule(broadcaster_id);
        CREATE INDEX IF NOT EXISTS idx_streaming_schedule_start ON streaming_schedule(scheduled_start);
        CREATE INDEX IF NOT EXISTS idx_streaming_schedule_status ON streaming_schedule(status);
        CREATE INDEX IF NOT EXISTS idx_streaming_schedule_source ON streaming_schedule(source_account_id);
      `);
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Streams CRUD
  // =============================================================================

  async createStream(input: CreateStreamInput): Promise<Stream> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_streams (
          source_account_id, title, description, category, tags, broadcaster_id,
          quality_preset, enable_chat, enable_recording, enable_dvr, dvr_duration_seconds,
          visibility, requires_password, password_hash,
          allowed_countries, blocked_countries, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING *`,
        [
          this.sourceAccountId, input.title, input.description ?? null,
          input.category ?? null, JSON.stringify(input.tags ?? []),
          input.broadcaster_id, input.quality_preset ?? 'hd',
          input.enable_chat ?? true, input.enable_recording ?? false,
          input.enable_dvr ?? true, input.dvr_duration_seconds ?? 7200,
          input.visibility ?? 'public', input.requires_password ?? false,
          input.password ?? null,
          JSON.stringify(input.allowed_countries ?? []),
          JSON.stringify(input.blocked_countries ?? []),
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getStream(id: string): Promise<Stream | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM streaming_streams WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listStreams(query: ListStreamsQuery): Promise<{ streams: Stream[]; total: number }> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['source_account_id = $1'];
      const params: unknown[] = [this.sourceAccountId];
      let paramIdx = 2;

      if (query.status) {
        conditions.push(`status = $${paramIdx}`);
        params.push(query.status);
        paramIdx++;
      }
      if (query.category) {
        conditions.push(`category = $${paramIdx}`);
        params.push(query.category);
        paramIdx++;
      }
      if (query.broadcaster_id) {
        conditions.push(`broadcaster_id = $${paramIdx}`);
        params.push(query.broadcaster_id);
        paramIdx++;
      }
      if (query.visibility) {
        conditions.push(`visibility = $${paramIdx}`);
        params.push(query.visibility);
        paramIdx++;
      }

      const where = conditions.join(' AND ');
      const limit = parseInt(query.limit ?? '50', 10);
      const offset = parseInt(query.offset ?? '0', 10);

      const countResult = await client.query(`SELECT COUNT(*) FROM streaming_streams WHERE ${where}`, params);
      const total = parseInt(countResult.rows[0].count, 10);

      params.push(limit, offset);
      const result = await client.query(
        `SELECT * FROM streaming_streams WHERE ${where} ORDER BY created_at DESC LIMIT $${paramIdx} OFFSET $${paramIdx + 1}`,
        params
      );
      return { streams: result.rows, total };
    } finally {
      client.release();
    }
  }

  async updateStream(id: string, input: UpdateStreamInput): Promise<Stream | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        title: input.title, description: input.description, category: input.category,
        quality_preset: input.quality_preset, enable_chat: input.enable_chat,
        enable_recording: input.enable_recording, enable_dvr: input.enable_dvr,
        dvr_duration_seconds: input.dvr_duration_seconds, visibility: input.visibility,
        requires_password: input.requires_password,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.tags !== undefined) {
        fields.push(`tags = $${paramIdx}`);
        params.push(JSON.stringify(input.tags));
        paramIdx++;
      }
      if (input.password !== undefined) {
        fields.push(`password_hash = $${paramIdx}`);
        params.push(input.password);
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE streaming_streams SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteStream(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM streaming_streams WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  async startStream(id: string): Promise<Stream | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE streaming_streams SET status = 'live', started_at = NOW(), updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2 RETURNING *`,
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async stopStream(id: string): Promise<Stream | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE streaming_streams SET status = 'ended', ended_at = NOW(),
         duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INT, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2 RETURNING *`,
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Stream Keys
  // =============================================================================

  async generateStreamKey(streamId: string, name: string): Promise<StreamKey> {
    const client = await this.getClient();
    try {
      const key = generateStreamKey();
      const result = await client.query(
        `INSERT INTO streaming_keys (source_account_id, stream_id, key, name) VALUES ($1, $2, $3, $4) RETURNING *`,
        [this.sourceAccountId, streamId, key, name]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async listStreamKeys(streamId: string): Promise<StreamKey[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM streaming_keys WHERE stream_id = $1 AND source_account_id = $2 ORDER BY created_at',
        [streamId, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async deleteStreamKey(keyId: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM streaming_keys WHERE id = $1 AND source_account_id = $2',
        [keyId, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  async rotateStreamKey(keyId: string): Promise<StreamKey | null> {
    const client = await this.getClient();
    try {
      const newKey = generateStreamKey();
      const result = await client.query(
        `UPDATE streaming_keys SET key = $1 WHERE id = $2 AND source_account_id = $3 RETURNING *`,
        [newKey, keyId, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Viewers
  // =============================================================================

  async addViewer(streamId: string, userId: string | null, anonymousId: string | null, ipAddress: string | null, userAgent: string | null): Promise<Viewer> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_viewers (source_account_id, stream_id, user_id, anonymous_id, ip_address, user_agent)
         VALUES ($1, $2, $3, $4, $5::INET, $6) RETURNING *`,
        [this.sourceAccountId, streamId, userId, anonymousId, ipAddress, userAgent]
      );
      // Increment total views
      await client.query(
        'UPDATE streaming_streams SET total_views = total_views + 1 WHERE id = $1 AND source_account_id = $2',
        [streamId, this.sourceAccountId]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getActiveViewers(streamId: string): Promise<Viewer[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT * FROM streaming_viewers WHERE stream_id = $1 AND source_account_id = $2
         AND left_at IS NULL AND last_heartbeat > NOW() - INTERVAL '30 seconds'`,
        [streamId, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async getViewerCount(streamId: string): Promise<number> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT COUNT(*) FROM streaming_viewers WHERE stream_id = $1 AND source_account_id = $2
         AND left_at IS NULL AND last_heartbeat > NOW() - INTERVAL '30 seconds'`,
        [streamId, this.sourceAccountId]
      );
      return parseInt(result.rows[0].count, 10);
    } finally {
      client.release();
    }
  }

  async viewerHeartbeat(viewerId: string): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(
        `UPDATE streaming_viewers SET last_heartbeat = NOW(),
         watch_duration_seconds = EXTRACT(EPOCH FROM (NOW() - joined_at))::INT, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [viewerId, this.sourceAccountId]
      );
    } finally {
      client.release();
    }
  }

  async viewerLeave(viewerId: string): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(
        `UPDATE streaming_viewers SET left_at = NOW(),
         watch_duration_seconds = EXTRACT(EPOCH FROM (NOW() - joined_at))::INT, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [viewerId, this.sourceAccountId]
      );
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Recordings
  // =============================================================================

  async createRecording(streamId: string, title: string, videoUrl: string, durationSeconds: number): Promise<Recording> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_recordings (source_account_id, stream_id, title, video_url, duration_seconds, recorded_at)
         VALUES ($1, $2, $3, $4, $5, NOW()) RETURNING *`,
        [this.sourceAccountId, streamId, title, videoUrl, durationSeconds]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getRecording(id: string): Promise<Recording | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM streaming_recordings WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listRecordings(streamId: string): Promise<Recording[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM streaming_recordings WHERE stream_id = $1 AND source_account_id = $2 ORDER BY recorded_at DESC',
        [streamId, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async deleteRecording(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM streaming_recordings WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Clips
  // =============================================================================

  async createClip(input: CreateClipInput): Promise<Clip> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_clips (
          source_account_id, stream_id, recording_id, title, description, created_by,
          start_offset_seconds, duration_seconds, video_url, thumbnail_url
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *`,
        [
          this.sourceAccountId, input.stream_id ?? null, input.recording_id ?? null,
          input.title, input.description ?? null, input.created_by,
          input.start_offset_seconds, input.duration_seconds,
          input.video_url, input.thumbnail_url ?? null,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async deleteClip(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM streaming_clips WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Analytics
  // =============================================================================

  async getStreamAnalytics(streamId: string, startDate?: string, endDate?: string): Promise<StreamAnalytics[]> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['stream_id = $1', 'source_account_id = $2'];
      const params: unknown[] = [streamId, this.sourceAccountId];
      let paramIdx = 3;

      if (startDate) {
        conditions.push(`bucket_start >= $${paramIdx}`);
        params.push(startDate);
        paramIdx++;
      }
      if (endDate) {
        conditions.push(`bucket_end <= $${paramIdx}`);
        params.push(endDate);
        paramIdx++;
      }

      const result = await client.query(
        `SELECT * FROM streaming_analytics WHERE ${conditions.join(' AND ')} ORDER BY bucket_start`,
        params
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Chat
  // =============================================================================

  async sendChatMessage(streamId: string, userId: string, content: string): Promise<ChatMessage> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_chat_messages (source_account_id, stream_id, user_id, content) VALUES ($1, $2, $3, $4) RETURNING *`,
        [this.sourceAccountId, streamId, userId, content]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getChatMessages(streamId: string, limit = 50, before?: string): Promise<ChatMessage[]> {
    const client = await this.getClient();
    try {
      if (before) {
        const result = await client.query(
          `SELECT * FROM streaming_chat_messages WHERE stream_id = $1 AND source_account_id = $2
           AND is_deleted = FALSE AND created_at < $3 ORDER BY created_at DESC LIMIT $4`,
          [streamId, this.sourceAccountId, before, limit]
        );
        return result.rows.reverse();
      }
      const result = await client.query(
        `SELECT * FROM streaming_chat_messages WHERE stream_id = $1 AND source_account_id = $2
         AND is_deleted = FALSE ORDER BY created_at DESC LIMIT $3`,
        [streamId, this.sourceAccountId, limit]
      );
      return result.rows.reverse();
    } finally {
      client.release();
    }
  }

  async deleteChatMessage(messageId: string, deletedBy: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE streaming_chat_messages SET is_deleted = TRUE, deleted_by = $1, deleted_at = NOW()
         WHERE id = $2 AND source_account_id = $3 RETURNING id`,
        [deletedBy, messageId, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Moderators
  // =============================================================================

  async addModerator(streamId: string, userId: string, permissions: ModeratorPermissionsInput): Promise<Moderator> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_moderators (source_account_id, stream_id, user_id,
         can_delete_messages, can_timeout_users, can_ban_users, can_manage_moderators)
         VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *`,
        [
          this.sourceAccountId, streamId, userId,
          permissions.can_delete_messages ?? true, permissions.can_timeout_users ?? true,
          permissions.can_ban_users ?? true, permissions.can_manage_moderators ?? false,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async removeModerator(streamId: string, userId: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM streaming_moderators WHERE stream_id = $1 AND user_id = $2 AND source_account_id = $3',
        [streamId, userId, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Reports
  // =============================================================================

  async createReport(streamId: string, input: CreateReportInput): Promise<Report> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_reports (source_account_id, stream_id, reported_by, reason, description)
         VALUES ($1, $2, $3, $4, $5) RETURNING *`,
        [this.sourceAccountId, streamId, input.reported_by, input.reason, input.description ?? null]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getReports(streamId: string): Promise<Report[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM streaming_reports WHERE stream_id = $1 AND source_account_id = $2 ORDER BY created_at DESC',
        [streamId, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async takedownStream(streamId: string, reason: string): Promise<Stream | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `UPDATE streaming_streams SET is_taken_down = TRUE, takedown_reason = $1, taken_down_at = NOW(),
         status = 'offline', updated_at = NOW() WHERE id = $2 AND source_account_id = $3 RETURNING *`,
        [reason, streamId, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Schedule
  // =============================================================================

  async createSchedule(input: CreateScheduleInput): Promise<ScheduledStream> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO streaming_schedule (source_account_id, broadcaster_id, title, description,
         scheduled_start, estimated_duration_minutes, notify_followers, metadata)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
        [
          this.sourceAccountId, input.broadcaster_id, input.title, input.description ?? null,
          input.scheduled_start, input.estimated_duration_minutes ?? null,
          input.notify_followers ?? true, JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async listScheduledStreams(broadcasterId?: string): Promise<ScheduledStream[]> {
    const client = await this.getClient();
    try {
      if (broadcasterId) {
        const result = await client.query(
          `SELECT * FROM streaming_schedule WHERE broadcaster_id = $1 AND source_account_id = $2
           AND status = 'scheduled' ORDER BY scheduled_start`,
          [broadcasterId, this.sourceAccountId]
        );
        return result.rows;
      }
      const result = await client.query(
        `SELECT * FROM streaming_schedule WHERE source_account_id = $1 AND status = 'scheduled' ORDER BY scheduled_start`,
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Statistics
  // =============================================================================

  async getStats(): Promise<Record<string, number>> {
    const client = await this.getClient();
    try {
      const streams = await client.query('SELECT COUNT(*) FROM streaming_streams WHERE source_account_id = $1', [this.sourceAccountId]);
      const live = await client.query(`SELECT COUNT(*) FROM streaming_streams WHERE source_account_id = $1 AND status = 'live'`, [this.sourceAccountId]);
      const recordings = await client.query('SELECT COUNT(*) FROM streaming_recordings WHERE source_account_id = $1', [this.sourceAccountId]);
      const clips = await client.query('SELECT COUNT(*) FROM streaming_clips WHERE source_account_id = $1', [this.sourceAccountId]);
      return {
        total_streams: parseInt(streams.rows[0].count, 10),
        live_streams: parseInt(live.rows[0].count, 10),
        total_recordings: parseInt(recordings.rows[0].count, 10),
        total_clips: parseInt(clips.rows[0].count, 10),
      };
    } finally {
      client.release();
    }
  }
}

export const db = new DatabaseClient();
