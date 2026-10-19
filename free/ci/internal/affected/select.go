package affected

// Purpose: the affected selector (Epic D15). It maps the changed paths to
// packages, adds every package that depends on them (reverse closure) and
// selects the jobs whose working directory covers an affected package. It
// answers full, never fewer jobs, whenever it cannot prove the narrower set.
// Inputs:  plan.SelectionInput (changed paths, branch flags, jobs by
// workdir, detector facts including the pkggraph facts).
// Outputs: model.Selection{Mode, Reason, Digest} and the sorted job ids.
// Constraints: pure; release pipelines and default-branch runs are always
// full; full also on no base, an empty diff, a non-certain fact, a forcing
// path (ForcesFull), an unmapped path, an uncertain graph, or no job left.
// Without pkggraph facts the path-glob default (plan.DefaultSelector) decides.

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// Selector is the plan.Selector this package provides.
func Selector(in plan.SelectionInput) (model.Selection, []string) {
	g, present, certain := FromFacts(in.Facts)
	if !present {
		return plan.DefaultSelector(in)
	}
	return selectWith(g, certain, in)
}

func selectWith(g *Graph, graphCertain bool, in plan.SelectionInput) (model.Selection, []string) {
	all := make([]string, 0, len(in.Jobs))
	for _, r := range in.Jobs {
		all = append(all, r.ID)
	}
	sort.Strings(all)
	changed := normalize(in.Changed)
	full := func(reason model.Reason) (model.Selection, []string) {
		return model.Selection{Mode: "full", Reason: reason, Digest: Digest(changed, all)}, all
	}
	switch {
	case !in.BaseKnown:
		return full("selection.no_base")
	case in.DefaultBranch || in.Release || !graphCertain || len(changed) == 0:
		return full("selection.full_required")
	}
	for _, f := range in.Facts {
		if f.Confidence != detect.Certain {
			return full("selection.full_required")
		}
	}
	workdirs := map[string][]string{}
	for _, r := range in.Jobs {
		w := path.Clean(strings.TrimPrefix(r.Workdir, "./"))
		workdirs[w] = append(workdirs[w], r.ID)
	}
	seeds, direct := map[string]bool{}, map[string]bool{}
	for _, p := range changed {
		if ForcesFull(p) {
			return full("selection.full_required")
		}
		node, hasNode := g.owner(p)
		wd, hasWd := nearest(p, func(d string) bool { return workdirs[d] != nil })
		if !hasNode && !hasWd {
			return full("selection.full_required") // unmapped
		}
		if hasNode {
			seeds[node] = true
		}
		if hasWd {
			for _, id := range workdirs[wd] {
				direct[id] = true
			}
		}
	}
	affected := g.Dependants(keys(seeds)...)
	picked := map[string]bool{}
	for id := range direct {
		picked[id] = true
	}
	for _, r := range in.Jobs {
		w := path.Clean(strings.TrimPrefix(r.Workdir, "./"))
		for d := range affected {
			if within(d, w) || (d != "." && within(w, d)) {
				picked[r.ID] = true
				break
			}
		}
	}
	if len(picked) == 0 {
		return full("selection.full_required")
	}
	ids := keys(picked)
	return model.Selection{Mode: "affected", Reason: "selection.affected", Digest: Digest(changed, ids)}, ids
}

// owner finds the package directory a changed path belongs to: the nearest
// package directory above it. The repository root owns only the files
// directly in it, so a root manifest or main package does not claim every
// unmapped path.
func (g *Graph) owner(p string) (string, bool) {
	d, ok := nearest(p, func(d string) bool { _, has := g.eco[d]; return has })
	if ok && d == "." && dirOf(p) != "." {
		return "", false
	}
	return d, ok
}

// nearest walks from the directory of p up to the root and returns the first
// directory for which has is true.
func nearest(p string, has func(dir string) bool) (string, bool) {
	for d := dirOf(p); ; d = path.Dir(d) {
		if has(d) {
			return d, true
		}
		if d == "." || d == "/" {
			return "", false
		}
	}
}

func normalize(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = path.Clean(strings.TrimPrefix(p, "./"))
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Digest is sha256 over the sorted changed paths and the sorted selected
// job ids (Epic D15).
func Digest(changed, jobs []string) string {
	c, j := append([]string(nil), changed...), append([]string(nil), jobs...)
	sort.Strings(c)
	sort.Strings(j)
	sum := sha256.Sum256([]byte(strings.Join(c, "\x00") + "\x01" + strings.Join(j, "\x00")))
	return hex.EncodeToString(sum[:])
}
