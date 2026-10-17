package plan

// Purpose: the default affected selection: map changed paths to jobs by the
// job's workdir, and fall back to a full run whenever the mapping is not
// certain.
// Inputs:  SelectionInput.
// Outputs: selection.no_base (full) without a base; selection.full_required
// (full) when the diff is empty, a config or lockfile changed, a path maps to no job, or a
// detector fact is uncertain; otherwise selection.affected with the jobs
// whose workdir owns a changed path (longest workdir prefix wins).
// Constraints: pure; never selects fewer jobs than the evidence proves.

import (
	"path"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var lockBases = map[string]bool{
	"go.sum": true, "go.work": true, "go.work.sum": true, "package-lock.json": true, "npm-shrinkwrap.json": true,
	"pnpm-lock.yaml": true, "yarn.lock": true, "bun.lock": true, "bun.lockb": true,
	"Cargo.lock": true, "pubspec.lock": true,
}

// DefaultSelector is the path-glob selector Build uses until SetSelector.
func DefaultSelector(in SelectionInput) (model.Selection, []string) {
	all := make([]string, 0, len(in.Jobs))
	for _, r := range in.Jobs {
		all = append(all, r.ID)
	}
	sort.Strings(all)
	full := func(reason model.Reason) (model.Selection, []string) {
		return model.Selection{Mode: "full", Reason: reason}, all
	}
	if !in.BaseKnown {
		return full("selection.no_base")
	}
	if len(in.Changed) == 0 {
		return full("selection.full_required") // an empty diff proves nothing: run all
	}
	for _, f := range in.Facts {
		if f.Confidence != detect.Certain {
			return full("selection.full_required")
		}
	}
	dirs := map[string][]string{}
	for _, r := range in.Jobs {
		w := cleanDir(r.Workdir)
		dirs[w] = append(dirs[w], r.ID)
	}
	hit := map[string]bool{}
	for _, raw := range in.Changed {
		p := path.Clean(strings.TrimPrefix(raw, "./"))
		if isConfigOrLock(p) {
			return full("selection.full_required")
		}
		owner, ok := owningDir(p, dirs)
		if !ok {
			return full("selection.full_required")
		}
		for _, id := range dirs[owner] {
			hit[id] = true
		}
	}
	jobs := make([]string, 0, len(hit))
	for id := range hit {
		jobs = append(jobs, id)
	}
	sort.Strings(jobs)
	return model.Selection{Mode: "affected", Reason: "selection.affected"}, jobs
}

func cleanDir(w string) string {
	w = path.Clean(strings.TrimPrefix(w, "./"))
	if w == "" {
		return "."
	}
	return w
}

func isConfigOrLock(p string) bool {
	return p == ".nself/ci.yaml" || strings.HasPrefix(p, ".nself/") || lockBases[path.Base(p)]
}

// owningDir returns the longest workdir that contains p.
func owningDir(p string, dirs map[string][]string) (string, bool) {
	best, found := "", false
	for d := range dirs {
		in := d == "." || p == d || strings.HasPrefix(p, d+"/")
		if in && (!found || specificity(d) > specificity(best)) {
			best, found = d, true
		}
	}
	return best, found
}

// specificity ranks workdirs: the repo root is the least specific.
func specificity(dir string) int {
	if dir == "." {
		return 0
	}
	return len(dir)
}
