package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// TestLeaseEvidenceVerifies proves coordinator context survives the wire, job execution,
// agent signing and the coordinator's remote verifier, including retry numbering.
func TestLeaseEvidenceVerifies(t *testing.T) {
	for _, tc := range []struct {
		name, selection string
		n               int
	}{{"first", "full", 1}, {"retry", "affected", 3}} {
		t.Run(tc.name, func(t *testing.T) {
			useState(t)
			facts := attest.AttemptFacts{RunnerID: "node-1", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RunID: "coordinator-run", RepoID: "repo-42", SourceTrust: "owner", PolicyDigest: "policy-42", SelectionMode: tc.selection, Job: "unit-tests"}
			body, err := json.Marshal(map[string]any{
				"lease_id": "lease-42", "epoch": 1, "token": "", "compat_mode": protocol.Compat15, "secrets": []protocol.SecretRef{}, "expires_at": "2030-01-01T00:00:00Z",
				"attempt_id": "attempt-42", "job": model.Job{Kind: "test", Run: []string{"true"}, Isolation: "process"},
				"checkout": protocol.Checkout{Revision: facts.Revision},
				"evidence": map[string]any{"run_id": facts.RunID, "repo_id": facts.RepoID, "source_trust": facts.SourceTrust, "policy_digest": facts.PolicyDigest, "selection_mode": facts.SelectionMode, "job": facts.Job, "attempt_n": tc.n},
			})
			if err != nil {
				t.Fatal(err)
			}
			epoch := uint64(1)
			frame, err := json.Marshal(protocol.Message{V: 1, Type: "lease", Seq: 1, LeaseID: "lease-42", Epoch: &epoch, Body: json.RawMessage(body)})
			if err != nil {
				t.Fatal(err)
			}
			message, err := protocol.NewDecoder(bytes.NewReader(append(frame, '\n')), protocol.CarrierPolicy{}).Decode()
			if err != nil {
				t.Fatal(err)
			}
			l := *message.Body.(*protocol.Lease)
			res, err := (leaseRunner{coordinator: "coordinator"}).Run(context.Background(), session.JobRun{Lease: l})
			if err != nil {
				t.Fatal(err)
			}
			var ev model.Evidence
			if err = json.Unmarshal(res.Evidence, &ev); err != nil {
				t.Fatal(err)
			}
			key, err := nodekey.LoadOrCreate(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			signer := evidenceSigner{key: key}
			raw, err := signer.Sign(res.Evidence)
			if err != nil {
				t.Fatal(err)
			}
			keyID, value := signer.KeyID(), base64.RawStdEncoding.EncodeToString(raw)
			ev.Producer.Kind = "signed-remote"
			ev.Signature = model.Signature{Kind: "runner-ed25519", KeyID: &keyID, Value: &value}
			doc, err := json.Marshal(ev)
			if err != nil {
				t.Fatal(err)
			}
			ledger := &contextLedger{facts: facts, key: attest.KeyNode{NodeID: facts.RunnerID, Public: key.Public(), SigningEligible: true}}
			verifier := attest.RunnerVerifier{Ledger: func() attest.Ledger { return ledger }}
			if err = verifier.VerifyEvidence(doc, ev); err != nil {
				t.Fatalf("agent evidence cannot verify: %v", err)
			}
			if ev.Attempt == nil || ev.Attempt.ID != l.AttemptID || ev.Attempt.N != tc.n || !ledger.consumed {
				t.Fatalf("attempt context lost: %+v, consumed=%v", ev.Attempt, ledger.consumed)
			}
		})
	}
}

type contextLedger struct {
	facts    attest.AttemptFacts
	key      attest.KeyNode
	consumed bool
}

func (l *contextLedger) AttemptFacts(context.Context, string) (attest.AttemptFacts, bool, error) {
	return l.facts, true, nil
}
func (l *contextLedger) NodesByKey(context.Context, string) ([]attest.KeyNode, error) {
	return []attest.KeyNode{l.key}, nil
}
func (*contextLedger) Revoked(context.Context, string) (bool, error) { return false, nil }
func (*contextLedger) Replayed(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (l *contextLedger) Consume(context.Context, attest.Use) error { l.consumed = true; return nil }
