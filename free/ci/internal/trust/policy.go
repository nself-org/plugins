package trust

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Tightening uint8

const (
	TightenAny Tightening = iota
	TightenLower
	TightenHigher
	TightenSubset
	TightenSuperset
	TightenOnlyOn
	TightenFixed
)

type KeySpec struct {
	Key     string
	Owner   string
	Kind    Tightening
	Order   []string
	Spend   bool
	Secret  bool
	Decode  func(json.RawMessage) (any, error)
	Default any
}
type Registry struct{ specs []KeySpec }

func NewRegistry(specs ...KeySpec) (Registry, error) {
	r := Registry{}
	for _, spec := range specs {
		if spec.Key == "" || spec.Owner == "" || strings.Contains(spec.Key, "**") || strings.Count(spec.Key, "*") > 1 || (strings.Contains(spec.Key, "*") && !strings.HasSuffix(spec.Key, ".*")) {
			return Registry{}, fmt.Errorf("E672 invalid key spec %q", spec.Key)
		}
		for _, old := range r.specs {
			if old.Key == spec.Key || strings.HasSuffix(old.Key, ".*") && strings.HasPrefix(spec.Key, strings.TrimSuffix(old.Key, "*")) || strings.HasSuffix(spec.Key, ".*") && strings.HasPrefix(old.Key, strings.TrimSuffix(spec.Key, "*")) {
				return Registry{}, fmt.Errorf("E672 overlapping key %q", spec.Key)
			}
		}
		r.specs = append(r.specs, spec)
	}
	return r, nil
}
func (r Registry) Find(key string) (KeySpec, bool) {
	for _, s := range r.specs {
		if s.Key == key || strings.HasSuffix(s.Key, ".*") && strings.HasPrefix(key, strings.TrimSuffix(s.Key, "*")) && !strings.Contains(strings.TrimPrefix(key, strings.TrimSuffix(s.Key, "*")), ".") && key != strings.TrimSuffix(s.Key, "*") {
			return s, true
		}
	}
	return KeySpec{}, false
}

type Source string

const (
	SourceDefault         Source = "default"
	SourceUser            Source = "user"
	SourceTeam            Source = "team"
	SourceProject         Source = "project"
	SourcePipeline        Source = "pipeline"
	SourceJob             Source = "job"
	SourceProtectedBranch Source = "protected-branch"
	SourceRevision        Source = "revision"
)

type Scope struct {
	Kind             Source
	Values           map[string]json.RawMessage
	PinnedCommit     string
	publicVisibility bool
}
type Ignored struct {
	Key    string
	Scope  Source
	Reason string
}
type entry struct {
	value  any
	source Source
}
type Effective struct {
	values        map[string]entry
	ignored       []Ignored
	hostedPublic  bool
	hostedProject bool
}

func (e Effective) Value(key string) (any, Source, bool) {
	v, ok := e.values[key]
	return v.value, v.source, ok
}
func (e Effective) Ignored() []Ignored { return append([]Ignored(nil), e.ignored...) }

type Digest string
