package internal

import "time"

type League struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ExternalID      string     `json:"external_id"`
	Provider        string     `json:"provider"`
	Name            string     `json:"name"`
	Abbreviation    *string    `json:"abbreviation,omitempty"`
	Sport           string     `json:"sport"`
	Country         *string    `json:"country,omitempty"`
	SeasonType      *string    `json:"season_type,omitempty"`
	CurrentSeason   *string    `json:"current_season,omitempty"`
	LogoURL         *string    `json:"logo_url,omitempty"`
	Metadata        *string    `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	SyncedAt        time.Time  `json:"synced_at"`
}

type Team struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	LeagueID        *string    `json:"league_id,omitempty"`
	ExternalID      string     `json:"external_id"`
	Provider        string     `json:"provider"`
	Name            string     `json:"name"`
	Abbreviation    *string    `json:"abbreviation,omitempty"`
	City            *string    `json:"city,omitempty"`
	Conference      *string    `json:"conference,omitempty"`
	Division        *string    `json:"division,omitempty"`
	LogoURL         *string    `json:"logo_url,omitempty"`
	PrimaryColor    *string    `json:"primary_color,omitempty"`
	SecondaryColor  *string    `json:"secondary_color,omitempty"`
	VenueName       *string    `json:"venue_name,omitempty"`
	VenueCity       *string    `json:"venue_city,omitempty"`
	VenueTimezone   *string    `json:"venue_timezone,omitempty"`
	Metadata        *string    `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	SyncedAt        time.Time  `json:"synced_at"`
}

type Event struct {
	ID                       string     `json:"id"`
	SourceAccountID          string     `json:"source_account_id"`
	ExternalID               string     `json:"external_id"`
	Provider                 string     `json:"provider"`
	CanonicalID              *string    `json:"canonical_id,omitempty"`
	LeagueID                 *string    `json:"league_id,omitempty"`
	HomeTeamID               *string    `json:"home_team_id,omitempty"`
	AwayTeamID               *string    `json:"away_team_id,omitempty"`
	EventType                string     `json:"event_type"`
	Status                   string     `json:"status"`
	ScheduledAt              time.Time  `json:"scheduled_at"`
	StartedAt                *time.Time `json:"started_at,omitempty"`
	EndedAt                  *time.Time `json:"ended_at,omitempty"`
	VenueName                *string    `json:"venue_name,omitempty"`
	VenueCity                *string    `json:"venue_city,omitempty"`
	VenueTimezone            *string    `json:"venue_timezone,omitempty"`
	BroadcastNetwork         *string    `json:"broadcast_network,omitempty"`
	BroadcastChannel         *string    `json:"broadcast_channel,omitempty"`
	Season                   *string    `json:"season,omitempty"`
	SeasonType               *string    `json:"season_type,omitempty"`
	Week                     *int       `json:"week,omitempty"`
	HomeScore                *int       `json:"home_score,omitempty"`
	AwayScore                *int       `json:"away_score,omitempty"`
	Period                   *string    `json:"period,omitempty"`
	Clock                    *string    `json:"clock,omitempty"`
	IsFinal                  bool       `json:"is_final"`
	IsLocked                 bool       `json:"is_locked"`
	LockReason               *string    `json:"lock_reason,omitempty"`
	LockedAt                 *time.Time `json:"locked_at,omitempty"`
	OperatorOverride         bool       `json:"operator_override"`
	OperatorNotes            *string    `json:"operator_notes,omitempty"`
	RecordingTriggerSent     bool       `json:"recording_trigger_sent"`
	RecordingTriggerSentAt   *time.Time `json:"recording_trigger_sent_at,omitempty"`
	Metadata                 *string    `json:"metadata,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	SyncedAt                 time.Time  `json:"synced_at"`
	DeletedAt                *time.Time `json:"deleted_at,omitempty"`
}

type ProviderSync struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Provider        string     `json:"provider"`
	ResourceType    string     `json:"resource_type"`
	Status          string     `json:"status"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	RecordsSynced   int        `json:"records_synced"`
	Errors          *string    `json:"errors,omitempty"`
	Metadata        *string    `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ScheduleCache struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Provider        string    `json:"provider"`
	CacheKey        string    `json:"cache_key"`
	Data            string    `json:"data"`
	FetchedAt       time.Time `json:"fetched_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type WebhookEvent struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Provider        string     `json:"provider"`
	EventType       string     `json:"event_type"`
	EventID         *string    `json:"event_id,omitempty"`
	Payload         string     `json:"payload"`
	Processed       bool       `json:"processed"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
	Error           *string    `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Standing struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	LeagueID        string    `json:"league_id"`
	TeamID          string    `json:"team_id"`
	Wins            int       `json:"wins"`
	Losses          int       `json:"losses"`
	Draws           int       `json:"draws"`
	Points          int       `json:"points"`
	Rank            int       `json:"rank"`
	Season          string    `json:"season"`
	SyncedAt        time.Time `json:"synced_at"`
}

type Favorite struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	UserID          string    `json:"user_id"`
	TeamID          string    `json:"team_id"`
	NotifyLive      bool      `json:"notify_live"`
	AutoRecord      bool      `json:"auto_record"`
	CreatedAt       time.Time `json:"created_at"`
}
