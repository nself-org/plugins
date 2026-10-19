package plan

// Purpose: the selector hook (SCHED F14). A Selector decides which jobs an
// affected run needs; Build calls it, then re-adds every required deep job
// on default-branch and release plans, whatever the selector returned.
// Inputs:  SelectionInput (changed paths, branch flags, release pipeline, job refs, facts).
// Outputs: a model.Selection and the sorted selected job ids (all when full).
// Constraints: SetSelector(nil) restores DefaultSelector; the selector is
// process-wide state guarded by a mutex; a bad selector answer degrades to
// full, never to fewer jobs.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// JobRef names a candidate job and the directory it runs in.
type JobRef struct{ ID, Workdir string }

// SelectionInput is what a Selector sees.
type SelectionInput struct {
	Changed                           []string
	BaseKnown, DefaultBranch, Release bool
	ReleasePipeline                   bool // --pipeline names the release pipeline: every selector answers full
	Jobs                              []JobRef
	Facts                             []detect.Fact
}

// Selector picks the jobs to run; jobs is sorted and holds every job when
// the mode is full.
type Selector func(in SelectionInput) (sel model.Selection, jobs []string)

var (
	selMu    sync.RWMutex
	selector Selector = DefaultSelector
)

// SetSelector installs s; nil restores the path-glob default.
func SetSelector(s Selector) {
	selMu.Lock()
	defer selMu.Unlock()
	if s == nil {
		s = DefaultSelector
	}
	selector = s
}

func currentSelector() Selector {
	selMu.RLock()
	defer selMu.RUnlock()
	return selector
}

// runSelector calls the selector and normalises its answer: an invalid mode
// means full, a full answer selects every job, unknown ids are dropped. A
// digest the selector set survives; otherwise Build computes it over the
// final job set.
func runSelector(in SelectionInput) (model.Selection, map[string]bool) {
	sel, ids := currentSelector()(in)
	known := map[string]bool{}
	for _, r := range in.Jobs {
		known[r.ID] = true
	}
	picked := map[string]bool{}
	if sel.Mode != "affected" || !sel.Reason.Valid() {
		sel.Digest = "" // the selector's digest described an answer that no longer stands
		sel.Mode = "full"
		if sel.Reason != "selection.no_base" {
			sel.Reason = "selection.full_required"
		}
		for id := range known {
			picked[id] = true
		}
	} else {
		for _, id := range ids {
			if known[id] {
				picked[id] = true
			}
		}
	}
	return sel, picked
}

// selectionDigest is sha256 over the sorted changed paths and the sorted
// selected job ids (Epic D15), the same shape as affected.Digest.
func selectionDigest(changed []string, picked map[string]bool) string {
	ids := make([]string, 0, len(picked))
	for id := range picked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	paths := append([]string(nil), changed...)
	sort.Strings(paths)
	sum := sha256.Sum256([]byte(strings.Join(paths, "\x00") + "\x01" + strings.Join(ids, "\x00")))
	return hex.EncodeToString(sum[:])
}
