//go:build cachesec

package cachesec

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// releaseFacts builds the facts of a release whose one dep ended with status
// (the coordinator's own evidence, consistent by construction).
func releaseFacts(status string) job.Facts {
	d := model.InputDigests{Config: strings.Repeat("c", 64), Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}}}
	return job.BuildFacts(job.RunState{RunID: "pbuild", RepoID: "project", Revision: strings.Repeat("a", 40), PolicyDigest: "policy", ConfigDigest: d.Config, SourceTrust: "owner", Trigger: "local",
		Pipeline: model.PipelineRef{ID: "pbuild", Name: "release", Kind: "default"}, Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "digest"}, Deps: []string{"build"},
		Outcomes: []job.Dep{{Name: "build", Status: status, Required: true}}, Clean: true, Bound: &d, Now: &d})
}

type evidenceSigner struct{ priv ed25519.PrivateKey }

func (s evidenceSigner) KeyID() string                 { return nodekey.KeyIDOf(s.priv.Public().(ed25519.PublicKey)) }
func (s evidenceSigner) Sign(b []byte) ([]byte, error) { return ed25519.Sign(s.priv, b), nil }

// evidenceLedger supplies registry facts while recording consumption and
// quarantine in the actual local store.
type evidenceLedger struct {
	store.AttestLedger
	facts attest.AttemptFacts
	pub   ed25519.PublicKey
}

func (l evidenceLedger) AttemptFacts(context.Context, string) (attest.AttemptFacts, bool, error) {
	return l.facts, true, nil
}
func (l evidenceLedger) NodesByKey(context.Context, string) ([]attest.KeyNode, error) {
	return []attest.KeyNode{{NodeID: "node", Public: l.pub, SigningEligible: true}}, nil
}

func (r *rig) release(id string) error {
	f := releaseFacts("passed")
	switch id {
	case "release-no-deps":
		// The dep failed: the evidence says so in its result, its check and its counts.
		f = releaseFacts("failed")
	case "untrusted-promotion":
		f.Evidence.SourceTrust = "untrusted"
	case "release-signed-remote", "release-revoked-remote":
		r.running("build", owner())
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		must(r.t, err)
		must(r.t, r.st.NodePut(r.ctx, store.NodeRecord{ID: "node", Name: "node", State: "online", Capability: []byte("{}")}, "test"))
		f.Evidence.Attempt = &model.AttemptRef{ID: "build", N: 1}
		name := "build"
		f.Evidence.Job = &name
		f.Evidence, err = attest.Sign(f.Evidence, evidenceSigner{priv})
		must(r.t, err)
		led := evidenceLedger{AttestLedger: store.AttestLedger{S: r.st}, pub: pub, facts: attest.AttemptFacts{RunnerID: "node", Revision: f.Revision, RunID: "pbuild", RepoID: f.RepoID, SourceTrust: "owner", PolicyDigest: f.PolicyDigest, SelectionMode: "full", Job: "build"}}
		v := &attest.RunnerVerifier{Ledger: func() attest.Ledger { return led }}
		attest.RegisterRemoteVerifier(v)
		defer attest.RegisterRemoteVerifier(nil)
		raw, err := json.Marshal(f.Evidence)
		must(r.t, err)
		must(r.t, v.VerifyEvidence(raw, f.Evidence))
		if e := job.Evaluate(f); e != nil {
			r.t.Fatalf("valid signed-remote control refused: %v", e)
		}
		if id == "release-signed-remote" {
			return nil
		}
		_, err = (attest.Revoker{Ledger: led.AttestLedger, Audit: r.st.AppendAudit}).Revoke(r.ctx, "node", "test")
		must(r.t, err)
		q, err := led.Quarantined(r.ctx, "pbuild")
		must(r.t, err)
		if !q {
			r.t.Error("revoked evidence was not quarantined")
		}
		if e := job.Evaluate(f); e == nil {
			r.t.Error("revoked node evidence admitted a release")
		}
		return v.VerifyEvidence(raw, f.Evidence)
	}
	// The admission path a release attempt takes (run's Dispatch wrapper), with the run's store as audit sink.
	st := job.NewStash()
	st.Audit = r.st.AppendAudit
	queued := false
	err := st.Guard(r.ctx, "release-attempt", job.Static(f), func() error { queued = true; return nil })
	if err != nil && queued {
		r.t.Error("a refused release attempt was queued")
	}
	return err
}
