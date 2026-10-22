package run

// Purpose: wire internal/hygiene into the local run loop (Epic D12). Every attempt the run starts is
// cleaned and leak-checked when the run closes, and orphans of crashed runs are swept once per
// process when the first attempt starts.
// Inputs:  the attempts seen by the AttemptStart hook with the run's home (Options.Home, else
// NSELF_CI_HOME or ~/.nself/ci, the executor's own default), and the run's job outcomes.
// Outputs: a Finalizer error carrying E681 (a note on the run, never a changed verdict) and a call
// to the registered drain callback with the reason; the agent registers the callback that drains
// its node.
// Constraints: the attempt hook never fails a job; the sweep spares attempts of this process, any
// attempt with a live process marker, and roots younger than orphanGrace (a sibling `nself ci`
// may still own them); a LeakCheck here runs with no job UID, so it matches by the attempt tag.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

const (
	orphanGrace    = 12 * time.Hour
	hygieneTimeout = 90 * time.Second
	sweepTimeout   = 30 * time.Second
)

// hygieneHook is the AttemptStartHook and Finalizer for workspace hygiene.
type hygieneHook struct {
	mu       sync.Mutex
	byRun    map[string][]seenAttempt
	once     sync.Once
	sweepErr error
	drain    func(reason string)
	// rt is the container runtime view; nil means the docker or podman CLI (tests inject a fake).
	rt hygiene.Runtime
}

// seenAttempt is an attempt the run started and the home it runs in.
type seenAttempt struct {
	id, home  string
	container bool // the job asked for a container runtime level: its runtime must answer
}

var hygieneHooks = &hygieneHook{byRun: map[string][]seenAttempt{}}

func init() {
	RegisterAttemptStart(hygieneHooks)
	RegisterFinalizer(hygieneHooks)
}

// RegisterHygieneDrain sets the callback the finalizer calls with the leak reason, so the node can
// stop taking work (single slot; a second registration replaces the first).
func RegisterHygieneDrain(f func(reason string)) {
	hygieneHooks.mu.Lock()
	hygieneHooks.drain = f
	hygieneHooks.mu.Unlock()
}

// ciHome is the state directory of a run: Options.Home, else what the executor defaults to.
func ciHome(home string) string {
	dir := slotDir(home)
	if dir == "" {
		return ""
	}
	return filepath.Dir(dir)
}

func hygieneHost(home string, rt hygiene.Runtime) *hygiene.Host {
	return &hygiene.Host{Base: filepath.Join(home, "work"), Home: home, MinAge: orphanGrace, Runtime: rt}
}

// managed reports whether hygiene manages this home: a work directory that was relocated through a
// symlink (offloaded to another disk) is the operator's layout, which hygiene's no-follow rules
// would refuse on every run, so the executor's own cleanup is left to handle it.
func managed(home string) bool {
	info, err := os.Lstat(filepath.Join(home, "work"))
	return err != nil || (info.Mode()&os.ModeSymlink == 0 && info.IsDir())
}

// AttemptStart records the attempt and, once per process, sweeps orphans.
func (h *hygieneHook) AttemptStart(ctx context.Context, a AttemptInfo) error {
	if runtime.GOOS == "windows" {
		return nil // confined removal is not supported there (Epic D35); exec refuses local jobs too
	}
	home := ciHome(a.Home)
	h.mu.Lock()
	h.byRun[a.RunID] = append(h.byRun[a.RunID], seenAttempt{a.AttemptID, home, a.Job.Isolation == "container" || a.Job.Isolation == "sandboxed-container"})
	h.mu.Unlock()
	h.once.Do(func() {
		if home == "" || !managed(home) {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, sweepTimeout) // follows the job's context: Ctrl-C stops it
		defer cancel()
		_, err := hygieneHost(home, hygiene.BatchRuntime(h.rt)).SweepOrphans(sctx, h.liveAttempts(home))
		h.mu.Lock()
		h.sweepErr = err
		h.mu.Unlock()
	})
	return nil
}

// liveAttempts are the attempts a sweep must spare: this process's and those with a process marker.
func (h *hygieneHook) liveAttempts(home string) []string {
	h.mu.Lock()
	var live []string
	for _, seen := range h.byRun {
		for _, a := range seen {
			live = append(live, a.id)
		}
	}
	h.mu.Unlock()
	if entries, err := os.ReadDir(filepath.Join(home, "processes")); err == nil {
		for _, e := range entries {
			if name, ok := strings.CutSuffix(e.Name(), ".json"); ok && markerLive(filepath.Join(home, "processes", e.Name())) {
				live = append(live, name)
			}
		}
	}
	return live
}

// markerLive reports whether a process marker names a process that still exists; a corrupt marker
// or one whose leader is gone protects nothing (the executor's own recovery removes such markers).
func markerLive(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(data, &m) != nil || m.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(m.PID)
	if err != nil {
		return false
	}
	defer func() { _ = proc.Release() }()
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Name orders the finalizer (names sort; hygiene runs after the evidence finalizers).
func (h *hygieneHook) Name() string { return "zz-hygiene" }

// Finalize cleans and leak-checks every attempt of the run, each in the home it ran in.
func (h *hygieneHook) Finalize(ctx context.Context, r RunInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	h.mu.Lock()
	seen := h.byRun[r.RunID]
	delete(h.byRun, r.RunID)
	sweepErr, drain := h.sweepErr, h.drain
	h.sweepErr = nil
	h.mu.Unlock()
	known := map[string]bool{}
	var attempts []seenAttempt
	for _, a := range seen {
		if a.id != "" && !known[a.id] {
			known[a.id] = true
			attempts = append(attempts, a)
		}
	}
	for _, id := range outcomeAttempts(r.Jobs) {
		if id != "" && !known[id] {
			known[id] = true
			attempts = append(attempts, seenAttempt{id: id, home: ciHome("")}) // not seen starting: the default home
		}
	}
	if len(attempts) == 0 {
		return nil
	}
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hygieneTimeout)
	defer cancel()
	var problems, reasons []string
	rt := hygiene.BatchRuntime(h.rt) // one listing per resource kind for the whole run
	if sweepErr != nil {
		problems = append(problems, "orphan sweep: "+sweepErr.Error())
	}
	for _, a := range attempts {
		if a.home == "" {
			problems = append(problems, "attempt "+a.id+": no state directory to clean")
			continue
		}
		if !managed(a.home) {
			continue
		}
		host := hygieneHost(a.home, rt)
		host.RuntimeRequired = a.container
		if err := host.Cleanup(hctx, a.id); err != nil {
			problems = append(problems, err.Error())
		}
		if rep, err := host.LeakCheck(hctx, a.id, 0); err != nil {
			problems = append(problems, err.Error())
			reasons = append(reasons, rep.Reason())
		}
	}
	if len(problems) == 0 {
		return nil
	}
	reason := strings.Join(reasons, "; ")
	if reason == "" {
		reason = strings.Join(problems, "; ")
	}
	if drain != nil {
		drain(reason)
	}
	return &hygiene.Error{Code: hygiene.CodeLeak, Reason: hygiene.ReasonLeak, Message: reason}
}

func outcomeAttempts(jobs []JobOutcome) []string {
	out := make([]string, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.AttemptID)
	}
	return out
}
