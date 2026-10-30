package watch

// Purpose: the watcher's own log writer and heartbeat (P7-AUTO-11): JSON lines under
//          $NSELF_CI_HOME/logs/watch/, size-capped and rotated; the service units of P7-AUTO-12 only
//          point at this directory.
// Inputs:  the directory, a size cap and a file count.
// Outputs: watch.log, watch.log.1 .. watch.log.N, and the heartbeat file.
// Constraints: files 0600 in a 0700 directory; a line is written whole or not at all; an event never
//          carries a token, an environment value or a path a caller did not put in its fields; a write
//          failure is returned, never a panic.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Log defaults.
const (
	DefaultLogBytes = 1 << 20
	DefaultLogKeep  = 5
	logName         = "watch.log"
	heartbeatName   = "heartbeat"
)

// LogDir is the watcher's log directory under home ($NSELF_CI_HOME).
func LogDir(home string) string { return filepath.Join(home, "logs", "watch") }

// RotatingLog is a size-capped JSON-lines log. The zero MaxBytes and Keep take the defaults.
type RotatingLog struct {
	Dir      string
	MaxBytes int64
	Keep     int
	Now      func() time.Time

	mu sync.Mutex
}

// Event writes one line; failures are dropped, so logging never stops a pass.
func (l *RotatingLog) Event(event string, fields map[string]any) { _ = l.Write(event, fields) }

// Write writes one line {"ts","event",fields...}, rotating first when the line would pass the cap.
func (l *RotatingLog) Write(event string, fields map[string]any) error {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	rec := map[string]any{}
	for k, v := range fields {
		rec[k] = v
	}
	rec["ts"], rec["event"] = now().UTC().Format(time.RFC3339), event
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if err = os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}
	p := filepath.Join(l.Dir, logName)
	if fi, serr := os.Stat(p); serr == nil && fi.Size()+int64(len(line)) > l.max() {
		if err = l.rotate(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (l *RotatingLog) max() int64 {
	if l.MaxBytes > 0 {
		return l.MaxBytes
	}
	return DefaultLogBytes
}

// rotate shifts watch.log.(n-1) to watch.log.n down to watch.log -> watch.log.1; the oldest is dropped.
func (l *RotatingLog) rotate() error {
	keep := l.Keep
	if keep <= 0 {
		keep = DefaultLogKeep
	}
	base := filepath.Join(l.Dir, logName)
	_ = os.Remove(fmt.Sprintf("%s.%d", base, keep))
	for i := keep - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", base, i)
		if _, err := os.Stat(from); err == nil {
			if err = os.Rename(from, fmt.Sprintf("%s.%d", base, i+1)); err != nil {
				return err
			}
		}
	}
	return os.Rename(base, base+".1")
}

// Heartbeat returns a writer of dir/heartbeat: the time of the last pass, replaced atomically.
func Heartbeat(dir string) func(time.Time) error {
	return func(at time.Time) error {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".heartbeat-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(tmp.Name()) }()
		if _, err = tmp.WriteString(at.UTC().Format(time.RFC3339) + "\n"); err != nil {
			_ = tmp.Close()
			return err
		}
		if err = tmp.Close(); err != nil {
			return err
		}
		return os.Rename(tmp.Name(), filepath.Join(dir, heartbeatName))
	}
}
