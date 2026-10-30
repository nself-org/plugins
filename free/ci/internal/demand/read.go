package demand

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ReadConfig configures reading of the demand document.
type ReadConfig struct {
	Dir   string
	Clock Clock
}

// Read returns the last complete document and its age.
// A missing file returns a zero Snapshot with Alive=false and no error.
func Read(cfg ReadConfig) (Snapshot, error) {
	if cfg.Dir == "" {
		return Snapshot{}, fmt.Errorf("demand: empty directory")
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	target := filepath.Join(cfg.Dir, fileName)
	data, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{Doc: Document{Watcher: Watcher{Alive: false}}}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("demand: read %s: %w", target, err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Snapshot{}, fmt.Errorf("demand: unmarshal %s: %w", target, err)
	}
	ts, err := time.Parse(time.RFC3339, doc.TS)
	if err != nil {
		return Snapshot{}, fmt.Errorf("demand: parse ts %q: %w", doc.TS, err)
	}
	age := cfg.Clock.Now().Sub(ts)
	if age < 0 {
		age = 0
	}
	doc.Watcher.Alive = true
	return Snapshot{Doc: doc, Age: age}, nil
}
