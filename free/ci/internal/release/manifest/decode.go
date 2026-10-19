package manifest

// Purpose: Decode, the one reader of release-manifest payload bytes.
// Inputs:  the payload of a DSSE envelope (never the envelope).
// Outputs: a validated Manifest. ErrSchema for another schema (a v2 payload is
// refused whole), ErrInvalid for anything else wrong.
// Constraints: bounded (size, depth); a repeated object key is refused so two
// readers cannot disagree on which value counts; unknown additive fields are
// accepted and dropped (X19); a key that differs from a field of the contract
// only in letter case is refused, per struct of the contract (encoding/json would
// fold it onto the field, and a payload could carry two spellings; a key inside
// an ignored extension object is not a field of any struct and is left alone); every field of the contract must be
// present (a nullable one may be null), so a missing size is not a zero size;
// everything it accepts, Build writes back within the size cap; the decoded Manifest is not re-signed, so the bytes a verifier checks stay the
// bytes the signature covers.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxDepth = 16

// Decode parses and validates a release-manifest payload.
func Decode(payload []byte) (Manifest, error) {
	var m Manifest
	if len(payload) > maxPayload {
		return m, invalid("payload is larger than %d bytes", maxPayload)
	}
	if !utf8.Valid(payload) {
		return m, invalid("payload is not valid UTF-8")
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
	var tree any
	if err := json.Unmarshal(payload, &tree); err != nil {
		return Manifest{}, invalid("%s", err)
	}
	if err := checkPresence(reflect.TypeOf(m), tree, "payload"); err != nil {
		return Manifest{}, err
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return Manifest{}, invalid("%s", err)
	}
	if _, err := Build(m); err != nil { // what Decode accepts, Build can write back
		return Manifest{}, err
	}
	return m, nil
}

// frame is one open JSON object or array of the shape walk.
type frame struct {
	object    bool
	expectKey bool
	keys      map[string]bool // exact spellings
	folded    map[string]bool // case-folded spellings (foldKey)
}

// foldKey maps every rune to the smallest member of its Unicode simple-fold
// orbit, so two names equal under encoding/json's case folding (which also joins
// "K" U+212A with k and "ſ" U+017F with s) get one key.
func foldKey(s string) string {
	return strings.Map(func(r rune) rune {
		min := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < min {
				min = f
			}
		}
		return min
	}, s)
}

// checkPresence requires every field of the Go type in the decoded JSON value,
// and a non-null value for a field that is not a pointer.
func checkPresence(t reflect.Type, v any, path string) error {
	nullable := t.Kind() == reflect.Pointer
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v == nil {
		if nullable {
			return nil
		}
		return invalid("%s is null", path)
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := v.(map[string]any)
		if !ok {
			return invalid("%s is not an object", path)
		}
		fields := map[string]string{} // foldKey -> json name, for this struct only
		for i := 0; i < t.NumField(); i++ {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			fields[foldKey(name)] = name
		}
		for key := range obj { // a key of an ignored extension object is not checked: it is not in this struct
			if want, ok := fields[foldKey(key)]; ok && want != key {
				return invalid("%s: key %q is a case variant of %q", path, key, want)
			}
		}
		for i := 0; i < t.NumField(); i++ {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			child, present := obj[name]
			if !present {
				return invalid("%s.%s is missing", path, name)
			}
			if err := checkPresence(t.Field(i).Type, child, path+"."+name); err != nil {
				return err
			}
		}
	case reflect.Slice:
		list, ok := v.([]any)
		if !ok {
			return invalid("%s is not an array", path)
		}
		for i, item := range list {
			if err := checkPresence(t.Elem(), item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
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
				f := frame{object: t == '{', expectKey: t == '{'}
				if f.object { // arrays need no key sets: a 4 MiB array of empty objects stays cheap
					f.keys, f.folded = map[string]bool{}, map[string]bool{}
				}
				stack = append(stack, f)
			default:
				stack = stack[:len(stack)-1]
				valueDone()
			}
		case string:
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
				if stack[n-1].keys[t] {
					return invalid("key %q repeats", t)
				}
				lower := foldKey(t)
				if stack[n-1].folded[lower] {
					return invalid("key %q repeats in another letter case", t)
				}
				stack[n-1].keys[t], stack[n-1].folded[lower], stack[n-1].expectKey = true, true, false
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
