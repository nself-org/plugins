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
}

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

	return &Config{
		Port:            port,
		DBUrl:           dbURL,
		S3Endpoint:      endpoint,
		S3Region:        region,
		AccessKey:       accessKey,
		SecretKey:       secretKey,
		UseSSL:          useSSL,
		MaxLargeUploads: maxLarge,
		MaxUploads:      maxUploads,
		UploadSlotWait:  slotWait,
	}, nil
}
