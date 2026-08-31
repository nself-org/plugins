package internal

import "time"

// Zone represents a CDN zone record.
type Zone struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Provider        string     `json:"provider"`
	ZoneID          string     `json:"zone_id"`
	Name            string     `json:"name"`
	Domain          string     `json:"domain"`
	OriginURL       *string    `json:"origin_url"`
	SSLEnabled      bool       `json:"ssl_enabled"`
	CacheTTL        int        `json:"cache_ttl"`
	Status          string     `json:"status"`
	Config          []byte     `json:"config"`
	Metadata        []byte     `json:"metadata"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PurgeRequest represents a cache purge request record.
type PurgeRequest struct {
	ID                string     `json:"id"`
	SourceAccountID   string     `json:"source_account_id"`
	ZoneID            string     `json:"zone_id"`
	PurgeType         string     `json:"purge_type"`
	URLs              []byte     `json:"urls"`
	Tags              []byte     `json:"tags"`
	Prefixes          []byte     `json:"prefixes"`
	Status            string     `json:"status"`
	ProviderRequestID *string    `json:"provider_request_id"`
	RequestedBy       *string    `json:"requested_by"`
	CompletedAt       *time.Time `json:"completed_at"`
	Error             *string    `json:"error"`
	CreatedAt         time.Time  `json:"created_at"`
}

// Analytics represents a daily analytics record for a CDN zone.
type Analytics struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ZoneID          string    `json:"zone_id"`
	Date            string    `json:"date"`
	RequestsTotal   int64     `json:"requests_total"`
	RequestsCached  int64     `json:"requests_cached"`
	BandwidthTotal  int64     `json:"bandwidth_total"`
	BandwidthCached int64     `json:"bandwidth_cached"`
	UniqueVisitors  int64     `json:"unique_visitors"`
	ThreatsBlocked  int64     `json:"threats_blocked"`
	Status2xx       int64     `json:"status_2xx"`
	Status3xx       int64     `json:"status_3xx"`
	Status4xx       int64     `json:"status_4xx"`
	Status5xx       int64     `json:"status_5xx"`
	TopPaths        []byte    `json:"top_paths"`
	TopCountries    []byte    `json:"top_countries"`
	Metadata        []byte    `json:"metadata"`
	CreatedAt       time.Time `json:"created_at"`
}

// SignedURL represents a signed URL record.
type SignedURL struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ZoneID          string     `json:"zone_id"`
	OriginalURL     string     `json:"original_url"`
	SignedURLValue  string     `json:"signed_url"`
	ExpiresAt       time.Time  `json:"expires_at"`
	IPRestriction   *string    `json:"ip_restriction"`
	AccessCount     int        `json:"access_count"`
	MaxAccess       *int       `json:"max_access"`
	CreatedAt       time.Time  `json:"created_at"`
}

// PluginStats holds aggregate statistics for the CDN plugin.
type PluginStats struct {
	TotalZones           int64            `json:"total_zones"`
	ActiveZones          int64            `json:"active_zones"`
	TotalPurgeRequests   int64            `json:"total_purge_requests"`
	PendingPurges        int64            `json:"pending_purges"`
	TotalSignedURLs      int64            `json:"total_signed_urls"`
	ActiveSignedURLs     int64            `json:"active_signed_urls"`
	AnalyticsDaysTracked int64            `json:"analytics_days_tracked"`
	TotalRequestsTracked int64            `json:"total_requests_tracked"`
	TotalBandwidthTracked int64           `json:"total_bandwidth_tracked"`
	ByProvider           map[string]int64 `json:"by_provider"`
}
