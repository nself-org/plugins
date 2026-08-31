-- Purpose: Initial schema for the meetings plugin.
-- Tables use meetings_ prefix and source_account_id for multi-app isolation (Convention A).
-- Covers: calendars, rooms, events, attendees, calendar-shares,
--         external-calendars, templates, reminders, waitlist.
-- DO NOT HAND EDIT — managed by nself migrate.

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

CREATE INDEX IF NOT EXISTS idx_meetings_calendars_owner
    ON meetings_calendars (owner_id);
CREATE INDEX IF NOT EXISTS idx_meetings_calendars_source
    ON meetings_calendars (source_account_id);

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

CREATE INDEX IF NOT EXISTS idx_meetings_rooms_active
    ON meetings_rooms (is_active);
CREATE INDEX IF NOT EXISTS idx_meetings_rooms_capacity
    ON meetings_rooms (capacity);
CREATE INDEX IF NOT EXISTS idx_meetings_rooms_source
    ON meetings_rooms (source_account_id);

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
    status TEXT NOT NULL DEFAULT 'confirmed'
        CHECK (status IN ('confirmed', 'tentative', 'cancelled')),
    visibility TEXT NOT NULL DEFAULT 'public'
        CHECK (visibility IN ('public', 'private', 'confidential')),
    google_event_id TEXT,
    outlook_event_id TEXT,
    ical_uid TEXT,
    metadata JSONB DEFAULT '{}',
    CONSTRAINT valid_time_range CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_meetings_events_organizer
    ON meetings_events (organizer_id);
CREATE INDEX IF NOT EXISTS idx_meetings_events_start_time
    ON meetings_events (start_time);
CREATE INDEX IF NOT EXISTS idx_meetings_events_end_time
    ON meetings_events (end_time);
CREATE INDEX IF NOT EXISTS idx_meetings_events_calendar
    ON meetings_events (calendar_id);
CREATE INDEX IF NOT EXISTS idx_meetings_events_room
    ON meetings_events (room_id);
CREATE INDEX IF NOT EXISTS idx_meetings_events_status
    ON meetings_events (status);
CREATE INDEX IF NOT EXISTS idx_meetings_events_source
    ON meetings_events (source_account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_meetings_events_google_id
    ON meetings_events (source_account_id, google_event_id)
    WHERE google_event_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_meetings_events_outlook_id
    ON meetings_events (source_account_id, outlook_event_id)
    WHERE outlook_event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS meetings_attendees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_id UUID NOT NULL REFERENCES meetings_events(id) ON DELETE CASCADE,
    user_id TEXT,
    external_email TEXT,
    external_name TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'tentative')),
    response_time TIMESTAMPTZ,
    is_optional BOOLEAN DEFAULT FALSE,
    can_modify BOOLEAN DEFAULT FALSE,
    metadata JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_meetings_attendees_event
    ON meetings_attendees (event_id);
CREATE INDEX IF NOT EXISTS idx_meetings_attendees_user
    ON meetings_attendees (user_id);
CREATE INDEX IF NOT EXISTS idx_meetings_attendees_status
    ON meetings_attendees (status);
CREATE INDEX IF NOT EXISTS idx_meetings_attendees_source
    ON meetings_attendees (source_account_id);

CREATE TABLE IF NOT EXISTS meetings_calendar_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    calendar_id UUID NOT NULL REFERENCES meetings_calendars(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    permission TEXT NOT NULL DEFAULT 'read'
        CHECK (permission IN ('read', 'write', 'admin'))
);

CREATE INDEX IF NOT EXISTS idx_meetings_calendar_shares_calendar
    ON meetings_calendar_shares (calendar_id);
CREATE INDEX IF NOT EXISTS idx_meetings_calendar_shares_user
    ON meetings_calendar_shares (user_id);
CREATE INDEX IF NOT EXISTS idx_meetings_calendar_shares_source
    ON meetings_calendar_shares (source_account_id);

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
    sync_direction TEXT NOT NULL DEFAULT 'bidirectional'
        CHECK (sync_direction IN ('import', 'export', 'bidirectional')),
    metadata JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_meetings_external_calendars_user
    ON meetings_external_calendars (user_id);
CREATE INDEX IF NOT EXISTS idx_meetings_external_calendars_provider
    ON meetings_external_calendars (provider);
CREATE INDEX IF NOT EXISTS idx_meetings_external_calendars_source
    ON meetings_external_calendars (source_account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_meetings_external_calendars_unique
    ON meetings_external_calendars (user_id, provider, source_account_id);

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

CREATE INDEX IF NOT EXISTS idx_meetings_templates_creator
    ON meetings_templates (created_by);
CREATE INDEX IF NOT EXISTS idx_meetings_templates_public
    ON meetings_templates (is_public);
CREATE INDEX IF NOT EXISTS idx_meetings_templates_source
    ON meetings_templates (source_account_id);

CREATE TABLE IF NOT EXISTS meetings_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_id UUID NOT NULL REFERENCES meetings_events(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    remind_at TIMESTAMPTZ NOT NULL,
    minutes_before INT NOT NULL,
    method TEXT NOT NULL DEFAULT 'push'
        CHECK (method IN ('push', 'email', 'both')),
    sent BOOLEAN DEFAULT FALSE,
    sent_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_meetings_reminders_event
    ON meetings_reminders (event_id);
CREATE INDEX IF NOT EXISTS idx_meetings_reminders_user
    ON meetings_reminders (user_id);
CREATE INDEX IF NOT EXISTS idx_meetings_reminders_remind_at
    ON meetings_reminders (remind_at) WHERE NOT sent;
CREATE INDEX IF NOT EXISTS idx_meetings_reminders_source
    ON meetings_reminders (source_account_id);

CREATE TABLE IF NOT EXISTS meetings_waitlist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_id UUID NOT NULL REFERENCES meetings_events(id) ON DELETE CASCADE,
    room_id UUID NOT NULL REFERENCES meetings_rooms(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'waiting'
        CHECK (status IN ('waiting', 'confirmed', 'cancelled')),
    notified_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_meetings_waitlist_room
    ON meetings_waitlist (room_id);
CREATE INDEX IF NOT EXISTS idx_meetings_waitlist_status
    ON meetings_waitlist (status);
CREATE INDEX IF NOT EXISTS idx_meetings_waitlist_source
    ON meetings_waitlist (source_account_id);
