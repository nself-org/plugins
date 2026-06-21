package internal

import (
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config holds all environment-based configuration for the plugin.
type Config struct {
	DatabaseURL          string
	Port                 int
	MetadataEnrichmentURL string
	TorrentManagerURL    string
	VPNManagerURL        string
	SubtitleManagerURL   string
	MediaProcessingURL   string
	NTVBackendURL        string
	RedisHost            string
	RedisPort            int
	LogLevel             string
	RSSCheckInterval     int
}

// ---------------------------------------------------------------------------
// Quality Profile
// ---------------------------------------------------------------------------

type QualityProfile struct {
	ID                 string    `json:"id"`
	SourceAccountID    string    `json:"source_account_id"`
	Name               string    `json:"name"`
	Description        *string   `json:"description,omitempty"`
	PreferredQualities []string  `json:"preferred_qualities"`
	MaxSizeGB          *float64  `json:"max_size_gb,omitempty"`
	MinSizeGB          *float64  `json:"min_size_gb,omitempty"`
	PreferredSources   []string  `json:"preferred_sources"`
	ExcludedSources    []string  `json:"excluded_sources"`
	PreferredGroups    []string  `json:"preferred_groups,omitempty"`
	ExcludedGroups     []string  `json:"excluded_groups,omitempty"`
	PreferredLanguages []string  `json:"preferred_languages"`
	RequireSubtitles   bool      `json:"require_subtitles"`
	MinSeeders         int       `json:"min_seeders"`
	WaitForBetter      bool      `json:"wait_for_better_quality"`
	WaitHours          int       `json:"wait_hours"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// QualityProfilePreset is a built-in quality preset.
type QualityProfilePreset struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	MaxResolution     string   `json:"max_resolution"`
	MinResolution     string   `json:"min_resolution"`
	PreferredSources  []string `json:"preferred_sources"`
	MaxSizeMovieGB    float64  `json:"max_size_movie_gb"`
	MaxSizeEpisodeGB  float64  `json:"max_size_episode_gb"`
}

// QualityPresets contains the three built-in quality presets.
var QualityPresets = []QualityProfilePreset{
	{
		Name:             "Minimal",
		Description:      "Small file sizes for limited bandwidth or storage. 720p/480p max.",
		MaxResolution:    "720p",
		MinResolution:    "480p",
		PreferredSources: []string{"WEB-DL", "WEBRip", "HDTV"},
		MaxSizeMovieGB:   2,
		MaxSizeEpisodeGB: 0.5,
	},
	{
		Name:             "Balanced",
		Description:      "Best balance of quality and size. 1080p preferred, WEB-DL and above.",
		MaxResolution:    "1080p",
		MinResolution:    "720p",
		PreferredSources: []string{"WEB-DL", "WEBRip", "BluRay"},
		MaxSizeMovieGB:   8,
		MaxSizeEpisodeGB: 2,
	},
	{
		Name:             "4K Premium",
		Description:      "Maximum quality with 2160p/4K preferred. BluRay and Remux sources.",
		MaxResolution:    "2160p",
		MinResolution:    "1080p",
		PreferredSources: []string{"BluRay", "Remux", "WEB-DL"},
		MaxSizeMovieGB:   40,
		MaxSizeEpisodeGB: 10,
	},
}

// ---------------------------------------------------------------------------
// Subscription
// ---------------------------------------------------------------------------

type Subscription struct {
	ID                     string          `json:"id"`
	SourceAccountID        string          `json:"source_account_id"`
	SubscriptionType       string          `json:"subscription_type"`
	ContentID              *string         `json:"content_id,omitempty"`
	ContentName            string          `json:"content_name"`
	ContentMetadata        json.RawMessage `json:"content_metadata,omitempty"`
	QualityProfileID       *string         `json:"quality_profile_id,omitempty"`
	Enabled                bool            `json:"enabled"`
	AutoUpgrade            bool            `json:"auto_upgrade"`
	MonitorFutureSeasons   bool            `json:"monitor_future_seasons"`
	MonitorExistingSeasons bool            `json:"monitor_existing_seasons"`
	SeasonFolder           bool            `json:"season_folder"`
	LastCheckAt            *time.Time      `json:"last_check_at,omitempty"`
	LastDownloadAt         *time.Time      `json:"last_download_at,omitempty"`
	NextCheckAt            *time.Time      `json:"next_check_at,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// RSS Feed
// ---------------------------------------------------------------------------

type RSSFeed struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	Name                 string     `json:"name"`
	URL                  string     `json:"url"`
	FeedType             string     `json:"feed_type"`
	Enabled              bool       `json:"enabled"`
	CheckIntervalMinutes int        `json:"check_interval_minutes"`
	QualityProfileID     *string    `json:"quality_profile_id,omitempty"`
	LastCheckAt          *time.Time `json:"last_check_at,omitempty"`
	LastSuccessAt        *time.Time `json:"last_success_at,omitempty"`
	LastError            *string    `json:"last_error,omitempty"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Acquisition Queue Item
// ---------------------------------------------------------------------------

type AcquisitionQueueItem struct {
	ID                string          `json:"id"`
	SourceAccountID   string          `json:"source_account_id"`
	ContentType       string          `json:"content_type"`
	ContentName       string          `json:"content_name"`
	Year              *int            `json:"year,omitempty"`
	Season            *int            `json:"season,omitempty"`
	Episode           *int            `json:"episode,omitempty"`
	QualityProfileID  *string         `json:"quality_profile_id,omitempty"`
	RequestedBy       string          `json:"requested_by"`
	RequestSourceID   *string         `json:"request_source_id,omitempty"`
	Status            string          `json:"status"`
	Priority          int             `json:"priority"`
	Attempts          int             `json:"attempts"`
	MaxAttempts       int             `json:"max_attempts"`
	MatchedTorrent    json.RawMessage `json:"matched_torrent,omitempty"`
	DownloadID        *string         `json:"download_id,omitempty"`
	ErrorMessage      *string         `json:"error_message,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Acquisition History
// ---------------------------------------------------------------------------
