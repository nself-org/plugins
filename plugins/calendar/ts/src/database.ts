/**
 * Calendar Database Operations
 * Complete CRUD operations for calendars, events, attendees, and reminders
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  CalendarRecord,
  CalendarEventRecord,
  AttendeeRecord,
  ReminderRecord,
  ICalFeedRecord,
  CalendarStats,
} from './types.js';

const logger = createLogger('calendar:db');

export class CalendarDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): CalendarDatabase {
    return new CalendarDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing calendar schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Calendars
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS calendar_calendars (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        color VARCHAR(7) DEFAULT '#3B82F6',
        owner_id VARCHAR(255) NOT NULL,
        owner_type VARCHAR(32) DEFAULT 'user',
        is_default BOOLEAN DEFAULT false,
        timezone VARCHAR(64) DEFAULT 'UTC',
        visibility VARCHAR(16) DEFAULT 'private',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_calendars_source_account
        ON calendar_calendars(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_calendars_owner
        ON calendar_calendars(owner_id);

      -- =====================================================================
      -- Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS calendar_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        calendar_id UUID NOT NULL REFERENCES calendar_calendars(id) ON DELETE CASCADE,
        title VARCHAR(500) NOT NULL,
        description TEXT,
        event_type VARCHAR(32) DEFAULT 'event',
        start_at TIMESTAMP WITH TIME ZONE NOT NULL,
        end_at TIMESTAMP WITH TIME ZONE,
        all_day BOOLEAN DEFAULT false,
        timezone VARCHAR(64) DEFAULT 'UTC',
        location_name VARCHAR(255),
        location_address TEXT,
        location_lat DOUBLE PRECISION,
        location_lon DOUBLE PRECISION,
        recurrence_rule TEXT,
        recurrence_end_at TIMESTAMP WITH TIME ZONE,
        series_id UUID,
        is_exception BOOLEAN DEFAULT false,
        original_start_at TIMESTAMP WITH TIME ZONE,
        color VARCHAR(7),
        url TEXT,
        organizer_id VARCHAR(255),
        status VARCHAR(16) DEFAULT 'confirmed',
        visibility VARCHAR(16) DEFAULT 'default',
        reminder_minutes INTEGER[] DEFAULT '{15}',
        attendee_count INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_events_source_account
        ON calendar_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_events_calendar
        ON calendar_events(calendar_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_events_start
        ON calendar_events(start_at);
      CREATE INDEX IF NOT EXISTS idx_calendar_events_end
        ON calendar_events(end_at);
      CREATE INDEX IF NOT EXISTS idx_calendar_events_series
        ON calendar_events(series_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_events_type
        ON calendar_events(event_type);

      -- =====================================================================
      -- Attendees
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS calendar_attendees (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_id UUID NOT NULL REFERENCES calendar_events(id) ON DELETE CASCADE,
        user_id VARCHAR(255),
        email VARCHAR(255),
        name VARCHAR(255),
        rsvp_status VARCHAR(16) DEFAULT 'pending',
        rsvp_at TIMESTAMP WITH TIME ZONE,
        role VARCHAR(16) DEFAULT 'attendee',
        checked_in BOOLEAN DEFAULT false,
        checked_in_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(event_id, user_id)
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_attendees_source_account
        ON calendar_attendees(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_attendees_event
        ON calendar_attendees(event_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_attendees_user
        ON calendar_attendees(user_id);

      -- =====================================================================
      -- Reminders
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS calendar_reminders (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_id UUID NOT NULL REFERENCES calendar_events(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        remind_at TIMESTAMP WITH TIME ZONE NOT NULL,
        channel VARCHAR(16) DEFAULT 'push',
        sent BOOLEAN DEFAULT false,
        sent_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_reminders_source_account
        ON calendar_reminders(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_reminders_event
        ON calendar_reminders(event_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_reminders_user
        ON calendar_reminders(user_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_reminders_time
        ON calendar_reminders(remind_at) WHERE NOT sent;

      -- =====================================================================
      -- iCal Feeds
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS calendar_ical_feeds (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        calendar_id UUID NOT NULL REFERENCES calendar_calendars(id) ON DELETE CASCADE,
        token VARCHAR(128) NOT NULL UNIQUE,
        name VARCHAR(255),
        enabled BOOLEAN DEFAULT true,
        last_accessed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_ical_feeds_source_account
        ON calendar_ical_feeds(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_ical_feeds_calendar
        ON calendar_ical_feeds(calendar_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_ical_feeds_token
        ON calendar_ical_feeds(token);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS calendar_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_calendar_webhook_events_source_account
        ON calendar_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_calendar_webhook_events_processed
        ON calendar_webhook_events(processed);
    `;

    await this.execute(schema);
    logger.info('Calendar schema initialized successfully');
  }

  // =========================================================================
  // Calendar Operations
  // =========================================================================

  async createCalendar(calendar: Omit<CalendarRecord, 'id' | 'created_at' | 'updated_at'>): Promise<CalendarRecord> {
    const result = await this.query<CalendarRecord>(
      `INSERT INTO calendar_calendars (
        source_account_id, name, description, color, owner_id, owner_type,
        is_default, timezone, visibility, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        calendar.name,
        calendar.description,
        calendar.color,
        calendar.owner_id,
        calendar.owner_type,
        calendar.is_default,
        calendar.timezone,
        calendar.visibility,
        calendar.metadata,
      ]
    );

    return result.rows[0];
  }

  async getCalendar(id: string): Promise<CalendarRecord | null> {
    const result = await this.query<CalendarRecord>(
      `SELECT * FROM calendar_calendars WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listCalendars(ownerId?: string): Promise<CalendarRecord[]> {
    if (ownerId) {
      const result = await this.query<CalendarRecord>(
        `SELECT * FROM calendar_calendars
         WHERE source_account_id = $1 AND owner_id = $2
         ORDER BY is_default DESC, name ASC`,
        [this.sourceAccountId, ownerId]
      );
      return result.rows;
    }

    const result = await this.query<CalendarRecord>(
      `SELECT * FROM calendar_calendars
       WHERE source_account_id = $1
       ORDER BY is_default DESC, name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async updateCalendar(id: string, updates: Partial<CalendarRecord>): Promise<CalendarRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = ['name', 'description', 'color', 'is_default', 'timezone', 'visibility', 'metadata'];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (fields.length === 0) {
      return this.getCalendar(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<CalendarRecord>(
      `UPDATE calendar_calendars
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteCalendar(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM calendar_calendars WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  // =========================================================================
  // Event Operations
  // =========================================================================

  async createEvent(event: Omit<CalendarEventRecord, 'id' | 'created_at' | 'updated_at'>): Promise<CalendarEventRecord> {
    const result = await this.query<CalendarEventRecord>(
      `INSERT INTO calendar_events (
        source_account_id, calendar_id, title, description, event_type,
        start_at, end_at, all_day, timezone, location_name, location_address,
        location_lat, location_lon, recurrence_rule, recurrence_end_at,
        series_id, is_exception, original_start_at, color, url, organizer_id,
        status, visibility, reminder_minutes, attendee_count, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
                $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
      RETURNING *`,
      [
        this.sourceAccountId,
        event.calendar_id,
        event.title,
        event.description,
        event.event_type,
        event.start_at,
        event.end_at,
        event.all_day,
        event.timezone,
        event.location_name,
        event.location_address,
        event.location_lat,
        event.location_lon,
        event.recurrence_rule,
        event.recurrence_end_at,
        event.series_id,
        event.is_exception,
        event.original_start_at,
        event.color,
        event.url,
        event.organizer_id,
        event.status,
        event.visibility,
        event.reminder_minutes,
        event.attendee_count,
        event.metadata,
      ]
    );

    return result.rows[0];
  }

  async getEvent(id: string): Promise<CalendarEventRecord | null> {
    const result = await this.query<CalendarEventRecord>(
      `SELECT * FROM calendar_events WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listEvents(filters: {
    calendarId?: string;
    start?: Date;
    end?: Date;
    type?: string;
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<CalendarEventRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.calendarId) {
      conditions.push(`calendar_id = $${paramIndex}`);
      values.push(filters.calendarId);
      paramIndex++;
    }

    if (filters.start) {
      conditions.push(`start_at >= $${paramIndex}`);
      values.push(filters.start);
      paramIndex++;
    }

    if (filters.end) {
      conditions.push(`start_at <= $${paramIndex}`);
      values.push(filters.end);
      paramIndex++;
    }

    if (filters.type) {
      conditions.push(`event_type = $${paramIndex}`);
      values.push(filters.type);
      paramIndex++;
    }

    if (filters.status) {
      conditions.push(`status = $${paramIndex}`);
      values.push(filters.status);
      paramIndex++;
    }

    let query = `
      SELECT * FROM calendar_events
      WHERE ${conditions.join(' AND ')}
      ORDER BY start_at ASC
    `;

    if (filters.limit) {
      query += ` LIMIT ${filters.limit}`;
    }

    if (filters.offset) {
      query += ` OFFSET ${filters.offset}`;
    }

    const result = await this.query<CalendarEventRecord>(query, values);
    return result.rows;
  }

  async updateEvent(id: string, updates: Partial<CalendarEventRecord>): Promise<CalendarEventRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'title', 'description', 'event_type', 'start_at', 'end_at', 'all_day',
      'timezone', 'location_name', 'location_address', 'location_lat',
      'location_lon', 'recurrence_rule', 'recurrence_end_at', 'color',
      'url', 'status', 'visibility', 'reminder_minutes', 'metadata'
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (fields.length === 0) {
      return this.getEvent(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<CalendarEventRecord>(
      `UPDATE calendar_events
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteEvent(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM calendar_events WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  async getUpcomingEvents(limit = 10): Promise<CalendarEventRecord[]> {
    const result = await this.query<CalendarEventRecord>(
      `SELECT * FROM calendar_events
       WHERE source_account_id = $1
         AND start_at >= NOW()
         AND status != 'cancelled'
       ORDER BY start_at ASC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );

    return result.rows;
  }

  async getTodayEvents(): Promise<CalendarEventRecord[]> {
    const result = await this.query<CalendarEventRecord>(
      `SELECT * FROM calendar_events
       WHERE source_account_id = $1
         AND start_at >= CURRENT_DATE
         AND start_at < CURRENT_DATE + INTERVAL '1 day'
         AND status != 'cancelled'
       ORDER BY start_at ASC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async getUpcomingBirthdays(days = 30): Promise<CalendarEventRecord[]> {
    const result = await this.query<CalendarEventRecord>(
      `SELECT * FROM calendar_events
       WHERE source_account_id = $1
         AND event_type = 'birthday'
         AND start_at >= NOW()
         AND start_at <= NOW() + INTERVAL '${days} days'
         AND status != 'cancelled'
       ORDER BY start_at ASC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  // =========================================================================
  // Attendee Operations
  // =========================================================================

  async addAttendee(attendee: Omit<AttendeeRecord, 'id' | 'created_at'>): Promise<AttendeeRecord> {
    const result = await this.query<AttendeeRecord>(
      `INSERT INTO calendar_attendees (
        source_account_id, event_id, user_id, email, name, rsvp_status,
        rsvp_at, role, checked_in, checked_in_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      ON CONFLICT (event_id, user_id) DO UPDATE SET
        email = EXCLUDED.email,
        name = EXCLUDED.name,
        role = EXCLUDED.role
      RETURNING *`,
      [
        this.sourceAccountId,
        attendee.event_id,
        attendee.user_id,
        attendee.email,
        attendee.name,
        attendee.rsvp_status,
        attendee.rsvp_at,
        attendee.role,
        attendee.checked_in,
        attendee.checked_in_at,
      ]
    );

    // Update attendee count
    await this.execute(
      `UPDATE calendar_events
       SET attendee_count = (
         SELECT COUNT(*) FROM calendar_attendees
         WHERE event_id = $1 AND source_account_id = $2
       )
       WHERE id = $1 AND source_account_id = $2`,
      [attendee.event_id, this.sourceAccountId]
    );

    return result.rows[0];
  }

  async listAttendees(eventId: string): Promise<AttendeeRecord[]> {
    const result = await this.query<AttendeeRecord>(
      `SELECT * FROM calendar_attendees
       WHERE event_id = $1 AND source_account_id = $2
       ORDER BY role ASC, name ASC`,
      [eventId, this.sourceAccountId]
    );

    return result.rows;
  }

  async updateRSVP(eventId: string, userId: string, status: string): Promise<AttendeeRecord | null> {
    const result = await this.query<AttendeeRecord>(
      `UPDATE calendar_attendees
       SET rsvp_status = $1, rsvp_at = NOW()
       WHERE event_id = $2 AND user_id = $3 AND source_account_id = $4
       RETURNING *`,
      [status, eventId, userId, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async checkInAttendee(eventId: string, userId: string): Promise<AttendeeRecord | null> {
    const result = await this.query<AttendeeRecord>(
      `UPDATE calendar_attendees
       SET checked_in = true, checked_in_at = NOW()
       WHERE event_id = $1 AND user_id = $2 AND source_account_id = $3
       RETURNING *`,
      [eventId, userId, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async removeAttendee(eventId: string, userId: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM calendar_attendees
       WHERE event_id = $1 AND user_id = $2 AND source_account_id = $3`,
      [eventId, userId, this.sourceAccountId]
    );

    // Update attendee count
    await this.execute(
      `UPDATE calendar_events
       SET attendee_count = (
         SELECT COUNT(*) FROM calendar_attendees
         WHERE event_id = $1 AND source_account_id = $2
       )
       WHERE id = $1 AND source_account_id = $2`,
      [eventId, this.sourceAccountId]
    );

    return count > 0;
  }

  // =========================================================================
  // Reminder Operations
  // =========================================================================

  async createReminder(reminder: Omit<ReminderRecord, 'id' | 'created_at'>): Promise<ReminderRecord> {
    const result = await this.query<ReminderRecord>(
      `INSERT INTO calendar_reminders (
        source_account_id, event_id, user_id, remind_at, channel, sent, sent_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        reminder.event_id,
        reminder.user_id,
        reminder.remind_at,
        reminder.channel,
        reminder.sent,
        reminder.sent_at,
      ]
    );

    return result.rows[0];
  }

  async getPendingReminders(): Promise<ReminderRecord[]> {
    const result = await this.query<ReminderRecord>(
      `SELECT * FROM calendar_reminders
       WHERE source_account_id = $1
         AND NOT sent
         AND remind_at <= NOW()
       ORDER BY remind_at ASC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async markReminderSent(id: string): Promise<void> {
    await this.execute(
      `UPDATE calendar_reminders
       SET sent = true, sent_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // iCal Feed Operations
  // =========================================================================

  async createICalFeed(feed: Omit<ICalFeedRecord, 'id' | 'created_at'>): Promise<ICalFeedRecord> {
    const result = await this.query<ICalFeedRecord>(
      `INSERT INTO calendar_ical_feeds (
        source_account_id, calendar_id, token, name, enabled
      ) VALUES ($1, $2, $3, $4, $5)
      RETURNING *`,
      [this.sourceAccountId, feed.calendar_id, feed.token, feed.name, feed.enabled]
    );

    return result.rows[0];
  }

  async getICalFeedByToken(token: string): Promise<ICalFeedRecord | null> {
    const result = await this.query<ICalFeedRecord>(
      `SELECT * FROM calendar_ical_feeds WHERE token = $1 AND enabled = true`,
      [token]
    );

    if (result.rows[0]) {
      // Update last accessed time
      await this.execute(
        `UPDATE calendar_ical_feeds SET last_accessed_at = NOW() WHERE id = $1`,
        [result.rows[0].id]
      );
    }

    return result.rows[0] ?? null;
  }

  async listICalFeeds(calendarId?: string): Promise<ICalFeedRecord[]> {
    if (calendarId) {
      const result = await this.query<ICalFeedRecord>(
        `SELECT * FROM calendar_ical_feeds
         WHERE source_account_id = $1 AND calendar_id = $2
         ORDER BY created_at DESC`,
        [this.sourceAccountId, calendarId]
      );
      return result.rows;
    }

    const result = await this.query<ICalFeedRecord>(
      `SELECT * FROM calendar_ical_feeds
       WHERE source_account_id = $1
       ORDER BY created_at DESC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async deleteICalFeed(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM calendar_ical_feeds WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<CalendarStats> {
    const result = await this.query<{
      calendars: string;
      events: string;
      attendees: string;
      reminders: string;
      ical_feeds: string;
      upcoming_events: string;
      last_event_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM calendar_calendars WHERE source_account_id = $1) as calendars,
        (SELECT COUNT(*) FROM calendar_events WHERE source_account_id = $1) as events,
        (SELECT COUNT(*) FROM calendar_attendees WHERE source_account_id = $1) as attendees,
        (SELECT COUNT(*) FROM calendar_reminders WHERE source_account_id = $1) as reminders,
        (SELECT COUNT(*) FROM calendar_ical_feeds WHERE source_account_id = $1) as ical_feeds,
        (SELECT COUNT(*) FROM calendar_events
         WHERE source_account_id = $1 AND start_at >= NOW() AND status != 'cancelled') as upcoming_events,
        (SELECT MAX(start_at) FROM calendar_events WHERE source_account_id = $1) as last_event_at`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      calendars: parseInt(row.calendars, 10),
      events: parseInt(row.events, 10),
      attendees: parseInt(row.attendees, 10),
      reminders: parseInt(row.reminders, 10),
      icalFeeds: parseInt(row.ical_feeds, 10),
      upcomingEvents: parseInt(row.upcoming_events, 10),
      lastEventAt: row.last_event_at,
    };
  }
}
