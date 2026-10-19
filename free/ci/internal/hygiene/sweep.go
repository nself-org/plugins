package hygiene

// Purpose: SweepOrphans removes the roots and labelled resources of attempts that no live lease
// owns (a crashed agent or coordinator left them).
// Inputs:  the attempt ids of live leases.
// Outputs: a SweepReport; E681 when something could not be removed.
// Constraints: only directories named like an attempt are swept (Host.IsAttempt); a live attempt is never touched; a root younger than Host.MinAge is skipped; only
// entries with a safe attempt name and resources carrying the attempt label are considered, and a
// labelled resource counts only when its attempt has a root under Base (or Host.OwnsRuntime is set).

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// attemptShape is how the local executor names an attempt: <job row>.a<N>.
var attemptShape = regexp.MustCompile(`^.+\.a[0-9]+$`)

// isAttempt reports whether a directory name under Base is an attempt root this host may sweep.
func (h *Host) isAttempt(name string) bool {
	if h.IsAttempt != nil {
		return h.IsAttempt(name)
	}
	return attemptShape.MatchString(name)
}

// rootOwner returns the UID that owns an attempt root, so an orphan of a separated job is cleaned
// as the job user; 0 when the root is gone or belongs to the caller.
func (h *Host) rootOwner(name string) int {
	if h.Base == "" || h.Jobs == nil {
		return 0 // without a job runner no foreign UID is a job UID we may signal
	}
	info, err := os.Lstat(filepath.Join(h.Base, name))
	if err != nil {
		return 0
	}
	return ownerUID(info)
}

// SweepReport says what a sweep did.
type SweepReport struct {
	Removed []string         `json:"removed,omitempty"`
	Skipped []string         `json:"skipped,omitempty"`
	Failed  map[string]error `json:"-"`
}

// SweepOrphans cleans every attempt that is not in liveLeases.
func (h *Host) SweepOrphans(ctx context.Context, liveLeases []string) (SweepReport, error) {
	rep := SweepReport{Failed: map[string]error{}}
	live := make(map[string]bool, len(liveLeases))
	for _, id := range liveLeases {
		live[id] = true
	}
	young := map[string]bool{}
	candidates := map[string]bool{}
	running := map[string]bool{}
	containersChecked := true
	hasRoot := map[string]bool{}
	if h.Base != "" {
		if info, err := os.Lstat(h.Base); err == nil && (info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() || !ownedByCaller(info)) {
			// the base may point into another home: its entries are not evidence about this host
			return rep, leakError(ReasonUnconfined, "job root base is not a plain directory owned by the caller")
		}
		entries, err := os.ReadDir(h.Base)
		if err != nil && !os.IsNotExist(err) {
			return rep, err
		}
		for _, e := range entries {
			name := e.Name()
			if !ValidAttempt(name) || !h.isAttempt(name) {
				continue // not an attempt root (the services package keeps nselfci-* here)
			}
			hasRoot[name] = true
			if live[name] {
				continue
			}
			if h.MinAge > 0 {
				if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < h.MinAge {
					young[name] = true
					rep.Skipped = append(rep.Skipped, name)
					continue
				}
			}
			candidates[name] = true
		}
	}
	for _, kind := range Kinds {
		found, note, err := h.listResources(ctx, kind, "")
		if err != nil {
			rep.Failed["list "+string(kind)] = err
			if kind == exec.KindContainer {
				containersChecked = false
			}
			continue
		}
		if note != "" {
			rep.Skipped = append(rep.Skipped, note)
			if kind == exec.KindContainer {
				containersChecked = false
			}
		}
		for _, r := range found {
			if kind == exec.KindContainer && r.Running && !h.OwnsRuntime {
				running[r.Attempt] = true
			}
			// A labelled resource with no root here belongs to another CI home, user or agent on a
			// shared daemon unless this host owns the daemon.
			if !live[r.Attempt] && !young[r.Attempt] && (hasRoot[r.Attempt] || h.OwnsRuntime) {
				candidates[r.Attempt] = true
			}
		}
	}
	if !containersChecked && !h.OwnsRuntime {
		// without the container list a sibling's live job cannot be told from an orphan
		for name := range candidates {
			rep.Skipped = append(rep.Skipped, name)
		}
		candidates = map[string]bool{}
	}
	// A running container may belong to a sibling process; the executor's own recovery removes the
	// containers of dead coordinators, so a host that does not own the daemon leaves them alone.
	for name := range running {
		if candidates[name] {
			delete(candidates, name)
			rep.Skipped = append(rep.Skipped, name)
		}
	}
	names := make([]string, 0, len(candidates))
	for n := range candidates {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		if err := h.CleanupJob(ctx, name, h.rootOwner(name)); err != nil {
			rep.Failed[name] = err
			continue
		}
		rep.Removed = append(rep.Removed, name)
	}
	if len(rep.Failed) > 0 {
		return rep, leakError(ReasonCleanupFailed, "%d orphans could not be removed", len(rep.Failed))
	}
	return rep, nil
}
