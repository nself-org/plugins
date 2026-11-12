package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
)

// wire14Fixture is a passing document "written by the release job on another machine" (a run id this
// machine never recorded) plus a release key whose public half the operator may trust.
type wire14Fixture struct {
	r      *evRepo
	doc    []byte
	signer signing.Signer
	keyID  string
	pub    ed25519.PublicKey
}

func newWire14(t *testing.T) *wire14Fixture {
	t.Helper()
	r := newEvRepo(t)
	first, _ := r.ciJSON("--no-status", "--json")
	m := map[string]any{}
	b, _ := json.Marshal(first)
	_ = json.Unmarshal(b, &m)
	m["run_id"] = "r-built-on-another-machine"
	doc, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := signing.NewEd25519Signer(signing.PurposeCIRelease, priv)
	return &wire14Fixture{r: r, doc: doc, signer: s, keyID: s.KeyID(), pub: pub}
}

func (f *wire14Fixture) trust(t *testing.T) {
	t.Helper()
	dir := filepath.Join(f.r.home, "trust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := f.keyID + " " + base64.StdEncoding.EncodeToString(f.pub) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ci-release.keys"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *wire14Fixture) file(t *testing.T, name string, body []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *wire14Fixture) verify(t *testing.T, path string) int {
	t.Helper()
	_, code := f.r.ciJSON("evidence", "verify", "--file", path, "--json")
	return code
}

func TestWire14CIReleaseEvidenceVerifies(t *testing.T) {
	f := newWire14(t)
	f.trust(t)
	env, err := evidence.SignLocal(f.doc, f.signer)
	if err != nil {
		t.Fatal(err)
	}
	rep, code := f.r.ciJSON("evidence", "verify", "--file", f.file(t, "signed.json", env), "--json")
	if code != 0 || rep["result"] != "pass" {
		t.Fatalf("locally signed release evidence: exit %d %v", code, rep)
	}
	if f.r.runCount() != 1 {
		t.Errorf("a file check ran the pipeline: %d runs", f.r.runCount())
	}
}

func TestWire14UnsignedForeignEvidenceRefused(t *testing.T) {
	f := newWire14(t)
	f.trust(t)
	if code := f.verify(t, f.file(t, "unsigned.json", f.doc)); code != 11 {
		t.Fatalf("the same document unsigned: exit %d, want 11", code)
	}
}

// Fail-closed probes: every way the signature can be wrong or untrusted refuses the document.
func TestWire14LocalSignatureFailsClosed(t *testing.T) {
	f := newWire14(t)
	sign := func(doc []byte, s signing.Signer) signing.Envelope {
		raw, err := evidence.SignLocal(doc, s)
		if err != nil {
			t.Fatal(err)
		}
		var e signing.Envelope
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	write := func(name string, e signing.Envelope) string {
		b, _ := json.Marshal(e)
		return f.file(t, name, b)
	}
	good := sign(f.doc, f.signer)

	if code := f.verify(t, write("no-trust.json", good)); code != 11 {
		t.Errorf("no trusted keys on this machine: exit %d, want 11", code)
	}
	f.trust(t)
	if code := f.verify(t, write("good.json", good)); code != 0 {
		t.Fatalf("control: exit %d, want 0", code)
	}

	// the inner document edited after signing
	edited := good
	var m map[string]any
	_ = json.Unmarshal(f.doc, &m)
	m["result"] = "fail"
	b, _ := json.Marshal(m)
	edited.Payload = base64.StdEncoding.EncodeToString(b)
	if code := f.verify(t, write("edited.json", edited)); code != 11 {
		t.Errorf("edited payload: exit %d, want 11", code)
	}
	// a key that is not trusted
	if code := f.verify(t, write("stranger.json", sign(f.doc, wire14Other(t, signing.PurposeCIRelease)))); code != 11 {
		t.Errorf("untrusted release key: exit %d, want 11", code)
	}
	// a key of another class, even when listed
	other := wire14Other(t, signing.PurposeCINode)
	if code := f.verify(t, write("wrong-class.json", sign(f.doc, other))); code != 11 {
		t.Errorf("ci-node key signing evidence locally: exit %d, want 11", code)
	}
	// another payload type
	wrong := good
	wrong.PayloadType = "text/plain"
	if code := f.verify(t, write("type.json", wrong)); code != 11 {
		t.Errorf("another payload type: exit %d, want 11", code)
	}
	// revoked
	if err := os.WriteFile(filepath.Join(f.r.home, "trust", "ci-release.revoked"), []byte(f.keyID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := f.verify(t, write("revoked.json", good)); code != 11 {
		t.Errorf("revoked key: exit %d, want 11", code)
	}
	if !strings.HasPrefix(f.keyID, "ci-release-") {
		t.Errorf("key id %q is not of the release class", f.keyID)
	}
}

func wire14Other(t *testing.T, p signing.Purpose) signing.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signing.NewEd25519Signer(p, priv)
}
