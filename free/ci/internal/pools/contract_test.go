package pools

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestPoolContractGolden(t *testing.T) {
	runners := []Runner{{ID: "local", Source: SourceLocal, Location: "local", Assigned: []string{"pool.mine"}},
		{ID: "vps", Source: SourceRegistry, Location: "remote", Assigned: []string{"pool.mine", "region=eu"}},
		{ID: "t1", Source: SourceCoordinator, Location: "lan", Assigned: []string{"pool.team"}},
		{ID: "gh-ubuntu", Source: SourceProvider, Location: "hosted"}}
	got := Resolve(Input{Runners: runners, Allow: List{Set: true, IDs: []string{"team", "local"}}})
	body, err := json.MarshalIndent(got.Pools, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	const path = "testdata/pools.golden.json"
	if *update {
		if err = os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("pools differ from %s (run with -update):\n%s", path, body)
	}
	schema, err := os.ReadFile("../../schemas/pool.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.Pools {
		doc, _ := json.Marshal(p)
		if err = model.ValidateJSON(schema, doc); err != nil {
			t.Fatalf("pool %s: %v", p.ID, err)
		}
	}
	for name, doc := range map[string]string{
		"reserved scope":     `{"schema":"ci.pool/v1","id":"a","scope":"team","members":[],"effective":{"allowed":true,"reasons":[]}}`,
		"bad id":             `{"schema":"ci.pool/v1","id":"A","scope":"operator","members":[],"effective":{"allowed":true,"reasons":[]}}`,
		"excluded no reason": `{"schema":"ci.pool/v1","id":"a","scope":"operator","members":[],"effective":{"allowed":false,"reasons":[]}}`,
		"unknown detail":     `{"schema":"ci.pool/v1","id":"a","scope":"operator","members":[],"effective":{"allowed":false,"reasons":[{"code":"pool.excluded","detail":"x"}]}}`,
	} {
		if model.ValidateJSON(schema, []byte(doc)) == nil {
			t.Fatalf("%s: the schema accepted %s", name, doc)
		}
	}
}

type fakeLabels map[string][]store.NodeLabel

func (f fakeLabels) NodeLabels(_ context.Context, id string) ([]store.NodeLabel, error) {
	return f[id], nil
}

func allowPolicy(t *testing.T, ids string) trust.Effective {
	t.Helper()
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{KeyAllow: json.RawMessage(ids)}})
	if err != nil {
		t.Fatal(err)
	}
	return eff
}

func TestExclusionProvider(t *testing.T) {
	declared := sched.Runner{ID: "vps"}
	declared.Capability.Identity.Labels = []string{"pool.team"} // self-declared: never membership
	w := sched.World{Runners: []sched.Runner{declared, {ID: "t1"}, {ID: "local"}}, Hosted: []sched.Runner{{ID: "gh"}}}
	fn := ExclusionProvider(Config{Labels: fakeLabels{"t1": {{Key: "pool.team"}}}, Team: func() bool { return true }})
	if got, err := fn(context.Background(), w); err != nil || got != nil {
		t.Fatalf("no term set: %v %v", got, err)
	}
	w.Policy.Trust = allowPolicy(t, `["team"]`)
	got, err := fn(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"vps", "local", "gh"} {
		if len(got[id]) != 1 || got[id][0].Code != string(model.PoolExcluded) || got[id][0].Detail != DetailNotInProjectList {
			t.Fatalf("runner %s: %v", id, got[id])
		}
	}
	if _, kept := got["t1"]; kept {
		t.Fatalf("team member excluded: %v", got)
	}
}
