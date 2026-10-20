package attest_test

// Purpose: tests of the signed audit checkpoint (P7-TRUST-20, X6): sign and verify with a pinned
// fingerprint, refusal of every tamper, and the key file (0600, created once, never trusted from the data
// dir for verification).
// Inputs:  a ci-audit key in a temp dir.
// Outputs: pass or fail.
// Constraints: no network; the chain comparison with the live database is the command's, tested in cmd.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

func newCheckpoint(t *testing.T) (attest.Checkpoint, string, string) {
	t.Helper()
	dir := t.TempDir()
	signer, pub, err := attest.LoadOrCreateCheckpointKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := attest.NewCheckpoint(signer, pub, bytes.Repeat([]byte{0xab}, 32), 7, 7, 1_800_000_000)
	if err != nil {
		t.Fatal(err)
	}
	return cp, attest.Fingerprint(pub), dir
}

func TestCheckpointSignAndVerifyWithPinnedFingerprint(t *testing.T) {
	cp, fp, _ := newCheckpoint(t)
	if err := attest.VerifyCheckpoint(cp, fp); err != nil {
		t.Fatal(err)
	}
	if err := attest.VerifyCheckpoint(cp, "sha256:"+fp); err != nil {
		t.Fatalf("sha256: prefix: %v", err)
	}
	if len(fp) != 64 {
		t.Fatalf("fingerprint %q", fp)
	}
}

func TestCheckpointRefusesEveryTamper(t *testing.T) {
	cp, fp, _ := newCheckpoint(t)
	other, otherFP, _ := newCheckpoint(t)
	edit := map[string]func(c *attest.Checkpoint){
		"head":      func(c *attest.Checkpoint) { c.Head = "00" + c.Head[2:] },
		"seq":       func(c *attest.Checkpoint) { c.Seq++ },
		"rows":      func(c *attest.Checkpoint) { c.Rows++ },
		"ts":        func(c *attest.Checkpoint) { c.TS++ },
		"signature": func(c *attest.Checkpoint) { c.Signature = other.Signature },
		"key swap":  func(c *attest.Checkpoint) { c.KeyID, c.Public = other.KeyID, other.Public },
		"version":   func(c *attest.Checkpoint) { c.Version = 2 },
		"no key":    func(c *attest.Checkpoint) { c.Public = "" },
	}
	for name, f := range edit {
		c := cp
		f(&c)
		if err := attest.VerifyCheckpoint(c, fp); err == nil {
			t.Errorf("%s: a tampered checkpoint verified against the pinned fingerprint", name)
		}
	}
	// the attacker's own key and signature verify only against the attacker's fingerprint
	if err := attest.VerifyCheckpoint(other, fp); err == nil {
		t.Error("another coordinator's checkpoint verified against the pinned fingerprint")
	}
	if err := attest.VerifyCheckpoint(other, otherFP); err != nil {
		t.Errorf("control: the other checkpoint against its own fingerprint: %v", err)
	}
	for _, bad := range []string{"", "abc", fp[:63], fp + "0"} {
		if err := attest.VerifyCheckpoint(cp, bad); err == nil {
			t.Errorf("pinned %q accepted", bad)
		}
	}
}

func TestCheckpointKeyFileIsPrivateAndCreatedOnce(t *testing.T) {
	_, fp, dir := newCheckpoint(t)
	path := filepath.Join(dir, attest.CheckpointKeyFile)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", err, info)
	}
	_, pub, err := attest.LoadOrCreateCheckpointKey(dir)
	if err != nil || attest.Fingerprint(pub) != fp {
		t.Fatalf("second load made another key: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := attest.LoadOrCreateCheckpointKey(dir); err == nil {
		t.Fatal("a group-readable key file was used")
	}
	if _, err := attest.NewCheckpoint(nil, pub, []byte{1}, 1, 1, 1); err == nil {
		t.Fatal("signed with no signer")
	}
	signer, pub2, _ := attest.LoadOrCreateCheckpointKey(t.TempDir())
	if _, err := attest.NewCheckpoint(signer, pub, []byte{1}, 1, 1, 1); err == nil {
		t.Fatal("a signer for another key was accepted")
	}
	if _, err := attest.NewCheckpoint(signer, pub2, nil, 1, 1, 1); err == nil {
		t.Fatal("signed an empty chain")
	}
}
