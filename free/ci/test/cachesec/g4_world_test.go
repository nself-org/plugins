//go:build cachesec

package cachesec

// Purpose: the release-side fixtures of group G4 (P7-CACHE-11): a coordinator store holding one run ("run-1")
// with a build attempt that a remote node ran (lease holder "storetest", signing eligible, key pinned), the
// evidence that node signs, the dispatcher step that judges release attempts, and small audit readers.
// Inputs:  newWorld (real store). Outputs: g4Run, g4Evidence, g4Admit, g4AuditRows, g4Node.
// Constraints: production packages only (store, attest, release/job, dispatch, run.UseAttestStore); the node key
//          is a throwaway seed; the evidence key is registered through the production wiring (run's init
//          registers attest.RunnerVerifier over store.AttestLedger once UseAttestStore names the store).

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

const (
	g4Project  = "proj"
	g4Revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	g4Run      = "run-1"
	g4Node     = "storetest" // storetest.Admit leases every attempt to this runner id
)

// g4Stash is the Source of the dispatcher step registered below; run's own "release" step answers only for
// its private stash, so this second step (same production code, job.Step) judges the attempts of this suite.
var g4Stash = job.NewStash()

func init() {
	if err := dispatch.RegisterStep("g4release", job.Step(g4Stash)); err != nil {
		panic(err)
	}
}

// g4Signer is a node key from a fixed seed (a throwaway: it guards nothing).
type g4Signer struct{ priv ed25519.PrivateKey }

func newG4Signer(b byte) g4Signer {
	return g4Signer{ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))}
}
func (s g4Signer) pub() ed25519.PublicKey        { return s.priv.Public().(ed25519.PublicKey) }
func (s g4Signer) KeyID() string                 { return nodekey.KeyIDOf(s.pub()) }
func (s g4Signer) Sign(m []byte) ([]byte, error) { return ed25519.Sign(s.priv, m), nil }

var _ nodekey.Signer = g4Signer{}

func g4RepoID() string {
	sum := sha256.Sum256([]byte(g4Project))
	return hex.EncodeToString(sum[:8])
}

func g4Digests() model.InputDigests {
	return model.InputDigests{Config: "c" + string(bytes.Repeat([]byte("c"), 63)),
		Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: string(bytes.Repeat([]byte("1"), 64))}},
		Generated: []model.DigestRef{{Path: "gen/api.go", SHA256: string(bytes.Repeat([]byte("3"), 64))}}}
}

// g4Run creates run-1 with its build attempt leased to the node, the node registered, signing eligible and
// holding the pinned key, and the process-wide evidence verifier pointed at the world's store.
func g4Setup(w *world, trust string) g4Fixture {
	t, ctx, s := w.t, w.ctx, w.St
	steps := []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: g4Run, Revision: g4Revision, Project: g4Project, Trigger: "local", SourceTrust: trust, PrivacyZone: "local-only",
			PolicyDigest: "policy-1", InputDigest: "i", SelectionMode: "full", SelectionReason: "selection.full_required", SelectionDigest: "d", Status: "running"}),
		s.CreateJob(ctx, store.JobRow{ID: "j-build", PipelineID: g4Run, Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: "att-build", JobID: "j-build", N: 1}, ""),
		storetest.Admit(ctx, s, "att-build"),
		s.NodePut(ctx, store.NodeRecord{ID: g4Node, Name: g4Node, State: "online", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"),
			Capability: []byte(`{"separation":{"signing_eligible":{"value":true,"source":"assigned"}}}`), ObservedAt: 1}, "test"),
	}
	key := newG4Signer(7)
	steps = append(steps, s.PKINodeKeyPut(ctx, g4Node, key.KeyID(), base64.StdEncoding.EncodeToString(key.pub())))
	for i, err := range steps {
		if err != nil {
			t.Fatalf("g4 setup step %d: %v", i, err)
		}
	}
	cirun.UseAttestStore(s)
	t.Cleanup(func() { cirun.UseAttestStore(nil) })
	return g4Fixture{w: w, key: key}
}

// g4Fixture is run-1 with its node.
type g4Fixture struct {
	w   *world
	key g4Signer
}

// Facts builds the release facts of a release job depending on "build". When signed, the evidence of the
// build is the document the remote node signed; otherwise it is the coordinator's own record.
func (f g4Fixture) Facts(signed bool, bend func(*job.RunState)) job.Facts {
	f.w.t.Helper()
	d := g4Digests()
	st := job.RunState{RunID: g4Run, RepoID: g4RepoID(), Revision: g4Revision, PolicyDigest: "policy-1", ConfigDigest: d.Config, SourceTrust: "owner", Trigger: "local",
		Pipeline:  model.PipelineRef{ID: g4Run, Name: "release", Kind: "default"},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Deps:      []string{"build"}, Outcomes: []job.Dep{{Name: "build", Status: "passed", Required: true}},
		Clean: true, Bound: &d, Now: &d}
	if bend != nil {
		bend(&st)
	}
	facts := job.BuildFacts(st)
	if signed {
		ev := facts.Evidence
		jobName := "build"
		ev.Attempt, ev.Job = &model.AttemptRef{ID: "att-build", N: 1}, &jobName
		signedEv, err := attest.Sign(ev, f.key)
		if err != nil {
			f.w.t.Fatal(err)
		}
		facts.Evidence = signedEv
	}
	return facts
}

// Queue creates a queued release attempt in run-1 (job rel-<id>) and returns its id.
func (f g4Fixture) Queue(id string) string {
	f.w.t.Helper()
	ctx, s := f.w.ctx, f.w.St
	if err := s.CreateJob(ctx, store.JobRow{ID: "j-" + id, PipelineID: g4Run, Name: "publish-" + id, Kind: "release", Idempotent: false, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		f.w.t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j-" + id, N: 1}, ""); err != nil {
		f.w.t.Fatal(err)
	}
	return id
}

// Admit puts facts in the suite's stash and asks the real dispatcher to admit the release attempt on this
// machine. It returns the dispatcher's refusal (nil when admitted).
func (f g4Fixture) Admit(attempt string, facts job.Facts, local bool) *dispatch.Refusal {
	f.w.t.Helper()
	g4Stash.Put(attempt, facts)
	defer g4Stash.Forget(attempt)
	_, err := dispatch.Admit(f.w.ctx, f.w.St, dispatch.Request{AttemptID: attempt,
		Placement: sched.Placement{JobKey: attempt, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: sched.Runner{ID: "R", Provider: "operator"},
		Job: sched.Job{Key: attempt, ID: attempt}, Local: local, Host: "h-" + attempt, CapacityCPU: 4, CapacityMemMB: 4096})
	if err == nil {
		return nil
	}
	ref, ok := err.(*dispatch.Refusal)
	if !ok {
		f.w.t.Fatalf("admission failed without a refusal: %v", err)
	}
	return ref
}

// State is the attempt's state ("queued" after a refusal, "leased" after an admission).
func (f g4Fixture) State(attempt string) string {
	f.w.t.Helper()
	st, _, err := f.w.St.AttemptStateEpoch(f.w.ctx, attempt)
	if err != nil {
		f.w.t.Fatal(err)
	}
	return string(st)
}

// g4AuditRows lists the audit rows with the given action ("" for all), read from the store itself.
func g4AuditRows(w *world, action string) (out []store.AuditEntry) {
	w.t.Helper()
	rows, err := w.St.AuditEntries(context.Background(), 0, 10000)
	if err != nil {
		w.t.Fatal(err)
	}
	for _, r := range rows {
		if action == "" || r.Action == action {
			out = append(out, r)
		}
	}
	return out
}
