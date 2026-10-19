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
	"github.com/nself-org/plugins/free/ci/internal/provenance"
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
		writeSideFiles(t, dir, s, m)
	}
	_ = os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.5.0\n"), 0o644)
	return dir, writePub(t, pub)
}

// writeSideFiles writes the two SBOMs and the signed provenance of manifest m next to its binary (P7-CACHE-10).
func writeSideFiles(t *testing.T, dir string, s signing.Signer, m verify.Manifest) {
	t.Helper()
	name := verify.BinaryName(m.Target)
	cdx, spdx := []byte(`{"bomFormat":"CycloneDX"}`), []byte(`{"spdxVersion":"SPDX-2.3"}`)
	digest := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	st, err := provenance.BuildAgent(provenance.AgentFacts{Repo: "nself-org/plugins", Ref: "refs/tags/v1.5.0", Revision: strings.Repeat("a", 40), RunID: "1",
		Subjects: []provenance.FileDigest{{Name: name, SHA256: m.SHA256}},
		SBOMs:    []provenance.FileDigest{{Name: name + verify.SuffixCycloneDX, SHA256: digest(cdx)}, {Name: name + verify.SuffixSPDX, SHA256: digest(spdx)}}})
	if err != nil {
		t.Fatal(err)
	}
	env, err := signing.SignEnvelope(s, provenance.PayloadType, st)
	if err != nil {
		t.Fatal(err)
	}
	prov, _ := json.Marshal(env)
	for suffix, body := range map[string][]byte{verify.SuffixCycloneDX: cdx, verify.SuffixSPDX: spdx, verify.SuffixProvenance: prov} {
		if err = os.WriteFile(filepath.Join(dir, name+suffix), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
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
		{"missing provenance", func(d string) { _ = os.Remove(filepath.Join(d, "nself-ci-agent-linux-amd64"+verify.SuffixProvenance)) }, verify.ReasonProvenance},
		{"missing SBOM", func(d string) { _ = os.Remove(filepath.Join(d, "nself-ci-agent-darwin-arm64"+verify.SuffixSPDX)) }, verify.ReasonProvenance},
		{"edited SBOM", func(d string) {
			_ = os.WriteFile(filepath.Join(d, "nself-ci-agent-linux-amd64"+verify.SuffixCycloneDX), []byte(`{"bomFormat":"CycloneDX","x":1}`), 0o644)
		}, verify.ReasonProvenance},
		{"symlinked manifest", func(d string) {
			m := filepath.Join(d, "nself-ci-agent-linux-amd64.manifest.json")
			_ = os.Rename(m, filepath.Join(d, "real.json"))
			_ = os.Symlink(filepath.Join(d, "real.json"), m)
		}, verify.ReasonUnreadable},
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

func TestVerifyReleaseAcceptsAKeyFileWithTwoKeys(t *testing.T) {
	dir, pub := releaseDir(t)
	extra, _, _ := ed25519.GenerateKey(rand.Reader)
	first, _ := os.ReadFile(pub)
	second, _ := os.ReadFile(writePub(t, extra))
	if err := os.WriteFile(pub, append(first, second...), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runVR("--pub", pub, dir); code != 0 {
		t.Fatalf("code %d err %q", code, errs)
	}
}

func TestVerifyReleaseRefusesASideFileWithoutItsBinary(t *testing.T) {
	dir, pub := releaseDir(t)
	if err := os.WriteFile(filepath.Join(dir, "nself-ci-agent-linux-arm64"+verify.SuffixCycloneDX), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errs := runVR("--pub", pub, dir)
	if code != 1 || !strings.Contains(errs, verify.ReasonProvenance) || !strings.Contains(errs, "no binary") {
		t.Fatalf("code %d err %q", code, errs)
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

func TestVerifyReleaseRequireAllTargets(t *testing.T) {
	dir, pub := releaseDir(t) // linux-amd64 and darwin-arm64 only
	if code, _, _ := runVR("--pub", pub, dir); code != 0 {
		t.Fatal("a partial directory verifies without the flag")
	}
	code, _, errs := runVR("--pub", pub, "--require-all-targets", dir)
	if code != 1 || !strings.Contains(errs, "linux-arm64") || !strings.Contains(errs, "E665") {
		t.Fatalf("a missing target must fail with --require-all-targets: code %d err %q", code, errs)
	}
}

func TestVerifyReleaseRefusesARevokedKey(t *testing.T) {
	dir, pub := releaseDir(t)
	m, _ := os.ReadFile(filepath.Join(dir, "nself-ci-agent-linux-amd64.manifest.json"))
	var man verify.Manifest
	_ = json.Unmarshal(m, &man)
	rev := filepath.Join(t.TempDir(), "agent-revoked.txt")
	_ = os.WriteFile(rev, []byte("# leaked\n"+man.KeyID+"\n"), 0o644)
	code, _, errs := runVR("--pub", pub, "--revoked", rev, dir)
	if code != 1 || !strings.Contains(errs, verify.ReasonRevoked) {
		t.Fatalf("revoked key: code %d err %q", code, errs)
	}
	bad := filepath.Join(t.TempDir(), "bad.txt")
	_ = os.WriteFile(bad, []byte("not-a-derived-id\n"), 0o644)
	if code, _, _ = runVR("--pub", pub, "--revoked", bad, dir); code != 1 {
		t.Fatal("a revoked file with a malformed id must be refused")
	}
}
