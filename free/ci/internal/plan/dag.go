package plan

// Purpose: DAG validation and layering.
// Inputs:  candidate jobs (validation) or expanded entries (layering).
// Outputs: E616 for a dangling dep or a cycle (naming the jobs), E617 for a
// required job depending on an allow_failure job; topological layers; the
// transitive dependants of every node (the fail-fast propagation set).
// Constraints: deterministic: ids are visited in sorted order and a layer is
// ordered by priority (high first) then id.

import (
	"sort"
	"strings"
)

// validateGraph checks the whole candidate graph before anything is planned.
func validateGraph(cs map[string]*cand, ids []string) error {
	var dangling []string
	names := map[string]bool{}
	for _, id := range ids {
		for _, d := range cs[id].job.Deps {
			if cs[d] == nil {
				dangling = append(dangling, id+" -> "+d)
				names[id], names[d] = true, true
			}
		}
	}
	if len(dangling) > 0 {
		return fail(CodeCycle, "dependency on an undefined job: "+strings.Join(dangling, ", "), keys(names))
	}
	if cyc := findCycle(cs, ids); len(cyc) > 0 {
		return fail(CodeCycle, "dependency cycle: "+strings.Join(append(cyc, cyc[0]), " -> "), sortedUnique(cyc))
	}
	for _, id := range ids {
		if !cs[id].required {
			continue
		}
		if chain := allowFailureChain(cs, id); chain != nil {
			via := ""
			if len(chain) > 2 {
				via = " (via " + strings.Join(chain[1:len(chain)-1], " -> ") + ")"
			}
			return fail(CodeRequiredAllowFailure, "required job "+id+" depends on allow_failure job "+chain[len(chain)-1]+via, sortedUnique(chain))
		}
	}
	return nil
}

// allowFailureChain returns the first dependency path (id first) from id to
// an allow_failure job, direct or transitive, or nil. A failing
// allow_failure job still blocks everything that waits on it, so a required
// job may not reach one through any chain.
func allowFailureChain(cs map[string]*cand, id string) []string {
	seen := map[string]bool{}
	var walk func(cur string, path []string) []string
	walk = func(cur string, path []string) []string {
		for _, d := range cs[cur].job.Deps {
			if seen[d] {
				continue
			}
			seen[d] = true
			next := append(append([]string(nil), path...), d)
			if cs[d].allowFail {
				return next
			}
			if found := walk(d, next); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(id, []string{id})
}

// findCycle returns one cycle as an ordered job list starting at its
// smallest id, or nil.
func findCycle(cs map[string]*cand, ids []string) []string {
	state := map[string]int{} // 1 visiting, 2 done
	var stack []string
	var found []string
	var visit func(id string) bool
	visit = func(id string) bool {
		state[id] = 1
		stack = append(stack, id)
		for _, d := range cs[id].job.Deps {
			switch state[d] {
			case 1:
				for i, s := range stack {
					if s == d {
						found = append([]string(nil), stack[i:]...)
					}
				}
				return true
			case 0:
				if visit(d) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return false
	}
	for _, id := range ids {
		if state[id] == 0 && visit(id) {
			break
		}
	}
	if len(found) == 0 {
		return nil
	}
	low := 0
	for i, s := range found {
		if s < found[low] {
			low = i
		}
	}
	return append(append([]string(nil), found[low:]...), found[:low]...)
}

// closureOf returns the seeds and all their transitive deps.
func closureOf(cs map[string]*cand, seeds []string) map[string]bool {
	out := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		if out[id] || cs[id] == nil {
			return
		}
		out[id] = true
		for _, d := range cs[id].job.Deps {
			walk(d)
		}
	}
	for _, s := range seeds {
		walk(s)
	}
	return out
}

// node is one expanded entry in the layered graph.
type node struct {
	id   string
	deps []string
	prio int
}

func prioRank(p string) int {
	switch p {
	case "high":
		return 0
	case "low":
		return 2
	}
	return 1
}

// layer assigns each node the length of its longest dependency chain and
// returns the layers in order. A leftover node means a cycle (E616).
func layer(nodes map[string]*node) (map[string]int, [][]string, error) {
	at := map[string]int{}
	left := map[string]int{}
	rev := map[string][]string{}
	for id, n := range nodes {
		left[id] = len(n.deps)
		for _, d := range n.deps {
			rev[d] = append(rev[d], id)
		}
	}
	var ready []string
	for id, c := range left {
		if c == 0 {
			ready = append(ready, id)
		}
	}
	done := 0
	var layers [][]string
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			a, b := nodes[ready[i]], nodes[ready[j]]
			if a.prio != b.prio {
				return a.prio < b.prio
			}
			return a.id < b.id
		})
		layers = append(layers, ready)
		var next []string
		for _, id := range ready {
			at[id] = len(layers) - 1
			done++
			for _, c := range rev[id] {
				left[c]--
				if left[c] == 0 {
					next = append(next, c)
				}
			}
		}
		ready = next
	}
	if done != len(nodes) {
		return nil, nil, fail(CodeCycle, "dependency cycle among expanded jobs", nil)
	}
	return at, layers, nil
}

// dependants returns every node's transitive dependants, sorted.
func dependants(nodes map[string]*node) map[string][]string {
	rev := map[string][]string{}
	for id, n := range nodes {
		for _, d := range n.deps {
			rev[d] = append(rev[d], id)
		}
	}
	out := map[string][]string{}
	for id := range nodes {
		seen := map[string]bool{}
		stack := append([]string(nil), rev[id]...)
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[x] {
				continue
			}
			seen[x] = true
			stack = append(stack, rev[x]...)
		}
		out[id] = keys(seen)
	}
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
