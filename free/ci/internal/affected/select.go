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
	return selectWith(g, present, certain, in)
}

func selectWith(g *Graph, present, graphCertain bool, in plan.SelectionInput) (model.Selection, []string) {
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
	case in.DefaultBranch || in.Release || len(changed) == 0:
		return full("selection.full_required")
	}
	for _, f := range in.Facts {
		if f.Confidence != detect.Certain {
			return full("selection.full_required")
		}
	}
	for _, p := range changed {
		if ForcesFull(p) {
			return full("selection.full_required")
		}
	}
	if !present {
		return plan.DefaultSelector(in) // no Go or pnpm packages: the path-glob rules decide
	}
	if !graphCertain {
		return full("selection.full_required")
	}
	workdirs := map[string][]string{}
	for _, r := range in.Jobs {
		w := path.Clean(strings.TrimPrefix(r.Workdir, "./"))
		workdirs[w] = append(workdirs[w], r.ID)
	}
	seeds, picked := map[string]bool{}, map[string]bool{}
	for _, p := range changed {
		if g.inGeneratorZone(p) {
			return full("selection.full_required")
		}
		dir, kind := g.owner(p)
		covered := false
		for w, ids := range workdirs { // every job whose workdir holds the path
			if within(dirOf(p), w) {
				covered = covered || w != "."
				for _, id := range ids {
					picked[id] = true
				}
			}
		}
		switch kind {
		case ownOpaque:
			return full("selection.full_required")
		case ownNode:
			seeds[dir] = true
		default:
			if _, root := g.eco["."]; root {
				seeds["."] = true // a root package may embed any file under it
			}
			if !covered {
				return full("selection.full_required") // unmapped: only the root job or nothing holds it
			}
		}
	}
	affected := g.Dependants(keys(seeds)...)
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

const (
	ownNone = iota
	ownNode
	ownOpaque
)

// rootSource are the extensions of files directly in the repository root
// that a root package owns; any other root file is unowned (unmapped).
var rootSource = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".mts": true, ".cts": true, ".vue": true, ".svelte": true, ".css": true, ".scss": true,
}

// owner finds what a changed path belongs to: the nearest directory above it
// that is a package or an opaque manifest directory (opaque wins on a tie).
// The repository root owns only the source files directly in it (a root
// package) or, when opaque, everything not owned deeper.
func (g *Graph) owner(p string) (string, int) {
	for d := dirOf(p); ; d = path.Dir(d) {
		if _, ok := g.opaque[d]; ok {
			return d, ownOpaque
		}
		if _, ok := g.eco[d]; ok && (d != "." || (dirOf(p) == "." && rootSource[path.Ext(p)])) {
			return d, ownNode
		}
		if d == "." || d == "/" {
			return "", ownNone
		}
	}
}

func (g *Graph) inGeneratorZone(p string) bool {
	for d := range g.generators {
		if within(dirOf(p), d) {
			return true
		}
	}
	return false
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
