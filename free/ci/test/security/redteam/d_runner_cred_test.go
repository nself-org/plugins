//go:build security

package redteam

// Purpose: TM-43 red team. The lease runner_credential path (providers/runnerjob preparer, the
// transport delivery chain, the store) is attacked with: two concurrent deliveries of one attempt, a
// ledger write that fails after the mint, a byte scan of the persisted state for the credential, a
// spec with a dot-segment repository, and the printing of the credential types.
// Inputs:  an in-process fake forge minter, a capturing transport driver, a real store in t.TempDir().
// Outputs: pass = one live registration per attempt, no leak into error/state/log text (HELD).
// Constraints: no docker, no network; the credential is a synthetic string derived from fakeSecret.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const (
	dRepo   = "acme/widgets"
	dKind   = "d-fake-credential"
	dDriver = "d-runnerjob"
	dImage  = "ghcr.io/actions/actions-runner@sha256:4ffadc0002b2581327e06101fc8c06cd189232baf79fe561fac9caeb76f5e807"
)

var dEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// dForge is a minter that remembers live registrations and can stall inside Mint.
type dForge struct {
	mu      sync.Mutex
	n       int
	live    map[string]bool
	minted  []string
	barrier int          // when >0, Mint waits until this many calls are inside (or 400 ms)
	inside  atomic.Int32 // calls currently in Mint
	onMint  func()       // runs just before Mint returns
	dereg   []string
}

func (m *dForge) Provider() string { return "dfakeforge" }
func (m *dForge) Mint(_ context.Context, r runnerjob.Request) (runnerjob.Registration, error) {
	m.inside.Add(1)
	for deadline := time.Now().Add(400 * time.Millisecond); m.barrier > 0 && int(m.inside.Load()) < m.barrier && time.Now().Before(deadline); {
		time.Sleep(2 * time.Millisecond)
	}
	m.mu.Lock()
	m.n++
	h, cred := "repo:"+strconv.Itoa(100+m.n), fakeSecret+"-jit-"+strconv.Itoa(m.n)
	m.live[h] = true
	m.minted = append(m.minted, cred)
	hook := m.onMint
	m.mu.Unlock()
	if hook != nil {
		hook()
	}
	return runnerjob.Registration{Handle: h, Credential: cred, ExpiresAt: dEpoch.Add(time.Hour)}, nil
}
func (m *dForge) Deregister(_ context.Context, _, handle string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dereg = append(m.dereg, handle)
	delete(m.live, handle)
	return nil
}
func (*dForge) Orphans(context.Context, string, time.Time) ([]string, error) { return nil, nil }
func (m *dForge) alive() int                                                 { m.mu.Lock(); defer m.mu.Unlock(); return len(m.live) }

type dCapture struct {
	mu    sync.Mutex
	lease protocol.Lease
}

func (*dCapture) Kind() string { return dDriver }
func (c *dCapture) Run(_ context.Context, _ model.Capability, l protocol.Lease, _ transport.Sink) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lease = l
	return nil
}

// dFlaky is the store with a switch that makes every fact write fail.
type dFlaky struct {
	*store.Store
	fail atomic.Bool
}

func (f *dFlaky) ProviderFactPut(ctx context.Context, p store.ProviderFact) (bool, error) {
	if f.fail.Load() {
		return false, errors.New("disk full")
	}
	return f.Store.ProviderFactPut(ctx, p)
}

var (
	dForgeV  = &dForge{live: map[string]bool{}}
	dDrv     = &dCapture{}
	dSetup   sync.Once
	dSetupEr error
)

func dUse(t *testing.T) (*dFlaky, string) {
	t.Helper()
	dSetup.Do(func() {
		for _, err := range []error{runnerjob.RegisterMinter(dKind, dForgeV), transport.Register(dDrv), runnerjob.InstallCoordinator()} {
			if err != nil {
				dSetupEr = err
			}
		}
	})
	if dSetupEr != nil {
		t.Fatal(dSetupEr)
	}
	t.Setenv("NSELF_V15", "1")
	dForgeV.mu.Lock()
	dForgeV.n, dForgeV.live, dForgeV.minted, dForgeV.dereg, dForgeV.barrier, dForgeV.onMint = 0, map[string]bool{}, nil, nil, 0, nil
	dForgeV.mu.Unlock()
	dForgeV.inside.Store(0)
	dDrv.mu.Lock()
	dDrv.lease = protocol.Lease{}
	dDrv.mu.Unlock()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fl := &dFlaky{Store: st}
	t.Cleanup(func() { runnerjob.Use(nil); _ = st.Close() })
	runnerjob.Use(runnerjob.NewLedger(fl, func() time.Time { return dEpoch }))
	return fl, dir
}

func dFact[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", Confidence: "known", ObservedAt: dEpoch}
}

func dNode() model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity.ID, c.Identity.Transport = "node-d", dDriver
	c.Platform.OS = dFact("linux")
	c.Trust.Isolation = dFact(model.Isolation("sandboxed-container"))
	c.Trust.Network = dFact([]model.NetworkScope{"none", "restricted"})
	c.Tools.GVisor = dFact(true)
	return c
}

func dSpec(demand string) runnerjob.Spec {
	return runnerjob.Spec{Provider: "dfakeforge", Repo: dRepo, Demand: demand, Image: dImage, Credential: dKind, Arch: "amd64", Command: []string{"/home/runner/run.sh"}}
}

func dJob(t *testing.T, demand string) model.Job {
	t.Helper()
	p, err := runnerjob.Synthesize(dSpec(demand))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := runnerjob.CurrentLedger().Reserve(context.Background(), "dfakeforge", dRepo, demand, 50); err != nil || !ok {
		t.Fatalf("reserve: %v %v", ok, err)
	}
	return p.Jobs[0]
}

func dDeliver(job model.Job, attempt string) (protocol.Lease, error) {
	l := protocol.Lease{LeaseID: "lease-" + attempt, AttemptID: attempt, Job: job}
	err := transport.Deliver(context.Background(), dNode(), l, nil)
	dDrv.mu.Lock()
	defer dDrv.mu.Unlock()
	return dDrv.lease, err
}

// D30: the same attempt delivered twice at once (a retransmit racing the first delivery) must leave
// exactly one live registration, the one the ledger knows, so teardown can remove it.
func TestD30_ConcurrentDeliveryLeavesOneLiveCredential(t *testing.T) {
	if !openFinding(t, "D30") {
		return
	}
	_, _ = dUse(t)
	job := dJob(t, "7001")
	dForgeV.barrier = 2
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = dDeliver(job, "d-att-1") }()
	}
	wg.Wait()
	row, ok, err := runnerjob.CurrentLedger().Runner(context.Background(), "d-att-1")
	if err != nil || !ok {
		t.Fatalf("ledger row: ok=%v err=%v", ok, err)
	}
	if n := dForgeV.alive(); n != 1 || !dForgeV.live[row.Handle] {
		t.Fatalf("%d live registrations after two deliveries of one attempt; the ledger tracks only %q (minted %d)", n, row.Handle, len(dForgeV.minted))
	}
}

// D31: a ledger write that fails after the mint removes the registration again, hands out no
// credential and leaks nothing into the error text.
func TestD31_LedgerFailureAfterMintDeregistersAndDoesNotLeak(t *testing.T) {
	fl, _ := dUse(t)
	job := dJob(t, "7002")
	dForgeV.onMint = func() { fl.fail.Store(true) }
	l, err := dDeliver(job, "d-att-2")
	if err == nil {
		t.Fatal("delivery succeeded although the ledger could not record the registration")
	}
	if l.RunnerCredential != nil {
		t.Fatal("a lease carries a credential whose registration is not recorded")
	}
	for _, c := range dForgeV.minted {
		if strings.Contains(err.Error(), c) {
			t.Fatalf("error text carries the credential: %v", err)
		}
	}
	if n := dForgeV.alive(); n != 0 {
		t.Fatalf("%d registration(s) left live after the ledger failure", n)
	}
}

// D32: after a good delivery the credential is on the lease and nowhere in the persisted store files.
func TestD32_CredentialNotPersistedInTheStore(t *testing.T) {
	_, dir := dUse(t)
	l, err := dDeliver(dJob(t, "7003"), "d-att-3")
	if err != nil || l.RunnerCredential == nil {
		t.Fatalf("deliver: %v", err)
	}
	cred, handle := l.RunnerCredential.Value, "repo:101"
	var read, hitCred, hitHandle int
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		read++
		if bytes.Contains(b, []byte(cred)) {
			hitCred++
		}
		if bytes.Contains(b, []byte(handle)) {
			hitHandle++
		}
		return nil
	})
	if read == 0 || hitHandle == 0 {
		t.Fatalf("positive control failed: read %d files, handle found in %d", read, hitHandle)
	}
	if hitCred != 0 {
		t.Fatalf("the credential is persisted in %d store file(s)", hitCred)
	}
}

// D33: a repository value of "acme/.." passes Spec.Validate, so a dot-segment name reaches the minter.
func TestD33_SpecRefusesDotSegmentRepository(t *testing.T) {
	if !openFinding(t, "D33") {
		return
	}
	for _, repo := range []string{"acme/..", "acme/.", "../x"} {
		s := dSpec("1")
		s.Repo = repo
		if _, err := runnerjob.Synthesize(s); err == nil {
			t.Errorf("Synthesize accepted repository %q", repo)
		}
	}
}

// D34: the credential types print without their value in fmt, %#v and slog, as the JIT Minted type does.
func TestD34_CredentialTypesRedactWhenPrinted(t *testing.T) {
	if !openFinding(t, "D34") {
		return
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	pc := &protocol.RunnerCredential{Kind: "github-jitconfig", Value: fakeSecret, ExpiresAt: "x"}
	rc := providers.RunnerCredential{Kind: "github-jitconfig", Value: fakeSecret}
	log.Info("lease", "cred", pc, "provider_cred", rc)
	out := buf.String() + fmt.Sprintf("%v %+v %#v %s %v %+v", pc, pc, pc, pc, rc, rc)
	if strings.Contains(out, fakeSecret) {
		t.Fatalf("a printed credential type leaks its value: %.200s", out)
	}
}
