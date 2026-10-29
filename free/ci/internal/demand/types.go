package demand

import "time"

// Document is the contract:ci.demand v1 payload written to disk.
type Document struct {
	TS       string   `json:"ts"`
	Host     string   `json:"host"`
	Queued   int      `json:"queued"`
	Held     int      `json:"held"`
	Running  int      `json:"running"`
	Ready    bool     `json:"ready"`
	ReadyWhy []string `json:"ready_why"`
	Watcher  Watcher  `json:"watcher"`
}

// Watcher reports liveness and cadence of the watcher loop.
type Watcher struct {
	Alive       bool    `json:"alive"`
	LastPass    string  `json:"last_pass"`
	IntervalSec float64 `json:"interval_sec"`
}

// Snapshot is the result of Read, including the document age.
type Snapshot struct {
	Doc Document
	Age time.Duration
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

// realClock uses the standard library time.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
