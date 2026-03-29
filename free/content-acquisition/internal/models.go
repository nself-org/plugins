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

type AcquisitionHistoryItem struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ContentType     string     `json:"content_type"`
	ContentName     string     `json:"content_name"`
	Year            *int       `json:"year,omitempty"`
	Season          *int       `json:"season,omitempty"`
	Episode         *int       `json:"episode,omitempty"`
	TorrentTitle    *string    `json:"torrent_title,omitempty"`
	TorrentSource   *string    `json:"torrent_source,omitempty"`
	Quality         *string    `json:"quality,omitempty"`
	SizeBytes       *int64     `json:"size_bytes,omitempty"`
	DownloadID      *string    `json:"download_id,omitempty"`
	Status          string     `json:"status"`
	AcquiredFrom    string     `json:"acquired_from"`
	UpgradeOf       *string    `json:"upgrade_of,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Pipeline Run
// ---------------------------------------------------------------------------

type PipelineRun struct {
	ID                  int             `json:"id"`
	SourceAccountID     string          `json:"source_account_id"`
	TriggerType         string          `json:"trigger_type"`
	TriggerSource       *string         `json:"trigger_source,omitempty"`
	ContentTitle        string          `json:"content_title"`
	ContentType         *string         `json:"content_type,omitempty"`
	Status              string          `json:"status"`
	VPNCheckStatus      string          `json:"vpn_check_status"`
	TorrentStatus       string          `json:"torrent_status"`
	TorrentDownloadID   *string         `json:"torrent_download_id,omitempty"`
	MetadataStatus      string          `json:"metadata_status"`
	SubtitleStatus      string          `json:"subtitle_status"`
	EncodingStatus      string          `json:"encoding_status"`
	EncodingJobID       *string         `json:"encoding_job_id,omitempty"`
	PublishingStatus    string          `json:"publishing_status"`
	DetectedAt          time.Time       `json:"detected_at"`
	VPNCheckedAt        *time.Time      `json:"vpn_checked_at,omitempty"`
	TorrentSubmittedAt  *time.Time      `json:"torrent_submitted_at,omitempty"`
	DownloadCompletedAt *time.Time      `json:"download_completed_at,omitempty"`
	MetadataEnrichedAt  *time.Time      `json:"metadata_enriched_at,omitempty"`
	SubtitlesFetchedAt  *time.Time      `json:"subtitles_fetched_at,omitempty"`
	EncodingCompletedAt *time.Time      `json:"encoding_completed_at,omitempty"`
	PublishedAt         *time.Time      `json:"published_at,omitempty"`
	PipelineCompletedAt *time.Time      `json:"pipeline_completed_at,omitempty"`
	ErrorMessage        *string         `json:"error_message,omitempty"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Movie Monitoring
// ---------------------------------------------------------------------------

type MovieMonitoring struct {
	ID                 string     `json:"id"`
	SourceAccountID    string     `json:"source_account_id"`
	UserID             string     `json:"user_id"`
	MovieTitle         string     `json:"movie_title"`
	TmdbID             *int       `json:"tmdb_id,omitempty"`
	ReleaseDate        *time.Time `json:"release_date,omitempty"`
	DigitalReleaseDate *time.Time `json:"digital_release_date,omitempty"`
	QualityProfile     string     `json:"quality_profile"`
	AutoDownload       bool       `json:"auto_download"`
	AutoUpgrade        bool       `json:"auto_upgrade"`
	Status             string     `json:"status"`
	DownloadedQuality  *string    `json:"downloaded_quality,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Download (state-machine driven)
// ---------------------------------------------------------------------------

type Download struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	UserID          string     `json:"user_id"`
	ContentType     string     `json:"content_type"`
	Title           string     `json:"title"`
	State           string     `json:"state"`
	Progress        float32    `json:"progress"`
	MagnetURI       *string    `json:"magnet_uri,omitempty"`
	TorrentID       *string    `json:"torrent_id,omitempty"`
	EncodingJobID   *string    `json:"encoding_job_id,omitempty"`
	QualityProfile  string     `json:"quality_profile"`
	RetryCount      int        `json:"retry_count"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	ShowID          *string    `json:"show_id,omitempty"`
	SeasonNumber    *int       `json:"season_number,omitempty"`
	EpisodeNumber   *int       `json:"episode_number,omitempty"`
	TmdbID          *int       `json:"tmdb_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// DownloadStateTransition records a state change in the download lifecycle.
type DownloadStateTransition struct {
	ID         string          `json:"id"`
	DownloadID string          `json:"download_id"`
	FromState  *string         `json:"from_state"`
	ToState    string          `json:"to_state"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Download Rule
// ---------------------------------------------------------------------------

type DownloadRule struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	UserID          string          `json:"user_id"`
	Name            string          `json:"name"`
	Conditions      json.RawMessage `json:"conditions"`
	Action          string          `json:"action"`
	Priority        int             `json:"priority"`
	Enabled         bool            `json:"enabled"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Dashboard Summary
// ---------------------------------------------------------------------------

type DashboardSummary struct {
	ActiveDownloads     int `json:"active_downloads"`
	CompletedToday      int `json:"completed_today"`
	FailedToday         int `json:"failed_today"`
	ActiveSubscriptions int `json:"active_subscriptions"`
	MonitoredMovies     int `json:"monitored_movies"`
	EnabledFeeds        int `json:"enabled_feeds"`
	EnabledRules        int `json:"enabled_rules"`
	QueueDepth          int `json:"queue_depth"`
}

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

type CreateSubscriptionRequest struct {
	ContentType      string  `json:"contentType"`
	ContentID        *string `json:"contentId,omitempty"`
	ContentName      string  `json:"contentName"`
	QualityProfileID *string `json:"qualityProfileId,omitempty"`
}

type UpdateSubscriptionRequest struct {
	ContentType      *string `json:"contentType,omitempty"`
	ContentID        *string `json:"contentId,omitempty"`
	ContentName      *string `json:"contentName,omitempty"`
	QualityProfileID *string `json:"qualityProfileId,omitempty"`
	Enabled          *bool   `json:"enabled,omitempty"`
	AutoUpgrade      *bool   `json:"autoUpgrade,omitempty"`
}

type CreateFeedRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	FeedType string `json:"feedType"`
}

type UpdateFeedRequest struct {
	Name                 *string `json:"name,omitempty"`
	URL                  *string `json:"url,omitempty"`
	FeedType             *string `json:"feedType,omitempty"`
	Enabled              *bool   `json:"enabled,omitempty"`
	CheckIntervalMinutes *int    `json:"checkIntervalMinutes,omitempty"`
}

type ValidateFeedRequest struct {
	URL string `json:"url"`
}

type AddToQueueRequest struct {
	ContentType string `json:"contentType"`
	ContentName string `json:"contentName"`
	Year        *int   `json:"year,omitempty"`
	Season      *int   `json:"season,omitempty"`
	Episode     *int   `json:"episode,omitempty"`
}

type CreateProfileRequest struct {
	Name               string   `json:"name"`
	PreferredQualities []string `json:"preferredQualities,omitempty"`
	MinSeeders         *int     `json:"minSeeders,omitempty"`
}

type CreateMovieRequest struct {
	Title          string  `json:"title"`
	TmdbID         *int    `json:"tmdbId,omitempty"`
	QualityProfile *string `json:"qualityProfile,omitempty"`
	AutoDownload   *bool   `json:"autoDownload,omitempty"`
	AutoUpgrade    *bool   `json:"autoUpgrade,omitempty"`
}

type UpdateMovieRequest struct {
	Title          *string `json:"title,omitempty"`
	TmdbID         *int    `json:"tmdbId,omitempty"`
	QualityProfile *string `json:"qualityProfile,omitempty"`
	AutoDownload   *bool   `json:"autoDownload,omitempty"`
	AutoUpgrade    *bool   `json:"autoUpgrade,omitempty"`
	Status         *string `json:"status,omitempty"`
}

type CreateDownloadRequest struct {
	ContentType    string  `json:"contentType"`
	Title          string  `json:"title"`
	MagnetURI      *string `json:"magnetUri,omitempty"`
	QualityProfile *string `json:"qualityProfile,omitempty"`
	ShowID         *string `json:"showId,omitempty"`
	SeasonNumber   *int    `json:"seasonNumber,omitempty"`
	EpisodeNumber  *int    `json:"episodeNumber,omitempty"`
	TmdbID         *int    `json:"tmdbId,omitempty"`
}

type CreateRuleRequest struct {
	Name       string          `json:"name"`
	Conditions json.RawMessage `json:"conditions"`
	Action     string          `json:"action"`
	Priority   *int            `json:"priority,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
}

type UpdateRuleRequest struct {
	Name       *string          `json:"name,omitempty"`
	Conditions *json.RawMessage `json:"conditions,omitempty"`
	Action     *string          `json:"action,omitempty"`
	Priority   *int             `json:"priority,omitempty"`
	Enabled    *bool            `json:"enabled,omitempty"`
}

type TestRuleRequest struct {
	Sample map[string]interface{} `json:"sample"`
}

type PipelineTriggerRequest struct {
	ContentTitle string  `json:"content_title"`
	ContentType  *string `json:"content_type,omitempty"`
	MagnetURL    *string `json:"magnet_url,omitempty"`
	TorrentURL   *string `json:"torrent_url,omitempty"`
}

type RSSPollRequest struct {
	URL      string                   `json:"url"`
	Criteria []map[string]interface{} `json:"criteria"`
	LastSeen *string                  `json:"lastSeen,omitempty"`
}

type RSSTestRequest struct {
	URL string `json:"url"`
}
