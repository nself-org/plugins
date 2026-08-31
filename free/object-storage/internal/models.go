package internal

import "time"

// Bucket maps to os_buckets.
type Bucket struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	Name            string         `json:"name"`
	Provider        string         `json:"provider"`
	ProviderConfig  map[string]any `json:"provider_config"`
	PublicRead      bool           `json:"public_read"`
	CORSOrigins     []string       `json:"cors_origins"`
	MaxFileSizeBytes int64         `json:"max_file_size_bytes"`
	AllowedMIMETypes []string      `json:"allowed_mime_types"`
	QuotaBytes      *int64         `json:"quota_bytes"`
	UsedBytes       int64          `json:"used_bytes"`
	ObjectCount     int            `json:"object_count"`
	LifecycleRules  []any          `json:"lifecycle_rules"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// Object maps to os_objects.
type Object struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	BucketID        string         `json:"bucket_id"`
	Key             string         `json:"key"`
	Filename        *string        `json:"filename"`
	ContentType     string         `json:"content_type"`
	SizeBytes       int64          `json:"size_bytes"`
	ChecksumSHA256  *string        `json:"checksum_sha256"`
	ETag            *string        `json:"etag"`
	StorageClass    string         `json:"storage_class"`
	Metadata        map[string]any `json:"metadata"`
	Tags            map[string]any `json:"tags"`
	OwnerID         *string        `json:"owner_id"`
	IsPublic        bool           `json:"is_public"`
	Version         int            `json:"version"`
	DeletedAt       *time.Time     `json:"deleted_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// UploadSession maps to os_upload_sessions.
type UploadSession struct {
	ID                string     `json:"id"`
	SourceAccountID   string     `json:"source_account_id"`
	BucketID          string     `json:"bucket_id"`
	Key               string     `json:"key"`
	ContentType       *string    `json:"content_type"`
	TotalSizeBytes    *int64     `json:"total_size_bytes"`
	UploadType        string     `json:"upload_type"`
	Status            string     `json:"status"`
	MultipartUploadID *string    `json:"multipart_upload_id"`
	PartsCompleted    int        `json:"parts_completed"`
	PartsTotal        *int       `json:"parts_total"`
	PresignedURL      *string    `json:"presigned_url"`
	PresignedExpiresAt *time.Time `json:"presigned_expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// AccessLog maps to os_access_logs.
type AccessLog struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	BucketID        *string    `json:"bucket_id"`
	ObjectID        *string    `json:"object_id"`
	Action          string     `json:"action"`
	ActorID         *string    `json:"actor_id"`
	IPAddress       *string    `json:"ip_address"`
	UserAgent       *string    `json:"user_agent"`
	Status          *int       `json:"status"`
	ResponseTimeMs  *int       `json:"response_time_ms"`
	BytesTransferred *int64    `json:"bytes_transferred"`
	CreatedAt       time.Time  `json:"created_at"`
}
