package dispatch_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	releasejob "github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// consumingVerifier isolates the verifier's real writer-lock dependency from
// cryptography, which is covered by attest's real-ledger signature tests.
type consumingVerifier struct {
	ledger store.AttestLedger
	calls  int
}

func (v *consumingVerifier) VerifyEvidence(_ []byte, ev model.Evidence) error {
	v.calls++
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	return v.ledger.Consume(ctx, attest.Use{AttemptID: "dep", KeyID: "key", SigDigest: "sig", NodeID: "signer", RunID: ev.RunID})
}

func TestAdmitSignedRemoteReleaseFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,created_at,updated_at) VALUES('signer','signer',1,'{}','{}','{}','online',1,1)`); err != nil {
		t.Fatal(err)
	}
	id := dispatch.QueueAttempt(t, st, "P", "J", "release")
	inputs := model.InputDigests{Config: "config"}
	f := releasejob.BuildFacts(releasejob.RunState{RunID: "P", RepoID: "repo", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PolicyDigest: "policy", ConfigDigest: inputs.Config, SourceTrust: "owner", Trigger: "local", Pipeline: model.PipelineRef{ID: "P", Name: "release", Kind: "default"}, Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"}, Deps: []string{"build"}, Outcomes: []releasejob.Dep{{Name: "build", Status: "passed", Required: true}}, Clean: true, Bound: &inputs, Now: &inputs})
	f.Evidence.Producer.Kind = "signed-remote"
	key, sig := "ci-node-0123456789abcdef", "c2ln"
	f.Evidence.Signature = model.Signature{Kind: "runner-ed25519", KeyID: &key, Value: &sig}
	verifier := &consumingVerifier{ledger: store.AttestLedger{S: st}}
	attest.RegisterRemoteVerifier(verifier)
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	stash := releasejob.NewStash()
	dispatch.ResetSteps("limits")
	t.Cleanup(func() { dispatch.ResetSteps("limits") })
	if err := dispatch.RegisterStep("release", releasejob.Step(stash)); err != nil {
		t.Fatal(err)
	}
	err = stash.Guard(context.Background(), id, releasejob.Static(f), func() error {
		r := dispatch.SchedRunner("R", "local", 8)
		q := dispatch.QueuedJob(id)
		_, err := dispatch.Admit(context.Background(), st, dispatch.Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: r, Job: q.Sched, Local: true, Host: "host", CapacityCPU: 4, CapacityMemMB: 4096})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if verifier.calls != 1 {
		t.Fatalf("verified %d times; admission must use the pre-transaction decision", verifier.calls)
	}
}
