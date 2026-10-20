package health_test

// Purpose: the real hosted stack the P7-HOST-11 tests run through: the GitHub-hosted provider (a
//          second instance with its own id, pointed at the fake API), the providers world
//          contributor, the scheduler, the dispatcher and the "provider" transport driver, over a
//          real store. This is the path `ci explain` and a hosted placement take.
// Inputs:  a fakeAPI and a tracker per test.
// Outputs: a stack with Submit (place and deliver one attempt), World/Explain (the placement
//          document `nself ci explain --json` prints) and the delivery errors seen.
// Constraints: no live network; the hosted provider's clock advances on its Wait, so a lookup
//          window ends without sleeping; the shared provider instance is registered once per
//          process (the registry has no unregister) and reads the fake of the current test.

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

const provID = "github-hosted-h11"

var (
	stackOnce  sync.Once
	stackAPI   atomic.Pointer[fakeAPI]
	stackClock = newClock()
)

// registerProvider registers the test instance of the GitHub-hosted provider once.
func registerProvider(t *testing.T) {
	t.Helper()
	stackOnce.Do(func() {
		p := hosted.New(hosted.Config{ID: provID,
			Client:   func(context.Context) (*rest.Client, error) { return stackAPI.Load().client(), nil },
			Settings: func(context.Context) (hosted.Settings, error) { return hosted.DefaultSettings(), nil },
			Clock:    stackClock.Now,
			Wait:     func(_ context.Context, d time.Duration) error { stackClock.advance(d); return nil },
		})
		if err := providers.Register(p); err != nil {
			t.Fatal(err)
		}
		if err := runtime.RegisterCredential(provID, func(context.Context) rest.AuthFunc {
			return func(context.Context) (string, error) { return testToken, nil }
		}); err != nil {
			t.Fatal(err)
		}
	})
}

// stack is one test's dispatcher, store and fake API.
type stack struct {
	t    *testing.T
	st   *store.Store
	api  *fakeAPI
	d    *dispatch.Dispatcher
	sink *recSink
	mu   sync.Mutex
	errs []error
	made int
}

// carrier adapts the transport registry to dispatch.Transport the way the production wiring does.
type carrier struct{ sink transport.Sink }

func (carrier) Known(kind string) bool { _, ok := transport.Lookup(kind); return ok }
func (c carrier) Deliver(ctx context.Context, d dispatch.Delivery) error {
	l := protocol.Lease{LeaseID: d.LeaseID, AttemptID: d.AttemptID, Epoch: uint64(d.Epoch), Token: d.Token, CompatMode: protocol.Compat15, Job: d.Job,
		Checkout: protocol.Checkout{Revision: d.Revision}, Secrets: []protocol.SecretRef{}, ExpiresAt: d.ExpiresAt.UTC().Format(time.RFC3339)}
	return transport.Deliver(ctx, d.Node, l, c.sink)
}

type recSink struct{}

func (recSink) Result(context.Context, protocol.Result) error           { return nil }
func (recSink) Log(context.Context, protocol.Log) error                 { return nil }
func (recSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (recSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (recSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, context.Canceled
}

// hostedPolicy is an effective policy whose project authorizes hosted execution.
func hostedPolicy(t *testing.T) trust.Effective {
	t.Helper()
	r, err := trust.NewRegistry(trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"})
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"hosted-allowed"`)}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// newStack wires the stack for api. The tracker is installed as the process-wide one.
func newStack(t *testing.T, api *fakeAPI, tr *health.Tracker) *stack {
	t.Helper()
	install(t, tr)
	registerProvider(t)
	stackAPI.Store(api)
	st := openStore(t, nil)
	runtime.UseStore(st)
	runtime.DefaultContributor().SetTargets(func(context.Context) ([]providers.Query, error) {
		return []providers.Query{{Project: "project", Repo: "o/r"}}, nil
	})
	runtime.DefaultContributor().ForgetCredentials()
	t.Cleanup(func() {
		runtime.DefaultContributor().SetTargets(nil)
		runtime.DefaultContributor().SetStore(nil)
	})
	s := &stack{t: t, st: st, api: api, sink: &recSink{}}
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", World: s.buildWorld, Transport: carrier{s.sink}, Poll: time.Millisecond, Sink: decisionLog{t},
		DeliveryError: func(_ dispatch.Admitted, err error) { s.mu.Lock(); s.errs = append(s.errs, err); s.mu.Unlock() }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	s.d = d
	return s
}

func (s *stack) policy(context.Context, string) (sched.Policy, error) {
	return sched.Policy{Preset: "balanced", Trust: hostedPolicy(s.t)}, nil
}

// buildWorld is the world of `ci explain` and of a run: this machine as "local" plus the hosted
// runners the providers contributor offers, with a cost and an allowance for each hosted runner.
func (s *stack) buildWorld(ctx context.Context, pl sched.Pipeline) (sched.World, string, error) {
	now := time.Now()
	capacity := execCapacity()
	w, digest, err := world.BuildForLocal(ctx, world.Deps{Store: s.st, Policy: s.policy, Clock: func() time.Time { return now }},
		world.LocalInput{Project: "project", Platform: "linux/amd64", Capacity: capacity, DockerUnknown: true, Now: now}, pl)
	if err != nil {
		return w, digest, err
	}
	w.Costs = map[string]sched.Cost{}
	for _, r := range w.Hosted {
		w.Costs[r.ID] = sched.Cost{Class: "included", RateMicro: 1, BillingIncrementMs: 60000}
	}
	w.Allowances = map[string]sched.Allowance{provID: {RemainingEstimated: 1_000_000, Known: true, CeilingKnown: true}}
	return w, digest, nil
}

func schedJob(id string, remoteOnly bool) sched.Job {
	return sched.Job{Key: id, ID: id, Name: id, Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "internet",
		TimeoutMs: 60000, PersonalOwnRun: true, Project: "project", RemoteOnly: remoteOnly}
}

// queue creates the pipeline (once), the job and attempt number n of id.
func (s *stack) queue(id string, n int) string {
	s.t.Helper()
	ctx := context.Background()
	s.mu.Lock()
	first := s.made == 0
	s.made++
	s.mu.Unlock()
	if first {
		if err := s.st.CreatePipeline(ctx, store.PipelineRow{ID: "pipe", Revision: testRev, Project: "project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
			s.t.Fatal(err)
		}
	}
	jobID := "job-" + id
	if n == 1 {
		if err := s.st.CreateJob(ctx, store.JobRow{ID: jobID, PipelineID: "pipe", Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
			s.t.Fatal(err)
		}
	}
	aid := id + ".a" + itoa(n)
	if err := s.st.CreateAttempt(ctx, store.AttemptRow{ID: aid, JobID: jobID, N: n}, ""); err != nil {
		s.t.Fatal(err)
	}
	return aid
}

// submit places and admits one attempt; it returns when admitted or when ctx ends.
func (s *stack) submit(ctx context.Context, attemptID, id string, remoteOnly bool) (dispatch.Admitted, error) {
	sj := schedJob(attemptID, remoteOnly)
	sj.Name = id
	return s.d.Submit(ctx, dispatch.Job{AttemptID: attemptID, Sched: sj, Revision: testRev, Model: model.Job{Kind: "test", Run: []string{"true"}, Timeout: "5m"}})
}

// deliveryErrors returns the errors the carriers ended with.
func (s *stack) deliveryErrors() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]error(nil), s.errs...)
}

// waitDeliveryErrors waits for n carrier errors.
func (s *stack) waitDeliveryErrors(n int) []error {
	s.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if got := s.deliveryErrors(); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.t.Fatalf("timed out waiting for %d delivery errors, have %v", n, s.deliveryErrors())
	return nil
}

// explain is the placement document `nself ci explain --json` prints for the jobs, as JSON.
func (s *stack) explain(jobs ...sched.Job) string {
	s.t.Helper()
	pipe := sched.Pipeline{ID: "pipe"}
	w, digest, err := s.buildWorld(context.Background(), pipe)
	if err != nil {
		s.t.Fatal(err)
	}
	doc := planview.Build(planview.Input{Name: "pipe", Revision: testRev, Trigger: "local", WorldDigest: digest, World: w, Jobs: jobs, Result: sched.Place(w, jobs)})
	raw, err := json.Marshal(doc)
	if err != nil {
		s.t.Fatal(err)
	}
	return string(raw)
}

func execCapacity() ciexec.Capacity {
	return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
}

// decisionLog logs every placement decision, so a job that never places says why.
type decisionLog struct{ t *testing.T }

func (l decisionLog) Record(_ context.Context, d dispatch.Decision) {
	l.t.Logf("decision outcome=%s reason=%s reasons=%+v", d.Outcome, d.Reason, d.Explanation.Reasons)
}

func storeDetail(epoch int64) store.TransitionDetail { return store.TransitionDetail{Epoch: epoch} }
