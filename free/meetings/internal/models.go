package internal

import "time"

type Calendar struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	Name            string         `json:"name"`
	Description     *string        `json:"description,omitempty"`
	Color           *string        `json:"color,omitempty"`
	OwnerID         string         `json:"owner_id"`
	IsDefault       bool           `json:"is_default"`
	IsPublic        bool           `json:"is_public"`
	Metadata        map[string]any `json:"metadata"`
}

type Room struct {
	ID                   string         `json:"id"`
	SourceAccountID      string         `json:"source_account_id"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	Name                 string         `json:"name"`
	Description          *string        `json:"description,omitempty"`
	Location             *string        `json:"location,omitempty"`
	Floor                *string        `json:"floor,omitempty"`
	Building             *string        `json:"building,omitempty"`
	Capacity             int            `json:"capacity"`
	HasVideoConference   bool           `json:"has_video_conference"`
	HasProjector         bool           `json:"has_projector"`
	HasWhiteboard        bool           `json:"has_whiteboard"`
	HasPhone             bool           `json:"has_phone"`
	Amenities            []any          `json:"amenities"`
	IsActive             bool           `json:"is_active"`
	BookingBufferMinutes int            `json:"booking_buffer_minutes"`
	IsPublic             bool           `json:"is_public"`
	AllowedGroups        []any          `json:"allowed_groups"`
	Metadata             map[string]any `json:"metadata"`
}

type Event struct {
	ID                  string         `json:"id"`
	SourceAccountID     string         `json:"source_account_id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	Title               string         `json:"title"`
	Description         *string        `json:"description,omitempty"`
	Location            *string        `json:"location,omitempty"`
	VideoLink           *string        `json:"video_link,omitempty"`
	StartTime           time.Time      `json:"start_time"`
	EndTime             time.Time      `json:"end_time"`
	AllDay              bool           `json:"all_day"`
	Timezone            string         `json:"timezone"`
	IsRecurring         bool           `json:"is_recurring"`
	RecurrenceRule      map[string]any `json:"recurrence_rule,omitempty"`
	RecurrenceParentID  *string        `json:"recurrence_parent_id,omitempty"`
	OrganizerID         string         `json:"organizer_id"`
	CalendarID          *string        `json:"calendar_id,omitempty"`
	RoomID              *string        `json:"room_id,omitempty"`
	Status              string         `json:"status"`
	Visibility          string         `json:"visibility"`
	GoogleEventID       *string        `json:"google_event_id,omitempty"`
	OutlookEventID      *string        `json:"outlook_event_id,omitempty"`
	IcalUID             *string        `json:"ical_uid,omitempty"`
	Metadata            map[string]any `json:"metadata"`
}

type Attendee struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	EventID         string         `json:"event_id"`
	UserID          *string        `json:"user_id,omitempty"`
	ExternalEmail   *string        `json:"external_email,omitempty"`
	ExternalName    *string        `json:"external_name,omitempty"`
	Status          string         `json:"status"`
	ResponseTime    *time.Time     `json:"response_time,omitempty"`
	IsOptional      bool           `json:"is_optional"`
	CanModify       bool           `json:"can_modify"`
	Metadata        map[string]any `json:"metadata"`
}

type CalendarShare struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	CreatedAt       time.Time `json:"created_at"`
	CalendarID      string    `json:"calendar_id"`
	UserID          string    `json:"user_id"`
	Permission      string    `json:"permission"`
}

type ExternalCalendar struct {
	ID                 string         `json:"id"`
	SourceAccountID    string         `json:"source_account_id"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	UserID             string         `json:"user_id"`
	Provider           string         `json:"provider"`
	AccessToken        *string        `json:"access_token,omitempty"`
	RefreshToken       *string        `json:"refresh_token,omitempty"`
	TokenExpiresAt     *time.Time     `json:"token_expires_at,omitempty"`
	ProviderCalendarID *string        `json:"provider_calendar_id,omitempty"`
	ProviderUserID     *string        `json:"provider_user_id,omitempty"`
	SyncEnabled        bool           `json:"sync_enabled"`
	LastSyncAt         *time.Time     `json:"last_sync_at,omitempty"`
	SyncDirection      string         `json:"sync_direction"`
	Metadata           map[string]any `json:"metadata"`
}

type Template struct {
	ID                     string         `json:"id"`
	SourceAccountID        string         `json:"source_account_id"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	Name                   string         `json:"name"`
	Description            *string        `json:"description,omitempty"`
	DefaultTitle           *string        `json:"default_title,omitempty"`
	DefaultDescription     *string        `json:"default_description,omitempty"`
	DefaultDurationMinutes int            `json:"default_duration_minutes"`
	DefaultLocation        *string        `json:"default_location,omitempty"`
	DefaultVideoLink       *string        `json:"default_video_link,omitempty"`
	DefaultAttendees       []any          `json:"default_attendees"`
	CreatedBy              string         `json:"created_by"`
	IsPublic               bool           `json:"is_public"`
	Metadata               map[string]any `json:"metadata"`
}

type Reminder struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	CreatedAt       time.Time  `json:"created_at"`
	EventID         string     `json:"event_id"`
	UserID          string     `json:"user_id"`
	RemindAt        time.Time  `json:"remind_at"`
	MinutesBefore   int        `json:"minutes_before"`
	Method          string     `json:"method"`
	Sent            bool       `json:"sent"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
}

type Waitlist struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	CreatedAt       time.Time  `json:"created_at"`
	EventID         string     `json:"event_id"`
	RoomID          string     `json:"room_id"`
	UserID          string     `json:"user_id"`
	Status          string     `json:"status"`
	NotifiedAt      *time.Time `json:"notified_at,omitempty"`
}
