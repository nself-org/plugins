package runnerjob_test

// Purpose: shared fixtures for the runnerjob tests: a real store, a fake forge minter, a capturing
// transport driver and the capability of a node that may carry a runner job.
// Inputs:  synthetic values only (forge "fakeforge", repository acme/widgets, credentials that exist nowhere).
// Outputs: helpers; no test lives here.
// Constraints: no network. The minter, the preparer, the finalizer and the sweep register once per test
//          binary (the registries are process-wide); each test resets the fake's behaviour.

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const (
	testRepo   = "acme/widgets"
	testKind   = "fake-credential"
	testDriver = "runnerjob-test"
	testImage  = "ghcr.io/actions/actions-runner@sha256:4ffadc0002b2581327e06101fc8c06cd189232baf79fe561fac9caeb76f5e807"
)

var epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// tick is the test clock: tests move it with advance.
var tick = struct {
	sync.Mutex
	t time.Time
}{t: epoch}

func clock() time.Time { tick.Lock(); defer tick.Unlock(); return tick.t }

func advance(d time.Duration) { tick.Lock(); tick.t = tick.t.Add(d); tick.Unlock() }

// fakeMinter is a forge: it hands out numbered credentials and remembers which registrations exist.
type fakeMinter struct {
	mu          sync.Mutex
	next        int
	live        map[string]time.Time // handle -> born
	minted      []string             // credentials, in order
	mintErr     error
	deregErr    error
	orphansErr  error
	deregCalls  []string
	orphanAfter map[string]bool // handles Orphans reports
	lastCutoff  time.Time
}

func (m *fakeMinter) Provider() string { return "fakeforge" }

func (m *fakeMinter) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next, m.live, m.minted, m.mintErr, m.deregErr, m.orphansErr, m.deregCalls, m.orphanAfter = 0, map[string]time.Time{}, nil, nil, nil, nil, nil, map[string]bool{}
}

func (m *fakeMinter) Mint(_ context.Context, r runnerjob.Request) (runnerjob.Registration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mintErr != nil {
		return runnerjob.Registration{}, m.mintErr
	}
	m.next++
	h, cred := "repo:"+strconv.Itoa(100+m.next), "synthetic-jit-"+r.Repo+"-"+strconv.Itoa(m.next)
	m.live[h] = epoch
	m.minted = append(m.minted, cred)
	return runnerjob.Registration{Handle: h, Credential: cred, ExpiresAt: epoch.Add(time.Hour)}, nil
}

func (m *fakeMinter) Deregister(_ context.Context, _, handle string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deregCalls = append(m.deregCalls, handle)
	if m.deregErr != nil {
		return m.deregErr
	}
	delete(m.live, handle)
	return nil
}

func (m *fakeMinter) Orphans(_ context.Context, _ string, before time.Time) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastCutoff = before
	if m.orphansErr != nil {
		return nil, m.orphansErr
	}
	var out []string
	for h := range m.orphanAfter {
		out = append(out, h)
	}
	return out, nil
}

func (m *fakeMinter) alive() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.live) }

// capture is a transport driver that keeps the lease it was handed.
type capture struct {
	mu    sync.Mutex
	lease protocol.Lease
	runs  int
}

func (*capture) Kind() string { return testDriver }
func (c *capture) Run(_ context.Context, _ model.Capability, l protocol.Lease, _ transport.Sink) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lease, c.runs = l, c.runs+1
	return nil
}

var (
	forge   = &fakeMinter{}
	driver  = &capture{}
	setupMu sync.Once
)

// use binds a fresh store and the fake forge for one test and returns the store.
func use(t *testing.T) *store.Store {
	t.Helper()
	st, _ := useAt(t)
	return st
}

// useAt is use that also returns the directory of the store.
func useAt(t *testing.T) (*store.Store, string) {
	t.Helper()
	setupMu.Do(func() {
		for _, err := range []error{runnerjob.RegisterMinter(testKind, forge), transport.Register(driver), runnerjob.InstallCoordinator()} {
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Setenv("NSELF_V15", "1")
	forge.reset()
	driver.mu.Lock()
	driver.lease, driver.runs = protocol.Lease{}, 0
	driver.mu.Unlock()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close(); runnerjob.Use(nil) })
	tick.Lock()
	tick.t = epoch
	tick.Unlock()
	runnerjob.Use(runnerjob.NewLedger(st, clock))
	return st, dir
}

func known[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", Confidence: "known", ObservedAt: epoch}
}

// goodNode is a Linux node with gVisor and the restricted scope.
func goodNode() model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity.ID, c.Identity.Transport = "node-1", testDriver
	c.Platform.OS = known("linux")
	c.Trust.Isolation = known(model.Isolation("sandboxed-container"))
	c.Trust.Network = known([]model.NetworkScope{"none", "restricted"})
	c.Tools.GVisor = known(true)
	c.Availability.PluggedIn, c.Availability.Interactive = known(true), known("idle") // the background band needs both
	return c
}

// runnerSpec is a valid Spec for the fake forge.
func runnerSpec(demand string) runnerjob.Spec {
	return runnerjob.Spec{Provider: "fakeforge", Repo: testRepo, Demand: demand, Image: testImage, Credential: testKind, Arch: "amd64", Command: []string{"/home/runner/run.sh"}}
}

func runnerJob(t *testing.T, demand string) model.Job {
	t.Helper()
	p, err := runnerjob.Synthesize(runnerSpec(demand))
	if err != nil {
		t.Fatal(err)
	}
	return p.Jobs[0]
}

func deliver(node model.Capability, attempt string, job model.Job) (protocol.Lease, error) {
	l := protocol.Lease{LeaseID: "lease-" + attempt, AttemptID: attempt, Job: job}
	err := transport.Deliver(context.Background(), node, l, nil)
	driver.mu.Lock()
	defer driver.mu.Unlock()
	return driver.lease, err
}

func ledgerRunner(attempt string) (runnerjob.Runner, bool, error) {
	return runnerjob.CurrentLedger().Runner(context.Background(), attempt)
}

func jobRoot() session.JobRoot { return session.JobRoot{Dir: "/nonexistent"} }
func outcome() session.Outcome { return session.Outcome{State: "passed"} }
