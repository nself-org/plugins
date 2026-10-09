package providers

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var registry = struct {
	sync.RWMutex
	providers map[string]Provider
	sources   map[string]TriggerSource
	demand    map[string]DemandHandler
	health    func(context.Context, string) Health
}{
	providers: map[string]Provider{}, sources: map[string]TriggerSource{}, demand: map[string]DemandHandler{},
}

func Register(p Provider) error {
	if p == nil || p.ID() == "" {
		return fmt.Errorf("provider id required")
	}
	registry.Lock()
	defer registry.Unlock()
	if _, exists := registry.providers[p.ID()]; exists {
		return fmt.Errorf("provider already registered: %s", p.ID())
	}
	registry.providers[p.ID()] = p
	return nil
}
func Lookup(id string) (Provider, bool) {
	registry.RLock()
	defer registry.RUnlock()
	p, ok := registry.providers[id]
	return p, ok
}
func All() []Provider {
	registry.RLock()
	defer registry.RUnlock()
	ids := make([]string, 0, len(registry.providers))
	for id := range registry.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		out = append(out, registry.providers[id])
	}
	return out
}
func RegisterSource(s TriggerSource) error {
	if s == nil || s.Name() == "" {
		return fmt.Errorf("source name required")
	}
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.sources[s.Name()]; ok {
		return fmt.Errorf("source already registered: %s", s.Name())
	}
	registry.sources[s.Name()] = s
	return nil
}
func Sources() []TriggerSource {
	registry.RLock()
	defer registry.RUnlock()
	names := make([]string, 0, len(registry.sources))
	for name := range registry.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]TriggerSource, 0, len(names))
	for _, name := range names {
		out = append(out, registry.sources[name])
	}
	return out
}

// RefetchFacts routes only to the named provider. API facts from the provider win.
func RefetchFacts(ctx context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
	p, ok := Lookup(f.Source)
	if !ok {
		return f, nil
	}
	t, ok := p.(TrustFacts)
	if !ok {
		return f, nil
	}
	fresh, err := t.Refetch(ctx, f)
	if err != nil {
		return f, err
	}
	mergeAPIFacts(&f, fresh)
	return f, nil
}
func mergeAPIFacts(dst *model.TriggerFacts, src model.TriggerFacts) {
	if src.Actor.Source == "api" {
		dst.Actor = src.Actor
	}
	if src.ActorPermission.Source == "api" {
		dst.ActorPermission = src.ActorPermission
	}
	if src.AuthorAssociation.Source == "api" {
		dst.AuthorAssociation = src.AuthorAssociation
	}
	if src.HeadRepo.Source == "api" {
		dst.HeadRepo = src.HeadRepo
	}
	if src.Fork.Source == "api" {
		dst.Fork = src.Fork
	}
	if src.FirstTime.Source == "api" {
		dst.FirstTime = src.FirstTime
	}
	if src.Bot.Source == "api" {
		dst.Bot = src.Bot
	}
	if src.ProtectedRef.Source == "api" {
		dst.ProtectedRef = src.ProtectedRef
	}
	if src.Visibility.Source == "api" {
		dst.Visibility = src.Visibility
	}
	if src.SHAReachable.Source == "api" {
		dst.SHAReachable = src.SHAReachable
	}
	if src.HeadCurrent.Source == "api" {
		dst.HeadCurrent = src.HeadCurrent
	}
	if src.ChangedFiles.Source == "api" {
		dst.ChangedFiles = src.ChangedFiles
	}
	if src.GroupProtectedOnly.Source == "api" {
		dst.GroupProtectedOnly = src.GroupProtectedOnly
	}
	if src.RunEvent.Source == "api" {
		dst.RunEvent = src.RunEvent
	}
	if src.RunnerLabels.Source == "api" {
		dst.RunnerLabels = src.RunnerLabels
	}
	if src.ProviderJobID.Source == "api" {
		dst.ProviderJobID = src.ProviderJobID
	}
	if src.ProviderRunID.Source == "api" {
		dst.ProviderRunID = src.ProviderRunID
	}
}
func RegisterHealth(fn func(context.Context, string) Health) {
	registry.Lock()
	registry.health = fn
	registry.Unlock()
}
func HealthOf(ctx context.Context, id string) Health {
	registry.RLock()
	fn := registry.health
	p := registry.providers[id]
	registry.RUnlock()
	if fn != nil {
		return fn(ctx, id)
	}
	if h, ok := p.(HealthProbe); ok {
		return h.Health(ctx)
	}
	return Health{State: "up"}
}

type DemandHandler interface {
	Synthesize(context.Context, model.TriggerFacts) (*model.Pipeline, error)
}

func RegisterDemandHandler(source string, h DemandHandler) error {
	if source == "" || h == nil {
		return fmt.Errorf("demand handler source and implementation required")
	}
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.demand[source]; ok {
		return fmt.Errorf("demand handler already registered: %s", source)
	}
	registry.demand[source] = h
	return nil
}
func DemandHandlerFor(source string) DemandHandler {
	registry.RLock()
	defer registry.RUnlock()
	return registry.demand[source]
}
