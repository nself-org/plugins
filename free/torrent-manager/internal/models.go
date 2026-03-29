package internal

import (
	"encoding/json"
	"time"
)

// --- Configuration ---

// Config holds all torrent-manager configuration loaded from environment variables.
type Config struct {
	DatabaseURL          string
	Port                 int
	VPNManagerURL        string
	VPNRequired          bool
	DefaultClient        string
	TransmissionHost     string
	TransmissionPort     int
	TransmissionUsername string
	TransmissionPassword string
	QBittorrentHost      string
	QBittorrentPort      int
	QBittorrentUsername  string
	QBittorrentPassword  string
	DownloadPath         string
	EnabledSources       string
	SearchTimeoutMS      int
	SearchCacheTTLSec    int
	SeedingRatioLimit    float64
	SeedingTimeLimitHrs  int
	MaxActiveDownloads   int
}

// --- Torrent Clients ---

// TorrentClient represents a configured torrent client row.
type TorrentClient struct {
	ID                string     `json:"id"`
	SourceAccountID   string     `json:"source_account_id"`
	ClientType        string     `json:"client_type"`
	Host              string     `json:"host"`
	Port              int        `json:"port"`
	Username          *string    `json:"username"`
	PasswordEncrypted *string    `json:"password_encrypted,omitempty"`
	IsDefault         bool       `json:"is_default"`
	Status            string     `json:"status"`
	LastConnectedAt   *time.Time `json:"last_connected_at"`
	LastError         *string    `json:"last_error"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// --- Torrent Downloads ---

// TorrentDownload represents a torrent download row.
type TorrentDownload struct {
	ID               string          `json:"id"`
	SourceAccountID  string          `json:"source_account_id"`
	ClientID         string          `json:"client_id"`
	ClientTorrentID  string          `json:"client_torrent_id"`
	Name             string          `json:"name"`
	InfoHash         string          `json:"info_hash"`
	MagnetURI        string          `json:"magnet_uri"`
	Status           string          `json:"status"`
	Category         string          `json:"category"`
	SizeBytes        int64           `json:"size_bytes"`
	DownloadedBytes  int64           `json:"downloaded_bytes"`
	UploadedBytes    int64           `json:"uploaded_bytes"`
	ProgressPercent  float64         `json:"progress_percent"`
	Ratio            float64         `json:"ratio"`
	DownloadSpeed    int64           `json:"download_speed_bytes"`
	UploadSpeed      int64           `json:"upload_speed_bytes"`
	Seeders          int             `json:"seeders"`
	Leechers         int             `json:"leechers"`
	PeersConnected   int             `json:"peers_connected"`
	DownloadPath     *string         `json:"download_path"`
	FilesCount       int             `json:"files_count"`
	StopAtRatio      *float64        `json:"stop_at_ratio"`
	StopAtTimeHours  *int            `json:"stop_at_time_hours"`
	VPNIP            *string         `json:"vpn_ip"`
	VPNInterface     *string         `json:"vpn_interface"`
	ErrorMessage     *string         `json:"error_message"`
	ContentID        *string         `json:"content_id"`
	RequestedBy      string          `json:"requested_by"`
	Metadata         json.RawMessage `json:"metadata"`
	AddedAt          time.Time       `json:"added_at"`
	StartedAt        *time.Time      `json:"started_at"`
	CompletedAt      *time.Time      `json:"completed_at"`
	StoppedAt        *time.Time      `json:"stopped_at"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// --- Search Cache ---

// TorrentSearchCache represents a cached search result row.
type TorrentSearchCache struct {
	ID               string          `json:"id"`
	SourceAccountID  string          `json:"source_account_id"`
	QueryHash        string          `json:"query_hash"`
	Query            string          `json:"query"`
	Results          json.RawMessage `json:"results"`
	ResultsCount     int             `json:"results_count"`
	SourcesSearched  []string        `json:"sources_searched"`
	SearchDurationMS *int            `json:"search_duration_ms"`
	CachedAt         time.Time       `json:"cached_at"`
	ExpiresAt        time.Time       `json:"expires_at"`
	CreatedAt        time.Time       `json:"created_at"`
}

// --- Seeding Policies ---

// SeedingPolicy represents a global seeding policy row.
type SeedingPolicy struct {
	ID                    string    `json:"id"`
	SourceAccountID       string    `json:"source_account_id"`
	PolicyName            string    `json:"policy_name"`
	Description           *string   `json:"description"`
	RatioLimit            *float64  `json:"ratio_limit"`
	RatioAction           string    `json:"ratio_action"`
	TimeLimitHours        *int      `json:"time_limit_hours"`
	TimeAction            string    `json:"time_action"`
	MaxSeedingSizeGB      *int      `json:"max_seeding_size_gb"`
	AppliesToCategories   []string  `json:"applies_to_categories"`
	Priority              int       `json:"priority"`
	IsActive              bool      `json:"is_active"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// DownloadSeedingPolicy represents a per-download seeding policy row.
type DownloadSeedingPolicy struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	DownloadID      string    `json:"download_id"`
	RatioLimit      float64   `json:"ratio_limit"`
	TimeLimitHours  int       `json:"time_limit_hours"`
	AutoRemove      bool      `json:"auto_remove"`
	KeepFiles       bool      `json:"keep_files"`
	Favorite        bool      `json:"favorite"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// --- Statistics ---

// TorrentStats holds aggregated download statistics.
type TorrentStats struct {
	TotalDownloads     int     `json:"total_downloads"`
	ActiveDownloads    int     `json:"active_downloads"`
	CompletedDownloads int     `json:"completed_downloads"`
	FailedDownloads    int     `json:"failed_downloads"`
	SeedingTorrents    int     `json:"seeding_torrents"`
	TotalDownloaded    int64   `json:"total_downloaded_bytes"`
	TotalUploaded      int64   `json:"total_uploaded_bytes"`
	OverallRatio       float64 `json:"overall_ratio"`
	DownloadSpeed      int64   `json:"download_speed_bytes"`
	UploadSpeed        int64   `json:"upload_speed_bytes"`
	DiskSpaceUsed      int64   `json:"disk_space_used_bytes"`
	DiskSpaceAvailable int64   `json:"disk_space_available_bytes"`
}

// --- Request/Response types ---

// AddDownloadRequest is the JSON body for POST /v1/downloads.
type AddDownloadRequest struct {
	MagnetURI    string  `json:"magnet_uri"`
	Category     *string `json:"category,omitempty"`
	DownloadPath *string `json:"download_path,omitempty"`
	RequestedBy  *string `json:"requested_by,omitempty"`
}

// SearchRequest is the JSON body for POST /v1/search.
type SearchRequest struct {
	Query      string `json:"query"`
	Type       string `json:"type,omitempty"`
	Quality    string `json:"quality,omitempty"`
	MinSeeders *int   `json:"minSeeders,omitempty"`
	MaxResults *int   `json:"maxResults,omitempty"`
}

// SmartSearchRequest is the JSON body for POST /v1/search/best-match.
type SmartSearchRequest struct {
	Title      string  `json:"title"`
	Year       *int    `json:"year,omitempty"`
	Season     *int    `json:"season,omitempty"`
	Episode    *int    `json:"episode,omitempty"`
	Quality    *string `json:"quality,omitempty"`
	MinSeeders *int    `json:"minSeeders,omitempty"`
}

// FetchMagnetRequest is the JSON body for POST /v1/magnet.
type FetchMagnetRequest struct {
	Source    string `json:"source"`
	SourceURL string `json:"sourceUrl"`
}

// SeedingConfigRequest is the JSON body for PUT /v1/seeding/:id/policy.
type SeedingConfigRequest struct {
	RatioLimit     *float64 `json:"ratio_limit,omitempty"`
	TimeLimitHours *int     `json:"time_limit_hours,omitempty"`
	AutoRemove     *bool    `json:"auto_remove,omitempty"`
	KeepFiles      *bool    `json:"keep_files,omitempty"`
	Favorite       *bool    `json:"favorite,omitempty"`
}

// SourceRegistryEntry describes a torrent search source.
type SourceRegistryEntry struct {
	Name       string   `json:"name"`
	ActiveFrom string   `json:"active_from"`
	RetiredAt  *string  `json:"retired_at"`
	Category   string   `json:"category"`
	TrustScore float64  `json:"trust_score"`
	Strengths  []string `json:"strengths"`
}

// TransmissionClientStats holds stats returned from Transmission RPC.
type TransmissionClientStats struct {
	TotalTorrents   int   `json:"total_torrents"`
	ActiveTorrents  int   `json:"active_torrents"`
	PausedTorrents  int   `json:"paused_torrents"`
	SeedingTorrents int   `json:"seeding_torrents"`
	DownloadSpeed   int64 `json:"download_speed_bytes"`
	UploadSpeed     int64 `json:"upload_speed_bytes"`
	Downloaded      int64 `json:"downloaded_bytes"`
	Uploaded        int64 `json:"uploaded_bytes"`
}
