package demand

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	fileName = "demand.json"
	fileMode = 0o600
	dirMode  = 0o700
)

// WriterConfig configures atomic publication of the demand document.
type WriterConfig struct {
	Dir   string
	Host  string
	Clock Clock
}

// Write publishes doc atomically to Dir/demand.json using temp+rename.
func Write(cfg WriterConfig, doc Document) error {
	if cfg.Dir == "" {
		return fmt.Errorf("demand: empty directory")
	}
	if err := os.MkdirAll(cfg.Dir, dirMode); err != nil {
		return fmt.Errorf("demand: mkdir %s: %w", cfg.Dir, err)
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	doc.TS = cfg.Clock.Now().UTC().Format(time.RFC3339)
	if doc.Host == "" {
		doc.Host = cfg.Host
	}
	if doc.ReadyWhy == nil {
		doc.ReadyWhy = []string{}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("demand: marshal: %w", err)
	}
	target := filepath.Join(cfg.Dir, fileName)
	tmp, err := os.CreateTemp(cfg.Dir, ".demand-*.tmp")
	if err != nil {
		return fmt.Errorf("demand: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("demand: chmod: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("demand: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("demand: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("demand: close: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("demand: rename: %w", err)
	}
	return nil
}
