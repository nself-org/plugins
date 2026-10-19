package main

// Purpose: TestSignHook (P7-TRUST-20): a signing-eligible node signs the evidence of a result frame under
// ci-node-<16 hex> over attest's payload; a node without the marker, or whose marker is unsafe, signs
// nothing; only `serve` installs the signer.
// Inputs:  a temp agent state dir (NSELF_CI_AGENT_HOME) with a real node key.
// Outputs: pass or fail; no network.
// Constraints: the process-wide session signer is replaced by a recorder, never left set.

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func hookEvidence(t *testing.T) []byte {
	t.Helper()
	ev := model.Evidence{Schema: "ci.evidence/v1", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RepoID: "unknown", RunID: "r1",
		Producer: model.Producer{Kind: "self-reported", Version: "1"}, Signature: model.Signature{Kind: "coordinator-recorded"},
		Attempt: &model.AttemptRef{ID: "att-1", N: 1}, Freshness: model.Freshness{State: "current"}}
	doc, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSignHook(t *testing.T) {
	state := t.TempDir()
	t.Setenv("NSELF_CI_AGENT_HOME", state)
	var installed []session.Signer
	old := setResultSigner
	setResultSigner = func(s session.Signer) { installed = append(installed, s) }
	t.Cleanup(func() { setResultSigner = old })
	ctx := context.Background()

	// no marker: nothing is installed and no key is created
	if err := signPreStart(ctx, "serve"); err != nil || len(installed) != 0 {
		t.Fatalf("without the marker: %v, %d signers", err, len(installed))
	}
	if _, err := os.Stat(filepath.Join(state, nodekey.FileName)); err == nil {
		t.Fatal("a node key was created for a node that does not sign")
	}

	// an unsafe marker (group-writable, or a symlink) does not count
	marker := filepath.Join(state, signingMarker)
	if err := os.WriteFile(marker, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(marker, 0o666); err != nil {
		t.Fatal(err)
	}
	if signingEligible(state) {
		t.Fatal("a world-writable marker made the node eligible")
	}
	_ = os.Remove(marker)
	if err := os.Symlink("/dev/null", marker); err != nil {
		t.Fatal(err)
	}
	if signingEligible(state) {
		t.Fatal("a symlink marker made the node eligible")
	}
	_ = os.Remove(marker)

	// a proper marker installs the signer, for serve only
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"exec", "join", "pull"} {
		if err := signPreStart(ctx, cmd); err != nil || len(installed) != 0 {
			t.Fatalf("%s installed a signer: %v", cmd, err)
		}
	}
	if err := signPreStart(ctx, "serve"); err != nil || len(installed) != 1 {
		t.Fatalf("serve: %v, %d signers", err, len(installed))
	}
	s := installed[0]
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ci-node-" + key.KeyID(); s.KeyID() != want {
		t.Fatalf("key id %q want %q", s.KeyID(), want)
	}

	// the frame's signature is the node key's over the signed-remote payload
	doc := hookEvidence(t)
	raw, err := s.Sign(doc)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	_ = json.Unmarshal(doc, &obj)
	obj["producer"].(map[string]any)["kind"] = "signed-remote"
	signedRemote, _ := json.Marshal(obj)
	payload, err := attest.SigningPayload(signedRemote)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(key.Public(), payload, raw) {
		t.Fatal("the result signature is not over the signed-remote payload")
	}
	// the session encodes the value as unpadded base64: the form attest.Sign puts in the document
	if _, err := base64.RawStdEncoding.Strict().DecodeString(base64.RawStdEncoding.EncodeToString(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sign([]byte(`not json`)); err == nil {
		t.Fatal("signed bytes that are not an evidence document")
	}
}
