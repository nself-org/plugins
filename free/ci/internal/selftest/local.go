package selftest

// Purpose: gather the Facts of the real host. Everything that needs the run store
// (delivery counters, claims, run results) is a function field the command wires, so
// this file has no store and tests need none.
// Inputs:  the repository root, the CI home, the explicit PATH, injectable seams.
// Outputs: Facts. Constraints: each fact fails alone: an error becomes that fact's Err.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

// Local reads one host.
type Local struct {
	Root, Home string
	Path       string   // the PATH the service runs with
	Tools      []string // gate tools; nil uses DefaultTools
	Capture    doctor.CaptureFunc
	Now        func() time.Time // nil: time.Now
	FreeMB     func(dir string) (int64, error)
	// Sinks, Claims and Infra read the run store; nil means no store: no sinks, no claim
	// source, no runs.
	Sinks  func(ctx context.Context) ([]Sink, error)
	Claims func(ctx context.Context) (Claims, error)
	Infra  func(ctx context.Context, since time.Time) (count, total int, err error)
}

// DefaultTools are the gate tools; git and go are required.
var DefaultTools = []string{"git", "go", "gh", "pnpm", "govulncheck"}

func (l Local) now() time.Time {
	if l.Now != nil {
		return l.Now().UTC()
	}
	return time.Now().UTC()
}

// Facts observes the host.
func (l Local) Facts(ctx context.Context) Facts {
	now := l.now()
	f := Facts{Now: now, Tools: l.tools(), Provider: l.provider(ctx), Repo: l.repo(ctx)}
	free := l.FreeMB
	if free == nil {
		free = FreeMB
	}
	if mb, err := free(l.Home); err != nil {
		f.Disk.Err = err.Error()
	} else {
		f.Disk.FreeMB = mb
	}
	if l.Sinks != nil {
		var err error
		if f.Sinks.List, err = l.Sinks(ctx); err != nil {
			f.Sinks.Err = err.Error()
		}
	}
	f.Watcher = l.watcher(now)
	if l.Claims != nil {
		var err error
		if f.Claims, err = l.Claims(ctx); err != nil {
			f.Claims.Err = err.Error()
		}
	}
	var err error
	if f.Orphans.List, err = OrphanRoots(filepath.Join(l.Home, "work"), OrphanMinAge, now); err != nil {
		f.Orphans.Err = err.Error()
	}
	if l.Infra != nil {
		if f.Infra.Count, f.Infra.Total, err = l.Infra(ctx, now.Add(-24*time.Hour)); err != nil {
			f.Infra.Err = err.Error()
		}
	}
	return f
}

func (l Local) tools() []Tool {
	names := l.Tools
	if names == nil {
		names = DefaultTools
	}
	var out []Tool
	for _, n := range names {
		p, ok := lookPath(l.Path, n)
		out = append(out, Tool{Name: n, Path: p, Found: ok, Required: n == "git" || n == "go"})
	}
	return out
}

// lookPath finds name in the directories of path only, never in the caller's own PATH.
func lookPath(path, name string) (string, bool) {
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		for _, cand := range []string{name, name + ".exe"} {
			p := filepath.Join(dir, cand)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && (runtime.GOOS == "windows" || fi.Mode()&0o111 != 0) {
				return p, true
			}
		}
	}
	return "", false
}

func (l Local) provider(ctx context.Context) Provider {
	if l.Capture == nil {
		return Provider{Err: "no process runner"}
	}
	if _, ok := lookPath(l.Path, "gh"); !ok {
		return Provider{Err: "gh is not on the service PATH"}
	}
	_, exit, err := l.Capture(ctx, "gh", "auth", "status")
	if err != nil {
		return Provider{Err: err.Error()}
	}
	if exit != 0 {
		return Provider{}
	}
	_, exit, err = l.Capture(ctx, "gh", "api", "rate_limit")
	return Provider{LoggedIn: true, Reachable: err == nil && exit == 0}
}

func (l Local) repo(ctx context.Context) Probe {
	if l.Capture == nil {
		return Probe{Err: "no process runner"}
	}
	out, exit, err := l.Capture(ctx, "git", "-C", l.Root, "rev-parse", "--git-dir")
	switch {
	case err != nil:
		return Probe{Err: err.Error()}
	case exit != 0:
		return Probe{Err: "git cannot read " + l.Root + ": " + firstLine(out)}
	}
	return Probe{OK: true}
}

func (l Local) watcher(now time.Time) Watcher {
	snap, err := demand.Read(demand.ReadConfig{Dir: l.Home, Clock: fixedClock(now)})
	switch {
	case err != nil && errors.Is(err, os.ErrPermission):
		return Watcher{Err: err.Error()}
	case err != nil:
		return Watcher{Present: true, Age: 1 << 62, Err: ""} // unreadable or half-written: treated as stale
	case !snap.Doc.Watcher.Alive:
		return Watcher{}
	}
	return Watcher{Present: true, Age: snap.Age, Interval: time.Duration(snap.Doc.Watcher.IntervalSec * float64(time.Second))}
}

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}
