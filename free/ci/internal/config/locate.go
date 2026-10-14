package config

// Purpose: turn a JSON-schema error into the JSON pointer of the offending
// instance value and a message that never repeats that value. The schema
// library reports the schema path (/properties/jobs/additionalProperties/
// properties/kind), not the instance path, so the path is walked in the
// schema tree and in the document together.
// Inputs:  the schema error, the decoded document and the parsed schema.
// Outputs: the instance pointer ("" for the root) and a message built from
// the failing keyword and the schema (allowed values, pattern, type), never
// from the value.
// Constraints: pure. Where one schema path covers several instance values
// (additionalProperties, items) each is validated against the sub-schema at
// that path and the first that fails is reported (sorted key / index order);
// a literal secret pasted where a reference belongs is therefore never echoed
// into logs or JSON envelopes, whatever the value looks like.

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

var (
	missingProp = regexp.MustCompile(`^required: missing properties: \[(.*?)\]$`)
	extraProp   = regexp.MustCompile(`^unexpected additional properties \[(.*?)\]$`)
	quotedName  = regexp.MustCompile(`"([^"]*)"`)
	keywordLead = regexp.MustCompile(`^(\w+): `)
)

type candidate struct {
	ptr   string
	value any
}

// chain splits a wrapped validation error into the schema path of its
// innermost "validating /..." layer and the leaf error. Each layer's own text
// is its message minus the wrapped error, so text inside a value cannot pose
// as a schema path.
func chain(err error) (string, string) {
	path, leaf := "", err
	for {
		inner := errors.Unwrap(leaf)
		if inner == nil {
			return path, leaf.Error()
		}
		layer := strings.TrimSuffix(leaf.Error(), ": "+inner.Error())
		if p, ok := strings.CutPrefix(layer, "validating /"); ok {
			path = "/" + p
		}
		leaf = inner
	}
}

// locate maps a schema error to an instance pointer and a safe message.
func locate(err error, doc any, root *jsonschema.Schema) (string, string) {
	path, msg := chain(err)
	cands, node := []candidate{{"", doc}}, root
	toks := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := 0; i < len(toks) && path != ""; i++ {
		switch tok := toks[i]; {
		case tok == "properties" && i+1 < len(toks):
			i++
			name := toks[i]
			cands = child(cands, func(m map[string]any) []string { return keep(m, name) })
			node = sub(node, func(s *jsonschema.Schema) *jsonschema.Schema { return s.Properties[name] })
		case tok == "additionalProperties", tok == "patternProperties":
			cands = child(cands, sortedKeys[any])
			node = sub(node, func(s *jsonschema.Schema) *jsonschema.Schema { return s.AdditionalProperties })
		case tok == "items":
			cands = items(cands)
			node = sub(node, func(s *jsonschema.Schema) *jsonschema.Schema { return s.Items })
		case (tok == "oneOf" || tok == "anyOf" || tok == "allOf") && i+1 < len(toks):
			i++
			n, _ := strconv.Atoi(toks[i])
			node = sub(node, func(s *jsonschema.Schema) *jsonschema.Schema {
				list := map[string][]*jsonschema.Schema{"oneOf": s.OneOf, "anyOf": s.AnyOf, "allOf": s.AllOf}[tok]
				if n < len(list) {
					return list[n]
				}
				return nil
			})
		case tok == "then", tok == "else":
			// Only instances that satisfy (then) or fail (else) the sibling `if` apply.
			if node != nil && node.If != nil {
				wantValid := tok == "then"
				cands = filter(cands, func(c candidate) bool { return accepts(node.If, c.value) == wantValid })
			}
			node = sub(node, func(s *jsonschema.Schema) *jsonschema.Schema {
				if tok == "then" {
					return s.Then
				}
				return s.Else
			})
		case tok == "if", tok == "not":
			node = sub(node, func(s *jsonschema.Schema) *jsonschema.Schema {
				if tok == "if" {
					return s.If
				}
				return s.Not
			})
		}
	}
	return pick(cands, msg, node)
}

func filter(in []candidate, keepIt func(candidate) bool) []candidate {
	var out []candidate
	for _, c := range in {
		if keepIt(c) {
			out = append(out, c)
		}
	}
	return out
}

// accepts reports whether the instance validates against the schema.
func accepts(s *jsonschema.Schema, v any) bool {
	res, err := s.Resolve(nil)
	return err == nil && res.Validate(jsonRoundTrip(v)) == nil
}

func sub(s *jsonschema.Schema, f func(*jsonschema.Schema) *jsonschema.Schema) *jsonschema.Schema {
	if s == nil {
		return nil
	}
	return f(s)
}

func keep(m map[string]any, name string) []string {
	if _, ok := m[name]; ok {
		return []string{name}
	}
	return nil
}

func child(in []candidate, names func(map[string]any) []string) []candidate {
	var out []candidate
	for _, c := range in {
		m, ok := c.value.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range names(m) {
			out = append(out, candidate{c.ptr + "/" + escape(k), m[k]})
		}
	}
	return out
}

func items(in []candidate) []candidate {
	var out []candidate
	for _, c := range in {
		if s, ok := c.value.([]any); ok {
			for i, v := range s {
				out = append(out, candidate{c.ptr + "/" + strconv.Itoa(i), v})
			}
		}
	}
	return out
}

// pick chooses the offending candidate and words the failure from the schema.
func pick(cands []candidate, msg string, node *jsonschema.Schema) (string, string) {
	if len(cands) == 0 {
		return "", describe(msg, node)
	}
	// Object-level failures name keys, which the message already carries.
	if m := extraProp.FindStringSubmatch(msg); m != nil {
		for _, c := range cands {
			if obj, ok := c.value.(map[string]any); ok {
				for _, q := range quotedName.FindAllStringSubmatch(m[1], -1) {
					if _, has := obj[q[1]]; has {
						return c.ptr + "/" + escape(q[1]), msg
					}
				}
			}
		}
	}
	if m := missingProp.FindStringSubmatch(msg); m != nil {
		for _, c := range cands {
			if obj, ok := c.value.(map[string]any); ok {
				for _, q := range quotedName.FindAllStringSubmatch(m[1], -1) {
					if _, has := obj[q[1]]; !has {
						return c.ptr, msg
					}
				}
			}
		}
	}
	// Value-level failure: the candidate the sub-schema rejects.
	if node != nil {
		if res, err := node.Resolve(nil); err == nil {
			for _, c := range cands {
				if res.Validate(jsonRoundTrip(c.value)) != nil {
					return c.ptr, describe(msg, node)
				}
			}
		}
	}
	return cands[0].ptr, describe(msg, node)
}

func jsonRoundTrip(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

// describe words a value-level failure from the failing keyword and the
// schema, never from the library text (which quotes the value).
func describe(msg string, node *jsonschema.Schema) string {
	m := keywordLead.FindStringSubmatch(msg)
	if m == nil || node == nil {
		if m != nil {
			return m[1] + " constraint not satisfied"
		}
		return "does not satisfy the schema"
	}
	switch m[1] {
	case "enum":
		return fmt.Sprintf("must be one of %v", node.Enum)
	case "pattern":
		return "must match the pattern " + node.Pattern
	case "type":
		types := node.Types
		if node.Type != "" {
			types = []string{node.Type}
		}
		return fmt.Sprintf("has the wrong type; want %s", strings.Join(types, " or "))
	case "const":
		if node.Const != nil {
			return fmt.Sprintf("must equal %v", *node.Const)
		}
	case "minimum":
		if node.Minimum != nil {
			return fmt.Sprintf("must be at least %v", *node.Minimum)
		}
	case "maximum":
		if node.Maximum != nil {
			return fmt.Sprintf("must be at most %v", *node.Maximum)
		}
	case "minItems":
		if node.MinItems != nil {
			return fmt.Sprintf("needs at least %d items", *node.MinItems)
		}
	case "maxItems":
		if node.MaxItems != nil {
			return fmt.Sprintf("allows at most %d items", *node.MaxItems)
		}
	}
	return m[1] + " constraint not satisfied"
}
