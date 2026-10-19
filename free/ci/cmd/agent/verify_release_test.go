package main

// Purpose: tests of `nself-ci-agent verify-release` (P7-NODE-15): a built release directory verifies against its one public key, and a
//          flipped byte, a missing manifest, a missing binary, an empty directory or another key all fail.
// Inputs:  a directory the test signs with a generated Ed25519 key.
// Constraints: no real key; no network.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
)

func writePub(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "k.pub")
	if err = os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// releaseDir signs a two-target release directory and returns it with the public key file.
func releaseDir(t *testing.T) (dir, pubFile string) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	s := signing.NewEd25519Signer(signing.PurposeAgent, priv)
	dir = t.TempDir()
	for _, target := range []string{"linux-amd64", "darwin-arm64"} {
		body := []byte("agent " + target)
		name := "nself-ci-agent-" + target
		_ = os.WriteFile(filepath.Join(dir, name), body, 0o755)
		sum := sha256.Sum256(body)
		m := verify.Manifest{Schema: verify.Schema, Version: "1.5.0", Target: target, SHA256: hex.EncodeToString(sum[:]), ProtocolMin: 1, ProtocolMax: 1, KeyID: s.KeyID(), Alg: "ed25519"}
		in, _ := verify.SigningInput(m)
		raw, _ := s.Sign(in)
		m.Sig = signing.EncodeSig(raw)
		b, _ := json.Marshal(m)
		_ = os.WriteFile(filepath.Join(dir, name+".manifest.json"), append(b, '\n'), 0o644)
	}
	_ = os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.5.0\n"), 0o644)
	return dir, writePub(t, pub)
}

func runVR(args ...string) (code int, out, errs string) {
	var o, e bytes.Buffer
	code = runVerifyRelease(context.Background(), args, IO{Stdin: strings.NewReader(""), Stdout: &o, Stderr: &e})
	return code, o.String(), e.String()
}

func TestVerifyReleaseAcceptsASignedDirectory(t *testing.T) {
	dir, pub := releaseDir(t)
	code, out, errs := runVR("--pub", pub, dir)
	if code != 0 || strings.Count(out, "ok nself-ci-agent-") != 2 {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
}

func TestVerifyReleaseFailures(t *testing.T) {
	tamper := func(dir string) {
		b, _ := os.ReadFile(filepath.Join(dir, "nself-ci-agent-linux-amd64"))
		b[3] ^= 1
		_ = os.WriteFile(filepath.Join(dir, "nself-ci-agent-linux-amd64"), b, 0o755)
	}
	cases := []struct {
		name   string
		mutate func(dir string)
		reason string
	}{
		{"flipped byte", tamper, verify.ReasonChecksumMismatch},
		{"missing manifest", func(d string) { _ = os.Remove(filepath.Join(d, "nself-ci-agent-linux-amd64.manifest.json")) }, verify.ReasonMissingSignature},
		{"manifest without binary", func(d string) { _ = os.Remove(filepath.Join(d, "nself-ci-agent-darwin-arm64")) }, "manifest without a binary"},
		{"wrong version in VERSION", func(d string) { _ = os.WriteFile(filepath.Join(d, "VERSION"), []byte("1.4.0\n"), 0o644) }, verify.ReasonVersionMismatch},
	}
	for _, c := range cases {
		dir, pub := releaseDir(t)
		c.mutate(dir)
		code, _, errs := runVR("--pub", pub, dir)
		if code != 1 || !strings.Contains(errs, c.reason) || !strings.Contains(errs, "E665") {
			t.Errorf("%s: code %d err %q", c.name, code, errs)
		}
	}
}

func TestVerifyReleaseRefusesAnotherKeyAndAnEmptyDirectory(t *testing.T) {
	dir, _ := releaseDir(t)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if code, _, errs := runVR("--pub", writePub(t, other), dir); code != 1 || !strings.Contains(errs, verify.ReasonUnknownKey) {
		t.Errorf("another key: code %d err %q", code, errs)
	}
	_, pub := releaseDir(t)
	if code, _, errs := runVR("--pub", pub, t.TempDir()); code != 1 || !strings.Contains(errs, "no nself-ci-agent") {
		t.Errorf("empty dir: code %d err %q", code, errs)
	}
}

func TestVerifyReleaseUsage(t *testing.T) {
	dir, pub := releaseDir(t)
	for _, args := range [][]string{{}, {dir}, {"--pub", pub}, {"--pub", filepath.Join(dir, "VERSION"), dir}} {
		if code, _, _ := runVR(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
	if code, _, _ := runVR("--pub", pub); code != 2 {
		t.Error("a missing DIR is a usage error (2)")
	}
}
