package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

func decodeValue(spec KeySpec, raw json.RawMessage) (any, error) {
	if spec.Decode != nil {
		return spec.Decode(raw)
	}
	var v any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("trailing JSON value")
	}
	if v == nil {
		return nil, fmt.Errorf("null value")
	}
	switch spec.Kind {
	case TightenLower, TightenHigher:
		s, ok := v.(string)
		if !ok || rank(spec.Order, s) < 0 {
			return nil, fmt.Errorf("value outside order")
		}
	case TightenSubset, TightenSuperset:
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected string list")
		}
		for _, item := range list {
			if _, ok := item.(string); !ok {
				return nil, fmt.Errorf("expected string list")
			}
		}
	case TightenOnlyOn:
		if _, ok := v.(bool); !ok {
			return nil, fmt.Errorf("expected boolean")
		}
	}
	return v, nil
}
func setValues(v any) map[string]bool {
	out := map[string]bool{}
	switch xs := v.(type) {
	case []string:
		for _, x := range xs {
			out[x] = true
		}
	case []any:
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}
func stricter(spec KeySpec, old, next any) bool {
	switch spec.Kind {
	case TightenAny:
		return true
	case TightenFixed:
		return reflect.DeepEqual(old, next)
	case TightenOnlyOn:
		a, ao := old.(bool)
		b, bo := next.(bool)
		return ao && bo && (!a || b)
	case TightenLower, TightenHigher:
		a, ao := old.(string)
		b, bo := next.(string)
		if !ao || !bo {
			return false
		}
		x, y := rank(spec.Order, a), rank(spec.Order, b)
		if x < 0 || y < 0 {
			return false
		}
		if spec.Kind == TightenLower {
			return y <= x
		}
		return y >= x
	case TightenSubset, TightenSuperset:
		a, b := setValues(old), setValues(next)
		if spec.Kind == TightenSubset {
			for x := range b {
				if !a[x] {
					return false
				}
			}
			return true
		}
		for x := range a {
			if !b[x] {
				return false
			}
		}
		return true
	}
	return false
}

// Merge applies already-loaded scopes using the supplied registry and pinned commit.
func (r Registry) Merge(pinnedCommit string, scopes ...Scope) (Effective, Digest, error) {
	e := Effective{values: map[string]entry{}}
	scopes = append([]Scope(nil), scopes...)
	order := map[Source]int{SourceDefault: 0, SourceUser: 1, SourceTeam: 2, SourceProject: 3, SourceProtectedBranch: 4, SourcePipeline: 5, SourceJob: 6, SourceRevision: 7}
	for _, scope := range scopes {
		if _, ok := order[scope.Kind]; !ok {
			return Effective{}, "", fmt.Errorf("E672 unknown scope %s", scope.Kind)
		}
	}
	sort.SliceStable(scopes, func(i, j int) bool { return order[scopes[i].Kind] < order[scopes[j].Kind] })
	for _, spec := range r.specs {
		if !strings.HasSuffix(spec.Key, ".*") && spec.Default != nil {
			e.values[spec.Key] = entry{spec.Default, SourceDefault}
		}
	}
	for _, scope := range scopes {
		if scope.PinnedCommit != "" {
			pinnedCommit = scope.PinnedCommit
		}
		keys := make([]string, 0, len(scope.Values))
		for k := range scope.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			spec, ok := r.Find(key)
			if !ok {
				return Effective{}, "", fmt.Errorf("E672 unknown key %s", key)
			}
			v, err := decodeValue(spec, scope.Values[key])
			if err != nil {
				return Effective{}, "", fmt.Errorf("E672 %s: %w", key, err)
			}
			if spec.Spend && (scope.Kind == SourceProtectedBranch || scope.Kind == SourceRevision || scope.Kind == SourcePipeline || scope.Kind == SourceJob) {
				e.ignored = append(e.ignored, Ignored{key, scope.Kind, "E671 spend key requires operator scope"})
				continue
			}
			old, exists := e.values[key]
			if exists && (!stricter(spec, old.value, v) || scope.Kind == SourceRevision && spec.Kind == TightenAny && !reflect.DeepEqual(old.value, v)) {
				if scope.Kind == SourceRevision {
					e.ignored = append(e.ignored, Ignored{key, scope.Kind, "E671 revision widening"})
					continue
				}
				return Effective{}, "", fmt.Errorf("E670 %s loosens %s from %s", scope.Kind, key, old.source)
			}
			if scope.Kind == SourceRevision && !exists {
				e.ignored = append(e.ignored, Ignored{key, scope.Kind, "E671 revision cannot introduce a key"})
				continue
			}
			e.values[key] = entry{v, scope.Kind}
		}
	}
	plain := map[string]any{}
	for key, value := range e.values {
		plain[key] = value.value
	}
	encoded, err := Canonical(plain)
	if err != nil {
		return Effective{}, "", err
	}
	sum := sha256.Sum256(append(encoded, []byte(pinnedCommit)...))
	return e, Digest(hex.EncodeToString(sum[:])), nil
}

// Merge accepts a registry as its first argument through the default registry.
var DefaultRegistry Registry

func Merge(scopes ...Scope) (Effective, Digest, error) { return DefaultRegistry.Merge("", scopes...) }
