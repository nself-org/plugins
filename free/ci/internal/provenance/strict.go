package provenance

// Purpose: CheckStrict, the one-JSON-value check every reader of a signed
// document runs before it decodes: valid UTF-8, a single value, bounded depth
// and no repeated object key.
// Inputs:  document bytes. Outputs: nil or an error wrapping ErrInvalid.
// Constraints: a key repeats when it is equal to an earlier one after Unicode
// simple case folding, because encoding/json folds field names the same way
// ("K" U+212A joins "k"): two readers must not pick different values from one
// signed document. Pure.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalid wraps every rejection of a malformed or inconsistent document.
var ErrInvalid = errors.New("provenance: invalid")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// MaxDepth is the deepest nesting CheckStrict accepts.
const MaxDepth = 16

// foldKey maps every rune to the smallest member of its simple-fold orbit.
func foldKey(s string) string {
	return strings.Map(func(r rune) rune {
		least := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < least {
				least = f
			}
		}
		return least
	}, s)
}

type frame struct {
	object    bool
	expectKey bool
	keys      map[string]bool
}

// CheckStrict refuses b unless it is exactly one JSON value without a repeated
// key (letter case ignored) nested at most MaxDepth deep.
func CheckStrict(b []byte) error {
	if !utf8.Valid(b) {
		return invalid("document is not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var stack []frame
	done := false
	valueDone := func() {
		if n := len(stack); n == 0 {
			done = true
		} else if stack[n-1].object {
			stack[n-1].expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("%s", err)
		}
		if done {
			return invalid("data follows the JSON value")
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				if len(stack) >= MaxDepth {
					return invalid("nesting is deeper than %d", MaxDepth)
				}
				f := frame{object: t == '{', expectKey: t == '{'}
				if f.object {
					f.keys = map[string]bool{}
				}
				stack = append(stack, f)
				continue
			}
			stack = stack[:len(stack)-1]
			valueDone()
		case string:
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
				k := foldKey(t)
				if stack[n-1].keys[k] {
					return invalid("key %q repeats", t)
				}
				stack[n-1].keys[k], stack[n-1].expectKey = true, false
				continue
			}
			valueDone()
		default:
			valueDone()
		}
	}
	if !done {
		return invalid("the document ends early")
	}
	return nil
}
