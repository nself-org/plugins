package internal

import (
	"encoding/json"
	"time"
)

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
