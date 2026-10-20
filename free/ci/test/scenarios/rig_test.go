//go:build scenarios

package scenarios

// Purpose: the shared rig of the SCHED acceptance suite (P7-SCHED-15, Epic D20). Everything runs in
// process: a real store on a temp file, a real dispatcher over a world rebuilt from the store, fake
// transport drivers and an injected clock. No Docker, no network, no ssh, no sleeps.
// Inputs:  none. Outputs: helpers the scenario files share.
// Constraints: tests behind this tag fail with t.Fatal, never skip, when a prerequisite is missing;
//          every generated sequence prints its seed on failure.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func ptr[T any](v T) *T { return &v }

// openStore opens a private store on a temp file.
func openStore(t testing.TB) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var rowSeq atomic.Int64

// queueAttempt creates pipeline pipe (project proj), a job named name and its queued attempt.
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

// runner builds a runner of location loc (local, lan, hosted) on platform os/arch with slots slots.
func runner(id, loc, platform string, slots int) sched.Runner {
	osName, arch, _ := strings.Cut(platform, "/")
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr(osName), ptr(arch)
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Availability.Slots.Value = &slots
	c.Identity.ID, c.Identity.Provider, c.Identity.Transport = id, "operator", "kind-"+id
	c.Identity.Labels = []string{"arch=" + arch}
	c.Location.Kind.Value = &loc
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: slots, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: loc == "local", NoLANRoute: true}}
}

// job builds a placement input for platform plat in project project.
func job(key, plat string) sched.Job {
	return sched.Job{Key: key, ID: key, Name: key, Project: "project", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{plat}},
		Requirements: model.PlacementRequirements{Platform: plat, CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
}

// registerNode puts a remote runner in the node registry as online, as world.Build finds it.
func registerNode(t testing.TB, st *store.Store, r sched.Runner) {
	t.Helper()
	plat := "linux/amd64"
	if r.Capability.Platform.OS.Value != nil && r.Capability.Platform.Arch.Value != nil {
		plat = *r.Capability.Platform.OS.Value + "/" + *r.Capability.Platform.Arch.Value // the stored row carries the runner's own platform
	}
	c, err := planview.LocalCapability(planview.LocalFacts{Project: "project", Platform: plat, CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = r.ID, r.ID, "ssh" // the stored kind is a schema enum; the world runner keeps its fake kind
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(context.Background(), store.NodeRecord{ID: r.ID, Name: r.ID, State: "online", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "scenarios"); err != nil && !strings.Contains(err.Error(), "UNIQUE") { // idempotent: the world is rebuilt every tick
		t.Fatal(err)
	}
}

// liveWorld rebuilds the world on every call, taking each runner's free slots and CPU from the
// store's reservations, so successive ticks see each other. tweak edits the policy.
func liveWorld(t testing.TB, st *store.Store, tweak func(*sched.World), runners ...sched.Runner) func(context.Context, sched.Pipeline) (sched.World, string, error) {
	return func(ctx context.Context, _ sched.Pipeline) (sched.World, string, error) {
		snap, err := st.WorldSnapshot(ctx)
		if err != nil {
			return sched.World{}, "", err
		}
		w := sched.World{Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}}
		for _, r := range runners {
			if r.Location != "local" && r.Location != "hosted" {
				registerNode(t, st, r)
			}
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
		if tweak != nil {
			tweak(&w)
		}
		return w, "digest-" + strconv.FormatInt(int64(len(snap.Active)), 10), nil
	}
}

// clock is the injected clock; Advance moves it.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)} }
func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// transport is the fake driver: it records every delivery and, when finish is set, walks the
// attempt to passed, which frees its capacity.
type transport struct {
	mu     sync.Mutex
	got    []dispatch.Delivery
	finish func(dispatch.Delivery) error
}

func (f *transport) Known(string) bool { return true }
func (f *transport) Deliver(_ context.Context, d dispatch.Delivery) error {
	f.mu.Lock()
	f.got = append(f.got, d)
	run := f.finish
	f.mu.Unlock()
	if run == nil {
		return nil
	}
	return run(d)
}
func (f *transport) deliveries() []dispatch.Delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dispatch.Delivery(nil), f.got...)
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

// newDispatcher builds a dispatcher over st and closes it with the test.
func newDispatcher(t testing.TB, cfg dispatch.Config) *dispatch.Dispatcher {
	t.Helper()
	if cfg.Host == "" {
		cfg.Host = "host"
	}
	d, err := dispatch.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// codes lists the reason codes of a reason list.
func codes(rs []model.PlacementReasonDetail) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Code
	}
	return out
}

func hasCode(rs []model.PlacementReasonDetail, code string) bool {
	for _, r := range rs {
		if r.Code == code {
			return true
		}
	}
	return false
}
