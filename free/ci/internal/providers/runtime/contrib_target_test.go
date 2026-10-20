package runtime

// Purpose: per-provider targets and the Describe cache of the world contributor (D-1394): a provider
//          that is given no target is not described, and a cached Describe is not repeated inside the TTL.
// Inputs:  the shared fake provider. Outputs: pass or fail. Constraints: no network.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestContributorTargetsFor(t *testing.T) {
	q1, q2 := providers.Query{Project: "o/a", Repo: "o/a"}, providers.Query{Project: "o/b", Repo: "o/b"}
	for _, tc := range []struct {
		name     string
		fn       func(context.Context, string) ([]providers.Query, error)
		want     []providers.Query
		wantHost int
		wantErr  bool
	}{
		{"queries for this provider", func(_ context.Context, p string) ([]providers.Query, error) {
			if p == "fake" {
				return []providers.Query{q1, q2}, nil
			}
			return nil, nil
		}, []providers.Query{q1, q2}, 1, false},
		{"a provider with no target is not described", func(context.Context, string) ([]providers.Query, error) { return nil, nil }, nil, 0, false},
		{"a target source error fails the pass", func(context.Context, string) ([]providers.Query, error) { return nil, errors.New("down") }, nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := useProvider(t)
			DefaultContributor().SetTargetsFor(tc.fn)
			var w sched.World
			err := DefaultContributor().Fill(context.Background(), &w)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			got := p.queries()
			if len(got) != len(tc.want) {
				t.Fatalf("described %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("described %+v, want %+v", got, tc.want)
				}
			}
			if !tc.wantErr && len(w.Hosted) != tc.wantHost {
				t.Fatalf("hosted = %d, want %d", len(w.Hosted), tc.wantHost)
			}
		})
	}
}

func TestContributorDescribeCache(t *testing.T) {
	p := useProvider(t)
	q := providers.Query{Project: "o/a", Repo: "o/a"}
	DefaultContributor().SetTargets(func(context.Context) ([]providers.Query, error) { return []providers.Query{q}, nil })
	fill := func() {
		t.Helper()
		var w sched.World
		if err := DefaultContributor().Fill(context.Background(), &w); err != nil {
			t.Fatal(err)
		}
	}
	fill()
	fill()
	if n := len(p.queries()); n != 2 {
		t.Fatalf("with no TTL every pass describes: %d lookups, want 2", n)
	}
	DefaultContributor().SetDescribeTTL(time.Hour)
	fill()
	fill()
	fill()
	if n := len(p.queries()); n != 3 {
		t.Fatalf("with a TTL one lookup serves the passes: %d lookups, want 3 in total", n)
	}
	DefaultContributor().SetDescribeTTL(time.Millisecond)
	fill()
	time.Sleep(5 * time.Millisecond)
	fill()
	if n := len(p.queries()); n != 5 {
		t.Fatalf("an expired entry is looked up again: %d lookups, want 5 in total", n)
	}
}
