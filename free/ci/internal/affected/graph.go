// Package affected is the pure package-graph detector and affected-job
// selector of Epic D15 (P7-SCHED-13).
package affected

// Purpose: the dependency graph of the packages in a repository (Go import
// graph, pnpm workspace graph) and its fact encoding. The detector emits
// `pkggraph` facts; the selector decodes them, so the planner needs no new
// input beyond the facts it already receives.
// Inputs:  an fs.FS snapshot (Build) or detect facts (FromFacts).
// Outputs: Graph with package directories and reverse edges; Facts.
// Constraints: pure (no os, exec or net); deterministic; a graph that cannot
// be trusted carries uncertain reasons, which become Unknown facts and make
// the selector return a full run.

import (
	"io/fs"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// FactKind is the detect fact kind this package registers.
const FactKind = "pkggraph"

const (
	nodePrefix = "node:"
	edgePrefix = "edge:"
)

// Graph holds package directories and, per directory, the directories that
// depend on it (reverse edges).
type Graph struct {
	eco       map[string]string   // package dir -> ecosystems ("go", "pnpm", "go+pnpm")
	rdeps     map[string][]string // dependency dir -> importer dirs
	edges     map[[2]string]bool
	reasons   []detect.Fact // Unknown facts
	truncated bool
}

func newGraph() *Graph {
	return &Graph{eco: map[string]string{}, rdeps: map[string][]string{}, edges: map[[2]string]bool{}}
}

func (g *Graph) addNode(dir, eco string) {
	switch cur := g.eco[dir]; {
	case cur == "":
		g.eco[dir] = eco
	case !strings.Contains(cur, eco):
		g.eco[dir] = cur + "+" + eco
	}
}

// addEdge records that from depends on to.
func (g *Graph) addEdge(from, to string) {
	k := [2]string{from, to}
	if from == to || g.edges[k] {
		return
	}
	g.edges[k] = true
	g.rdeps[to] = append(g.rdeps[to], from)
}

func (g *Graph) uncertain(p, reason string) {
	g.reasons = append(g.reasons, detect.Fact{Kind: FactKind, Name: "unknown", Path: p, Confidence: detect.Unknown, Reason: reason})
}

// Build reads the graph from a repository snapshot.
func Build(fsys fs.FS) (*Graph, error) {
	t, err := walk(fsys)
	if err != nil {
		return nil, err
	}
	g := newGraph()
	if t.truncated {
		g.uncertain(".", "more than 50000 files")
	}
	goGraph(fsys, t, g)
	pnpmGraph(fsys, t, g)
	return g, nil
}

// Facts encodes the graph: one `node:<eco>` fact per package directory, one
// `edge:<dependency dir>` fact per edge (Path is the importer), and one
// Unknown fact per reason the graph cannot be trusted. Sorted.
func (g *Graph) Facts() []detect.Fact {
	var out []detect.Fact
	for dir, eco := range g.eco {
		out = append(out, detect.Fact{Kind: FactKind, Name: nodePrefix + eco, Path: dir, Confidence: detect.Certain})
	}
	for k := range g.edges {
		out = append(out, detect.Fact{Kind: FactKind, Name: edgePrefix + k[1], Path: k[0], Confidence: detect.Certain})
	}
	out = append(out, g.reasons...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Name < b.Name
	})
	return out
}

// FromFacts decodes the pkggraph facts among facts. present is false when
// there are none; certain is false when any pkggraph fact is not Certain.
func FromFacts(facts []detect.Fact) (g *Graph, present, certain bool) {
	g, certain = newGraph(), true
	for _, f := range facts {
		if f.Kind != FactKind {
			continue
		}
		present = true
		switch {
		case f.Confidence != detect.Certain:
			certain = false
		case strings.HasPrefix(f.Name, nodePrefix):
			g.addNode(f.Path, strings.TrimPrefix(f.Name, nodePrefix))
		case strings.HasPrefix(f.Name, edgePrefix):
			g.addEdge(f.Path, strings.TrimPrefix(f.Name, edgePrefix))
		default:
			certain = false // a shape this version does not know
		}
	}
	return g, present, certain
}

// Dependants returns dirs and every directory that depends on them,
// transitively (reverse closure), as a set that includes dirs.
func (g *Graph) Dependants(dirs ...string) map[string]bool {
	seen := map[string]bool{}
	queue := append([]string(nil), dirs...)
	for len(queue) > 0 {
		d := queue[0]
		queue = queue[1:]
		if seen[d] {
			continue
		}
		seen[d] = true
		queue = append(queue, g.rdeps[d]...)
	}
	return seen
}

// withoutEdge returns a copy of g that lacks the edge from -> to (the
// mutation the reverse-edge test applies).
func (g *Graph) withoutEdge(from, to string) *Graph {
	c := newGraph()
	c.truncated, c.reasons = g.truncated, g.reasons
	for d, e := range g.eco {
		c.eco[d] = e
	}
	for k := range g.edges {
		if k != [2]string{from, to} {
			c.addEdge(k[0], k[1])
		}
	}
	return c
}

// Detector is the pkggraph detector for the P7-CI-33 registry. It runs when
// a Go or Node check is registered.
func Detector() detect.Detector {
	return detect.Detector{ID: "pkggraph", Ecosystem: "graph", CheckIDs: []string{"go.test", "node.test"}, Detect: func(s detect.Snapshot) []detect.Fact {
		g, err := Build(s.FS)
		if err != nil {
			return []detect.Fact{{Kind: FactKind, Name: "unknown", Path: ".", Confidence: detect.Unknown, Reason: "package graph unreadable"}}
		}
		return g.Facts()
	}}
}

// Register adds the pkggraph detector to r.
func Register(r *detect.Registry) error { return r.Register(Detector()) }
