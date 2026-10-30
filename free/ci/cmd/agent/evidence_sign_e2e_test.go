package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// signedRig is a coordinator store with one signing-eligible node whose pinned key is the agent's node key, and
// a source tree at a real commit the attempts run against.
type signedRig struct {
	*pullRig
	key  *nodekey.Key
	repo string
	head string
}

func newSignedRig(t *testing.T) *signedRig {
	t.Helper()
	skipUnixOnly(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	state := useState(t)
	r := &signedRig{pullRig: newPullRig(t), repo: t.TempDir()}
	r.enroll(state, "n-1", "https://127.0.0.1:1") // the laptop fixture is signing eligible
	var err error
	if r.key, err = nodekey.LoadOrCreate(state); err != nil {
		t.Fatal(err)
	}
	if err = r.st.PKINodeKeyPut(context.Background(), "n-1", r.key.KeyID(), r.key.PublicString()); err != nil {
		t.Fatal(err)
	}
	git(t, r.repo, "init", "-q")
	if err = os.WriteFile(filepath.Join(r.repo, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r.repo, "add", "f.txt")
	git(t, r.repo, "commit", "-q", "-m", "a")
	r.head = git(t, r.repo, "rev-parse", "HEAD")
	return r
}

// attempt records a running attempt of job "build" in project proj-1 at the rig's commit, leased to n-1.
func (r *signedRig) attempt(n string) string {
	r.t.Helper()
	ctx := context.Background()
	p := store.PipelineRow{ID: "p" + n, Project: "proj-1", Revision: r.head, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "policy-" + n,
		InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for _, f := range []func() error{
		func() error { return r.st.CreatePipeline(ctx, p) },
		func() error {
			return r.st.CreateJob(ctx, store.JobRow{ID: "j" + n, PipelineID: p.ID, Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a" + n, JobID: "j" + n, N: 1}, "") },
	} {
		if err := f(); err != nil {
			r.t.Fatal(err)
		}
	}
	if _, err := r.eng.Grant(ctx, "host", store.Demand{AttemptID: "a" + n, LeaseID: "l" + n, RunnerID: "n-1", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: r.leaseTTL()}, 1); err != nil {
		r.t.Fatal(err)
	}
	return "a" + n
}

// run runs the lease through the agent's lease path and returns the frame's evidence bytes (canonical JSON, as the
// session sends them) and the node's signature over them.
func (r *signedRig) run(l protocol.Lease) (frame []byte, keyID, sig string) {
	r.t.Helper()
	l.Checkout, l.Job = protocol.Checkout{Revision: r.head}, model.Job{Kind: "static", Run: []string{"true"}}
	res, err := leaseRunner{coordinator: "coord"}.Run(context.Background(), session.JobRun{Lease: l, Setup: session.JobSetup{SourceDir: r.repo}})
	if err != nil {
		r.t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(res.Evidence))
	dec.UseNumber()
	var v any
	if err = dec.Decode(&v); err != nil {
		r.t.Fatal(err)
	}
	if frame, err = json.Marshal(v); err != nil {
		r.t.Fatal(err)
	}
	s := evidenceSigner{key: r.key}
	raw, err := s.Sign(frame)
	if err != nil {
		r.t.Fatal(err)
	}
	return frame, s.KeyID(), base64.RawStdEncoding.EncodeToString(raw)
}

func (r *signedRig) verify(frame []byte, keyID, sig string) attest.RemoteReason {
	err := attest.VerifyFrame(func() attest.Ledger { return store.AttestLedger{S: r.st} }, frame, keyID, sig)
	if err == nil {
		return ""
	}
	var re *attest.RemoteError
	if !errors.As(err, &re) {
		r.t.Fatalf("VerifyFrame = %v", err)
	}
	return re.Reason
}

// D-1299 end to end: the coordinator puts the attempt's recorded facts into the lease (run.LeaseEvidence, the
// read the delivery path makes), the agent runs the lease against a real checkout and names them in its
// evidence, the node key signs the frame bytes, and the coordinator's frame check (attest.VerifyFrame with the
// real store ledger) accepts it. A lease without the context (the agent's own run id, repo unknown, source
// untrusted) and a context the node altered are both refused.
func TestSignedRemoteEvidenceVerifiesAgainstTheCoordinatorRecord(t *testing.T) {
	r := newSignedRig(t)
	ctx := context.Background()

	bare := r.attempt("1")
	if reason := r.verify(r.run(protocol.Lease{AttemptID: bare, Trust: "owner"})); reason != attest.RemoteContextMismatch {
		t.Fatalf("evidence without the lease's context: reason %q, want %s", reason, attest.RemoteContextMismatch)
	}

	forged := r.attempt("2")
	ev, err := cirun.LeaseEvidence(ctx, r.st, forged)
	if err != nil || ev == nil {
		t.Fatalf("LeaseEvidence = %v, %v", ev, err)
	}
	ev.PolicyDigest = "a-policy-the-node-prefers"
	if reason := r.verify(r.run(protocol.Lease{AttemptID: forged, Trust: "owner", Evidence: ev})); reason != attest.RemoteContextMismatch {
		t.Fatalf("evidence with an altered context: reason %q, want %s", reason, attest.RemoteContextMismatch)
	}

	good := r.attempt("3")
	if ev, err = cirun.LeaseEvidence(ctx, r.st, good); err != nil || ev == nil {
		t.Fatalf("LeaseEvidence = %v, %v", ev, err)
	}
	frame, keyID, sig := r.run(protocol.Lease{AttemptID: good, Trust: "owner", Evidence: ev})
	if reason := r.verify(frame, keyID, sig); reason != "" {
		t.Fatalf("signed evidence for the recorded attempt was refused: %s\n%s", reason, frame)
	}
	var doc model.Evidence
	if err = json.Unmarshal(frame, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Attempt == nil || doc.Attempt.ID != good || doc.Job == nil || *doc.Job != "build" || doc.Binding.State != "bound" || doc.RepoID != ev.RepoID {
		t.Fatalf("the signed document does not name the attempt it ran: %s", frame)
	}
}
