package dispatch

// Shared fixtures: a private store, queued attempts, runners and a world that
// reads its availability from the store, so successive ticks see each other.

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func ptr[T any](v T) *T { return &v }

func openStoreAt(t *testing.T, path string) *store.Store {
	t.Helper()
	s, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func openStore(t *testing.T) *store.Store {
	return openStoreAt(t, filepath.Join(t.TempDir(), "state.db"))
}

var rowSeq atomic.Int64

// queueAttempt creates pipeline pipe (project proj), a job named name and its
// queued attempt; two jobs of one name are matrix entries with their own rows.
func queueAttempt(t testing.TB, st *store.Store, pipe, proj, name string) string {
	t.Helper()
	ctx := context.Background()
	err := st.CreatePipeline(ctx, store.PipelineRow{ID: pipe, Revision: "rev", Project: proj, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatal(err)
	}
	id := pipe + "." + name + "." + strconv.FormatInt(rowSeq.Add(1), 10)
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: pipe, Name: name, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	return id
}

// testRunner is a runner every fixture job is eligible for.
func testRunner(id, loc string, slots int) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Availability.Slots.Value = &slots
	c.Identity.ID, c.Identity.Provider, c.Identity.Transport = id, "operator", "kind-"+id
	c.Identity.Labels = []string{"arch=amd64"}
	c.Location.Kind.Value = &loc
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: slots, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: loc == "local", NoLANRoute: true}}
}

func testJob(attempt string) sched.Job {
	return sched.Job{Key: attempt, ID: attempt, Name: attempt, Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true, Project: "project"}
}

func testQueued(attempt string) Job {
	return Job{AttemptID: attempt, Sched: testJob(attempt)}
}

// liveWorld rebuilds the world on every call, taking each runner's free slots
// and CPU from the store's reservations. policy tweaks the policy.
func liveWorld(st *store.Store, policy func(*sched.Policy), runners ...sched.Runner) func(context.Context, sched.Pipeline) (sched.World, string, error) {
	return func(ctx context.Context, _ sched.Pipeline) (sched.World, string, error) {
		snap, err := st.WorldSnapshot(ctx)
		if err != nil {
			return sched.World{}, "", err
		}
		w := sched.World{Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}}
		if policy != nil {
			policy(&w.Policy)
		}
		for _, r := range runners {
			rs := snap.Reservations[r.ID]
			r.Avail.Slots = max(0, *r.Capability.Availability.Slots.Value-int(rs.Slots))
			r.Avail.CPU -= rs.CPU
			r.Avail.MemMB -= rs.MemMB
			if r.Location == "hosted" {
				w.Hosted = append(w.Hosted, r)
			} else {
				w.Runners = append(w.Runners, r)
			}
		}
		return w, "digest-" + time.Now().Format("150405.000000"), nil
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type recSink struct {
	mu  sync.Mutex
	got []Decision
}

func (r *recSink) Record(_ context.Context, d Decision) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, d)
}
func (r *recSink) all() []Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Decision(nil), r.got...)
}

func newDispatcher(t testing.TB, cfg Config) *Dispatcher {
	t.Helper()
	if cfg.Host == "" {
		cfg.Host = "host"
	}
	d, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// pass walks an admitted attempt to a terminal state so its capacity frees.
func pass(t testing.TB, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	for _, step := range [][2]model.JobState{{"leased", "running"}, {"running", "finalizing"}, {"finalizing", "passed"}} {
		_, epoch, err := st.AttemptStateEpoch(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err = st.Transition(ctx, id, step[0], step[1], store.TransitionDetail{Epoch: epoch}); err != nil {
			t.Fatal(err)
		}
	}
}

func resolved(e *entry) bool {
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}
