package verify

// Purpose: tests of agent artifact verification (P7-NODE-15): every failure class gets its own E665 reason key and is refused
//          before a file is placed; the Go signing input equals the bytes scripts/release/agent/manifest.sh signs; the
//          compiled-in key set equals scripts/release/keys/agent-*.pub; one package verifies agent artifacts.
// Inputs:  keys generated in the tests; the repository scripts (bash + OpenSSL 3, skipped when missing).
// Constraints: every key is a throwaway generated here; no network; no secret.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

type signer struct {
	signing.Signer
	pub ed25519.PublicKey
	p   signing.Purpose
}

func newSigner(t *testing.T, p signing.Purpose) signer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signer{signing.NewEd25519Signer(p, priv), pub, p}
}

func (s signer) key() signing.Key { return signing.Key{ID: s.KeyID(), Purpose: s.p, Public: s.pub} }

// artifact writes a fake binary and returns its path with a fresh signed manifest.
type artifact struct {
	path string
	m    Manifest
}

func newArtifact(t *testing.T, s signer) artifact {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nself-ci-agent")
	body := []byte("agent bytes 1.5.0 linux amd64")
	if err := os.WriteFile(path, body, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	m := Manifest{Schema: Schema, Version: "1.5.0", Target: "linux-amd64", SHA256: hex.EncodeToString(sum[:]), ProtocolMin: 1, ProtocolMax: 1, KeyID: s.KeyID(), Alg: "ed25519"}
	return artifact{path, resign(t, s, m)}
}

func resign(t *testing.T, s signer, m Manifest) Manifest {
	t.Helper()
	in, err := SigningInput(m)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.Sign(in)
	if err != nil {
		t.Fatal(err)
	}
	m.Sig = signing.EncodeSig(raw)
	return m
}

func (a artifact) json(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(a.m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var pinned = Expect{Version: "1.5.0", Target: "linux-amd64"}

func mustVerifier(t *testing.T, revoked []string, keys ...signing.Key) *Verifier {
	t.Helper()
	v, err := New(keys, revoked)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func wantReason(t *testing.T, err error, reason string) {
	t.Helper()
	var ve *Error
	if !errors.As(err, &ve) || ve.Code() != "E665" || ve.Reason != reason {
		t.Fatalf("want E665 %s, got %v", reason, err)
	}
	if !strings.HasPrefix(err.Error(), "E665:") {
		t.Fatalf("error text must start with the code: %q", err)
	}
}

func TestArtifactAcceptsAValidRelease(t *testing.T) {
	s := newSigner(t, signing.PurposeAgent)
	a := newArtifact(t, s)
	m, err := mustVerifier(t, nil, s.key()).checkManifest(a.path, a.json(t), pinned)
	if err != nil || m.KeyID != s.KeyID() {
		t.Fatalf("valid artifact refused: %v", err)
	}
}

func TestArtifactFailureReasonsAreDistinct(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	plugin := newSigner(t, signing.PurposePlugins)
	other := newSigner(t, signing.PurposeAgent)
	good := newArtifact(t, agent)
	cases := []struct {
		name   string
		reason string
		run    func() error
	}{
		{"tampered binary", ReasonChecksumMismatch, func() error {
			a := newArtifact(t, agent)
			_ = os.WriteFile(a.path, []byte("tampered bytes!"), 0o700)
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), pinned)
			return err
		}},
		{"manifest signed with the plugin-purpose key", ReasonWrongPurpose, func() error {
			a := newArtifact(t, plugin)
			_, err := mustVerifier(t, nil, agent.key(), plugin.key()).checkManifest(a.path, a.json(t), pinned)
			return err
		}},
		{"signed by a key that is not trusted", ReasonUnknownKey, func() error {
			a := newArtifact(t, other)
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), pinned)
			return err
		}},
		{"missing signature", ReasonMissingSignature, func() error {
			a := good
			a.m.Sig = ""
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), pinned)
			return err
		}},
		{"signature over other fields", ReasonBadSignature, func() error {
			a := good
			a.m.Version = "1.5.1"
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), Expect{Version: "1.5.1", Target: "linux-amd64"})
			return err
		}},
		{"version differs from the pin", ReasonVersionMismatch, func() error {
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(good.path, good.json(t), Expect{Version: "1.4.0", Target: "linux-amd64"})
			return err
		}},
		{"target differs from the node", ReasonTargetMismatch, func() error {
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(good.path, good.json(t), Expect{Version: "1.5.0", Target: "darwin-arm64"})
			return err
		}},
		{"revoked key", ReasonRevoked, func() error {
			_, err := mustVerifier(t, []string{agent.KeyID()}, agent.key()).checkManifest(good.path, good.json(t), pinned)
			return err
		}},
		{"non-canonical signature", ReasonMalformedSig, func() error {
			a := good
			a.m.Sig = a.m.Sig[:len(a.m.Sig)-1]
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), pinned)
			return err
		}},
		{"key id that is not a key id", ReasonMalformedManifest, func() error {
			a := good
			a.m.KeyID = "bad id!"
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), pinned)
			return err
		}},
		{"unknown manifest field", ReasonMalformedManifest, func() error {
			b := strings.Replace(string(good.json(t)), `"alg"`, `"extra":1,"alg"`, 1)
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(good.path, []byte(b), pinned)
			return err
		}},
		{"a repeated field", ReasonMalformedManifest, func() error {
			b := strings.Replace(string(good.json(t)), `"alg":"ed25519"`, `"alg":"ed25519","version":"9.9.9"`, 1)
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(good.path, []byte(b), pinned)
			return err
		}},
		{"a symlinked binary", ReasonUnreadable, func() error {
			link := filepath.Join(t.TempDir(), "link")
			if err := os.Symlink(good.path, link); err != nil {
				t.Skip("symlinks unavailable")
			}
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(link, good.json(t), pinned)
			return err
		}},
		{"a field repeated in another case", ReasonMalformedManifest, func() error {
			b := strings.Replace(string(good.json(t)), `"alg":"ed25519"`, `"alg":"ed25519","VERSION":"9.9.9"`, 1)
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(good.path, []byte(b), pinned)
			return err
		}},
		{"two manifests in one file", ReasonMalformedManifest, func() error {
			b := append(good.json(t), good.json(t)...)
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(good.path, b, pinned)
			return err
		}},
		{"no key compiled in", ReasonNoTrustedKey, func() error {
			_, err := mustVerifier(t, nil).checkManifest(good.path, good.json(t), pinned)
			return err
		}},
		{"missing file", ReasonUnreadable, func() error {
			_, err := mustVerifier(t, nil, agent.key()).checkManifest(filepath.Join(t.TempDir(), "absent"), good.json(t), pinned)
			return err
		}},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		wantReason(t, c.run(), c.reason)
		seen[c.reason] = true
	}
	if len(seen) < 12 {
		t.Errorf("only %d distinct reason keys are covered", len(seen))
	}
}

func TestNoTrustedKeyNamesAgentBinaryFlag(t *testing.T) {
	err := mustVerifier(t, nil).Ready()
	wantReason(t, err, ReasonNoTrustedKey)
	if !strings.Contains(err.Error(), "--agent-binary") {
		t.Fatalf("the refusal must name --agent-binary: %v", err)
	}
}

func TestSignatureErrorsWrapSigningSentinels(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	other := newSigner(t, signing.PurposeAgent)
	a := newArtifact(t, other)
	_, err := mustVerifier(t, nil, agent.key()).checkManifest(a.path, a.json(t), pinned)
	if !errors.Is(err, signing.ErrUnknownKey) {
		t.Fatalf("the signing sentinel must stay reachable: %v", err)
	}
}

func TestSigningInputGolden(t *testing.T) {
	m := Manifest{Schema: Schema, Version: "1.5.0", Target: "linux-amd64", ProtocolMin: 1, ProtocolMax: 2, Alg: "ed25519",
		SHA256: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}
	got, err := SigningInput(m)
	if err != nil {
		t.Fatal(err)
	}
	want := "nself-agent-v1\nlinux-amd64\n1.5.0\n00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff\n1-2\n"
	if string(got) != want {
		t.Fatalf("signing input\n got %q\nwant %q", got, want)
	}
	m.Version = "1.5.0\nlinux-amd64"
	if _, err = SigningInput(m); err == nil {
		t.Fatal("a field with a newline must not produce a signing input")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "scripts/release/agent/manifest.sh")); err != nil {
		t.Fatalf("not in the plugins repo: %v", err)
	}
	return root
}

func needTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"bash", "openssl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	out, _ := exec.Command("openssl", "version").Output()
	if !strings.HasPrefix(string(out), "OpenSSL 3") {
		t.Skip("OpenSSL 3 required (set PATH so openssl is OpenSSL 3)")
	}
}

// TestManifestScriptAndGoAgree signs with scripts/release/agent/manifest.sh in rehearsal mode, then verifies in Go: the shell's
// signing input is byte for byte the Go one, and the manifest text parses as agent-release/v1.
func TestManifestScriptAndGoAgree(t *testing.T) {
	needTools(t)
	dir := t.TempDir()
	body := []byte("fake linux agent")
	if err := os.WriteFile(filepath.Join(dir, "nself-ci-agent-linux-amd64"), body, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.5.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts/release/agent/manifest.sh"), "--in", dir, "--rehearsal", "--protocol-min", "1", "--protocol-max", "3")
	cmd.Env = append(os.Environ(), "TMPDIR="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("manifest.sh: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "nself-ci-agent-linux-amd64.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(raw)
	if err != nil || m.ProtocolMax != 3 || m.Version != "1.5.0" {
		t.Fatalf("manifest.sh output does not parse as agent-release/v1: %v %+v", err, m)
	}
	pemBytes, _ := os.ReadFile(filepath.Join(dir, "rehearsal-test.pub"))
	block, _ := pem.Decode(pemBytes)
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pub := k.(ed25519.PublicKey)
	if m.KeyID != signing.KeyID(signing.PurposeAgent, pub) {
		t.Fatalf("manifest.sh key id %s is not the derived id", m.KeyID)
	}
	v := mustVerifier(t, nil, signing.Key{ID: m.KeyID, Purpose: signing.PurposeAgent, Public: pub})
	if _, err = v.checkManifest(filepath.Join(dir, "nself-ci-agent-linux-amd64"), raw, Expect{Version: "1.5.0", Target: "linux-amd64"}); err != nil {
		t.Fatalf("Go refused what manifest.sh signed: %v", err)
	}
}

// TestKeysGenHasNoDrift regenerates the key set from scripts/release/keys/agent-*.pub and compares it with the committed file.
func TestKeysGenHasNoDrift(t *testing.T) {
	needTools(t)
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "keys_gen.go")
	cmd := exec.Command("bash", filepath.Join(root, "scripts/release/agent/genkeys.sh"), out)
	cmd.Env = append(os.Environ(), "TMPDIR="+t.TempDir())
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("genkeys.sh: %v\n%s", err, b)
	}
	got, _ := os.ReadFile(out)
	want, err := os.ReadFile("keys_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("keys_gen.go is stale: run `go generate ./internal/agent/verify/...`")
	}
}

// TestGeneratedKeySetLoads generates a key, runs genkeys.sh over a directory holding it, and checks the id derivation and that
// a mis-named file is refused.
func TestGeneratedKeySetLoads(t *testing.T) {
	needTools(t)
	root := repoRoot(t)
	keys := t.TempDir()
	tmp := t.TempDir()
	priv := filepath.Join(tmp, "k.pem")
	if out, err := exec.Command("openssl", "genpkey", "-algorithm", "ed25519", "-out", priv).CombinedOutput(); err != nil {
		t.Fatalf("genpkey: %v\n%s", err, out)
	}
	pubPEM, err := exec.Command("openssl", "pkey", "-in", priv, "-pubout").Output()
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pubPEM)
	k, _ := x509.ParsePKIXPublicKey(block.Bytes)
	id := signing.KeyID(signing.PurposeAgent, k.(ed25519.PublicKey))
	gen := func(name string) (string, error) {
		_ = os.RemoveAll(keys)
		_ = os.MkdirAll(keys, 0o755)
		_ = os.WriteFile(filepath.Join(keys, name), pubPEM, 0o644)
		out := filepath.Join(tmp, "out.go")
		cmd := exec.Command("bash", filepath.Join(root, "scripts/release/agent/genkeys.sh"), "--keys-dir", keys, out)
		cmd.Env = append(os.Environ(), "TMPDIR="+tmp)
		b, err := cmd.CombinedOutput()
		src, _ := os.ReadFile(out)
		return string(b) + string(src), err
	}
	text, err := gen(id + ".pub")
	if err != nil || !strings.Contains(text, `ID: "`+id+`"`) {
		t.Fatalf("genkeys.sh did not list the derived id %s: %v\n%s", id, err, text)
	}
	rev := filepath.Join(keys, "agent-revoked.txt")
	_ = os.WriteFile(filepath.Join(keys, id+".pub"), pubPEM, 0o644)
	_ = os.WriteFile(rev, []byte("# leaked\n\n"+id+"\n"), 0o644)
	out := filepath.Join(tmp, "rev.go")
	cmd := exec.Command("bash", filepath.Join(root, "scripts/release/agent/genkeys.sh"), "--keys-dir", keys, out)
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp)
	if b, rerr := cmd.CombinedOutput(); rerr != nil {
		t.Fatalf("genkeys.sh with a revoked list: %v\n%s", rerr, b)
	}
	if src, _ := os.ReadFile(out); !strings.Contains(string(src), `"`+id+`",`) {
		t.Fatalf("the revoked id is missing from the generated file:\n%s", src)
	}
	_ = os.WriteFile(rev, []byte("plugins-0123456789abcdef\n"), 0o644)
	if b, rerr := exec.Command("bash", filepath.Join(root, "scripts/release/agent/genkeys.sh"), "--keys-dir", keys, out).CombinedOutput(); rerr == nil {
		t.Fatalf("a revoked entry that is not an agent id must be refused: %s", b)
	}
	_ = os.Remove(rev)
	if _, err = gen("agent-0000000000000000.pub"); err == nil {
		t.Fatal("a key file whose name is not its derived id must be refused")
	}
}

func TestDefaultUsesTheCompiledKeySet(t *testing.T) {
	s := newSigner(t, signing.PurposeAgent)
	old := generatedKeys
	defer func() { generatedKeys = old }()
	if v, err := Default(); err != nil || v.HasKeys() {
		t.Fatalf("the committed key set must be empty until P7-NODE-29: %v", err)
	}
	generatedKeys = []keyEntry{{ID: s.KeyID(), Public: base64.StdEncoding.EncodeToString(s.pub)}}
	v, err := Default()
	if err != nil || !v.HasKeys() {
		t.Fatalf("Default over a generated key set: %v", err)
	}
	a := newArtifact(t, s)
	if _, err = v.checkManifest(a.path, a.json(t), pinned); err != nil {
		t.Fatalf("artifact signed by the compiled-in key refused: %v", err)
	}
	oldRev := generatedRevoked
	defer func() { generatedRevoked = oldRev }()
	generatedRevoked = []string{s.KeyID()}
	rv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	_, err = rv.checkManifest(a.path, a.json(t), pinned)
	wantReason(t, err, ReasonRevoked)
	generatedRevoked = nil
	generatedKeys = []keyEntry{{ID: s.KeyID(), Public: "AAAA"}}
	if _, err = Default(); err == nil {
		t.Fatal("a malformed compiled-in key must be refused")
	}
}

// TestOnePackageVerifiesAgentArtifacts: the signing input string appears in this package only, and this package imports
// sdk/go/signing (it makes every key, purpose and revocation decision).
func TestOnePackageVerifiesAgentArtifacts(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	var owners []string
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(b), "nself-agent-v1") {
			owners = append(owners, filepath.Dir(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || filepath.Base(owners[0]) != "verify" {
		t.Fatalf("exactly one package may build the agent signing input, got %v", owners)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "verify.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, im := range f.Imports {
		found = found || strings.Trim(im.Path.Value, `"`) == "github.com/nself-org/cli/sdk/go/v2/signing"
	}
	if !found {
		t.Fatal("verify.go must import cli sdk/go/signing")
	}
}

// TestGeneratedFileIsGofmtCleanWithKeys: the Go gate runs gofmt, so a generated file that holds keys and revoked ids must already be formatted.
func TestGeneratedFileIsGofmtCleanWithKeys(t *testing.T) {
	needTools(t)
	root := repoRoot(t)
	keys, tmp := t.TempDir(), t.TempDir()
	for i := 0; i < 2; i++ {
		priv := filepath.Join(tmp, "k.pem")
		if out, err := exec.Command("openssl", "genpkey", "-algorithm", "ed25519", "-out", priv).CombinedOutput(); err != nil {
			t.Fatalf("genpkey: %v\n%s", err, out)
		}
		pubPEM, _ := exec.Command("openssl", "pkey", "-in", priv, "-pubout").Output()
		block, _ := pem.Decode(pubPEM)
		k, _ := x509.ParsePKIXPublicKey(block.Bytes)
		id := signing.KeyID(signing.PurposeAgent, k.(ed25519.PublicKey))
		_ = os.WriteFile(filepath.Join(keys, id+".pub"), pubPEM, 0o644)
		_ = os.WriteFile(filepath.Join(keys, "agent-revoked.txt"), []byte(id+"\n"), 0o644)
	}
	out := filepath.Join(tmp, "out.go")
	cmd := exec.Command("bash", filepath.Join(root, "scripts/release/agent/genkeys.sh"), "--keys-dir", keys, out)
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("genkeys.sh: %v\n%s", err, b)
	}
	src, _ := os.ReadFile(out)
	formatted, err := format.Source(src)
	if err != nil || string(formatted) != string(src) {
		t.Fatalf("generated file is not gofmt-clean (err %v):\n%s", err, src)
	}
}

// TestAgentKeyCannotBeAnotherPurposesKey: the plugin-release key renamed to an agent key is refused (purpose wall, hq NODE D11).
func TestAgentKeyCannotBeAnotherPurposesKey(t *testing.T) {
	needTools(t)
	root := repoRoot(t)
	keys, tmp := t.TempDir(), t.TempDir()
	priv := filepath.Join(tmp, "k.pem")
	if out, err := exec.Command("openssl", "genpkey", "-algorithm", "ed25519", "-out", priv).CombinedOutput(); err != nil {
		t.Fatalf("genpkey: %v\n%s", err, out)
	}
	pubPEM, _ := exec.Command("openssl", "pkey", "-in", priv, "-pubout").Output()
	block, _ := pem.Decode(pubPEM)
	k, _ := x509.ParsePKIXPublicKey(block.Bytes)
	id := signing.KeyID(signing.PurposeAgent, k.(ed25519.PublicKey))
	_ = os.WriteFile(filepath.Join(keys, "plugins-release-free-"+strings.TrimPrefix(id, "agent-")+".pub"), pubPEM, 0o644)
	_ = os.WriteFile(filepath.Join(keys, id+".pub"), pubPEM, 0o644)
	cmd := exec.Command("bash", filepath.Join(root, "scripts/release/agent/genkeys.sh"), "--keys-dir", keys, filepath.Join(tmp, "out.go"))
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp)
	if b, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(b), "non-agent key") {
		t.Fatalf("an agent key equal to a plugin key must be refused: %v\n%s", err, b)
	}
}
