// Package trace provides opt-in span tracing for performance diagnosis.
// When NSELF_CI_TRACE environment variable is set to a file path, each span
// is recorded as a JSON line. When unset, all operations are no-ops with
// zero allocations.
package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	enabled    bool
	startTime  time.Time
	outputPath string
	mu         sync.Mutex
	warnedOnce bool
	noopEnd    = func() {}
)

func init() {
	configure(os.Getenv("NSELF_CI_TRACE"))
}

// configure sets up tracing based on the given path.
// Exported only for testing within this package.
func configure(path string) {
	mu.Lock()
	defer mu.Unlock()
	outputPath = path
	if path != "" {
		enabled = true
		startTime = time.Now()
	} else {
		enabled = false
	}
	warnedOnce = false
}

// Start begins a new span with the given name and returns a function
// that ends the span and records it. When tracing is disabled, the
// returned function is a no-op with zero allocations.
func Start(name string) func() {
	if !enabled {
		return noopEnd
	}

	start := time.Now()
	return func() {
		end := time.Now()
		span := Span{
			Name:    name,
			StartMS: int64(start.Sub(startTime) / time.Millisecond),
			DurMS:   int64(end.Sub(start) / time.Millisecond),
			PID:     os.Getpid(),
		}
		writeSpan(span)
	}
}

// Span represents a recorded trace span.
type Span struct {
	Name    string `json:"span"`
	StartMS int64  `json:"start_ms"`
	DurMS   int64  `json:"dur_ms"`
	PID     int    `json:"pid"`
}

func writeSpan(span Span) {
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		if !warnedOnce {
			fmt.Fprintf(os.Stderr, "trace: failed to open %s: %v\n", outputPath, err)
			warnedOnce = true
		}
		return
	}
	defer f.Close()

	data, err := json.Marshal(span)
	if err != nil {
		if !warnedOnce {
			fmt.Fprintf(os.Stderr, "trace: failed to marshal span: %v\n", err)
			warnedOnce = true
		}
		return
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		if !warnedOnce {
			fmt.Fprintf(os.Stderr, "trace: failed to write span: %v\n", err)
			warnedOnce = true
		}
	}
}
