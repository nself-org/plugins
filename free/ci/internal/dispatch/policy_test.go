package dispatch_test

// A limit from the effective policy reaches the admission step: a revision may
// lower it (the lower value applies) and may not raise it (ignored, recorded).

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func limitsOf(t *testing.T, revision string) sched.Limits {
	t.Helper()
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	scope := func(kind trust.Source, raw string) trust.Scope {
		return trust.Scope{Kind: kind, Values: map[string]json.RawMessage{schedpolicy.KeyLimitGlobal: json.RawMessage(raw)}}
	}
	eff, _, err := reg.Merge("", scope(trust.SourceProject, "2"), scope(trust.SourceRevision, revision))
	if err != nil {
		t.Fatal(err)
	}
	p, err := schedpolicy.Placement(&eff)
	if err != nil {
		t.Fatal(err)
	}
	return p.Limits
}

func TestDispatchLimitFromPolicy(t *testing.T) {
	for _, c := range []struct {
		name, revision string
		wantAdmitted   int
	}{{"revision lowers 2 to 1", "1", 1}, {"revision cannot raise 2 to 5", "5", 2}} {
		t.Run(c.name, func(t *testing.T) {
			limits := limitsOf(t, c.revision)
			if limits.Global != c.wantAdmitted {
				t.Fatalf("effective global limit %d, want %d", limits.Global, c.wantAdmitted)
			}
			st := dispatch.OpenStore(t)
			live := dispatch.LiveWorld(st, dispatch.SchedRunner("local", "local", 8))
			d := newDispatcher(t, dispatch.Config{Store: st, World: func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
				w, digest, err := live(ctx, p)
				w.Policy.Limits = limits
				return w, digest, err
			}})
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			var mu sync.Mutex
			admitted := 0
			var wg sync.WaitGroup
			var ids []string
			for _, name := range []string{"a", "b", "c", "d"} {
				ids = append(ids, dispatch.QueueAttempt(t, st, "pipe", "proj", name))
			}
			for _, id := range ids {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := d.Submit(ctx, dispatch.QueuedJob(id)); err == nil {
						mu.Lock()
						admitted++
						mu.Unlock()
					} else if !errors.Is(err, context.DeadlineExceeded) {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			if admitted != c.wantAdmitted {
				t.Fatalf("%d admitted under global limit %d", admitted, limits.Global)
			}
			queued := 0
			for _, id := range ids {
				if reason, _, ok, _ := st.Refusal(context.Background(), id); ok && reason == "limit.global" {
					queued++
				}
			}
			if queued != len(ids)-c.wantAdmitted {
				t.Fatalf("%d jobs queued with limit.global, want %d", queued, len(ids)-c.wantAdmitted)
			}
		})
	}
}
