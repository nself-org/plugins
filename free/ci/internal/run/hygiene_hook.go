package run

// Purpose: wire internal/hygiene into the local run loop (Epic D12). Every attempt the run starts is
// cleaned and leak-checked when the run closes, and orphans of crashed runs are swept once per
// process when the first attempt starts.
// Inputs:  the attempts seen by the AttemptStart hook, the run's job outcomes, the CI home
// (NSELF_CI_HOME or ~/.nself/ci, the executor's own default).
// Outputs: a Finalizer error carrying E681 (a note on the run, never a changed verdict) and a call
// to the registered drain callback with the reason; the agent registers the callback that drains
// its node.
// Constraints: the attempt hook never fails a job; the sweep spares attempts of this process, any
// attempt with a live process marker, and roots younger than orphanGrace (a sibling `nself ci`
// may still own them); a LeakCheck here runs with no job UID, so it matches by the attempt tag.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

const (
	orphanGrace    = 12 * time.Hour
	hygieneTimeout = 90 * time.Second
)

// hygieneHook is the AttemptStartHook and Finalizer for workspace hygiene.
type hygieneHook struct {
	mu       sync.Mutex
	byRun    map[string][]string
	once     sync.Once
	sweepErr error
	drain    func(reason string)
}

var hygieneHooks = &hygieneHook{byRun: map[string][]string{}}

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

// ciHome is the state directory the executor uses when Options.Home is empty.
func ciHome() string {
	dir := slotDir("")
	if dir == "" {
		return ""
	}
	return filepath.Dir(dir)
}

func hygieneHost(home string) *hygiene.Host {
	return &hygiene.Host{Base: filepath.Join(home, "work"), Home: home, MinAge: orphanGrace}
}

// AttemptStart records the attempt and, once per process, sweeps orphans.
func (h *hygieneHook) AttemptStart(ctx context.Context, a AttemptInfo) error {
	h.mu.Lock()
	h.byRun[a.RunID] = append(h.byRun[a.RunID], a.AttemptID)
	h.mu.Unlock()
	h.once.Do(func() {
		home := ciHome()
		if home == "" {
			return
		}
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hygieneTimeout)
		defer cancel()
		_, err := hygieneHost(home).SweepOrphans(sctx, h.liveAttempts(home))
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
	for _, ids := range h.byRun {
		live = append(live, ids...)
	}
	h.mu.Unlock()
	if entries, err := os.ReadDir(filepath.Join(home, "processes")); err == nil {
		for _, e := range entries {
			if name, ok := strings.CutSuffix(e.Name(), ".json"); ok {
				live = append(live, name)
			}
		}
	}
	return live
}

// Name orders the finalizer (names sort; hygiene runs after the evidence finalizers).
func (h *hygieneHook) Name() string { return "zz-hygiene" }

// Finalize cleans and leak-checks every attempt of the run.
func (h *hygieneHook) Finalize(ctx context.Context, r RunInfo) error {
	h.mu.Lock()
	ids := h.byRun[r.RunID]
	delete(h.byRun, r.RunID)
	sweepErr, drain := h.sweepErr, h.drain
	h.sweepErr = nil
	h.mu.Unlock()
	seen := map[string]bool{}
	var attempts []string
	for _, id := range append(ids, outcomeAttempts(r.Jobs)...) {
		if id != "" && !seen[id] {
			seen[id] = true
			attempts = append(attempts, id)
		}
	}
	home := ciHome()
	if home == "" || len(attempts) == 0 {
		return nil
	}
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hygieneTimeout)
	defer cancel()
	host := hygieneHost(home)
	var problems, reasons []string
	if sweepErr != nil {
		problems = append(problems, "orphan sweep: "+sweepErr.Error())
	}
	for _, id := range attempts {
		if err := host.Cleanup(hctx, id); err != nil {
			problems = append(problems, err.Error())
		}
		if rep, err := host.LeakCheck(hctx, id, 0); err != nil {
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
