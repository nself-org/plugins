package run

// Purpose: the OffloadHook implementation for dev runs (P7-AUTO-13): a dirty
// tree becomes a snapshot revision, without run importing internal/offload.
// Inputs:  the request's Options.Root and the Revision GitRevision read.
// Outputs: an OffloadResult whose Rev is the snapshot and Release deletes the
// private ref; the excluded names go to Notes.
// Constraints: a clean revision or a nil Build keeps the HEAD path; a Build
// error refuses the run, so a node never tests a tree other than the user's.
// A snapshot run is a dev run: unbound evidence, never a status of record.

import (
	"context"
	"fmt"
	"strings"
)

// OffloadSnapshot is what the snapshot builder returns.
type OffloadSnapshot struct {
	SHA      string
	Excluded []string
	Release  func(context.Context) error
}

// SnapshotOffload is the OffloadHook backed by an injected snapshot builder.
type SnapshotOffload struct {
	// Build makes the snapshot of a dirty root (internal/offload via cmd wiring).
	Build func(ctx context.Context, root string) (*OffloadSnapshot, error)
	// Excluded receives the names left out of the snapshot (nil: dropped).
	Excluded func(names []string)
}

// Offload implements OffloadHook.
func (s SnapshotOffload) Offload(ctx context.Context, r OffloadRequest) (OffloadResult, error) {
	if !r.Rev.Dirty || s.Build == nil {
		return OffloadResult{}, nil
	}
	var snap *OffloadSnapshot
	if err := guard("offload hook", func() (e error) { snap, e = s.Build(ctx, r.Opts.Root); return e }); err != nil {
		return OffloadResult{}, err
	}
	if snap == nil || !fullSHA.MatchString(snap.SHA) {
		return OffloadResult{}, nil
	}
	if s.Excluded != nil && len(snap.Excluded) > 0 {
		s.Excluded(snap.Excluded)
	}
	res := OffloadResult{Rev: Revision{SHA: snap.SHA, Dirty: true}}
	if snap.Release != nil {
		res.Release = func() { _ = snap.Release(context.WithoutCancel(ctx)) }
	}
	return res, nil
}

// StatusOfRecord is always false for a snapshot run (CI-37 dirty-tree rule).
func StatusOfRecord(r Revision) bool { return !r.Dirty }

// DevSummary is the local summary of a failing dev job: the gate's exit code
// and the log tail.
func DevSummary(job string, exit int, tail string) string {
	return fmt.Sprintf("%s failed (exit %d)\n%s", job, exit, strings.TrimRight(tail, "\n"))
}
