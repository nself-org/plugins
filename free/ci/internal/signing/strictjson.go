package signing

// Purpose: refuse JSON that two parsers could read differently, before a
// payload is checked or signed (Epic CACHE D12): the signer checks what Go
// decodes and signs the raw bytes, so every other reader must see the same.
// Inputs:  one JSON document.
// Outputs: nil or an E717 payload_rejected error.
// Constraints: contract keys are spelled exactly; valid UTF-8; nesting at most maxDepth; no object has two keys
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

// strictJSON walks the token stream of raw. known lists the keys the contract
// names: a key that folds to one of them but is spelled differently is refused
// (encoding/json would read it as the contract key; a case-sensitive reader
// would not see it).
func strictJSON(raw []byte, what string, known ...string) error {
	bad := func(why string) error { return Failf(ReasonPayloadRejected, "%s: %s", what, why) }
	exact := make(map[string]string, len(known))
	for _, k := range known {
		exact[foldKey(k)] = k
	}
	if !utf8.Valid(raw) {
		return bad("not valid UTF-8")
	}
	if err := rejectAmbiguousEscapes(raw, what); err != nil {
		return err
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
			if want, ok := exact[k]; ok && want != t {
				return bad("object key " + echo(t) + " is spelled differently from the contract key " + want)
			}
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

// rejectAmbiguousEscapes refuses the escapes two parsers read differently: a
// \uD800-\uDFFF escape that is not half of a valid pair (encoding/json turns it
// into U+FFFD, other parsers keep the lone surrogate) and \u0000 (a C-string
// parser stops at it), so one payload would name two different strings.
func rejectAmbiguousEscapes(raw []byte, what string) error {
	bad := Failf(ReasonPayloadRejected, "%s: an unpaired UTF-16 surrogate escape or a \\u0000 escape", what)
	inString := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inString {
			inString = c == '"'
			continue
		}
		if c == '"' {
			inString = false
			continue
		}
		if c != '\\' {
			continue
		}
		if i+1 >= len(raw) || raw[i+1] != 'u' {
			i++ // the escaped byte is never a quote that ends the string
			continue
		}
		r, ok := hex4(raw, i+2)
		if !ok {
			i += 5
			continue
		}
		if r == 0 || (r >= 0xDC00 && r <= 0xDFFF) {
			return bad
		}
		if r >= 0xD800 && r <= 0xDBFF {
			if !lowSurrogateAt(raw, i+6) {
				return bad
			}
			i += 6 // the low half's escape is read here, not again
		}
		i += 5
	}
	return nil
}

// lowSurrogateAt reports whether raw[at:] starts with a \uDC00-\uDFFF escape.
func lowSurrogateAt(raw []byte, at int) bool {
	if at+6 > len(raw) || raw[at] != '\\' || raw[at+1] != 'u' {
		return false
	}
	low, ok := hex4(raw, at+2)
	return ok && low >= 0xDC00 && low <= 0xDFFF
}

// hex4 reads four hex digits at raw[at:].
func hex4(raw []byte, at int) (uint16, bool) {
	if at+4 > len(raw) {
		return 0, false
	}
	var v uint16
	for _, c := range raw[at : at+4] {
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 | uint16(c-'0')
		case c >= 'a' && c <= 'f':
			v = v<<4 | uint16(c-'a'+10)
		case c >= 'A' && c <= 'F':
			v = v<<4 | uint16(c-'A'+10)
		default:
			return 0, false
		}
	}
	return v, true
}
