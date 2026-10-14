package config

// Purpose: turn a JSON-schema error into the JSON pointer of the offending
// instance value. The schema library reports the schema path
// (/properties/jobs/additionalProperties/properties/kind), not the instance
// path, so the path is walked against the decoded document.
// Inputs:  the schema error and the decoded document.
// Outputs: the instance pointer ("" for the root) and the leaf message.
// Constraints: pure. Where one schema path covers several instance values
// (additionalProperties, items) the first value, in sorted key / index order,
// that the leaf message fits is chosen; the schema check still fails the
// whole document either way, so a wrong pick only moves the reported line.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	schemaStep    = regexp.MustCompile(`validating (/[^\s:]*): `)
	schemaRoot    = regexp.MustCompile(`validating https?://[^\s]+: `)
	missingProp   = regexp.MustCompile(`missing properties: \[(.*?)\]`)
	extraProp     = regexp.MustCompile(`unexpected additional properties \[(.*?)\]`)
	quotedName    = regexp.MustCompile(`"([^"]*)"`)
	combinators   = map[string]bool{"oneOf": true, "anyOf": true, "allOf": true}
	transparentKW = map[string]bool{"if": true, "then": true, "else": true, "not": true}
)

type candidate struct {
	ptr   string
	value any
}

// locate maps a schema error to an instance pointer and a leaf message.
func locate(err error, doc any) (string, string) {
	text := err.Error()
	msg := text
	if idx := schemaStep.FindAllStringIndex(text, -1); len(idx) > 0 {
		msg = text[idx[len(idx)-1][1]:]
	} else if m := schemaRoot.FindStringIndex(text); m != nil {
		msg = text[m[1]:]
	}
	path := ""
	if all := schemaStep.FindAllStringSubmatch(text, -1); len(all) > 0 {
		path = all[len(all)-1][1]
	}
	cands := []candidate{{"", doc}}
	toks := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := 0; i < len(toks) && path != ""; i++ {
		switch tok := toks[i]; {
		case tok == "properties" && i+1 < len(toks):
			i++
			cands = child(cands, func(m map[string]any) []string { return keep(m, toks[i]) })
		case tok == "additionalProperties", tok == "patternProperties":
			cands = child(cands, func(m map[string]any) []string { return sortedKeys(m) })
		case tok == "items":
			cands = items(cands)
		case combinators[tok] && i+1 < len(toks):
			i++
		case transparentKW[tok]:
		}
	}
	return pick(cands, msg), cleanMessage(msg)
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

// pick chooses the candidate the leaf message describes.
func pick(cands []candidate, msg string) string {
	if len(cands) == 0 {
		return ""
	}
	if m := extraProp.FindStringSubmatch(msg); m != nil {
		for _, c := range cands {
			if obj, ok := c.value.(map[string]any); ok {
				for _, q := range quotedName.FindAllStringSubmatch(m[1], -1) {
					if _, has := obj[q[1]]; has {
						return c.ptr + "/" + escape(q[1])
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
						return c.ptr
					}
				}
			}
		}
	}
	// "enum: weird does not equal ...", "type: 5 has type ...": the value sits
	// between the keyword and the first verb.
	lead := msg
	for _, verb := range []string{" does not", " has type", " has length", " must ", " is "} {
		if i := strings.Index(lead, verb); i >= 0 {
			lead = lead[:i]
		}
	}
	for _, c := range cands {
		if _, isObj := c.value.(map[string]any); isObj {
			continue
		}
		if strings.HasSuffix(lead, " "+fmt.Sprint(c.value)) {
			return c.ptr
		}
	}
	return cands[0].ptr
}

// cleanMessage drops the schema-path noise from a leaf message.
func cleanMessage(msg string) string {
	return strings.TrimSpace(strings.TrimPrefix(msg, "schema validation at "))
}
