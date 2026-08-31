package internal

import "time"

// Zone represents a row in cf_zones.
type Zone struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ZoneID          *string   `json:"zone_id,omitempty"`
	Name            string    `json:"name"`
	Status          *string   `json:"status,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	SyncedAt        time.Time `json:"synced_at"`
}

// DNSRecord represents a row in cf_dns_records.
type DNSRecord struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ZoneID          string    `json:"zone_id"`
	RecordID        *string   `json:"record_id,omitempty"`
	Type            string    `json:"type"`
	Name            string    `json:"name"`
	Content         string    `json:"content"`
	TTL             int       `json:"ttl"`
	Proxied         bool      `json:"proxied"`
	CreatedAt       time.Time `json:"created_at"`
	SyncedAt        time.Time `json:"synced_at"`
}

// R2Bucket represents a row in cf_r2_buckets.
type R2Bucket struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Name            string    `json:"name"`
	Location        *string   `json:"location,omitempty"`
	StorageClass    string    `json:"storage_class"`
	ObjectCount     int64     `json:"object_count"`
	TotalSizeBytes  int64     `json:"total_size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
	SyncedAt        time.Time `json:"synced_at"`
}

// CachePurgeLog represents a row in cf_cache_purge_log.
type CachePurgeLog struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ZoneID          string    `json:"zone_id"`
	PurgeType       string    `json:"purge_type"`
	URLs            []string  `json:"urls,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	Prefixes        []string  `json:"prefixes,omitempty"`
	Status          string    `json:"status"`
	CFResponse      *string   `json:"cf_response,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// Analytics represents a row in cf_analytics.
type Analytics struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ZoneID          string    `json:"zone_id"`
	Date            string    `json:"date"`
	RequestsTotal   int64     `json:"requests_total"`
	RequestsCached  int64     `json:"requests_cached"`
	BandwidthTotal  int64     `json:"bandwidth_total"`
	ThreatsTotal    int64     `json:"threats_total"`
	UniqueVisitors  int64     `json:"unique_visitors"`
	SyncedAt        time.Time `json:"synced_at"`
}

// Stats holds aggregate counts across all cloudflare tables.
type Stats struct {
	TotalZones       int64 `json:"total_zones"`
	TotalDNSRecords  int64 `json:"total_dns_records"`
	TotalR2Buckets   int64 `json:"total_r2_buckets"`
	TotalCachePurges int64 `json:"total_cache_purges"`
	TotalAnalytics   int64 `json:"total_analytics"`
}
