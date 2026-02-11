/**
 * Calendar Plugin Types
 * Complete type definitions for calendar, events, attendees, and reminders
 */

export interface CalendarPluginConfig {
  port: number;
  host: string;
  defaultTimezone: string;
  maxAttendees: number;
  reminderCheckInterval: number;
  maxRecurrenceExpand: number;
  icalTokenLength: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Calendar Objects
// =============================================================================

export interface CalendarRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  color: string;
  owner_id: string;
  owner_type: 'user' | 'group' | 'system';
  is_default: boolean;
  timezone: string;
  visibility: 'public' | 'shared' | 'private';
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CalendarEventRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  calendar_id: string;
  title: string;
  description: string | null;
  event_type: 'event' | 'birthday' | 'anniversary' | 'holiday' | 'reminder' | 'meeting' | 'trip';
  start_at: Date;
  end_at: Date | null;
  all_day: boolean;
  timezone: string;
  location_name: string | null;
  location_address: string | null;
  location_lat: number | null;
  location_lon: number | null;
  recurrence_rule: string | null;
  recurrence_end_at: Date | null;
  series_id: string | null;
  is_exception: boolean;
  original_start_at: Date | null;
  color: string | null;
  url: string | null;
  organizer_id: string | null;
  status: 'confirmed' | 'tentative' | 'cancelled';
  visibility: 'default' | 'public' | 'private' | 'confidential';
  reminder_minutes: number[];
  attendee_count: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface AttendeeRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  event_id: string;
  user_id: string | null;
  email: string | null;
  name: string | null;
  rsvp_status: 'pending' | 'accepted' | 'declined' | 'tentative';
  rsvp_at: Date | null;
  role: 'organizer' | 'attendee' | 'optional';
  checked_in: boolean;
  checked_in_at: Date | null;
  created_at: Date;
}

export interface ReminderRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  event_id: string;
  user_id: string;
  remind_at: Date;
  channel: 'push' | 'email' | 'sms';
  sent: boolean;
  sent_at: Date | null;
  created_at: Date;
}

export interface ICalFeedRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  calendar_id: string;
  token: string;
  name: string | null;
  enabled: boolean;
  last_accessed_at: Date | null;
  created_at: Date;
}

export interface WebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// API Request/Response Types
// =============================================================================

export interface CreateCalendarRequest {
  name: string;
  description?: string;
  color?: string;
  owner_id: string;
  owner_type?: 'user' | 'group' | 'system';
  is_default?: boolean;
  timezone?: string;
  visibility?: 'public' | 'shared' | 'private';
  metadata?: Record<string, unknown>;
}

export interface UpdateCalendarRequest {
  name?: string;
  description?: string;
  color?: string;
  is_default?: boolean;
  timezone?: string;
  visibility?: 'public' | 'shared' | 'private';
  metadata?: Record<string, unknown>;
}

export interface CreateEventRequest {
  calendar_id: string;
  title: string;
  description?: string;
  event_type?: 'event' | 'birthday' | 'anniversary' | 'holiday' | 'reminder' | 'meeting' | 'trip';
  start_at: string | Date;
  end_at?: string | Date;
  all_day?: boolean;
  timezone?: string;
  location_name?: string;
  location_address?: string;
  location_lat?: number;
  location_lon?: number;
  recurrence_rule?: string;
  recurrence_end_at?: string | Date;
  color?: string;
  url?: string;
  organizer_id?: string;
  status?: 'confirmed' | 'tentative' | 'cancelled';
  visibility?: 'default' | 'public' | 'private' | 'confidential';
  reminder_minutes?: number[];
  attendees?: CreateAttendeeRequest[];
  metadata?: Record<string, unknown>;
}

export interface UpdateEventRequest {
  title?: string;
  description?: string;
  event_type?: 'event' | 'birthday' | 'anniversary' | 'holiday' | 'reminder' | 'meeting' | 'trip';
  start_at?: string | Date;
  end_at?: string | Date;
  all_day?: boolean;
  timezone?: string;
  location_name?: string;
  location_address?: string;
  location_lat?: number;
  location_lon?: number;
  recurrence_rule?: string;
  recurrence_end_at?: string | Date;
  color?: string;
  url?: string;
  status?: 'confirmed' | 'tentative' | 'cancelled';
  visibility?: 'default' | 'public' | 'private' | 'confidential';
  reminder_minutes?: number[];
  metadata?: Record<string, unknown>;
}

export interface CreateAttendeeRequest {
  user_id?: string;
  email?: string;
  name?: string;
  role?: 'organizer' | 'attendee' | 'optional';
}

export interface RSVPRequest {
  status: 'accepted' | 'declined' | 'tentative';
  user_id?: string;
}

export interface CheckInRequest {
  user_id: string;
}

export interface CreateICalFeedRequest {
  calendar_id: string;
  name?: string;
}

export interface ListEventsQuery {
  calendar_id?: string;
  start?: string;
  end?: string;
  type?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

export interface EventOccurrence extends CalendarEventRecord {
  occurrence_start: Date;
  occurrence_end: Date | null;
  is_recurring_instance: boolean;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface CalendarStats {
  calendars: number;
  events: number;
  attendees: number;
  reminders: number;
  icalFeeds: number;
  upcomingEvents: number;
  lastEventAt?: Date | null;
}

// =============================================================================
// iCalendar Types
// =============================================================================

export interface ICalendarOptions {
  prodId?: string;
  calendarName?: string;
  timezone?: string;
}
