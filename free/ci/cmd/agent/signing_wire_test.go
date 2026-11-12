package main

// Purpose: P7-WIRE-11 through the real entries. A system-mode agent (the shim seams name the system agent
// user and its state dir, the job user lookup is replaced because no nself-ci-job user exists on a test
// host) starts as `serve`: RunPreStart provisions the signing-eligible marker, the installed signer signs a
// real lease's evidence, and the coordinator's frame check (the verifier `ci evidence verify` uses for
// signed-remote evidence) accepts it. After run.RevokeNode (the call `nodes revoke` makes) the same
// evidence is refused. Evidence with no source trust is not stamped owner.
// Constraints: unix only; the seams are the existing job_shim_run.go ones plus lookupJobUser.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// startSystemServe makes the process look like the system agent user with separated job users and runs the
// registered PreStart hooks of `serve`; it returns the signer they installed (nil: none).
func startSystemServe(t *testing.T, state string, jobUID int) session.Signer {
	t.Helper()
	h, u, e, l, s := shimAgentHome, shimUserName, shimExecutable, lookupJobUser, setResultSigner
	var got session.Signer
	shimAgentHome, shimUserName = func() string { return bootstrap.StateDir }, func() string { return bootstrap.AgentUser }
	shimExecutable = func() string { return "/elsewhere" }
	lookupJobUser = func(string) (int, error) { return jobUID, nil }
	setResultSigner = func(sg session.Signer) { got = sg }
	// restored before any job runs: a lease under the shim seams would drain the host's hygiene state
	defer func() { shimAgentHome, shimUserName, shimExecutable, lookupJobUser, setResultSigner = h, u, e, l, s }()
	if err := RunPreStart(context.Background(), "serve"); err != nil {
		t.Fatalf("PreStart serve: %v", err)
	}
	return got
}

func TestWire11RemoteRunSignedEvidence(t *testing.T) {
	r := newSignedRig(t)
	state := os.Getenv("NSELF_CI_AGENT_HOME")
	_ = os.Remove(filepath.Join(state, signingMarker)) // the rig's operator marker: the agent must write its own
	if signingEligible(state) {
		t.Fatal("marker still present")
	}
	// no uid separation (job user shares the agent's uid): no marker, no signer
	if sg := startSystemServe(t, state, os.Geteuid()); sg != nil || signingEligible(state) {
		t.Fatalf("a node without uid separation became signing eligible (signer %v)", sg)
	}
	sg := startSystemServe(t, state, os.Geteuid()+1)
	if sg == nil || !signingEligible(state) {
		t.Fatalf("separated node: signer %v, marker %v", sg, signingEligible(state))
	}
	ctx := context.Background()
	ev, err := cirun.LeaseEvidence(ctx, r.st, r.attempt("1"))
	if err != nil || ev == nil {
		t.Fatalf("LeaseEvidence = %v, %v", ev, err)
	}
	frame, _, _ := r.run(protocol.Lease{AttemptID: "a1", Trust: "owner", Evidence: ev})
	raw, err := sg.Sign(frame)
	if err != nil {
		t.Fatal(err)
	}
	sig := base64.RawStdEncoding.EncodeToString(raw)
	if reason := r.verify(frame, sg.KeyID(), sig); reason != "" {
		t.Fatalf("evidence signed by the provisioned signer was refused: %s", reason)
	}

	// probe: after the node is revoked its earlier evidence is refused
	if _, err = cirun.RevokeNode(ctx, r.st, nil, "n-1", "cli:test"); err != nil {
		t.Fatalf("RevokeNode: %v", err)
	}
	if reason := r.verify(frame, sg.KeyID(), sig); reason != attest.RemoteKeyRevoked {
		t.Fatalf("revoked node's evidence: reason %q, want %s", reason, attest.RemoteKeyRevoked)
	}
}

func TestWire11RevokedNodeEvidenceRefused(t *testing.T) {
	r := newSignedRig(t)
	ctx := context.Background()
	ev, err := cirun.LeaseEvidence(ctx, r.st, r.attempt("1"))
	if err != nil || ev == nil {
		t.Fatalf("LeaseEvidence = %v, %v", ev, err)
	}
	frame, keyID, sig := r.run(protocol.Lease{AttemptID: "a1", Trust: "owner", Evidence: ev})
	if reason := r.verify(frame, keyID, sig); reason != "" {
		t.Fatalf("before the revoke: %s", reason)
	}
	if _, err = cirun.RevokeNode(ctx, r.st, nil, "n-1", "cli:test"); err != nil {
		t.Fatal(err)
	}
	if reason := r.verify(frame, keyID, sig); reason != attest.RemoteKeyRevoked {
		t.Fatalf("after the revoke: reason %q, want %s", reason, attest.RemoteKeyRevoked)
	}
}

func TestWire11EmptySourceTrustNotOwner(t *testing.T) {
	r := newSignedRig(t)
	ctx := context.Background()
	ev, err := cirun.LeaseEvidence(ctx, r.st, r.attempt("1"))
	if err != nil || ev == nil {
		t.Fatalf("LeaseEvidence = %v, %v", ev, err)
	}
	ev.SourceTrust = ""
	frame, _, _ := r.run(protocol.Lease{AttemptID: "a1", Evidence: ev})
	var doc model.Evidence
	if err = json.Unmarshal(frame, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SourceTrust == "owner" || doc.SourceTrust == "" {
		t.Fatalf("evidence with no source trust carries %q", doc.SourceTrust)
	}
}
