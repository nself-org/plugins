//go:build security

package redteam

// Purpose: TM-46 and TM-47 red team. TM-46: the claim ledger (P7-AUTO-10) and the trusted_creators key
// are not built in this tree, so the attack runs against the code that exists today and reads commit
// statuses: the GitHub head scan, which decides whether a head still needs CI. TM-47: pool intersection
// (P7-AUTO-03) is not built either; the attack runs against the node authorization that exists, the
// registry and the scheduler's per-project eligibility, and asks whether a personal node can make itself
// eligible for a team project.
// Inputs:  a fake GitHub API over loopback TLS; a real store and registry in t.TempDir().
// Outputs: pass = a foreign status cannot suppress CI; a node cannot authorize itself (control HELD).
// Constraints: no real network; fixtures only.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type dMem struct {
	mu    sync.Mutex
	marks map[string]time.Time
	facts map[string]store.ProviderFact
}

func (m *dMem) ProviderMarkGet(_ context.Context, n, k string) (time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.marks[n+"|"+k]
	return at, ok, nil
}
func (m *dMem) ProviderMarkPut(_ context.Context, n, k string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marks[n+"|"+k] = at
	return nil
}
func (m *dMem) ProviderFactGet(_ context.Context, p, s, k string) (store.ProviderFact, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.facts[p+"|"+s+"|"+k]
	return f, ok, nil
}
func (m *dMem) ProviderFactPut(_ context.Context, f store.ProviderFact) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.facts[f.Provider+"|"+f.Scope+"|"+f.Key] = f
	return true, nil
}

type dEnq struct {
	mu  sync.Mutex
	got []coordinator.Delivery
}

func (e *dEnq) Enqueue(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.got = append(e.got, d)
	return coordinator.Outcome{Kind: "enqueued", ID: "in"}, nil
}

// dScan runs one head scan of o/r whose default head carries the given commit statuses and returns how
// many deliveries the scan enqueued.
func dScan(t *testing.T, statuses string) int {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	sha := strings.Repeat("c", 40)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/o/r":
			_, _ = w.Write([]byte(`{"id":100,"full_name":"o/r","default_branch":"main","private":true,"visibility":"private"}`))
		case r.URL.Path == "/repos/o/r/pulls":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/repos/o/r/branches":
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"` + sha + `"},"protected":true}]`))
		case r.URL.Path == "/repos/o/r/commits/"+sha+"/status":
			_, _ = w.Write([]byte(`{"statuses":` + statuses + `}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e := reconcile.NewEngine()
	now := time.Now().UTC().Truncate(time.Second)
	e.Now, e.Jitter = func() time.Time { return now }, func() float64 { return 0.5 }
	e.NewAPI = func(context.Context) (*rest.Client, error) {
		return &rest.Client{BaseURL: srv.URL, AllowPrivateBase: true, Provider: "github", TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig}, nil
	}
	e.Repos = func(context.Context) ([]string, error) { return []string{"o/r"}, nil }
	e.Use(&dMem{marks: map[string]time.Time{}, facts: map[string]store.ProviderFact{}})
	enq := &dEnq{}
	if err := e.GitHub().Reconcile(context.Background(), enq); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return len(enq.got)
}

// D40: anyone who can write a commit status posts `nself-ci` = success from their own identity. The
// head scan must not treat that as the station's result and skip the commit.
func TestD40_ForeignFinalStatusCannotSuppressCI(t *testing.T) {
	if got := dScan(t, `[]`); got != 1 {
		t.Fatalf("positive control: a head with no status enqueued %d deliveries, want 1", got)
	}
	for _, ctx := range []string{"nself-ci/claim", "nself-ci ", "NSELF-CI", "nself-ci/other"} {
		for _, state := range []string{"success", "pending", "failure"} {
			if got := dScan(t, `[{"context":"`+ctx+`","state":"`+state+`","creator":{"login":"mallory"}}]`); got != 1 {
				t.Errorf("status context %q state %s from a foreign writer suppressed CI (%d deliveries)", ctx, state, got)
			}
		}
	}
	if !openFinding(t, "D40") {
		return
	}
	for _, state := range []string{"success", "failure", "error"} {
		if got := dScan(t, `[{"context":"nself-ci","state":"`+state+`","creator":{"login":"mallory"}}]`); got != 1 {
			t.Errorf("a foreign nself-ci %s status from mallory suppressed CI for the head (%d deliveries, want 1)", state, got)
		}
	}
}

func dCapability(t *testing.T) model.Capability {
	t.Helper()
	b, err := os.ReadFile("../../../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func dEligibility(t *testing.T, n registry.Node, project string) []model.PlacementReasonDetail {
	t.Helper()
	loc := ""
	if n.Capability.Location.Kind.Value != nil {
		loc = *n.Capability.Location.Kind.Value
	}
	c := n.Capability
	c.Lifecycle.Discovered = false // the admin has enabled the node; only the project test is left
	r := sched.Runner{ID: c.Identity.ID, Capability: c, Location: loc, Provider: "operator", Class: "owned",
		Decl: sched.DeclarationFromCapability(c, false, ""), Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192}}
	job := sched.Job{Key: "j", ID: "j", Project: project, Pipeline: sched.Pipeline{ID: "p", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64"}, Trust: "owner"}
	w := sched.World{Runners: []sched.Runner{r}, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	got := sched.Place(w, []sched.Job{job})
	if len(got.Explanations) == 0 || len(got.Explanations[0].Alternatives) == 0 {
		return nil
	}
	return got.Explanations[0].Alternatives[0].Reasons
}

func dHas(rs []model.PlacementReasonDetail, code model.PlacementReason) bool {
	for _, r := range rs {
		if r.Code == string(code) {
			return true
		}
	}
	return false
}

func dRegistry(t *testing.T) (*registry.Registry, model.Capability) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r, c := registry.New(st), dCapability(t)
	if _, err = r.Register(context.Background(), c, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Authorize(context.Background(), c.Identity.ID, "personal-proj", "admin"); err != nil {
		t.Fatal(err)
	}
	return r, c
}

// D41: a personal node authorized for its owner's project reports itself as a hosted node. Hosted
// nodes skip the per-project authorization test, so the node would become eligible for a team project.
func TestD41_NodeCannotSelfDeclareHostedToSkipProjectAuthorization(t *testing.T) {
	r, c := dRegistry(t)
	ctx := context.Background()
	n, _ := r.Get(ctx, c.Identity.ID)
	if !dHas(dEligibility(t, n, "team-proj"), model.NodeDiscovered) {
		t.Fatal("positive control: an unauthorized node was not refused for team-proj")
	}
	if dHas(dEligibility(t, n, "personal-proj"), model.NodeDiscovered) {
		t.Fatal("positive control: the authorized project was refused")
	}
	forged := n.Capability
	hosted := "hosted"
	forged.Location.Kind = model.Fact[string]{Value: &hosted, Source: "self", ObservedAt: time.Now().UTC(), Confidence: "known"}
	forged.Lifecycle.AuthorizedProjects = []string{"team-proj"} // a second, blunter claim
	updated, _, err := r.UpdateCapability(ctx, c.Identity.ID, forged, "node")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(updated.Capability.Lifecycle.AuthorizedProjects) != 1 {
		t.Fatalf("a node report rewrote its authorized projects: %v", updated.Capability.Lifecycle.AuthorizedProjects)
	}
	if !dHas(dEligibility(t, updated, "team-proj"), model.NodeDiscovered) {
		t.Fatal("after reporting location.kind=hosted the node is no longer refused for team-proj (not authorized)")
	}
}

// D42: node-reported labels are kept verbatim, including the pool.* namespace the pool design assigns.
func TestD42_NodeCannotAssignItselfPoolLabels(t *testing.T) {
	r, c := dRegistry(t)
	forged := dCapability(t)
	forged.Identity.Labels = append(append([]string(nil), forged.Identity.Labels...), "pool.team", "pool.record")
	updated, _, err := r.UpdateCapability(context.Background(), c.Identity.ID, forged, "node")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	for _, l := range updated.Capability.Identity.Labels {
		if strings.HasPrefix(l, "pool.") {
			t.Fatalf("a node report assigned the label %q to itself", l)
		}
	}
}
