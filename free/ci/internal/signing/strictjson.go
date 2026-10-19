package signing

// Purpose: refuse JSON that two parsers could read differently, before a
// payload is checked or signed (Epic CACHE D12): the signer checks what Go
// decodes and signs the raw bytes, so every other reader must see the same.
// Inputs:  one JSON document.
// Outputs: nil or an E717 payload_rejected error.
// Constraints: valid UTF-8; nesting at most maxDepth; no object has two keys
// that are equal or equal under Unicode simple case folding (encoding/json
// matches struct fields case-insensitively and keeps the last duplicate); no
// data after the one top-level value.

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode"
	"unicode/utf8"
)

const maxDepth = 32

// fold maps a rune to the smallest member of its simple-fold orbit.
func fold(r rune) rune {
	m := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		m = min(m, f)
	}
	return m
}

func foldKey(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		b = append(b, fold(r))
	}
	return string(b)
}

// frame is one open container of the walk.
type frame struct {
	object bool
	key    bool // an object frame: the next string is a key
	seen   map[string]bool
}

// strictJSON walks the token stream of raw.
func strictJSON(raw []byte, what string) error {
	bad := func(why string) error { return Failf(ReasonPayloadRejected, "%s: %s", what, why) }
	if !utf8.Valid(raw) {
		return bad("not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var stack []frame
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].key = true
		}
	}
	for first := true; first || len(stack) > 0; first = false {
		tok, err := dec.Token()
		if err != nil {
			return bad("not a valid JSON document")
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, frame{object: true, key: true, seen: map[string]bool{}})
			case '[':
				stack = append(stack, frame{})
			default:
				stack = stack[:len(stack)-1]
				valueDone()
			}
			if len(stack) > maxDepth {
				return bad("nested too deeply")
			}
		case string:
			n := len(stack)
			if n == 0 || !stack[n-1].object || !stack[n-1].key {
				valueDone()
				break
			}
			k := foldKey(t)
			if stack[n-1].seen[k] {
				return bad("duplicate or case-variant object key")
			}
			stack[n-1].seen[k] = true
			stack[n-1].key = false
		default:
			valueDone()
		}
	}
	if _, err := dec.Token(); err != io.EOF {
		return bad("data after the document")
	}
	return nil
}
