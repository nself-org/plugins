package config

// Purpose: turn a yaml.v3 node tree into plain Go values (the form the JSON
// schema validates), recording the line of every JSON pointer.
// Inputs:  a document node.
// Outputs: map[string]any / []any / scalars, a pointer -> line table and the
// problems found.
// Constraints: refuses non-string keys, duplicate keys and merge keys; an
// alias is expanded in place, so a cyclic alias (`a: &x [*x]`) or an alias
// bomb is stopped by the depth and node limits instead of exhausting memory
// or the stack. A mapping key's line wins over its value's line, so a problem
// in a block mapping points at the key.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxDepth = 64
	maxNodes = 200_000
	maxBytes = 4 << 20 // scalar bytes after alias expansion
)

type walker struct {
	file   string
	lines  map[string]int
	probs  []Problem
	budget int
	bytes  int
}

func (w *walker) bad(n *yaml.Node, ptr, msg string) {
	w.probs = append(w.probs, Problem{File: w.file, Line: n.Line, Pointer: ptr, Message: msg})
}

// inPolicy is true inside the opaque policy block, which the protected policy
// loader owns: it may hold nulls and any number.
func inPolicy(ptr string) bool { return ptr == "/policy" || strings.HasPrefix(ptr, "/policy/") }

// overLimit reports the expansion limit once.
func (w *walker) overLimit(n *yaml.Node, ptr string) {
	if len(w.probs) == 0 || w.probs[len(w.probs)-1].Message != tooBig {
		w.bad(n, ptr, tooBig)
	}
}

func (w *walker) value(n *yaml.Node, ptr string, depth int) any {
	if w.budget--; w.budget < 0 || depth > maxDepth {
		w.overLimit(n, ptr)
		return nil
	}
	if n.Kind == yaml.AliasNode {
		return w.value(n.Alias, ptr, depth+1)
	}
	w.lines[ptr] = n.Line
	switch n.Kind {
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				if k.Tag == "!!merge" {
					w.bad(k, ptr, "merge keys (<<) are not supported")
				} else {
					w.bad(k, ptr, "keys must be strings")
				}
				continue
			}
			if w.bytes -= len(k.Value); w.bytes < 0 {
				w.overLimit(k, ptr)
				return m
			}
			child := ptr + "/" + escape(k.Value)
			if _, dup := m[k.Value]; dup {
				w.bad(k, child, "duplicate key")
				continue
			}
			m[k.Value] = w.value(v, child, depth+1)
			w.lines[child] = k.Line
		}
		return m
	case yaml.SequenceNode:
		s := make([]any, len(n.Content))
		for i, c := range n.Content {
			s[i] = w.value(c, ptr+"/"+strconv.Itoa(i), depth+1)
		}
		return s
	case yaml.ScalarNode:
		if w.bytes -= len(n.Value); w.bytes < 0 {
			w.overLimit(n, ptr)
			return nil
		}
		if n.Tag == "!!timestamp" || n.Tag == "!!binary" {
			return n.Value
		}
		var v any
		if err := n.Decode(&v); err != nil {
			w.bad(n, ptr, err.Error())
			return nil
		}
		switch x := v.(type) {
		case nil:
			// `key:` with no value or `~`: only the opaque policy block may hold one.
			if !inPolicy(ptr) {
				w.bad(n, ptr, "null is not allowed here; omit the key")
			}
		case uint64:
			if !inPolicy(ptr) {
				w.bad(n, ptr, "integer is out of range")
			}
		case float64:
			if math.IsInf(x, 0) || math.IsNaN(x) {
				w.bad(n, ptr, "number must be finite")
			} else if plainInteger.MatchString(n.Value) && !inPolicy(ptr) {
				w.bad(n, ptr, "integer is out of range")
			}
		}
		return v
	}
	w.bad(n, ptr, fmt.Sprintf("unsupported YAML node kind %d", n.Kind))
	return nil
}

// plainInteger matches digits only: an integer too large for int64 is
// resolved as a float, and the typed decode would fail without a location.
var plainInteger = regexp.MustCompile(`^[+-]?[0-9][0-9_]*$`)

const tooBig = "document is too large or nests too deeply (limits apply after alias expansion)"
