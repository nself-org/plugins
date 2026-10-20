package conformance_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/conformance"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
)

// wrapper lets a test break exactly one rule of an otherwise correct provider.
type wrapper struct {
	burst.Provider
	provision func(context.Context, burst.Intent, burst.Provider) (burst.Resource, error)
	teardown  func(context.Context, string, burst.Provider) error
	list      func(context.Context, map[string]string, burst.Provider) ([]burst.Resource, error)
}

func (w wrapper) Provision(ctx context.Context, in burst.Intent) (burst.Resource, error) {
	if w.provision != nil {
		return w.provision(ctx, in, w.Provider)
	}
	return w.Provider.Provision(ctx, in)
}
func (w wrapper) Teardown(ctx context.Context, id string) error {
	if w.teardown != nil {
		return w.teardown(ctx, id, w.Provider)
	}
	return w.Provider.Teardown(ctx, id)
}
func (w wrapper) List(ctx context.Context, l map[string]string) ([]burst.Resource, error) {
	if w.list != nil {
		return w.list(ctx, l, w.Provider)
	}
	return w.Provider.List(ctx, l)
}

func factory(wrap func(burst.Provider) burst.Provider) conformance.Factory {
	return func(t *testing.T) conformance.Subject {
		c := fakecloud.New(fakecloud.NewClock(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)))
		return conformance.Subject{Provider: wrap(c), Foreign: func(l map[string]string) string { return c.AddForeign(l) }, Outage: c.Outage}
	}
}

// TestConformanceKitCatchesBrokenProviders: each rule the lifecycle relies on has a
// case that fails when a provider breaks it, and the correct provider has no violation.
func TestConformanceKitCatchesBrokenProviders(t *testing.T) {
	if v := conformance.Violations(t, factory(func(p burst.Provider) burst.Provider { return p })); len(v) != 0 {
		t.Fatalf("the correct fake violates the kit: %v", v)
	}
	var n int
	for _, c := range []struct {
		name, wantCase string
		wrap           func(burst.Provider) burst.Provider
	}{
		{"provision creates a second machine", "provision_idempotent_by_intent", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, provision: func(ctx context.Context, in burst.Intent, p burst.Provider) (burst.Resource, error) {
				n++
				fresh := in
				fresh.Token = fmt.Sprintf("%s-%d", in.Token[:20], n)
				fresh.Labels = burst.IntentLabels(in.Labels[burst.LabelCoordinator], fresh.Token)
				return p.Provision(ctx, fresh)
			}}
		}},
		{"teardown of a missing machine fails", "teardown_idempotent_not_found_is_success", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, teardown: func(ctx context.Context, id string, p burst.Provider) error {
				rs, err := p.List(ctx, nil)
				if err != nil {
					return err
				}
				for _, r := range rs {
					if r.ID == id {
						return p.Teardown(ctx, id)
					}
				}
				return burst.ErrNotFound
			}}
		}},
		{"list ignores the selector", "list_filters_by_labels", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, list: func(ctx context.Context, _ map[string]string, p burst.Provider) ([]burst.Resource, error) {
				return p.List(ctx, nil)
			}}
		}},
		{"created at is zero", "provision_labels_and_created_at", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, provision: func(ctx context.Context, in burst.Intent, p burst.Provider) (burst.Resource, error) {
				r, err := p.Provision(ctx, in)
				r.CreatedAt = time.Time{}
				return r, err
			}}
		}},
		{"long labels accepted", "labels_within_63_characters", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, provision: func(ctx context.Context, in burst.Intent, p burst.Provider) (burst.Resource, error) {
				if len(in.Token) > burst.MaxLabelValue {
					in.Token = in.Token[:26]
					in.Labels = burst.IntentLabels(in.Labels[burst.LabelCoordinator], in.Token)
				}
				return p.Provision(ctx, in)
			}}
		}},
		{"outage errors are unclassified", "outage_is_classified", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, teardown: func(ctx context.Context, id string, p burst.Provider) error {
				if err := p.Teardown(ctx, id); err != nil {
					return errors.New(err.Error())
				}
				return nil
			}}
		}},
		{"cancelled context is ignored", "cancelled_context_creates_nothing", func(p burst.Provider) burst.Provider {
			return wrapper{Provider: p, provision: func(_ context.Context, in burst.Intent, p burst.Provider) (burst.Resource, error) {
				return p.Provision(context.Background(), in)
			}}
		}},
	} {
		t.Run(strings.ReplaceAll(c.name, " ", "_"), func(t *testing.T) {
			v := conformance.Violations(t, factory(c.wrap))
			if len(v[c.wantCase]) == 0 {
				t.Errorf("the kit did not fail case %s for %q; violations: %v", c.wantCase, c.name, v)
			}
		})
	}
}
