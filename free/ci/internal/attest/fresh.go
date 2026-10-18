package attest

// Purpose: decide, at read time, whether evidence still describes the current
// inputs. Freshness is computed here and never stored as truth: the
// `freshness` field of a document is ignored.
// Inputs:  the evidence and the digests of the inputs as they are now.
// Outputs: model.Freshness: current, stale (with the input named) or unknown.
// Constraints: pure; unknown never satisfies a gate (see GateAccept); a path
// that was added or removed counts as a change.

import (
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Fresh compares the digests bound in ev with the digests of the inputs now.
func Fresh(ev model.Evidence, current model.InputDigests) model.Freshness {
	switch {
	case current.Config == "":
		return freshness("unknown", "current inputs were not supplied")
	case ev.InputDigests.Config == "":
		return freshness("unknown", "evidence binds no config digest")
	case ev.InputDigests.Config != current.Config:
		return freshness("stale", "config changed")
	}
	if f := compareFiles("lockfile", ev.InputDigests.Lockfiles, current.Lockfiles); f != nil {
		return *f
	}
	if f := compareFiles("generated", ev.InputDigests.Generated, current.Generated); f != nil {
		return *f
	}
	return model.Freshness{State: "current"}
}

func freshness(state model.FreshnessState, reason string) model.Freshness {
	return model.Freshness{State: state, Reason: &reason}
}

// compareFiles returns nil when both lists bind the same paths and digests.
func compareFiles(kind string, bound, current []model.DigestRef) *model.Freshness {
	want, okW := byPath(bound)
	have, okH := byPath(current)
	if !okW || !okH {
		f := freshness("unknown", kind+" list repeats a path or has an empty path")
		return &f
	}
	paths := make([]string, 0, len(want)+len(have))
	for p := range want {
		paths = append(paths, p)
	}
	for p := range have {
		if _, ok := want[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		w, inBound := want[p]
		h, inNow := have[p]
		var change string
		switch {
		case !inBound:
			change = "added"
		case !inNow:
			change = "removed"
		case w == "" || h == "" || w != h:
			change = "changed"
		}
		if change != "" {
			f := freshness("stale", kind+" "+p+" "+change)
			return &f
		}
	}
	return nil
}

func byPath(refs []model.DigestRef) (map[string]string, bool) {
	out := make(map[string]string, len(refs))
	for _, r := range refs {
		if _, dup := out[r.Path]; dup || r.Path == "" {
			return nil, false
		}
		out[r.Path] = r.SHA256
	}
	return out, true
}
