package internal

import "time"

// ROMMetadata maps to np_romdisc_metadata.
type ROMMetadata struct {
	ID                    string     `json:"id"`
	SourceAccountID       string     `json:"source_account_id"`
	RomTitle              string     `json:"rom_title"`
	RomTitleNormalized    string     `json:"rom_title_normalized"`
	Platform              string     `json:"platform"`
	Region                *string    `json:"region,omitempty"`
	FileName              string     `json:"file_name"`
	FileSizeBytes         *int64     `json:"file_size_bytes,omitempty"`
	FileHashMD5           *string    `json:"file_hash_md5,omitempty"`
	FileHashSHA256        *string    `json:"file_hash_sha256,omitempty"`
	FileHashCRC32         *string    `json:"file_hash_crc32,omitempty"`
	DownloadURL           *string    `json:"download_url,omitempty"`
	DownloadSource        *string    `json:"download_source,omitempty"`
	DownloadURLVerifiedAt *time.Time `json:"download_url_verified_at,omitempty"`
	DownloadURLDead       bool       `json:"download_url_dead"`
	ReleaseYear           *int       `json:"release_year,omitempty"`
	ReleaseMonth          *int       `json:"release_month,omitempty"`
	ReleaseDay            *int       `json:"release_day,omitempty"`
	Version               *string    `json:"version,omitempty"`
	QualityScore          int        `json:"quality_score"`
	PopularityScore       int        `json:"popularity_score"`
	ReleaseGroup          *string    `json:"release_group,omitempty"`
	IsVerifiedDump        bool       `json:"is_verified_dump"`
	IsHack                bool       `json:"is_hack"`
	IsTranslation         bool       `json:"is_translation"`
	IsHomebrew            bool       `json:"is_homebrew"`
	IsPublicDomain        bool       `json:"is_public_domain"`
	GameTitle             *string    `json:"game_title,omitempty"`
	Genre                 *string    `json:"genre,omitempty"`
	Publisher             *string    `json:"publisher,omitempty"`
	Developer             *string    `json:"developer,omitempty"`
	Description           *string    `json:"description,omitempty"`
	IgdbID                *int       `json:"igdb_id,omitempty"`
	MobygamesID           *int       `json:"mobygames_id,omitempty"`
	BoxArtURL             *string    `json:"box_art_url,omitempty"`
	ScreenshotURLs        []string   `json:"screenshot_urls"`
	IsCommunityRom        bool       `json:"is_community_rom"`
	CommunitySourceURL    *string    `json:"community_source_url,omitempty"`
	CommunityUpdateYear   *int       `json:"community_update_year,omitempty"`
	ScrapedFrom           *string    `json:"scraped_from,omitempty"`
	ScrapedAt             *time.Time `json:"scraped_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// DownloadQueue maps to np_romdisc_download_queue.
type DownloadQueue struct {
	ID                    string     `json:"id"`
	SourceAccountID       string     `json:"source_account_id"`
	UserID                *string    `json:"user_id,omitempty"`
	RomMetadataID         string     `json:"rom_metadata_id"`
	JobID                 *string    `json:"job_id,omitempty"`
	Status                string     `json:"status"`
	DownloadStartedAt     *time.Time `json:"download_started_at,omitempty"`
	DownloadCompletedAt   *time.Time `json:"download_completed_at,omitempty"`
	DownloadProgressPct   int        `json:"download_progress_percent"`
	DownloadedBytes       int64      `json:"downloaded_bytes"`
	TotalBytes            int64      `json:"total_bytes"`
	ObjectStoragePath     *string    `json:"object_storage_path,omitempty"`
	ChecksumVerified      bool       `json:"checksum_verified"`
	ErrorMessage          *string    `json:"error_message,omitempty"`
	RetryCount            int        `json:"retry_count"`
	MaxRetries            int        `json:"max_retries"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// ScraperJob maps to np_romdisc_scraper_jobs.
type ScraperJob struct {
	ID                     string     `json:"id"`
	ScraperName            string     `json:"scraper_name"`
	ScraperType            string     `json:"scraper_type"`
	ScraperSourceURL       string     `json:"scraper_source_url"`
	Enabled                bool       `json:"enabled"`
	LastRunAt              *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus          *string    `json:"last_run_status,omitempty"`
	LastRunDurationSeconds *int       `json:"last_run_duration_seconds,omitempty"`
	RomsFound              int        `json:"roms_found"`
	RomsAdded              int        `json:"roms_added"`
	RomsUpdated            int        `json:"roms_updated"`
	RomsRemoved            int        `json:"roms_removed"`
	Errors                 []string   `json:"errors"`
	CronSchedule           string     `json:"cron_schedule"`
	NextRunAt              *time.Time `json:"next_run_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// Popularity maps to np_romdisc_popularity.
type Popularity struct {
	ID                     string     `json:"id"`
	RomMetadataID          string     `json:"rom_metadata_id"`
	DownloadCount          int        `json:"download_count"`
	SearchCount            int        `json:"search_count"`
	PlayCount              int        `json:"play_count"`
	ArchiveOrgDownloads    int        `json:"archive_org_downloads"`
	ComputedPopularityScore int       `json:"computed_popularity_score"`
	LastScoreUpdateAt      *time.Time `json:"last_score_update_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// AuditLog maps to np_romdisc_audit_log.
type AuditLog struct {
	ID            int64      `json:"id"`
	SourceAccountID string   `json:"source_account_id"`
	UserID        string     `json:"user_id"`
	Action        string     `json:"action"`
	RomMetadataID *string    `json:"rom_metadata_id,omitempty"`
	RomName       *string    `json:"rom_name,omitempty"`
	RomPlatform   *string    `json:"rom_platform,omitempty"`
	RomSource     *string    `json:"rom_source,omitempty"`
	IPAddress     *string    `json:"ip_address,omitempty"`
	UserAgent     *string    `json:"user_agent,omitempty"`
	Details       []byte     `json:"details"`
	CreatedAt     time.Time  `json:"created_at"`
}

// LegalAcceptance maps to np_romdisc_legal_acceptance.
type LegalAcceptance struct {
	ID                int64     `json:"id"`
	SourceAccountID   string    `json:"source_account_id"`
	UserID            string    `json:"user_id"`
	DisclaimerVersion string    `json:"disclaimer_version"`
	AcceptedAt        time.Time `json:"accepted_at"`
	IPAddress         *string   `json:"ip_address,omitempty"`
	UserAgent         *string   `json:"user_agent,omitempty"`
}
