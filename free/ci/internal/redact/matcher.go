package redact

// Purpose: a compact Aho-Corasick automaton over byte patterns, used for the
// registered-value families. Patterns are exact bytes; the base64 family is
// scanned with line breaks ignored (scanSkip).
// Inputs:  pattern byte strings. Outputs: matched spans and the length of the
// longest suffix of the scanned bytes that is a proper prefix of a pattern
// (the bytes a streaming caller must keep).
// Constraints: the root has a dense table, deeper nodes keep sorted edge lists,
// so memory is proportional to the total pattern bytes.

import (
	"sort"
	"sync"
)

type span struct{ s, e int }

// edge is one labelled transition of a node.
type edge struct {
	b  byte
	to int32
}

// node is one trie state with its failure link and match data.
type node struct {
	edges []edge
	fail  int32
	depth int32
	out   int32 // length of the longest pattern that is a suffix of this node's string
	dict  int32 // nearest node on the failure chain that ends a pattern (0 none)
	hold  int32 // nearest node on the chain (self included) that has children: the longest partial match
	term  bool
}

type automaton struct {
	root   [256]int32
	nodes  []node
	maxLen int
}

// child returns the transition on c.
func (n *node) child(c byte) (int32, bool) {
	for _, e := range n.edges {
		if e.b == c {
			return e.to, true
		}
	}
	return 0, false
}

// build returns nil for an empty pattern set.
func build(patterns [][]byte) *automaton {
	if len(patterns) == 0 {
		return nil
	}
	a := &automaton{nodes: make([]node, 1, 64)}
	for _, p := range patterns {
		if len(p) == 0 {
			continue
		}
		if len(p) > a.maxLen {
			a.maxLen = len(p)
		}
		s := int32(0)
		for i, c := range p {
			var next int32
			if s == 0 {
				next = a.root[c]
			} else {
				next, _ = a.nodes[s].child(c)
			}
			if next == 0 {
				a.nodes = append(a.nodes, node{depth: int32(i + 1)})
				next = int32(len(a.nodes) - 1)
				if s == 0 {
					a.root[c] = next
				} else {
					a.nodes[s].edges = append(a.nodes[s].edges, edge{c, next})
				}
			}
			s = next
		}
		a.nodes[s].term = true
	}
	for i := range a.nodes {
		e := a.nodes[i].edges
		sort.Slice(e, func(x, y int) bool { return e[x].b < e[y].b })
	}
	a.link()
	return a
}

// link computes failure links, output lengths and hold depths in BFS order.
func (a *automaton) link() {
	queue := make([]int32, 0, len(a.nodes))
	for c := 0; c < 256; c++ {
		if v := a.root[c]; v != 0 {
			queue = append(queue, v)
		}
	}
	for _, v := range queue {
		a.finishOne(v)
	}
	for h := 0; h < len(queue); h++ {
		u := queue[h]
		for _, e := range a.nodes[u].edges {
			f := a.nodes[u].fail
			for f != 0 {
				if _, ok := a.nodes[f].child(e.b); ok {
					break
				}
				f = a.nodes[f].fail
			}
			var t int32
			if f == 0 {
				t = a.root[e.b]
			} else {
				t, _ = a.nodes[f].child(e.b)
			}
			if t == e.to {
				t = 0
			}
			a.nodes[e.to].fail = t
			queue = append(queue, e.to)
			a.finishOne(e.to)
		}
	}
}

func (a *automaton) finishOne(v int32) {
	n := &a.nodes[v]
	f := &a.nodes[n.fail]
	n.out = f.out
	if n.term {
		n.out = n.depth
	}
	n.dict = f.dict
	if f.term {
		n.dict = n.fail
	}
	n.hold = f.hold
	if len(n.edges) > 0 {
		n.hold = v
	}
}

// step follows failure links until a transition on c exists.
func (a *automaton) step(s int32, c byte) int32 {
	for {
		if s == 0 {
			return a.root[c]
		}
		if t, ok := a.nodes[s].child(c); ok {
			return t
		}
		s = a.nodes[s].fail
	}
}

// find appends the span of the longest pattern ending at each position.
// hold is the number of trailing bytes that may begin a longer match.
func (a *automaton) find(b []byte, out []span) (spans []span, hold int) {
	s := int32(0)
	for i, c := range b {
		s = a.step(s, c)
		if o := a.nodes[s].out; o > 0 {
			out = append(out, span{i + 1 - int(o), i + 1})
		}
	}
	return out, int(a.nodes[a.nodes[s].hold].depth)
}

func isBreak(c byte) bool { return c == '\n' || c == '\r' }

// spanCap bounds how many line-break bytes a base64 match may swallow: one
// break of two bytes per 16 characters, which covers any wrap width of 32 or more.
func spanCap(n int) int { return n + 2*(n/16+2) }

// findSkip is find with CR and LF ignored inside a match. A match whose span
// exceeds spanCap is dropped, which keeps the carry-over window bounded. The
// original offsets of the last maxLen non-break bytes are kept in a ring once
// the first break is seen, so finding a match start costs O(1).
func (a *automaton) findSkip(b []byte, out []span) (spans []span, hold int) {
	s := int32(0)
	var ring []int
	count := 0
	pos := func(k int) int { // original offset of the k-th non-break byte
		if ring == nil {
			return k
		}
		return ring[k%a.maxLen]
	}
	for i, c := range b {
		if isBreak(c) {
			if ring == nil {
				ring = ringPool.Get(a.maxLen)
				for k := max(0, count-a.maxLen); k < count; k++ {
					ring[k%a.maxLen] = k
				}
			}
			continue
		}
		if ring != nil {
			ring[count%a.maxLen] = i
		}
		count++
		s = a.step(s, c)
		t := s
		if !a.nodes[t].term {
			t = a.nodes[t].dict
		}
		for ; t != 0; t = a.nodes[t].dict { // longest first; a shorter one stands in when the cap rejects it
			o := int(a.nodes[t].depth)
			if st := pos(count - o); i+1-st <= spanCap(o) {
				out = append(out, span{st, i + 1})
				break
			}
		}
	}
	for h := a.nodes[s].hold; h != 0; h = a.nodes[a.nodes[h].fail].hold {
		d := int(a.nodes[h].depth)
		if st := pos(count - d); len(b)-st <= spanCap(a.maxLen) { // a partial may still grow into the longest pattern
			hold = len(b) - st
			break
		}
	}
	if ring != nil {
		ringPool.Put(ring)
	}
	return out, hold
}

// ringPool recycles the offset rings so a chunk with line breaks does not
// allocate one per call.
type ringBuffers struct{ p sync.Pool }

var ringPool ringBuffers

func (r *ringBuffers) Get(n int) []int {
	if v, ok := r.p.Get().(*[]int); ok && cap(*v) >= n {
		return (*v)[:n]
	}
	return make([]int, n)
}

func (r *ringBuffers) Put(b []int) { r.p.Put(&b) }
