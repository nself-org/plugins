package manifest

// Purpose: Decode, the one reader of release-manifest payload bytes.
// Inputs:  the payload of a DSSE envelope (never the envelope).
// Outputs: a validated Manifest. ErrSchema for another schema (a v2 payload is
// refused whole), ErrInvalid for anything else wrong.
// Constraints: bounded (size, depth); a repeated object key is refused so two
// readers cannot disagree on which value counts; unknown additive fields are
// accepted and dropped (X19); the decoded Manifest is not re-signed, so the
// bytes a verifier checks stay the bytes the signature covers.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const maxDepth = 16

// Decode parses and validates a release-manifest payload.
func Decode(payload []byte) (Manifest, error) {
	var m Manifest
	if len(payload) > maxPayload {
		return m, invalid("payload is larger than %d bytes", maxPayload)
	}
	if err := checkShape(payload); err != nil {
		return m, err
	}
	var head struct {
		Schema *string `json:"schema"`
	}
	if err := json.Unmarshal(payload, &head); err != nil || head.Schema == nil {
		return m, invalid("schema is missing or not a string")
	}
	if *head.Schema != Schema {
		return m, fmt.Errorf("%w: %q", ErrSchema, *head.Schema)
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return Manifest{}, invalid("%s", err)
	}
	if err := validate(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

type frame struct {
	object    bool
	expectKey bool
	keys      map[string]bool
}

// checkShape walks the tokens once: one top-level object, no repeated key,
// bounded depth, nothing after the value.
func checkShape(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	var stack []frame
	done := false
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].expectKey = true
		} else if n == 0 {
			done = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("not JSON: %s", err)
		}
		if done {
			return invalid("data after the top-level value")
		}
		if len(stack) == 0 {
			if d, ok := tok.(json.Delim); !ok || d != '{' {
				return invalid("the payload is not a JSON object")
			}
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{', '[':
				if len(stack) >= maxDepth {
					return invalid("nesting deeper than %d", maxDepth)
				}
				stack = append(stack, frame{object: t == '{', expectKey: t == '{', keys: map[string]bool{}})
			default:
				stack = stack[:len(stack)-1]
				valueDone()
			}
		case string:
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
				if stack[n-1].keys[t] {
					return invalid("key %q repeats", t)
				}
				stack[n-1].keys[t], stack[n-1].expectKey = true, false
				continue
			}
			valueDone()
		default:
			valueDone()
		}
	}
	if !done || len(stack) != 0 {
		return invalid("the payload ends early")
	}
	return nil
}
