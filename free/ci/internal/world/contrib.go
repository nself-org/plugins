package world

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// Contributor owns exactly one sched.World field. Registration is process wide.
type Contributor struct {
	Name, Field string
	Fn          func(context.Context, *sched.World) error
}

var contributions = struct {
	sync.RWMutex
	byField map[string]Contributor
}{byField: map[string]Contributor{}}

// Register rejects duplicate field owners and invalid fields with E699.
func Register(c Contributor) error {
	if c.Name == "" || c.Fn == nil {
		return failed("contributor name and function required", nil)
	}
	field, ok := reflect.TypeOf(sched.World{}).FieldByName(c.Field)
	if !ok || !field.IsExported() {
		return failed("unknown world field "+c.Field, nil)
	}
	switch c.Field {
	case "Runners", "Policy", "Previous", "NowMs", "OpenRegistry", "InteractiveQueued":
		return failed("builder owns "+c.Field, nil)
	}
	contributions.Lock()
	defer contributions.Unlock()
	if old, exists := contributions.byField[c.Field]; exists {
		return failed(fmt.Sprintf("%s owned by %s and %s", c.Field, old.Name, c.Name), nil)
	}
	for _, old := range contributions.byField {
		if old.Name == c.Name {
			return failed("duplicate contributor name "+c.Name, nil)
		}
	}
	contributions.byField[c.Field] = c
	return nil
}

func runContributors(ctx context.Context, w *sched.World) error {
	contributions.RLock()
	list := make([]Contributor, 0, len(contributions.byField))
	for _, c := range contributions.byField {
		list = append(list, c)
	}
	contributions.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	for _, c := range list {
		before, err := fields(w)
		if err != nil {
			return failed("encode contributor input", err)
		}
		if err = c.Fn(ctx, w); err != nil {
			return failed("contributor "+c.Name, err)
		}
		after, err := fields(w)
		if err != nil {
			return failed("encode contributor output", err)
		}
		for field, value := range before {
			if field != c.Field && !bytes.Equal(value, after[field]) {
				return failed(fmt.Sprintf("contributor %s changed %s outside %s", c.Name, field, c.Field), nil)
			}
		}
	}
	return nil
}
func fields(w *sched.World) (map[string]json.RawMessage, error) {
	v := reflect.ValueOf(w).Elem()
	t := v.Type()
	out := map[string]json.RawMessage{}
	for i := 0; i < v.NumField(); i++ {
		name := t.Field(i).Name
		b, err := json.Marshal(v.Field(i).Interface())
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
	return out, nil
}

var hooks = struct {
	sync.RWMutex
	labels func(context.Context, string) ([]string, error)
	facts  func(context.Context) (CoordinatorFacts, error)
}{}

type CoordinatorFacts struct {
	Serving    bool
	Mode       string
	AgentPeers map[string]string // Node ID to last observed agent peer IP.
}

func RegisterLabelSource(fn func(context.Context, string) ([]string, error)) error {
	hooks.Lock()
	defer hooks.Unlock()
	if fn == nil || hooks.labels != nil {
		return failed("label source already registered or nil", nil)
	}
	hooks.labels = fn
	return nil
}
func RegisterCoordinatorFacts(fn func(context.Context) (CoordinatorFacts, error)) error {
	hooks.Lock()
	defer hooks.Unlock()
	if fn == nil || hooks.facts != nil {
		return failed("coordinator facts already registered or nil", nil)
	}
	hooks.facts = fn
	return nil
}
func CoordinatorFactsRegistered() bool {
	hooks.RLock()
	defer hooks.RUnlock()
	return hooks.facts != nil
}
func currentHooks() (func(context.Context, string) ([]string, error), func(context.Context) (CoordinatorFacts, error)) {
	hooks.RLock()
	defer hooks.RUnlock()
	return hooks.labels, hooks.facts
}
