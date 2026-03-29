package internal

import (
	"encoding/json"
	"time"
)

// ContentType represents a valid content type.
// Valid values: movie, episode, video, audio, article, course.
type ContentType string

const (
	ContentTypeMovie   ContentType = "movie"
	ContentTypeEpisode ContentType = "episode"
	ContentTypeVideo   ContentType = "video"
	ContentTypeAudio   ContentType = "audio"
	ContentTypeArticle ContentType = "article"
	ContentTypeCourse  ContentType = "course"
)

// ValidContentTypes contains all valid content type values.
var ValidContentTypes = map[ContentType]bool{
	ContentTypeMovie:   true,
	ContentTypeEpisode: true,
	ContentTypeVideo:   true,
	ContentTypeAudio:   true,
	ContentTypeArticle: true,
	ContentTypeCourse:  true,
}

// ProgressAction represents a valid progress action.
type ProgressAction string

const (
	ActionPlay     ProgressAction = "play"
	ActionPause    ProgressAction = "pause"
	ActionSeek     ProgressAction = "seek"
	ActionComplete ProgressAction = "complete"
	ActionResume   ProgressAction = "resume"
)

// ProgressPosition represents a row in np_progress_positions.
type ProgressPosition struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	UserID          string          `json:"user_id"`
	ContentType     string          `json:"content_type"`
	ContentID       string          `json:"content_id"`
	PositionSeconds float64         `json:"position_seconds"`
	DurationSeconds *float64        `json:"duration_seconds"`
	ProgressPercent float64         `json:"progress_percent"`
	Completed       bool            `json:"completed"`
	CompletedAt     *time.Time      `json:"completed_at"`
	DeviceID        *string         `json:"device_id"`
	AudioTrack      *string         `json:"audio_track"`
	SubtitleTrack   *string         `json:"subtitle_track"`
	Quality         *string         `json:"quality"`
	Metadata        json.RawMessage `json:"metadata"`
	UpdatedAt       time.Time       `json:"updated_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

// ProgressHistory represents a row in np_progress_history.
type ProgressHistory struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	UserID          string     `json:"user_id"`
	ContentType     string     `json:"content_type"`
	ContentID       string     `json:"content_id"`
	Action          string     `json:"action"`
	PositionSeconds *float64   `json:"position_seconds"`
	DeviceID        *string    `json:"device_id"`
	SessionID       *string    `json:"session_id"`
	CreatedAt       time.Time  `json:"created_at"`
}

// WatchlistItem represents a row in np_progress_watchlists.
type WatchlistItem struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	UserID          string    `json:"user_id"`
	ContentType     string    `json:"content_type"`
	ContentID       string    `json:"content_id"`
	Priority        int       `json:"priority"`
	AddedFrom       *string   `json:"added_from"`
	Notes           *string   `json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
}

// FavoriteItem represents a row in np_progress_favorites.
type FavoriteItem struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	UserID          string    `json:"user_id"`
	ContentType     string    `json:"content_type"`
	ContentID       string    `json:"content_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// WebhookEvent represents a row in np_progress_webhook_events.
type WebhookEvent struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	EventType       *string         `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	Processed       bool            `json:"processed"`
	ProcessedAt     *time.Time      `json:"processed_at"`
	Error           *string         `json:"error"`
	CreatedAt       time.Time       `json:"created_at"`
}

// ContinueWatchingItem is a projection of np_progress_positions for in-progress content.
type ContinueWatchingItem struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	UserID          string          `json:"user_id"`
	ContentType     string          `json:"content_type"`
	ContentID       string          `json:"content_id"`
	PositionSeconds float64         `json:"position_seconds"`
	DurationSeconds *float64        `json:"duration_seconds"`
	ProgressPercent float64         `json:"progress_percent"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Metadata        json.RawMessage `json:"metadata"`
}

// RecentlyWatchedItem is a projection of np_progress_positions for recently accessed content.
type RecentlyWatchedItem struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	UserID          string          `json:"user_id"`
	ContentType     string          `json:"content_type"`
	ContentID       string          `json:"content_id"`
	PositionSeconds float64         `json:"position_seconds"`
	DurationSeconds *float64        `json:"duration_seconds"`
	ProgressPercent float64         `json:"progress_percent"`
	Completed       bool            `json:"completed"`
	CompletedAt     *time.Time      `json:"completed_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Metadata        json.RawMessage `json:"metadata"`
}

// UserStats contains aggregated per-user statistics.
type UserStats struct {
	TotalWatchTimeSeconds float64    `json:"total_watch_time_seconds"`
	TotalWatchTimeHours   float64    `json:"total_watch_time_hours"`
	ContentCompleted      int64      `json:"content_completed"`
	ContentInProgress     int64      `json:"content_in_progress"`
	WatchlistCount        int64      `json:"watchlist_count"`
	FavoritesCount        int64      `json:"favorites_count"`
	MostWatchedType       *string    `json:"most_watched_type"`
	RecentActivity        *time.Time `json:"recent_activity"`
}

// PluginStats contains aggregated plugin-wide statistics.
type PluginStats struct {
	TotalUsers        int64      `json:"total_users"`
	TotalPositions    int64      `json:"total_positions"`
	TotalCompleted    int64      `json:"total_completed"`
	TotalInProgress   int64      `json:"total_in_progress"`
	TotalWatchlist    int64      `json:"total_watchlist"`
	TotalFavorites    int64      `json:"total_favorites"`
	TotalHistoryEvts  int64      `json:"total_history_events"`
	LastActivity      *time.Time `json:"last_activity"`
}

// --- Request types ---

// UpdateProgressRequest is the JSON body for updating playback progress.
type UpdateProgressRequest struct {
	UserID          string                  `json:"user_id"`
	ContentType     string                  `json:"content_type"`
	ContentID       string                  `json:"content_id"`
	PositionSeconds float64                 `json:"position_seconds"`
	DurationSeconds *float64                `json:"duration_seconds,omitempty"`
	DeviceID        *string                 `json:"device_id,omitempty"`
	AudioTrack      *string                 `json:"audio_track,omitempty"`
	SubtitleTrack   *string                 `json:"subtitle_track,omitempty"`
	Quality         *string                 `json:"quality,omitempty"`
	Metadata        map[string]interface{}  `json:"metadata,omitempty"`
}

// AddToWatchlistRequest is the JSON body for adding an item to the watchlist.
type AddToWatchlistRequest struct {
	UserID      string `json:"user_id"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id"`
	Priority    *int   `json:"priority,omitempty"`
	AddedFrom   *string `json:"added_from,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// UpdateWatchlistRequest is the JSON body for updating a watchlist item.
type UpdateWatchlistRequest struct {
	Priority *int    `json:"priority,omitempty"`
	Notes    *string `json:"notes,omitempty"`
}

// AddToFavoritesRequest is the JSON body for adding an item to favorites.
type AddToFavoritesRequest struct {
	UserID      string `json:"user_id"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id"`
}
