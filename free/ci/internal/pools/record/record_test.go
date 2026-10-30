package record

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestSoloDefaultRecord(t *testing.T) {
	for _, source := range []string{pools.SourceLocal, pools.SourceRegistry, pools.SourceProvider, pools.SourceCoordinator, ""} {
		want := source != pools.SourceCoordinator && source != ""
		if got := Allows(pools.List{}, pools.Runner{ID: "r", Source: source}); got != want {
			t.Fatalf("source %q: got %v want %v", source, got, want)
		}
	}
	if Allows(pools.List{Set: true}, pools.Runner{ID: "r", Source: pools.SourceLocal}) {
		t.Fatal("an explicit empty list must deny all")
	}
}

func TestRevisionNeverWidensRecord(t *testing.T) {
	const seed = int64(321518)
	rng := rand.New(rand.NewSource(seed))
	reg, err := trust.NewRegistry(Spec())
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"local", "hosted", "mine", "team", "gpu"}
	for n := range 1000 {
		var project, revision []string
		project, revision = []string{}, []string{}
		for _, id := range ids {
			if rng.Intn(2) == 0 {
				project = append(project, id)
			}
			if rng.Intn(2) == 0 {
				revision = append(revision, id)
			}
		}
		p, _ := json.Marshal(project)
		r, _ := json.Marshal(revision)
		branch := trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{Key: p}}
		rev := trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{Key: r}}
		eff, _, err := reg.Merge("pinned", branch, rev)
		if err != nil {
			t.Fatalf("seed %d case %d: %v", seed, n, err)
		}
		for _, id := range FromPolicy(eff).IDs {
			if !slices.Contains(project, id) {
				t.Fatalf("seed %d case %d: revision added %s", seed, n, id)
			}
		}
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{Key: json.RawMessage(`["team"]`)}})
	if err != nil || FromPolicy(eff).Set {
		t.Fatalf("revision supplied an unset default: %v %v", eff, err)
	}
}

type brokenLabels struct{}

func (brokenLabels) NodeLabels(context.Context, string) ([]store.NodeLabel, error) {
	return nil, errors.New("unreadable")
}

func TestProviderUnreadableFacts(t *testing.T) {
	w := sched.World{Runners: []sched.Runner{{ID: "local"}, {ID: "vps"}}, Hosted: []sched.Runner{{ID: "gh"}}}
	fn := ExclusionProvider(pools.Config{Labels: brokenLabels{}}, func(context.Context) bool { return true })
	got, err := fn(context.Background(), w)
	if err != nil || len(got) != 3 {
		t.Fatalf("unreadable facts must exclude every runner: %v %v", got, err)
	}
	for _, why := range got {
		if why[0].Code != "pool.excluded" || why[0].Detail != Reason {
			t.Fatal(why)
		}
	}
	fn = ExclusionProvider(pools.Config{Labels: brokenLabels{}}, func(context.Context) bool { return false })
	if got, err = fn(context.Background(), w); err != nil || len(got) != 0 {
		t.Fatal("dev was restricted by record", got, err)
	}
}

func TestRecordKeyDecoder(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `["Bad"]`, `["team"] true`, `[3]`} {
		if _, err := decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
