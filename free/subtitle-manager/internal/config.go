package internal

import (
	"log"
	"os"
	"strconv"
)

// LoadConfig reads all subtitle-manager environment variables and returns a Config.
func LoadConfig() *Config {
	port := 3204
	if v := os.Getenv("SUBTITLE_MANAGER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	storagePath := os.Getenv("SUBTITLE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "/tmp/subtitles"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	alassPath := os.Getenv("ALASS_PATH")
	if alassPath == "" {
		alassPath = "alass"
	}

	ffsubsyncPath := os.Getenv("FFSUBSYNC_PATH")
	if ffsubsyncPath == "" {
		ffsubsyncPath = "ffsubsync"
	}

	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		Port:             port,
		OpenSubtitlesKey: os.Getenv("OPENSUBTITLES_API_KEY"),
		StoragePath:      storagePath,
		LogLevel:         logLevel,
		AlassPath:        alassPath,
		FfsubsyncPath:    ffsubsyncPath,
	}

	log.Printf("subtitle-manager: config loaded (port=%d, storage=%s)", cfg.Port, cfg.StoragePath)
	return cfg
}
