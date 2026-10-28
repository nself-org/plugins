// Package internal provides the plugin-storage server internals.
//
// Purpose: Configuration loading for plugin-storage.
// Inputs: Environment variables.
// Outputs: Config struct.
// Constraints: S3_ENDPOINT must be set from config only; not user-overridable per request.
package internal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all plugin-storage configuration.
type Config struct {
	Port       string
	DBUrl      string
	S3Endpoint string
	S3Region   string
	AccessKey  string
	SecretKey  string
	UseSSL     bool
	// MaxLargeUploads caps concurrent uploads above 1 MiB (STORAGE_MAX_LARGE_UPLOADS, default 8).
	MaxLargeUploads int
	// MaxUploads caps concurrent uploads of any size (STORAGE_MAX_UPLOADS, default 64).
	MaxUploads int
	// UploadSlotWait is how long an upload waits for a slot before 503 (STORAGE_UPLOAD_SLOT_WAIT, default 30s).
	UploadSlotWait time.Duration
	// MaxObjectBytes caps one object (STORAGE_MAX_OBJECT_BYTES, default 5 GiB). Larger PUTs answer 413.
	MaxObjectBytes int64
	// AccountQuotaBytes caps the summed size of one account's objects (STORAGE_ACCOUNT_QUOTA_BYTES); 0 = off.
	AccountQuotaBytes int64
	// InternalSecret is PLUGIN_INTERNAL_SECRET. /storage/* routes require it in X-Internal-Token; empty = routes
	// answer 503 (fail closed). Surrounding whitespace is trimmed, so a whitespace-only value counts as unset (a header
	// value never carries it, so such a secret could never match and every call would answer 401 with no hint).
	InternalSecret string
}

// defaultMaxObjectBytes is the per-object cap when STORAGE_MAX_OBJECT_BYTES is unset (5 GiB, the S3 single-PutObject size).
const defaultMaxObjectBytes int64 = 5 << 30

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*Config, error) {
	port := os.Getenv("STORAGE_PLUGIN_PORT")
	if port == "" {
		port = "9007"
	}
	dbURL := os.Getenv("NSELF_DB_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("NSELF_DB_URL is required")
	}
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "minio:9000"
	}
	region := os.Getenv("S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	useSSL := os.Getenv("S3_USE_SSL") == "true"

	maxLarge := 8
	if v := os.Getenv("STORAGE_MAX_LARGE_UPLOADS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("STORAGE_MAX_LARGE_UPLOADS must be a positive integer, got %q", v)
		}
		maxLarge = n
	}

	maxUploads := 64
	if v := os.Getenv("STORAGE_MAX_UPLOADS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("STORAGE_MAX_UPLOADS must be a positive integer, got %q", v)
		}
		maxUploads = n
	}
	slotWait := 30 * time.Second
	if v := os.Getenv("STORAGE_UPLOAD_SLOT_WAIT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("STORAGE_UPLOAD_SLOT_WAIT must be a positive duration like 30s, got %q", v)
		}
		slotWait = d
	}

	maxObject := defaultMaxObjectBytes
	if v := os.Getenv("STORAGE_MAX_OBJECT_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("STORAGE_MAX_OBJECT_BYTES must be a positive integer (bytes), got %q", v)
		}
		maxObject = n
	}
	var quota int64 // 0 = off
	if v := os.Getenv("STORAGE_ACCOUNT_QUOTA_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("STORAGE_ACCOUNT_QUOTA_BYTES must be a non-negative integer (bytes, 0 = off), got %q", v)
		}
		quota = n
	}

	return &Config{
		Port:              port,
		DBUrl:             dbURL,
		S3Endpoint:        endpoint,
		S3Region:          region,
		AccessKey:         accessKey,
		SecretKey:         secretKey,
		UseSSL:            useSSL,
		MaxLargeUploads:   maxLarge,
		MaxUploads:        maxUploads,
		UploadSlotWait:    slotWait,
		MaxObjectBytes:    maxObject,
		AccountQuotaBytes: quota,
		InternalSecret:    strings.TrimSpace(os.Getenv("PLUGIN_INTERNAL_SECRET")),
	}, nil
}
