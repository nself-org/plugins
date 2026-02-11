/**
 * EPG Database Operations
 * Complete CRUD operations for channels, programs, schedules, and channel groups
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ChannelRecord,
  ProgramRecord,
  ScheduleRecord,
  ChannelGroupRecord,
  ChannelGroupMemberRecord,
  ScheduleEntry,
  ChannelSchedule,
  WhatsOnNowEntry,
  EpgStats,
} from './types.js';

const logger = createLogger('epg:db');

export class EpgDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): EpgDatabase {
    return new EpgDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');

    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing EPG schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Channels
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS epg_channels (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        channel_number VARCHAR(10),
        call_sign VARCHAR(20),
        name VARCHAR(255) NOT NULL,
        display_name VARCHAR(255),
        logo_url TEXT,
        category VARCHAR(50),
        language VARCHAR(10) DEFAULT 'en',
        country VARCHAR(10) DEFAULT 'US',
        stream_url TEXT,
        stream_type VARCHAR(20),
        is_hd BOOLEAN DEFAULT false,
        is_4k BOOLEAN DEFAULT false,
        is_active BOOLEAN DEFAULT true,
        sort_order INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, call_sign)
      );

      CREATE INDEX IF NOT EXISTS idx_epg_channels_source_app
        ON epg_channels(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_epg_channels_number
        ON epg_channels(source_account_id, channel_number);
      CREATE INDEX IF NOT EXISTS idx_epg_channels_category
        ON epg_channels(source_account_id, category);

      -- =====================================================================
      -- Programs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS epg_programs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        external_id VARCHAR(255),
        title VARCHAR(500) NOT NULL,
        episode_title VARCHAR(500),
        description TEXT,
        long_description TEXT,
        categories TEXT[] DEFAULT '{}',
        genre VARCHAR(100),
        season_number INTEGER,
        episode_number INTEGER,
        original_air_date DATE,
        year INTEGER,
        duration_minutes INTEGER,
        content_rating VARCHAR(20),
        star_rating DOUBLE PRECISION,
        poster_url TEXT,
        thumbnail_url TEXT,
        directors TEXT[] DEFAULT '{}',
        actors TEXT[] DEFAULT '{}',
        is_new BOOLEAN DEFAULT false,
        is_live BOOLEAN DEFAULT false,
        is_premiere BOOLEAN DEFAULT false,
        is_finale BOOLEAN DEFAULT false,
        is_movie BOOLEAN DEFAULT false,
        language VARCHAR(10) DEFAULT 'en',
        subtitles TEXT[] DEFAULT '{}',
        audio_format VARCHAR(20),
        video_format VARCHAR(20),
        production_code VARCHAR(50),
        search_vector tsvector,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_epg_programs_source_app
        ON epg_programs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_epg_programs_external
        ON epg_programs(source_account_id, external_id);
      CREATE INDEX IF NOT EXISTS idx_epg_programs_title
        ON epg_programs(source_account_id, title);
      CREATE INDEX IF NOT EXISTS idx_epg_programs_search
        ON epg_programs USING GIN(search_vector);

      -- =====================================================================
      -- Schedules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS epg_schedules (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        channel_id UUID NOT NULL REFERENCES epg_channels(id) ON DELETE CASCADE,
        program_id UUID NOT NULL REFERENCES epg_programs(id) ON DELETE CASCADE,
        start_time TIMESTAMPTZ NOT NULL,
        end_time TIMESTAMPTZ NOT NULL,
        is_rerun BOOLEAN DEFAULT false,
        is_live BOOLEAN DEFAULT false,
        metadata JSONB DEFAULT '{}',
        UNIQUE(source_account_id, channel_id, start_time)
      );

      CREATE INDEX IF NOT EXISTS idx_epg_schedules_source_app
        ON epg_schedules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_epg_schedules_channel
        ON epg_schedules(channel_id, start_time);
      CREATE INDEX IF NOT EXISTS idx_epg_schedules_time
        ON epg_schedules(source_account_id, start_time, end_time);
      CREATE INDEX IF NOT EXISTS idx_epg_schedules_program
        ON epg_schedules(program_id);

      -- =====================================================================
      -- Channel Groups
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS epg_channel_groups (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        sort_order INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_epg_channel_groups_source_app
        ON epg_channel_groups(source_account_id);

      -- =====================================================================
      -- Channel Group Members
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS epg_channel_group_members (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        group_id UUID NOT NULL REFERENCES epg_channel_groups(id) ON DELETE CASCADE,
        channel_id UUID NOT NULL REFERENCES epg_channels(id) ON DELETE CASCADE,
        sort_order INTEGER DEFAULT 0,
        UNIQUE(source_account_id, group_id, channel_id)
      );

      CREATE INDEX IF NOT EXISTS idx_epg_group_members_source_app
        ON epg_channel_group_members(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_epg_group_members_group
        ON epg_channel_group_members(group_id);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS epg_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_epg_webhook_events_source_app
        ON epg_webhook_events(source_account_id);
    `;

    await this.execute(schema);
    logger.info('EPG schema initialized successfully');
  }

  // =========================================================================
  // Channel Operations
  // =========================================================================

  async createChannel(channel: Omit<ChannelRecord, 'id' | 'created_at' | 'updated_at'>): Promise<ChannelRecord> {
    const result = await this.query<ChannelRecord>(
      `INSERT INTO epg_channels (
        source_account_id, channel_number, call_sign, name, display_name,
        logo_url, category, language, country, stream_url, stream_type,
        is_hd, is_4k, is_active, sort_order, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
      RETURNING *`,
      [
        this.sourceAccountId, channel.channel_number, channel.call_sign,
        channel.name, channel.display_name, channel.logo_url, channel.category,
        channel.language, channel.country, channel.stream_url, channel.stream_type,
        channel.is_hd, channel.is_4k, channel.is_active, channel.sort_order,
        JSON.stringify(channel.metadata),
      ]
    );

    return result.rows[0];
  }

  async getChannel(id: string): Promise<ChannelRecord | null> {
    const result = await this.query<ChannelRecord>(
      `SELECT * FROM epg_channels WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listChannels(filters: {
    category?: string; isActive?: boolean; groupId?: string;
    limit?: number; offset?: number;
  }): Promise<ChannelRecord[]> {
    const conditions: string[] = ['c.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;
    let joinClause = '';

    if (filters.category) {
      conditions.push(`c.category = $${paramIndex}`);
      values.push(filters.category);
      paramIndex++;
    }

    if (filters.isActive !== undefined) {
      conditions.push(`c.is_active = $${paramIndex}`);
      values.push(filters.isActive);
      paramIndex++;
    }

    if (filters.groupId) {
      joinClause = `JOIN epg_channel_group_members m ON c.id = m.channel_id`;
      conditions.push(`m.group_id = $${paramIndex}`);
      values.push(filters.groupId);
      paramIndex++;
    }

    let sql = `
      SELECT c.* FROM epg_channels c
      ${joinClause}
      WHERE ${conditions.join(' AND ')}
      ORDER BY c.sort_order ASC, c.channel_number ASC, c.name ASC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<ChannelRecord>(sql, values);
    return result.rows;
  }

  async updateChannel(id: string, updates: Partial<ChannelRecord>): Promise<ChannelRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'channel_number', 'call_sign', 'name', 'display_name', 'logo_url',
      'category', 'stream_url', 'stream_type', 'is_hd', 'is_4k',
      'is_active', 'sort_order',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (updates.metadata !== undefined) {
      fields.push(`metadata = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.metadata));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getChannel(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<ChannelRecord>(
      `UPDATE epg_channels
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteChannel(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM epg_channels WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Program Operations
  // =========================================================================

  async createProgram(program: Omit<ProgramRecord, 'id' | 'created_at' | 'updated_at'>): Promise<ProgramRecord> {
    const result = await this.query<ProgramRecord>(
      `INSERT INTO epg_programs (
        source_account_id, external_id, title, episode_title, description,
        long_description, categories, genre, season_number, episode_number,
        original_air_date, year, duration_minutes, content_rating, star_rating,
        poster_url, thumbnail_url, directors, actors,
        is_new, is_live, is_premiere, is_finale, is_movie,
        language, subtitles, audio_format, video_format, production_code,
        metadata,
        search_vector
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
        to_tsvector('english', $3 || ' ' || COALESCE($4, '') || ' ' || COALESCE($5, ''))
      )
      RETURNING *`,
      [
        this.sourceAccountId, program.external_id, program.title,
        program.episode_title, program.description, program.long_description,
        program.categories, program.genre, program.season_number,
        program.episode_number, program.original_air_date, program.year,
        program.duration_minutes, program.content_rating, program.star_rating,
        program.poster_url, program.thumbnail_url, program.directors, program.actors,
        program.is_new, program.is_live, program.is_premiere, program.is_finale,
        program.is_movie, program.language, program.subtitles, program.audio_format,
        program.video_format, program.production_code, JSON.stringify(program.metadata),
      ]
    );

    return result.rows[0];
  }

  async getProgram(id: string): Promise<ProgramRecord | null> {
    const result = await this.query<ProgramRecord>(
      `SELECT * FROM epg_programs WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async searchPrograms(filters: {
    query: string; genre?: string; contentRating?: string;
    isMovie?: boolean; language?: string; limit?: number;
  }): Promise<ProgramRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.query) {
      conditions.push(`search_vector @@ plainto_tsquery('english', $${paramIndex})`);
      values.push(filters.query);
      paramIndex++;
    }

    if (filters.genre) {
      conditions.push(`genre = $${paramIndex}`);
      values.push(filters.genre);
      paramIndex++;
    }

    if (filters.contentRating) {
      conditions.push(`content_rating = $${paramIndex}`);
      values.push(filters.contentRating);
      paramIndex++;
    }

    if (filters.isMovie !== undefined) {
      conditions.push(`is_movie = $${paramIndex}`);
      values.push(filters.isMovie);
      paramIndex++;
    }

    if (filters.language) {
      conditions.push(`language = $${paramIndex}`);
      values.push(filters.language);
      paramIndex++;
    }

    const limit = filters.limit ?? 50;

    const result = await this.query<ProgramRecord>(
      `SELECT * FROM epg_programs
       WHERE ${conditions.join(' AND ')}
       ORDER BY title ASC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  // =========================================================================
  // Schedule Operations
  // =========================================================================

  async createSchedule(schedule: Omit<ScheduleRecord, 'id'>): Promise<ScheduleRecord> {
    const result = await this.query<ScheduleRecord>(
      `INSERT INTO epg_schedules (
        source_account_id, channel_id, program_id, start_time, end_time,
        is_rerun, is_live, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      ON CONFLICT (source_account_id, channel_id, start_time) DO UPDATE SET
        program_id = EXCLUDED.program_id,
        end_time = EXCLUDED.end_time,
        is_rerun = EXCLUDED.is_rerun,
        is_live = EXCLUDED.is_live,
        metadata = EXCLUDED.metadata
      RETURNING *`,
      [
        this.sourceAccountId, schedule.channel_id, schedule.program_id,
        schedule.start_time, schedule.end_time, schedule.is_rerun,
        schedule.is_live, JSON.stringify(schedule.metadata),
      ]
    );

    return result.rows[0];
  }

  async getScheduleGrid(filters: {
    channelIds?: string[];
    startTime: Date;
    endTime: Date;
  }): Promise<ChannelSchedule[]> {
    const conditions: string[] = [
      's.source_account_id = $1',
      's.start_time < $2',
      's.end_time > $3',
    ];
    const values: unknown[] = [this.sourceAccountId, filters.endTime, filters.startTime];
    let paramIndex = 4;

    if (filters.channelIds && filters.channelIds.length > 0) {
      const placeholders = filters.channelIds.map((_, i) => `$${paramIndex + i}`).join(', ');
      conditions.push(`s.channel_id IN (${placeholders})`);
      values.push(...filters.channelIds);
      paramIndex += filters.channelIds.length;
    }

    const result = await this.query<ScheduleEntry & {
      channel_id: string;
      channel_name: string;
      channel_number: string | null;
      logo_url: string | null;
    }>(
      `SELECT
        s.id as schedule_id,
        s.channel_id,
        c.name as channel_name,
        c.channel_number,
        c.logo_url,
        p.id as program_id,
        p.title,
        p.episode_title,
        p.description,
        p.categories,
        p.content_rating,
        p.duration_minutes,
        s.start_time,
        s.end_time,
        s.is_live,
        p.is_new,
        s.is_rerun
       FROM epg_schedules s
       JOIN epg_channels c ON s.channel_id = c.id
       JOIN epg_programs p ON s.program_id = p.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY c.sort_order ASC, c.channel_number ASC, s.start_time ASC`,
      values
    );

    // Group by channel
    const channelMap = new Map<string, ChannelSchedule>();

    for (const row of result.rows) {
      if (!channelMap.has(row.channel_id)) {
        channelMap.set(row.channel_id, {
          channel_id: row.channel_id,
          channel_name: row.channel_name,
          channel_number: row.channel_number,
          logo_url: row.logo_url,
          programs: [],
        });
      }

      channelMap.get(row.channel_id)!.programs.push({
        schedule_id: row.schedule_id,
        program_id: row.program_id,
        title: row.title,
        episode_title: row.episode_title,
        description: row.description,
        categories: row.categories,
        content_rating: row.content_rating,
        start_time: row.start_time,
        end_time: row.end_time,
        duration_minutes: row.duration_minutes,
        is_live: row.is_live,
        is_new: row.is_new,
        is_rerun: row.is_rerun,
      });
    }

    return Array.from(channelMap.values());
  }

  async getWhatsOnNow(channelIds?: string[]): Promise<WhatsOnNowEntry[]> {
    const conditions: string[] = [
      'c.source_account_id = $1',
      'c.is_active = true',
    ];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (channelIds && channelIds.length > 0) {
      const placeholders = channelIds.map((_, i) => `$${paramIndex + i}`).join(', ');
      conditions.push(`c.id IN (${placeholders})`);
      values.push(...channelIds);
      paramIndex += channelIds.length;
    }

    // Get active channels
    const channelsResult = await this.query<ChannelRecord>(
      `SELECT * FROM epg_channels c WHERE ${conditions.join(' AND ')} ORDER BY c.sort_order ASC, c.channel_number ASC`,
      values
    );

    const entries: WhatsOnNowEntry[] = [];

    for (const channel of channelsResult.rows) {
      // Get current program
      const currentResult = await this.query<ScheduleEntry & { program_id: string }>(
        `SELECT s.id as schedule_id, s.start_time, s.end_time, s.is_live, s.is_rerun,
                p.id as program_id, p.title, p.episode_title, p.description, p.categories,
                p.content_rating, p.duration_minutes, p.is_new
         FROM epg_schedules s
         JOIN epg_programs p ON s.program_id = p.id
         WHERE s.source_account_id = $1
           AND s.channel_id = $2
           AND s.start_time <= NOW()
           AND s.end_time > NOW()
         ORDER BY s.start_time DESC
         LIMIT 1`,
        [this.sourceAccountId, channel.id]
      );

      // Get next program
      const nextResult = await this.query<ScheduleEntry & { program_id: string }>(
        `SELECT s.id as schedule_id, s.start_time, s.end_time, s.is_live, s.is_rerun,
                p.id as program_id, p.title, p.episode_title, p.description, p.categories,
                p.content_rating, p.duration_minutes, p.is_new
         FROM epg_schedules s
         JOIN epg_programs p ON s.program_id = p.id
         WHERE s.source_account_id = $1
           AND s.channel_id = $2
           AND s.start_time > NOW()
         ORDER BY s.start_time ASC
         LIMIT 1`,
        [this.sourceAccountId, channel.id]
      );

      entries.push({
        channel_id: channel.id,
        channel_name: channel.name,
        channel_number: channel.channel_number,
        logo_url: channel.logo_url,
        current_program: currentResult.rows[0] ?? null,
        next_program: nextResult.rows[0] ?? null,
      });
    }

    return entries;
  }

  async getScheduleForChannel(channelId: string, startDate: Date, days = 7): Promise<ScheduleEntry[]> {
    const endDate = new Date(startDate.getTime() + days * 24 * 60 * 60 * 1000);

    const result = await this.query<ScheduleEntry>(
      `SELECT s.id as schedule_id, s.start_time, s.end_time, s.is_live, s.is_rerun,
              p.id as program_id, p.title, p.episode_title, p.description, p.categories,
              p.content_rating, p.duration_minutes, p.is_new
       FROM epg_schedules s
       JOIN epg_programs p ON s.program_id = p.id
       WHERE s.source_account_id = $1
         AND s.channel_id = $2
         AND s.start_time >= $3
         AND s.start_time < $4
       ORDER BY s.start_time ASC`,
      [this.sourceAccountId, channelId, startDate, endDate]
    );

    return result.rows;
  }

  async getUpcomingAirings(programId: string, days = 14): Promise<(ScheduleEntry & { channel_name: string; channel_number: string | null })[]> {
    const result = await this.query<ScheduleEntry & { channel_name: string; channel_number: string | null }>(
      `SELECT s.id as schedule_id, s.start_time, s.end_time, s.is_live, s.is_rerun,
              p.id as program_id, p.title, p.episode_title, p.description, p.categories,
              p.content_rating, p.duration_minutes, p.is_new,
              c.name as channel_name, c.channel_number
       FROM epg_schedules s
       JOIN epg_programs p ON s.program_id = p.id
       JOIN epg_channels c ON s.channel_id = c.id
       WHERE s.source_account_id = $1
         AND s.program_id = $2
         AND s.start_time >= NOW()
         AND s.start_time < NOW() + INTERVAL '${days} days'
       ORDER BY s.start_time ASC`,
      [this.sourceAccountId, programId]
    );

    return result.rows;
  }

  // =========================================================================
  // Channel Group Operations
  // =========================================================================

  async createChannelGroup(group: Omit<ChannelGroupRecord, 'id' | 'created_at'>): Promise<ChannelGroupRecord> {
    const result = await this.query<ChannelGroupRecord>(
      `INSERT INTO epg_channel_groups (
        source_account_id, name, description, sort_order, metadata
      ) VALUES ($1, $2, $3, $4, $5)
      RETURNING *`,
      [
        this.sourceAccountId, group.name, group.description,
        group.sort_order, JSON.stringify(group.metadata),
      ]
    );

    return result.rows[0];
  }

  async listChannelGroups(): Promise<ChannelGroupRecord[]> {
    const result = await this.query<ChannelGroupRecord>(
      `SELECT * FROM epg_channel_groups
       WHERE source_account_id = $1
       ORDER BY sort_order ASC, name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async updateChannelGroup(id: string, updates: Partial<ChannelGroupRecord>): Promise<ChannelGroupRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = ['name', 'description', 'sort_order'];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (fields.length === 0) {
      const result = await this.query<ChannelGroupRecord>(
        `SELECT * FROM epg_channel_groups WHERE id = $1 AND source_account_id = $2`,
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    }

    values.push(id, this.sourceAccountId);

    const result = await this.query<ChannelGroupRecord>(
      `UPDATE epg_channel_groups
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteChannelGroup(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM epg_channel_groups WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async addChannelToGroup(groupId: string, channelId: string, sortOrder = 0): Promise<ChannelGroupMemberRecord> {
    const result = await this.query<ChannelGroupMemberRecord>(
      `INSERT INTO epg_channel_group_members (
        source_account_id, group_id, channel_id, sort_order
      ) VALUES ($1, $2, $3, $4)
      ON CONFLICT (source_account_id, group_id, channel_id) DO UPDATE SET
        sort_order = EXCLUDED.sort_order
      RETURNING *`,
      [this.sourceAccountId, groupId, channelId, sortOrder]
    );

    return result.rows[0];
  }

  async removeChannelFromGroup(groupId: string, channelId: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM epg_channel_group_members
       WHERE group_id = $1 AND channel_id = $2 AND source_account_id = $3`,
      [groupId, channelId, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Import Operations
  // =========================================================================

  async upsertChannelByCallSign(channel: Omit<ChannelRecord, 'id' | 'created_at' | 'updated_at'>): Promise<ChannelRecord> {
    const result = await this.query<ChannelRecord>(
      `INSERT INTO epg_channels (
        source_account_id, channel_number, call_sign, name, display_name,
        logo_url, category, language, country, is_active, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, $10)
      ON CONFLICT (source_account_id, call_sign) DO UPDATE SET
        channel_number = COALESCE(EXCLUDED.channel_number, epg_channels.channel_number),
        name = EXCLUDED.name,
        display_name = EXCLUDED.display_name,
        logo_url = COALESCE(EXCLUDED.logo_url, epg_channels.logo_url),
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, channel.channel_number, channel.call_sign,
        channel.name, channel.display_name, channel.logo_url,
        channel.category, channel.language, channel.country,
        JSON.stringify(channel.metadata),
      ]
    );

    return result.rows[0];
  }

  async cleanupOldSchedules(days: number): Promise<number> {
    const count = await this.execute(
      `DELETE FROM epg_schedules
       WHERE source_account_id = $1
         AND end_time < NOW() - INTERVAL '${days} days'`,
      [this.sourceAccountId]
    );
    return count;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<EpgStats> {
    const result = await this.query<{
      total_channels: string;
      active_channels: string;
      total_programs: string;
      total_schedules: string;
      total_channel_groups: string;
      oldest_schedule: Date | null;
      newest_schedule: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM epg_channels WHERE source_account_id = $1) as total_channels,
        (SELECT COUNT(*) FROM epg_channels WHERE source_account_id = $1 AND is_active = true) as active_channels,
        (SELECT COUNT(*) FROM epg_programs WHERE source_account_id = $1) as total_programs,
        (SELECT COUNT(*) FROM epg_schedules WHERE source_account_id = $1) as total_schedules,
        (SELECT COUNT(*) FROM epg_channel_groups WHERE source_account_id = $1) as total_channel_groups,
        (SELECT MIN(start_time) FROM epg_schedules WHERE source_account_id = $1) as oldest_schedule,
        (SELECT MAX(end_time) FROM epg_schedules WHERE source_account_id = $1) as newest_schedule`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_channels: parseInt(row.total_channels, 10),
      active_channels: parseInt(row.active_channels, 10),
      total_programs: parseInt(row.total_programs, 10),
      total_schedules: parseInt(row.total_schedules, 10),
      total_channel_groups: parseInt(row.total_channel_groups, 10),
      oldest_schedule: row.oldest_schedule,
      newest_schedule: row.newest_schedule,
    };
  }
}
