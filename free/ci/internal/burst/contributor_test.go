package burst

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func engineWithStore(t *testing.T) *Engine {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e, err := New(Options{Store: st, Coordinator: "c1", Spend: economics.StaticSpend(economics.Policy{}),
		Exclusion: func(context.Context) (deployguard.Set, error) { return deployguard.Set{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestBurstContributorAddsPendingCapacityOnly(t *testing.T) {
	ctx := context.Background()
	w := &sched.World{}
	if err := contributePending(ctx, w); err != nil || len(w.Pending) != 0 {
		t.Fatalf("an unbound process adds nothing: %v %+v", err, w.Pending)
	}
	e := engineWithStore(t)
	restore := Bind(e)
	defer restore()
	if err := contributePending(ctx, w); err != nil || len(w.Pending) != 0 {
		t.Fatalf("no intent, no pending capacity: %v %+v", err, w.Pending)
	}
	put := func(tok, plat string, join bool) {
		err := e.o.Store.BurstWrite(ctx, func(b store.BurstTx) error {
			if err := b.PutIntent(store.BurstIntent{Token: tok, Coordinator: "c1", Provider: "cloud", JobKey: tok, Platform: plat, CreatedAt: 1}, store.BurstLease{}); err != nil {
				return err
			}
			if join {
				_, err := b.SetNode(tok, "node-"+tok, 2)
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	put("A", "linux/amd64", false)
	put("B", "linux/arm64", false)
	put("C", "darwin/arm64", true) // joined: it is a runner now, not pending
	if err := contributePending(ctx, w); err != nil {
		t.Fatal(err)
	}
	want := []sched.Pending{{Provider: "cloud", Platform: "linux/amd64", Slots: 1}, {Provider: "cloud", Platform: "linux/arm64", Slots: 1}}
	if len(w.Pending) != 2 || w.Pending[0] != want[0] || w.Pending[1] != want[1] {
		t.Fatalf("pending %+v, want %+v", w.Pending, want)
	}
}

func TestBurstBindRestoresThePreviousEngine(t *testing.T) {
	a, b := engineWithStore(t), engineWithStore(t)
	restoreA := Bind(a)
	restoreB := Bind(b)
	if current() != b {
		t.Fatal("the last Bind wins")
	}
	restoreB()
	if current() != a {
		t.Fatal("restore brings the previous engine back")
	}
	restoreA()
	if current() != nil {
		t.Fatal("and then none")
	}
}

func TestBurstConfigLifetimeIsTheSumOfItsParts(t *testing.T) {
	if got, want := (Config{}).LifetimeMs(), int64((60+10+5+30)*60_000); got != want {
		t.Fatalf("default lifetime %d, want %d", got, want)
	}
	if got := (Config{MaxLifetime: 90 * 60e9, Grace: 5 * 60e9, SweepEvery: 1 * 60e9, RetryWindow: 4 * 60e9}).LifetimeMs(); got != 100*60_000 {
		t.Fatalf("%d", got)
	}
	if got := (Config{MaxLifetime: 60 * 60e9, Grace: 10 * 60e9}).OrphanAfter().Minutes(); got != 70 {
		t.Fatalf("%v", got)
	}
}
