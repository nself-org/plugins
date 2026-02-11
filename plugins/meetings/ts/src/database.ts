/**
 * Database client for meetings operations
 * Multi-app aware: all queries are scoped by source_account_id
 */

import { Pool, PoolClient } from 'pg';
import { config } from './config.js';
import {
  MeetingEvent, CreateEventInput, UpdateEventInput,
  Attendee, CreateAttendeeInput, RsvpInput,
  Room, CreateRoomInput, UpdateRoomInput,
  Calendar, CreateCalendarInput, UpdateCalendarInput,
  ExternalCalendar,
  MeetingTemplate, CreateTemplateInput, UpdateTemplateInput,
  Reminder, ReminderMethod,
  AvailabilityResult,
  ListEventsQuery, ListRoomsQuery,
} from './types.js';

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
        CREATE TABLE IF NOT EXISTS meetings_calendars (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          name TEXT NOT NULL,
          description TEXT,
          color TEXT,
          owner_id TEXT NOT NULL,
          is_default BOOLEAN DEFAULT FALSE,
          is_public BOOLEAN DEFAULT FALSE,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_calendars_owner ON meetings_calendars(owner_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_calendars_source ON meetings_calendars(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_rooms (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          name TEXT NOT NULL,
          description TEXT,
          location TEXT,
          floor TEXT,
          building TEXT,
          capacity INT NOT NULL CHECK (capacity > 0),
          has_video_conference BOOLEAN DEFAULT FALSE,
          has_projector BOOLEAN DEFAULT FALSE,
          has_whiteboard BOOLEAN DEFAULT FALSE,
          has_phone BOOLEAN DEFAULT FALSE,
          amenities JSONB DEFAULT '[]',
          is_active BOOLEAN DEFAULT TRUE,
          booking_buffer_minutes INT DEFAULT 0,
          is_public BOOLEAN DEFAULT TRUE,
          allowed_groups JSONB DEFAULT '[]',
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_rooms_active ON meetings_rooms(is_active);
        CREATE INDEX IF NOT EXISTS idx_meetings_rooms_capacity ON meetings_rooms(capacity);
        CREATE INDEX IF NOT EXISTS idx_meetings_rooms_source ON meetings_rooms(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_events (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          title TEXT NOT NULL,
          description TEXT,
          location TEXT,
          video_link TEXT,
          start_time TIMESTAMPTZ NOT NULL,
          end_time TIMESTAMPTZ NOT NULL,
          all_day BOOLEAN DEFAULT FALSE,
          timezone TEXT NOT NULL DEFAULT 'UTC',
          is_recurring BOOLEAN DEFAULT FALSE,
          recurrence_rule JSONB,
          recurrence_parent_id UUID REFERENCES meetings_events(id) ON DELETE CASCADE,
          organizer_id TEXT NOT NULL,
          calendar_id UUID REFERENCES meetings_calendars(id) ON DELETE SET NULL,
          room_id UUID REFERENCES meetings_rooms(id) ON DELETE SET NULL,
          status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed', 'tentative', 'cancelled')),
          visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private', 'confidential')),
          google_event_id TEXT,
          outlook_event_id TEXT,
          ical_uid TEXT,
          metadata JSONB DEFAULT '{}',
          CONSTRAINT valid_time_range CHECK (end_time > start_time)
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_events_organizer ON meetings_events(organizer_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_events_start_time ON meetings_events(start_time);
        CREATE INDEX IF NOT EXISTS idx_meetings_events_end_time ON meetings_events(end_time);
        CREATE INDEX IF NOT EXISTS idx_meetings_events_calendar ON meetings_events(calendar_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_events_room ON meetings_events(room_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_events_status ON meetings_events(status);
        CREATE INDEX IF NOT EXISTS idx_meetings_events_source ON meetings_events(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_attendees (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          event_id UUID NOT NULL REFERENCES meetings_events(id) ON DELETE CASCADE,
          user_id TEXT,
          external_email TEXT,
          external_name TEXT,
          status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined', 'tentative')),
          response_time TIMESTAMPTZ,
          is_optional BOOLEAN DEFAULT FALSE,
          can_modify BOOLEAN DEFAULT FALSE,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_attendees_event ON meetings_attendees(event_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_attendees_user ON meetings_attendees(user_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_attendees_status ON meetings_attendees(status);
        CREATE INDEX IF NOT EXISTS idx_meetings_attendees_source ON meetings_attendees(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_calendar_shares (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          calendar_id UUID NOT NULL REFERENCES meetings_calendars(id) ON DELETE CASCADE,
          user_id TEXT NOT NULL,
          permission TEXT NOT NULL DEFAULT 'read' CHECK (permission IN ('read', 'write', 'admin'))
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_calendar_shares_calendar ON meetings_calendar_shares(calendar_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_calendar_shares_user ON meetings_calendar_shares(user_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_calendar_shares_source ON meetings_calendar_shares(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_external_calendars (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          user_id TEXT NOT NULL,
          provider TEXT NOT NULL CHECK (provider IN ('google', 'outlook', 'ical')),
          access_token TEXT,
          refresh_token TEXT,
          token_expires_at TIMESTAMPTZ,
          provider_calendar_id TEXT,
          provider_user_id TEXT,
          sync_enabled BOOLEAN DEFAULT TRUE,
          last_sync_at TIMESTAMPTZ,
          sync_direction TEXT NOT NULL DEFAULT 'bidirectional' CHECK (sync_direction IN ('import', 'export', 'bidirectional')),
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_external_calendars_user ON meetings_external_calendars(user_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_external_calendars_provider ON meetings_external_calendars(provider);
        CREATE INDEX IF NOT EXISTS idx_meetings_external_calendars_source ON meetings_external_calendars(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_templates (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          name TEXT NOT NULL,
          description TEXT,
          default_title TEXT,
          default_description TEXT,
          default_duration_minutes INT NOT NULL DEFAULT 60,
          default_location TEXT,
          default_video_link TEXT,
          default_attendees JSONB DEFAULT '[]',
          created_by TEXT NOT NULL,
          is_public BOOLEAN DEFAULT FALSE,
          metadata JSONB DEFAULT '{}'
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_templates_creator ON meetings_templates(created_by);
        CREATE INDEX IF NOT EXISTS idx_meetings_templates_public ON meetings_templates(is_public);
        CREATE INDEX IF NOT EXISTS idx_meetings_templates_source ON meetings_templates(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_reminders (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          event_id UUID NOT NULL REFERENCES meetings_events(id) ON DELETE CASCADE,
          user_id TEXT NOT NULL,
          remind_at TIMESTAMPTZ NOT NULL,
          minutes_before INT NOT NULL,
          method TEXT NOT NULL DEFAULT 'push' CHECK (method IN ('push', 'email', 'both')),
          sent BOOLEAN DEFAULT FALSE,
          sent_at TIMESTAMPTZ
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_reminders_event ON meetings_reminders(event_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_reminders_user ON meetings_reminders(user_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_reminders_remind_at ON meetings_reminders(remind_at) WHERE NOT sent;
        CREATE INDEX IF NOT EXISTS idx_meetings_reminders_source ON meetings_reminders(source_account_id);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS meetings_waitlist (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          event_id UUID NOT NULL REFERENCES meetings_events(id) ON DELETE CASCADE,
          room_id UUID NOT NULL REFERENCES meetings_rooms(id) ON DELETE CASCADE,
          user_id TEXT NOT NULL,
          status TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'confirmed', 'cancelled')),
          notified_at TIMESTAMPTZ
        );
        CREATE INDEX IF NOT EXISTS idx_meetings_waitlist_room ON meetings_waitlist(room_id);
        CREATE INDEX IF NOT EXISTS idx_meetings_waitlist_status ON meetings_waitlist(status);
        CREATE INDEX IF NOT EXISTS idx_meetings_waitlist_source ON meetings_waitlist(source_account_id);
      `);
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Events CRUD
  // =============================================================================

  async createEvent(input: CreateEventInput): Promise<MeetingEvent> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO meetings_events (
          source_account_id, title, description, location, video_link,
          start_time, end_time, all_day, timezone,
          is_recurring, recurrence_rule, recurrence_parent_id,
          organizer_id, calendar_id, room_id, status, visibility, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
        RETURNING *`,
        [
          this.sourceAccountId, input.title, input.description ?? null,
          input.location ?? null, input.video_link ?? null,
          input.start_time, input.end_time, input.all_day ?? false,
          input.timezone ?? 'UTC', input.is_recurring ?? false,
          input.recurrence_rule ? JSON.stringify(input.recurrence_rule) : null,
          input.recurrence_parent_id ?? null, input.organizer_id,
          input.calendar_id ?? null, input.room_id ?? null,
          input.status ?? 'confirmed', input.visibility ?? 'public',
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getEvent(id: string): Promise<MeetingEvent | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_events WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listEvents(query: ListEventsQuery): Promise<{ events: MeetingEvent[]; total: number }> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['source_account_id = $1'];
      const params: unknown[] = [this.sourceAccountId];
      let paramIdx = 2;

      if (query.start_time) {
        conditions.push(`start_time >= $${paramIdx}`);
        params.push(query.start_time);
        paramIdx++;
      }
      if (query.end_time) {
        conditions.push(`end_time <= $${paramIdx}`);
        params.push(query.end_time);
        paramIdx++;
      }
      if (query.calendar_id) {
        conditions.push(`calendar_id = $${paramIdx}`);
        params.push(query.calendar_id);
        paramIdx++;
      }
      if (query.room_id) {
        conditions.push(`room_id = $${paramIdx}`);
        params.push(query.room_id);
        paramIdx++;
      }
      if (query.organizer_id) {
        conditions.push(`organizer_id = $${paramIdx}`);
        params.push(query.organizer_id);
        paramIdx++;
      }
      if (query.status) {
        conditions.push(`status = $${paramIdx}`);
        params.push(query.status);
        paramIdx++;
      }

      const where = conditions.join(' AND ');
      const limit = parseInt(query.limit ?? '50', 10);
      const offset = parseInt(query.offset ?? '0', 10);

      const countResult = await client.query(
        `SELECT COUNT(*) FROM meetings_events WHERE ${where}`, params
      );
      const total = parseInt(countResult.rows[0].count, 10);

      params.push(limit, offset);
      const result = await client.query(
        `SELECT * FROM meetings_events WHERE ${where} ORDER BY start_time ASC LIMIT $${paramIdx} OFFSET $${paramIdx + 1}`,
        params
      );

      return { events: result.rows, total };
    } finally {
      client.release();
    }
  }

  async updateEvent(id: string, input: UpdateEventInput): Promise<MeetingEvent | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        title: input.title, description: input.description, location: input.location,
        video_link: input.video_link, start_time: input.start_time, end_time: input.end_time,
        all_day: input.all_day, timezone: input.timezone, calendar_id: input.calendar_id,
        room_id: input.room_id, status: input.status, visibility: input.visibility,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.recurrence_rule !== undefined) {
        fields.push(`recurrence_rule = $${paramIdx}`);
        params.push(JSON.stringify(input.recurrence_rule));
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE meetings_events SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteEvent(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM meetings_events WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Attendees CRUD
  // =============================================================================

  async addAttendee(eventId: string, input: CreateAttendeeInput): Promise<Attendee> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO meetings_attendees (
          source_account_id, event_id, user_id, external_email, external_name, is_optional, can_modify
        ) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *`,
        [
          this.sourceAccountId, eventId, input.user_id ?? null,
          input.external_email ?? null, input.external_name ?? null,
          input.is_optional ?? false, input.can_modify ?? false,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getEventAttendees(eventId: string): Promise<Attendee[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_attendees WHERE event_id = $1 AND source_account_id = $2 ORDER BY created_at',
        [eventId, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async updateAttendeeRsvp(eventId: string, input: RsvpInput): Promise<Attendee | null> {
    const client = await this.getClient();
    try {
      let result;
      if (input.user_id) {
        result = await client.query(
          `UPDATE meetings_attendees SET status = $1, response_time = NOW(), updated_at = NOW()
           WHERE event_id = $2 AND user_id = $3 AND source_account_id = $4 RETURNING *`,
          [input.status, eventId, input.user_id, this.sourceAccountId]
        );
      } else if (input.external_email) {
        result = await client.query(
          `UPDATE meetings_attendees SET status = $1, response_time = NOW(), updated_at = NOW()
           WHERE event_id = $2 AND external_email = $3 AND source_account_id = $4 RETURNING *`,
          [input.status, eventId, input.external_email, this.sourceAccountId]
        );
      } else {
        return null;
      }
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Rooms CRUD
  // =============================================================================

  async createRoom(input: CreateRoomInput): Promise<Room> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO meetings_rooms (
          source_account_id, name, description, location, floor, building,
          capacity, has_video_conference, has_projector, has_whiteboard, has_phone,
          amenities, booking_buffer_minutes, is_public, allowed_groups, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *`,
        [
          this.sourceAccountId, input.name, input.description ?? null,
          input.location ?? null, input.floor ?? null, input.building ?? null,
          input.capacity, input.has_video_conference ?? false,
          input.has_projector ?? false, input.has_whiteboard ?? false,
          input.has_phone ?? false, JSON.stringify(input.amenities ?? []),
          input.booking_buffer_minutes ?? 0, input.is_public ?? true,
          JSON.stringify(input.allowed_groups ?? []),
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getRoom(id: string): Promise<Room | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_rooms WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listRooms(query: ListRoomsQuery): Promise<{ rooms: Room[]; total: number }> {
    const client = await this.getClient();
    try {
      const conditions: string[] = ['source_account_id = $1'];
      const params: unknown[] = [this.sourceAccountId];
      let paramIdx = 2;

      if (query.is_active !== undefined) {
        conditions.push(`is_active = $${paramIdx}`);
        params.push(query.is_active === 'true');
        paramIdx++;
      }
      if (query.min_capacity) {
        conditions.push(`capacity >= $${paramIdx}`);
        params.push(parseInt(query.min_capacity, 10));
        paramIdx++;
      }
      if (query.has_video_conference !== undefined) {
        conditions.push(`has_video_conference = $${paramIdx}`);
        params.push(query.has_video_conference === 'true');
        paramIdx++;
      }

      const where = conditions.join(' AND ');
      const limit = parseInt(query.limit ?? '50', 10);
      const offset = parseInt(query.offset ?? '0', 10);

      const countResult = await client.query(`SELECT COUNT(*) FROM meetings_rooms WHERE ${where}`, params);
      const total = parseInt(countResult.rows[0].count, 10);

      params.push(limit, offset);
      const result = await client.query(
        `SELECT * FROM meetings_rooms WHERE ${where} ORDER BY name ASC LIMIT $${paramIdx} OFFSET $${paramIdx + 1}`,
        params
      );
      return { rooms: result.rows, total };
    } finally {
      client.release();
    }
  }

  async updateRoom(id: string, input: UpdateRoomInput): Promise<Room | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        name: input.name, description: input.description, location: input.location,
        floor: input.floor, building: input.building, capacity: input.capacity,
        has_video_conference: input.has_video_conference, has_projector: input.has_projector,
        has_whiteboard: input.has_whiteboard, has_phone: input.has_phone,
        is_active: input.is_active, booking_buffer_minutes: input.booking_buffer_minutes,
        is_public: input.is_public,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.amenities !== undefined) {
        fields.push(`amenities = $${paramIdx}`);
        params.push(JSON.stringify(input.amenities));
        paramIdx++;
      }
      if (input.allowed_groups !== undefined) {
        fields.push(`allowed_groups = $${paramIdx}`);
        params.push(JSON.stringify(input.allowed_groups));
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE meetings_rooms SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteRoom(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM meetings_rooms WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  async checkRoomAvailability(roomId: string, startTime: string, endTime: string, excludeEventId?: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT COUNT(*) FROM meetings_events
         WHERE room_id = $1 AND source_account_id = $2 AND status != 'cancelled'
         AND ($3::UUID IS NULL OR id != $3)
         AND (start_time, end_time) OVERLAPS ($4::TIMESTAMPTZ, $5::TIMESTAMPTZ)`,
        [roomId, this.sourceAccountId, excludeEventId ?? null, startTime, endTime]
      );
      return parseInt(result.rows[0].count, 10) === 0;
    } finally {
      client.release();
    }
  }

  async suggestRooms(attendeeCount: number, startTime: string, endTime: string): Promise<Room[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT r.* FROM meetings_rooms r
         WHERE r.source_account_id = $1 AND r.is_active = TRUE AND r.capacity >= $2
         AND NOT EXISTS (
           SELECT 1 FROM meetings_events e
           WHERE e.room_id = r.id AND e.status != 'cancelled'
           AND (e.start_time, e.end_time) OVERLAPS ($3::TIMESTAMPTZ, $4::TIMESTAMPTZ)
         )
         ORDER BY r.capacity ASC`,
        [this.sourceAccountId, attendeeCount, startTime, endTime]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Calendars CRUD
  // =============================================================================

  async createCalendar(input: CreateCalendarInput): Promise<Calendar> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO meetings_calendars (source_account_id, name, description, color, owner_id, is_default, is_public, metadata)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
        [
          this.sourceAccountId, input.name, input.description ?? null,
          input.color ?? null, input.owner_id, input.is_default ?? false,
          input.is_public ?? false, JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getCalendar(id: string): Promise<Calendar | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_calendars WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listCalendars(ownerId?: string): Promise<Calendar[]> {
    const client = await this.getClient();
    try {
      if (ownerId) {
        const result = await client.query(
          'SELECT * FROM meetings_calendars WHERE source_account_id = $1 AND owner_id = $2 ORDER BY name',
          [this.sourceAccountId, ownerId]
        );
        return result.rows;
      }
      const result = await client.query(
        'SELECT * FROM meetings_calendars WHERE source_account_id = $1 ORDER BY name',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async updateCalendar(id: string, input: UpdateCalendarInput): Promise<Calendar | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        name: input.name, description: input.description, color: input.color,
        is_default: input.is_default, is_public: input.is_public,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE meetings_calendars SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteCalendar(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM meetings_calendars WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Templates CRUD
  // =============================================================================

  async createTemplate(input: CreateTemplateInput): Promise<MeetingTemplate> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO meetings_templates (
          source_account_id, name, description, default_title, default_description,
          default_duration_minutes, default_location, default_video_link,
          default_attendees, created_by, is_public, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *`,
        [
          this.sourceAccountId, input.name, input.description ?? null,
          input.default_title ?? null, input.default_description ?? null,
          input.default_duration_minutes, input.default_location ?? null,
          input.default_video_link ?? null,
          JSON.stringify(input.default_attendees ?? []),
          input.created_by, input.is_public ?? false,
          JSON.stringify(input.metadata ?? {}),
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getTemplate(id: string): Promise<MeetingTemplate | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_templates WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async listTemplates(): Promise<MeetingTemplate[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_templates WHERE source_account_id = $1 ORDER BY name',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async updateTemplate(id: string, input: UpdateTemplateInput): Promise<MeetingTemplate | null> {
    const client = await this.getClient();
    try {
      const fields: string[] = ['updated_at = NOW()'];
      const params: unknown[] = [];
      let paramIdx = 1;

      const fieldMap: Record<string, unknown> = {
        name: input.name, description: input.description,
        default_title: input.default_title, default_description: input.default_description,
        default_duration_minutes: input.default_duration_minutes,
        default_location: input.default_location, default_video_link: input.default_video_link,
        is_public: input.is_public,
      };

      for (const [key, value] of Object.entries(fieldMap)) {
        if (value !== undefined) {
          fields.push(`${key} = $${paramIdx}`);
          params.push(value);
          paramIdx++;
        }
      }
      if (input.default_attendees !== undefined) {
        fields.push(`default_attendees = $${paramIdx}`);
        params.push(JSON.stringify(input.default_attendees));
        paramIdx++;
      }
      if (input.metadata !== undefined) {
        fields.push(`metadata = $${paramIdx}`);
        params.push(JSON.stringify(input.metadata));
        paramIdx++;
      }

      params.push(id, this.sourceAccountId);
      const result = await client.query(
        `UPDATE meetings_templates SET ${fields.join(', ')} WHERE id = $${paramIdx} AND source_account_id = $${paramIdx + 1} RETURNING *`,
        params
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async deleteTemplate(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM meetings_templates WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Reminders
  // =============================================================================

  async createReminder(eventId: string, userId: string, minutesBefore: number, method: ReminderMethod): Promise<Reminder> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO meetings_reminders (source_account_id, event_id, user_id, remind_at, minutes_before, method)
         SELECT $1, $2, $3, e.start_time - ($4 || ' minutes')::INTERVAL, $4, $5
         FROM meetings_events e WHERE e.id = $2
         RETURNING *`,
        [this.sourceAccountId, eventId, userId, minutesBefore, method]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async deleteReminder(id: string): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM meetings_reminders WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return (result.rowCount ?? 0) > 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Availability
  // =============================================================================

  async getUserAvailability(userId: string, startTime: string, endTime: string): Promise<AvailabilityResult> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT e.id FROM meetings_events e
         LEFT JOIN meetings_attendees a ON a.event_id = e.id
         WHERE (e.organizer_id = $1 OR a.user_id = $1)
         AND e.source_account_id = $2
         AND e.status != 'cancelled'
         AND (a.status IS NULL OR a.status IN ('accepted', 'tentative'))
         AND (e.start_time, e.end_time) OVERLAPS ($3::TIMESTAMPTZ, $4::TIMESTAMPTZ)`,
        [userId, this.sourceAccountId, startTime, endTime]
      );
      return {
        is_available: result.rows.length === 0,
        conflicting_events: result.rows.map((r: { id: string }) => r.id),
      };
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Sync Status
  // =============================================================================

  async getExternalCalendars(userId: string): Promise<ExternalCalendar[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM meetings_external_calendars WHERE user_id = $1 AND source_account_id = $2',
        [userId, this.sourceAccountId]
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
      const events = await client.query(
        'SELECT COUNT(*) FROM meetings_events WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      const rooms = await client.query(
        'SELECT COUNT(*) FROM meetings_rooms WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      const calendars = await client.query(
        'SELECT COUNT(*) FROM meetings_calendars WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      const templates = await client.query(
        'SELECT COUNT(*) FROM meetings_templates WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      const upcoming = await client.query(
        `SELECT COUNT(*) FROM meetings_events WHERE source_account_id = $1 AND status != 'cancelled' AND start_time > NOW()`,
        [this.sourceAccountId]
      );

      return {
        total_events: parseInt(events.rows[0].count, 10),
        total_rooms: parseInt(rooms.rows[0].count, 10),
        total_calendars: parseInt(calendars.rows[0].count, 10),
        total_templates: parseInt(templates.rows[0].count, 10),
        upcoming_events: parseInt(upcoming.rows[0].count, 10),
      };
    } finally {
      client.release();
    }
  }
}

export const db = new DatabaseClient();
