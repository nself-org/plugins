/**
 * Meetings Plugin Type Definitions
 */

// =============================================================================
// Enums / Union Types
// =============================================================================

export type EventStatus = 'confirmed' | 'tentative' | 'cancelled';
export type EventVisibility = 'public' | 'private' | 'confidential';
export type AttendeeStatus = 'pending' | 'accepted' | 'declined' | 'tentative';
export type CalendarPermission = 'read' | 'write' | 'admin';
export type CalendarProvider = 'google' | 'outlook' | 'ical';
export type SyncDirection = 'import' | 'export' | 'bidirectional';
export type ReminderMethod = 'push' | 'email' | 'both';
export type WaitlistStatus = 'waiting' | 'confirmed' | 'cancelled';

// =============================================================================
// Event Types
// =============================================================================

export interface MeetingEvent {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  title: string;
  description: string | null;
  location: string | null;
  video_link: string | null;

  start_time: Date;
  end_time: Date;
  all_day: boolean;
  timezone: string;

  is_recurring: boolean;
  recurrence_rule: Record<string, unknown> | null;
  recurrence_parent_id: string | null;

  organizer_id: string;
  calendar_id: string | null;
  room_id: string | null;

  status: EventStatus;
  visibility: EventVisibility;

  google_event_id: string | null;
  outlook_event_id: string | null;
  ical_uid: string | null;

  metadata: Record<string, unknown>;
}

export interface CreateEventInput {
  title: string;
  description?: string;
  location?: string;
  video_link?: string;
  start_time: string;
  end_time: string;
  all_day?: boolean;
  timezone?: string;
  is_recurring?: boolean;
  recurrence_rule?: Record<string, unknown>;
  recurrence_parent_id?: string;
  organizer_id: string;
  calendar_id?: string;
  room_id?: string;
  status?: EventStatus;
  visibility?: EventVisibility;
  attendees?: CreateAttendeeInput[];
  metadata?: Record<string, unknown>;
}

export interface UpdateEventInput {
  title?: string;
  description?: string;
  location?: string;
  video_link?: string;
  start_time?: string;
  end_time?: string;
  all_day?: boolean;
  timezone?: string;
  recurrence_rule?: Record<string, unknown>;
  calendar_id?: string;
  room_id?: string;
  status?: EventStatus;
  visibility?: EventVisibility;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Attendee Types
// =============================================================================

export interface Attendee {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  event_id: string;
  user_id: string | null;
  external_email: string | null;
  external_name: string | null;

  status: AttendeeStatus;
  response_time: Date | null;

  is_optional: boolean;
  can_modify: boolean;

  metadata: Record<string, unknown>;
}

export interface CreateAttendeeInput {
  user_id?: string;
  external_email?: string;
  external_name?: string;
  is_optional?: boolean;
  can_modify?: boolean;
}

export interface RsvpInput {
  status: AttendeeStatus;
  user_id?: string;
  external_email?: string;
}

// =============================================================================
// Room Types
// =============================================================================

export interface Room {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  name: string;
  description: string | null;
  location: string | null;
  floor: string | null;
  building: string | null;

  capacity: number;

  has_video_conference: boolean;
  has_projector: boolean;
  has_whiteboard: boolean;
  has_phone: boolean;
  amenities: string[];

  is_active: boolean;
  booking_buffer_minutes: number;

  is_public: boolean;
  allowed_groups: string[];

  metadata: Record<string, unknown>;
}

export interface CreateRoomInput {
  name: string;
  description?: string;
  location?: string;
  floor?: string;
  building?: string;
  capacity: number;
  has_video_conference?: boolean;
  has_projector?: boolean;
  has_whiteboard?: boolean;
  has_phone?: boolean;
  amenities?: string[];
  booking_buffer_minutes?: number;
  is_public?: boolean;
  allowed_groups?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateRoomInput {
  name?: string;
  description?: string;
  location?: string;
  floor?: string;
  building?: string;
  capacity?: number;
  has_video_conference?: boolean;
  has_projector?: boolean;
  has_whiteboard?: boolean;
  has_phone?: boolean;
  amenities?: string[];
  is_active?: boolean;
  booking_buffer_minutes?: number;
  is_public?: boolean;
  allowed_groups?: string[];
  metadata?: Record<string, unknown>;
}

export interface RoomSuggestion {
  room: Room;
  score: number;
  reason: string;
}

// =============================================================================
// Calendar Types
// =============================================================================

export interface Calendar {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  name: string;
  description: string | null;
  color: string | null;

  owner_id: string;
  is_default: boolean;

  is_public: boolean;

  metadata: Record<string, unknown>;
}

export interface CreateCalendarInput {
  name: string;
  description?: string;
  color?: string;
  owner_id: string;
  is_default?: boolean;
  is_public?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateCalendarInput {
  name?: string;
  description?: string;
  color?: string;
  is_default?: boolean;
  is_public?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Calendar Share Types
// =============================================================================

export interface CalendarShare {
  id: string;
  source_account_id: string;
  created_at: Date;

  calendar_id: string;
  user_id: string;

  permission: CalendarPermission;
}

// =============================================================================
// External Calendar Types
// =============================================================================

export interface ExternalCalendar {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  user_id: string;
  provider: CalendarProvider;

  access_token: string | null;
  refresh_token: string | null;
  token_expires_at: Date | null;

  provider_calendar_id: string | null;
  provider_user_id: string | null;

  sync_enabled: boolean;
  last_sync_at: Date | null;
  sync_direction: SyncDirection;

  metadata: Record<string, unknown>;
}

export interface ConnectCalendarInput {
  user_id: string;
  auth_code: string;
  provider: CalendarProvider;
}

// =============================================================================
// Template Types
// =============================================================================

export interface MeetingTemplate {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  name: string;
  description: string | null;

  default_title: string | null;
  default_description: string | null;
  default_duration_minutes: number;
  default_location: string | null;
  default_video_link: string | null;

  default_attendees: string[];

  created_by: string;
  is_public: boolean;

  metadata: Record<string, unknown>;
}

export interface CreateTemplateInput {
  name: string;
  description?: string;
  default_title?: string;
  default_description?: string;
  default_duration_minutes: number;
  default_location?: string;
  default_video_link?: string;
  default_attendees?: string[];
  created_by: string;
  is_public?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateTemplateInput {
  name?: string;
  description?: string;
  default_title?: string;
  default_description?: string;
  default_duration_minutes?: number;
  default_location?: string;
  default_video_link?: string;
  default_attendees?: string[];
  is_public?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Reminder Types
// =============================================================================

export interface Reminder {
  id: string;
  source_account_id: string;
  created_at: Date;

  event_id: string;
  user_id: string;

  remind_at: Date;
  minutes_before: number;

  method: ReminderMethod;
  sent: boolean;
  sent_at: Date | null;
}

// =============================================================================
// Waitlist Types
// =============================================================================

export interface WaitlistEntry {
  id: string;
  source_account_id: string;
  created_at: Date;

  event_id: string;
  room_id: string;
  user_id: string;

  status: WaitlistStatus;
  notified_at: Date | null;
}

// =============================================================================
// Availability Types
// =============================================================================

export interface AvailabilityResult {
  is_available: boolean;
  conflicting_events: string[];
}

export interface MeetingTimeSuggestion {
  suggested_start: Date;
  suggested_end: Date;
  all_available: boolean;
}

export interface CheckAvailabilityInput {
  user_ids: string[];
  start_time: string;
  end_time: string;
}

export interface SuggestTimesInput {
  attendee_ids: string[];
  duration_minutes: number;
  date: string;
  business_hours_start?: string;
  business_hours_end?: string;
}

// =============================================================================
// Configuration Types
// =============================================================================

export interface MeetingsConfig {
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };
  server: {
    port: number;
    host: string;
  };
  calendar: {
    default_timezone: string;
    business_hours_start: string;
    business_hours_end: string;
    slot_duration_minutes: number;
    suggestion_window_days: number;
  };
  rooms: {
    default_buffer_minutes: number;
    max_advance_booking_days: number;
    auto_release_minutes: number;
  };
  sync: {
    sync_interval_minutes: number;
    google: {
      client_id: string;
      client_secret: string;
      redirect_uri: string;
    };
    outlook: {
      client_id: string;
      client_secret: string;
      redirect_uri: string;
    };
  };
  reminders: {
    default_reminder_minutes: number[];
    max_reminders_per_event: number;
  };
}

// =============================================================================
// API Response Types
// =============================================================================

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface ListEventsQuery {
  start_time?: string;
  end_time?: string;
  calendar_id?: string;
  room_id?: string;
  organizer_id?: string;
  status?: EventStatus;
  limit?: string;
  offset?: string;
}

export interface ListRoomsQuery {
  is_active?: string;
  min_capacity?: string;
  has_video_conference?: string;
  limit?: string;
  offset?: string;
}
