package affected

// Purpose: the affected selector (Epic D15). It maps the changed paths to
// packages, adds every package that depends on them (reverse closure) and
// selects the jobs whose working directory covers an affected package. It
// answers full, never fewer jobs, whenever it cannot prove the narrower set.
// Inputs:  plan.SelectionInput (changed paths, branch flags, jobs by
// workdir, detector facts including the pkggraph facts).
// Outputs: model.Selection{Mode, Reason, Digest} and the sorted job ids.
// Constraints: pure; release pipelines and default-branch runs are always
// full (as is a release pipeline); full also on no base, an empty diff, a non-certain fact (outside
// testdata/), a forcing path (ForcesFull), a path in a generator or opaque
// directory, an unmapped path (no package owns it and no job below the
// repository root covers it), an uncertain graph, or no job left. Without
// pkggraph facts the graph is empty, so only job workdirs map paths.

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
	g, _, certain := FromFacts(in.Facts)
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
	case in.DefaultBranch || in.Release || in.ReleasePipeline || len(changed) == 0:
		return full("selection.full_required")
	}
	for _, f := range in.Facts {
		if f.Confidence != detect.Certain && notTestdata(dirOf(f.Path)) {
			return full("selection.full_required")
		}
	}
	for _, p := range changed {
		if ForcesFull(p) {
			return full("selection.full_required")
		}
	}
	if !graphCertain {
		return full("selection.full_required")
	}
	workdirs := map[string][]string{}
	for _, r := range in.Jobs {
		w := path.Clean(strings.TrimPrefix(r.Workdir, "./"))
		workdirs[w] = append(workdirs[w], r.ID)
	}
	ancestors := g.nodeAncestors()
	seeds, picked := map[string]bool{}, map[string]bool{}
	for _, p := range changed {
		if g.inGeneratorZone(p) {
			return full("selection.full_required")
		}
		if ancestors[p] {
			return full("selection.full_required") // a directory-level change (submodule, symlink swap)
		}
		dir, kind := g.owner(p)
		covered := false
		for w, ids := range workdirs {
			// every job whose workdir holds the path, and (for a path no
			// package owns) every job below its directory
			if within(dirOf(p), w) || (kind == ownNone && dirOf(p) != "." && within(w, dirOf(p))) {
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
			if !covered {
				return full("selection.full_required") // unmapped: only the root job or nothing holds it
			}
		}
		for d := dirOf(p); ; d = path.Dir(d) {
			if strings.Contains(g.eco[d], "go") {
				seeds[d] = true // a Go package may embed any file under its directory
			}
			if d == "." || d == "/" {
				break
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

// owner finds what a changed path belongs to: the nearest directory above it
// that is a package or an opaque manifest directory (opaque wins on a tie).
// The repository root owns only the .go files directly in it (a root Go
// package; root JS or TS files are nearly always shared tool config) or, when
// opaque, everything not owned deeper.
func (g *Graph) owner(p string) (string, int) {
	for d := p; ; d = path.Dir(d) { // p itself first: a submodule bump names the directory
		if _, ok := g.opaque[d]; ok {
			return d, ownOpaque
		}
		if _, ok := g.eco[d]; ok && (d != "." || (dirOf(p) == "." && path.Ext(p) == ".go")) {
			return d, ownNode
		}
		if d == "." || d == "/" {
			return "", ownNone
		}
	}
}

// nodeAncestors indexes directories strictly above package and opaque nodes once
// per selection, so changed paths take one lookup instead of scanning the graph.
func (g *Graph) nodeAncestors() map[string]bool {
	ancestors := map[string]bool{}
	add := func(node string) {
		for d := path.Dir(node); d != "." && d != "/"; d = path.Dir(d) {
			if ancestors[d] {
				break
			}
			ancestors[d] = true
		}
	}
	for d := range g.eco {
		add(d)
	}
	for d := range g.opaque {
		add(d)
	}
	return ancestors
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
