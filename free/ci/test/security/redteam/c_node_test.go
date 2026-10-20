//go:build security

package redteam

// Purpose: red team group C, node and agent (TM-19 tampered agent image, TM-20 compromised runner
// node, TM-22 job reads agent keys). The attacker edits, downgrades or re-targets a signed agent
// manifest and swaps the binary; plants a symlink or a group-readable file where the node key
// lives; runs with a permissive umask; and looks at what environment a job inherits from the agent.
// Inputs:  throwaway agent signing keys, temp dirs, the production verifier, key loader and env policy.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: temp dirs only; no network; the umask is restored by the test.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// TestC19AgentManifestAttacks (TM-19): a manifest that does not match what was signed, pinned or
// shipped is refused with its own reason; the honest manifest passes the manifest stage and stops only
// at the missing provenance (the SBOM and DSSE files are not part of this attack).
func TestC19AgentManifestAttacks(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer := signing.NewEd25519Signer(signing.PurposeAgent, priv)
	key := signing.Key{ID: signer.KeyID(), Purpose: signing.PurposeAgent, Public: pub}
	dir := t.TempDir()
	bin := filepath.Join(dir, "nself-ci-agent-linux-amd64")
	content := []byte("#!agent binary bytes")
	if err := os.WriteFile(bin, content, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	mk := func(edit func(*verify.Manifest)) []byte {
		m := verify.Manifest{Schema: verify.Schema, Version: "1.5.0", Target: "linux-amd64", SHA256: hex.EncodeToString(sum[:]), ProtocolMin: 1, ProtocolMax: 1, KeyID: signer.KeyID(), Alg: "ed25519"}
		in, err := verify.SigningInput(m)
		if err != nil {
			t.Fatal(err)
		}
		sig, _ := signer.Sign(in)
		m.Sig = signing.EncodeSig(sig)
		if edit != nil {
			edit(&m)
		}
		b, _ := json.Marshal(m)
		return b
	}
	want := verify.Expect{Version: "1.5.0", Target: "linux-amd64"}
	v, err := verify.New([]signing.Key{key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Artifact(bin, mk(nil), want, verify.Files{}); verify.ReasonOf(err) != verify.ReasonProvenance {
		t.Fatalf("control: the honest manifest should reach the provenance stage, got %v", err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	otherID := signing.KeyID(signing.PurposeAgent, other)
	cases := map[string]struct {
		manifest []byte
		want     verify.Expect
		v        func() *verify.Verifier
		reason   string
	}{
		"edited sha256":             {mk(func(m *verify.Manifest) { m.SHA256 = strings.Repeat("0", 64) }), want, nil, verify.ReasonBadSignature},
		"edited protocol range":     {mk(func(m *verify.Manifest) { m.ProtocolMax = 9 }), want, nil, verify.ReasonBadSignature},
		"retargeted manifest":       {mk(func(m *verify.Manifest) { m.Target = "darwin-arm64" }), want, nil, verify.ReasonBadSignature},
		"downgrade, signed older":   {mk(func(m *verify.Manifest) { m.Version = "1.4.0" }), want, nil, verify.ReasonBadSignature},
		"pinned newer than signed":  {mk(nil), verify.Expect{Version: "1.6.0", Target: "linux-amd64"}, nil, verify.ReasonVersionMismatch},
		"node is another target":    {mk(nil), verify.Expect{Version: "1.5.0", Target: "darwin-arm64"}, nil, verify.ReasonTargetMismatch},
		"unknown signing key":       {mk(func(m *verify.Manifest) { m.KeyID = otherID }), want, nil, verify.ReasonUnknownKey},
		"key id of another purpose": {mk(func(m *verify.Manifest) { m.KeyID = strings.Replace(m.KeyID, "agent-", "plugins-", 1) }), want, nil, ""},
		"revoked key": {mk(nil), want, func() *verify.Verifier {
			r, _ := verify.New([]signing.Key{key}, []string{signer.KeyID()})
			return r
		}, verify.ReasonRevoked},
		"signature padded":         {mk(func(m *verify.Manifest) { m.Sig += "=" }), want, nil, ""},
		"signature with newline":   {mk(func(m *verify.Manifest) { m.Sig += "\n" }), want, nil, ""},
		"missing signature":        {mk(func(m *verify.Manifest) { m.Sig = "" }), want, nil, verify.ReasonMissingSignature},
		"version with newline":     {mk(func(m *verify.Manifest) { m.Version = "1.5.0\nlinux-amd64" }), want, nil, verify.ReasonMalformedManifest},
		"unknown field":            {[]byte(strings.Replace(string(mk(nil)), `"alg"`, `"extra":1,"alg"`, 1)), want, nil, verify.ReasonMalformedManifest},
		"duplicate field, case":    {[]byte(strings.Replace(string(mk(nil)), `"version"`, `"Version":"9.9.9","version"`, 1)), want, nil, verify.ReasonMalformedManifest},
		"duplicate field, escaped": {[]byte(strings.Replace(string(mk(nil)), `"version"`, `"v`+"\\"+`u0065rsion":"9.9.9","version"`, 1)), want, nil, verify.ReasonMalformedManifest},
		"trailing object":          {append(mk(nil), []byte(` {"schema":"x"}`)...), want, nil, verify.ReasonMalformedManifest},
		"empty manifest":           {nil, want, nil, verify.ReasonMalformedManifest},
		"no key compiled in":       {mk(nil), want, func() *verify.Verifier { r, _ := verify.New(nil, nil); return r }, verify.ReasonNoTrustedKey},
	}
	for name, c := range cases {
		vv := v
		if c.v != nil {
			vv = c.v()
		}
		_, err := vv.Artifact(bin, c.manifest, c.want, verify.Files{})
		got := verify.ReasonOf(err)
		if err == nil || got == verify.ReasonProvenance || c.reason != "" && got != c.reason {
			t.Errorf("%s: reason %q (err %v), want %q", name, got, err, c.reason)
		}
	}
	// the file, not the manifest, is swapped
	if err := os.WriteFile(bin, append(content, 'x'), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Artifact(bin, mk(nil), want, verify.Files{}); verify.ReasonOf(err) != verify.ReasonChecksumMismatch {
		t.Errorf("swapped binary: %v", err)
	}
	if err := os.WriteFile(bin, content, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Artifact(link, mk(nil), want, verify.Files{}); err == nil || verify.ReasonOf(err) == verify.ReasonProvenance {
		t.Errorf("a symlink was accepted as the artifact (verify then place by path is a swap window): %v", err)
	}
}

// TestC22NodeKeyFile (TM-22): the node key is created 0600 in a 0700 directory whatever the umask, a
// symlink, a group-readable file or a dangling link in its place is refused, and a loose directory is tightened.
func TestC22NodeKeyFile(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	k, err := nodekey.LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, nodekey.FileName)
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("key file mode %v (%v)", st, err)
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("state dir mode %v", st.Mode().Perm())
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".tmp-*")); len(matches) != 0 {
		t.Errorf("temporary key files left behind: %v", matches)
	}
	if k2, err := nodekey.LoadOrCreate(dir); err != nil || k2.KeyID() != k.KeyID() {
		t.Errorf("reload: %v", err)
	}
	for name, setup := range map[string]func(){
		"group readable": func() { _ = os.Chmod(path, 0o640) },
		"world readable": func() { _ = os.Chmod(path, 0o644) },
		"symlink to a 0600 copy": func() {
			_ = os.Chmod(path, 0o600)
			raw, _ := os.ReadFile(path)
			_ = os.Remove(path)
			real := filepath.Join(t.TempDir(), "real.key")
			_ = os.WriteFile(real, raw, 0o600)
			_ = os.Symlink(real, path)
		},
		"dangling symlink": func() { _ = os.Remove(path); _ = os.Symlink(filepath.Join(dir, "nowhere"), path) },
		"directory":        func() { _ = os.Remove(path); _ = os.Mkdir(path, 0o700) },
	} {
		setup()
		if _, err := nodekey.LoadOrCreate(dir); err == nil {
			t.Errorf("%s: the key was loaded", name)
		}
		_ = os.RemoveAll(path)
		if _, err := nodekey.LoadOrCreate(dir); err != nil {
			t.Fatalf("recreate after %s: %v", name, err)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := nodekey.EnsureDir(link); err == nil {
		t.Error("a symlinked state directory was accepted")
	}
}

// TestC22JobEnvironment (TM-22, TM-21): the environment a job inherits from the agent or the
// coordinator holds none of the agent key, token, state or credential variables, in any case.
func TestC22JobEnvironment(t *testing.T) {
	secret := map[string]string{
		"NSELF_AGENT_SIGNING_KEY": fakeSecret, "NSELF_AGENT_SIGNING_KEY_ID": fakeSecret, "NSELF_CI_HOME": "/home/agent/.nself/ci", "NSELF_CI_AGENT_HOME": "/home/agent",
		"NSELF_MCP_TOKEN": fakeSecret, "NSELF_CI_JOB_TOKEN": fakeSecret, "SSH_AUTH_SOCK": "/tmp/agent.sock", "AWS_SECRET_ACCESS_KEY": fakeSecret, "GITHUB_TOKEN": fakeSecret,
		"GH_TOKEN": fakeSecret, "npm_config__authToken": fakeSecret, "KUBECONFIG": "/home/agent/.kube/config", "DOCKER_CONFIG": "/home/agent/.docker", "nself_ci_job_token": fakeSecret,
		"GOFLAGS_SECRET": fakeSecret, "HOMEBREW_GITHUB_API_TOKEN": fakeSecret, "PATH_SECRET": fakeSecret,
	}
	for k, v := range secret {
		t.Setenv(k, v)
	}
	env, err := exec.AllowlistEnv{}.Environment(context.Background(), model.Job{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range env {
		name, val, _ := strings.Cut(kv, "=")
		if _, bad := secret[name]; bad || strings.Contains(val, fakeSecret) {
			t.Errorf("the job environment carries %s", name)
		}
	}
}

// TestC20AgeRecipientPinIsWired (TM-20, C04): the hello handler compares the node's age recipient with
// the pinned one only when Config.PinnedAge is set; without it the pin is the hello's own value. The
// production listener must set it (static check of the listener source: cmd is package main).
func TestC20AgeRecipientPinIsWired(t *testing.T) {
	if !openFinding(t, "C04") {
		return
	}
	src, err := os.ReadFile("../../../cmd/serve_agent_listener.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "PinnedAge") {
		t.Error("cmd/serve_agent_listener.go builds agentapi.Config without PinnedAge: a node may present any age recipient at hello and the pin compares it with itself")
	}
}
