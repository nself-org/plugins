package internal

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
)

// Config holds runtime configuration for nself-post.
type Config struct {
	DatabaseURL      string
	Port             int
	InternalSecret   string
	EncryptionKey    string
}

// LoadConfig reads configuration from environment variables.
// PORT defaults to 3129. DATABASE_URL, POST_INTERNAL_SECRET, and
// POST_ENCRYPTION_KEY are required.
func LoadConfig() (*Config, error) {
	port := 3129
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	return &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		Port:           port,
		InternalSecret: os.Getenv("POST_INTERNAL_SECRET"),
		EncryptionKey:  os.Getenv("POST_ENCRYPTION_KEY"),
	}, nil
}

// ParseEncryptionKey converts a 64-char hex string or 44-char base64 string
// into a 32-byte AES-256 key. Mirrors the Rust load_encryption_key() logic.
func ParseEncryptionKey(raw string) ([32]byte, error) {
	var key [32]byte
	// Try hex first (64 chars)
	if len(raw) == 64 {
		b, err := hex.DecodeString(raw)
		if err == nil && len(b) == 32 {
			copy(key[:], b)
			return key, nil
		}
	}
	// Try base64 (44 chars standard padding)
	b, err := base64.StdEncoding.DecodeString(raw)
	if err == nil && len(b) == 32 {
		copy(key[:], b)
		return key, nil
	}
	return key, fmt.Errorf("POST_ENCRYPTION_KEY must be 64 hex chars or 44 base64 chars (32 bytes); got %d chars", len(raw))
}
