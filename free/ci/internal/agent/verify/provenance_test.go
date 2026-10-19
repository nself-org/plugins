package verify

// Purpose: tests of the agent supply-chain checks (P7-CACHE-10): the SBOMs and the signed provenance must belong to the manifest's
//          binary, and every failure is E665 reason "provenance" with nothing accepted.
// Inputs:  keys generated in the tests (agent, plugins, ci-release purposes); statements built by internal/provenance.BuildAgent.
// Constraints: every key is a throwaway generated here; no network; no secret.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// release is one binary with its manifest and its three side files, all signed by one signer.
type release struct {
	artifact
	files Files
}

func (r release) statement(t *testing.T, mutate func(*provenance.AgentFacts)) []byte {
	t.Helper()
	name := BinaryName(r.m.Target)
	f := provenance.AgentFacts{Repo: "nself-org/plugins", Ref: "refs/tags/v1.5.0", Revision: strings.Repeat("a", 40), RunID: "42",
		Subjects: []provenance.FileDigest{{Name: name, SHA256: r.m.SHA256}},
		SBOMs:    []provenance.FileDigest{{Name: name + SuffixCycloneDX, SHA256: sum(r.files.CycloneDX)}, {Name: name + SuffixSPDX, SHA256: sum(r.files.SPDX)}}}
	if mutate != nil {
		mutate(&f)
	}
	st, err := provenance.BuildAgent(f)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func envelopeBytes(t *testing.T, s signer, payloadType string, payload []byte) []byte {
	t.Helper()
	env, err := signing.SignEnvelope(s, payloadType, payload)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newRelease(t *testing.T, s signer) release {
	t.Helper()
	r := release{artifact: newArtifact(t, s)}
	r.files = Files{CycloneDX: []byte(cdxDoc(BinaryName(r.m.Target))), SPDX: []byte(spdxDoc(BinaryName(r.m.Target)))}
	r.files.Provenance = envelopeBytes(t, s, provenance.PayloadType, r.statement(t, nil))
	return r
}

func TestArtifactAcceptsAReleaseWithSBOMsAndProvenance(t *testing.T) {
	s := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, s)
	m, err := mustVerifier(t, nil, s.key()).Artifact(r.path, r.json(t), pinned, r.files)
	if err != nil || m.SHA256 != r.m.SHA256 {
		t.Fatalf("valid release refused: %v", err)
	}
}

func TestArtifactRefusesWhatProvenanceCannotVouchFor(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	plugin := newSigner(t, signing.PurposePlugins)
	ciRelease := newSigner(t, signing.PurposeCIRelease)
	otherAgent := newSigner(t, signing.PurposeAgent)
	good := newRelease(t, agent)
	signedBy := func(s signer, mutate func(*provenance.AgentFacts)) release {
		r := good
		r.files.Provenance = envelopeBytes(t, s, provenance.PayloadType, good.statement(t, mutate))
		return r
	}
	cases := []struct {
		name    string
		release release
		keys    []signing.Key
	}{
		{"provenance missing", func() release { r := good; r.files.Provenance = nil; return r }(), nil},
		{"CycloneDX SBOM missing", func() release { r := good; r.files.CycloneDX = nil; return r }(), nil},
		{"SPDX SBOM missing", func() release { r := good; r.files.SPDX = nil; return r }(), nil},
		{"oversized provenance", func() release {
			r := good
			r.files.Provenance = bytes.Repeat([]byte(" "), MaxProvenanceBytes+1)
			return r
		}(), nil},
		{"provenance is not an envelope", func() release { r := good; r.files.Provenance = []byte(`{"hello":1}`); return r }(), nil},
		{"provenance for another digest", signedBy(agent, func(f *provenance.AgentFacts) {
			f.Subjects[0].SHA256 = strings.Repeat("b", 64)
		}), nil},
		{"provenance for another binary name", signedBy(agent, func(f *provenance.AgentFacts) { f.Subjects[0].Name = "nself-ci-agent-darwin-arm64" }), nil},
		{"provenance with two subjects", signedBy(agent, func(f *provenance.AgentFacts) {
			f.Subjects = append(f.Subjects, provenance.FileDigest{Name: "nself-ci-agent-darwin-arm64", SHA256: strings.Repeat("c", 64)})
		}), nil},
		{"CycloneDX byproduct digest differs from the SBOM", signedBy(agent, func(f *provenance.AgentFacts) { f.SBOMs[0].SHA256 = strings.Repeat("d", 64) }), nil},
		{"SPDX byproduct digest differs from the SBOM", signedBy(agent, func(f *provenance.AgentFacts) { f.SBOMs[1].SHA256 = strings.Repeat("d", 64) }), nil},
		{"SBOM edited after signing", func() release { r := good; r.files.SPDX = []byte(`{"spdxVersion":"SPDX-2.3","x":1}`); return r }(), nil},
		{"one SBOM byproduct only", signedBy(agent, func(f *provenance.AgentFacts) { f.SBOMs = f.SBOMs[:1] }), nil},
		{"extra byproduct", signedBy(agent, func(f *provenance.AgentFacts) {
			f.SBOMs = append(f.SBOMs, provenance.FileDigest{Name: "extra.json", SHA256: strings.Repeat("e", 64)})
		}), nil},
		{"built by another repository", signedBy(agent, func(f *provenance.AgentFacts) { f.Repo = "evil/plugins" }), nil},
		{"built from another tag", signedBy(agent, func(f *provenance.AgentFacts) { f.Ref = "refs/tags/v9.9.9" }), nil},
		{"signed by a ci-release key the set holds", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, ciRelease, provenance.PayloadType, good.statement(t, nil))
			return r
		}(), []signing.Key{agent.key(), ciRelease.key()}},
		{"signed by a ci-release key", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, ciRelease, provenance.PayloadType, good.statement(t, nil))
			return r
		}(), []signing.Key{agent.key()}},
		{"signed by a plugins key the set holds", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, plugin, provenance.PayloadType, good.statement(t, nil))
			return r
		}(), []signing.Key{agent.key(), plugin.key()}},
		{"signed by an agent key nobody trusts", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, otherAgent, provenance.PayloadType, good.statement(t, nil))
			return r
		}(), []signing.Key{agent.key()}},
		{"wrong payload type", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, agent, "application/json", good.statement(t, nil))
			return r
		}(), nil},
		{"release build type", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, agent, provenance.PayloadType,
				bytes.ReplaceAll(good.statement(t, nil), []byte(provenance.BuildTypeAgent), []byte(provenance.BuildTypeRelease)))
			return r
		}(), nil},
		{"builder is another workflow", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, agent, provenance.PayloadType,
				bytes.ReplaceAll(good.statement(t, nil), []byte("/release-agent.yml@"), []byte("/other.yml@")))
			return r
		}(), nil},
		{"statement is not valid JSON", func() release {
			r := good
			r.files.Provenance = envelopeBytes(t, agent, provenance.PayloadType, []byte("not json"))
			return r
		}(), nil},
	}
	// Every row must fail for the reason it names, not on an earlier check: the message says which rule refused.
	why := map[string]string{
		"provenance missing": "no provenance file", "CycloneDX SBOM missing": "no CycloneDX SBOM file", "SPDX SBOM missing": "no SPDX SBOM file",
		"oversized provenance": "larger than", "provenance is not an envelope": "not a DSSE envelope",
		"provenance for another digest": "provenance subject", "provenance for another binary name": "provenance subject", "provenance with two subjects": "provenance subject",
		"CycloneDX byproduct digest differs from the SBOM": "not the SBOM the provenance names", "SPDX byproduct digest differs from the SBOM": "not the SBOM the provenance names",
		"SBOM edited after signing": "not the SBOM the provenance names", "one SBOM byproduct only": "byproducts", "extra byproduct": "byproducts",
		"built by another repository": "not the agent release workflow of", "built from another tag": "not the tag of version",
		"signed by a ci-release key the set holds": "wrong_purpose", "signed by a ci-release key": "unknown_key",
		"signed by a plugins key the set holds": "wrong_purpose", "signed by an agent key nobody trusts": "unknown_key",
		"wrong payload type": "payload type", "release build type": "build type", "builder is another workflow": "not the agent release workflow",
		"statement is not valid JSON": "statement is not valid",
	}
	for _, c := range cases {
		keys := c.keys
		if keys == nil {
			keys = []signing.Key{agent.key()}
		}
		m, err := mustVerifier(t, nil, keys...).Artifact(good.path, good.json(t), pinned, c.release.files)
		if err == nil || m.SHA256 != "" {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		var ve *Error
		if !asError(err, &ve) || ve.Reason != ReasonProvenance || ve.Code() != "E665" {
			t.Errorf("%s: want E665 %s, got %v", c.name, ReasonProvenance, err)
		} else if w := why[c.name]; w == "" || !strings.Contains(err.Error(), w) {
			t.Errorf("%s: refused for another reason than %q: %v", c.name, w, err)
		}
	}
}

func TestProvenanceRefusesARevokedKeyAndABrokenSignature(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, agent)
	_, err := mustVerifier(t, []string{agent.KeyID()}, agent.key()).Artifact(r.path, r.json(t), pinned, r.files)
	if err == nil {
		t.Fatal("a revoked key must fail (the manifest already carries it)")
	}
	var env signing.Envelope
	if err = json.Unmarshal(r.files.Provenance, &env); err != nil {
		t.Fatal(err)
	}
	sig, _ := signing.DecodeSig(env.Signatures[0].Sig)
	sig[0] ^= 1
	env.Signatures[0].Sig = signing.EncodeSig(sig)
	r.files.Provenance, _ = json.Marshal(env)
	_, err = mustVerifier(t, nil, agent.key()).Artifact(r.path, r.json(t), pinned, r.files)
	wantReason(t, err, ReasonProvenance)
	if !strings.Contains(err.Error(), ReasonBadSignature) {
		t.Fatalf("the message should name the signing reason: %v", err)
	}
}

func TestProvenanceRefusesARevokedProvenanceKeyAndAForeignExtraSignature(t *testing.T) {
	manifestKey := newSigner(t, signing.PurposeAgent)
	provKey := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, manifestKey)
	r.files.Provenance = envelopeBytes(t, provKey, provenance.PayloadType, r.statement(t, nil))
	ok, err := mustVerifier(t, nil, manifestKey.key(), provKey.key()).AllowRehearsalKeys().Artifact(r.path, r.json(t), pinned, r.files)
	if err != nil || ok.SHA256 != r.m.SHA256 {
		t.Fatalf("two trusted agent keys must verify in a rehearsal: %v", err)
	}
	// Only the provenance key is revoked: the manifest still verifies, so the refusal is the provenance check's.
	_, err = mustVerifier(t, []string{provKey.KeyID()}, manifestKey.key(), provKey.key()).AllowRehearsalKeys().Artifact(r.path, r.json(t), pinned, r.files)
	wantReason(t, err, ReasonProvenance)
	if !strings.Contains(err.Error(), ReasonRevoked) {
		t.Fatalf("the message should name the revoked key: %v", err)
	}
	// A valid agent signature plus one more from a trusted key of another purpose: the whole envelope fails.
	plugin := newSigner(t, signing.PurposePlugins)
	var env signing.Envelope
	if err = json.Unmarshal(r.files.Provenance, &env); err != nil {
		t.Fatal(err)
	}
	payload, _ := base64Std(env.Payload)
	extra, _ := plugin.Sign(signing.PAE(env.PayloadType, payload))
	env.Signatures = append(env.Signatures, signing.EnvelopeSig{KeyID: plugin.KeyID(), Sig: signing.EncodeSig(extra)})
	r.files.Provenance, _ = json.Marshal(env)
	_, err = mustVerifier(t, nil, manifestKey.key(), provKey.key(), plugin.key()).AllowRehearsalKeys().Artifact(r.path, r.json(t), pinned, r.files)
	wantReason(t, err, ReasonProvenance)
}

func TestProvenanceEnvelopeIsReadStrictly(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, agent)
	v := mustVerifier(t, nil, agent.key())
	good := string(r.files.Provenance)
	for name, b := range map[string]string{
		"unknown field":          strings.Replace(good, `{"payloadType"`, `{"extra":1,"payloadType"`, 1),
		"repeated field":         strings.Replace(good, `{"payloadType"`, `{"payloadType":"x","payloadType"`, 1),
		"case-folded repeat":     strings.Replace(good, `{"payloadType"`, `{"PAYLOADTYPE":"x","payloadType"`, 1),
		"trailing data":          good + `{}`,
		"folded repeat (U+017F)": strings.Replace(good, `{"payloadType"`, `{"ſignatures":[],"payloadType"`, 1),
		"not JSON":               "DSSE",
	} {
		r.files.Provenance = []byte(b)
		_, err := v.Artifact(r.path, r.json(t), pinned, r.files)
		if err == nil || ReasonOf(err) != ReasonProvenance {
			t.Errorf("%s: want E665 provenance, got %v", name, err)
		}
	}
}

func TestProvenanceIsCheckedAfterTheManifest(t *testing.T) {
	// A tampered binary is a checksum failure even when the provenance is also wrong: the manifest comes first.
	agent := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, agent)
	r.files.Provenance = nil
	if err := os.WriteFile(r.path, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := mustVerifier(t, nil, agent.key()).Artifact(r.path, r.json(t), pinned, r.files)
	wantReason(t, err, ReasonChecksumMismatch)
}

func TestVerifyProvenanceNeedsATrustedKey(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, agent)
	wantReason(t, mustVerifier(t, nil).VerifyProvenance(r.m, r.files), ReasonNoTrustedKey)
}

// TestInstallReachesArtifact keeps the supply-chain check on the one path: every package that places an agent calls
// verify.Artifact, and none of them verifies a provenance or a signature itself.
func TestInstallReachesArtifact(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "install", "release.go"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "release.go", src, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	imports := false
	for _, imp := range f.Imports {
		imports = imports || strings.HasSuffix(imp.Path.Value, `/internal/agent/verify"`)
		if strings.Contains(imp.Path.Value, "internal/provenance") || strings.Contains(imp.Path.Value, "sdk/go/v2/signing") {
			t.Errorf("install must not verify on its own: it imports %s", imp.Path.Value)
		}
	}
	if !imports || !bytes.Contains(src, []byte(".Artifact(")) {
		t.Fatal("install must call verify.Artifact")
	}
}

func base64Std(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

func asError(err error, target **Error) bool {
	ve, ok := err.(*Error)
	if ok {
		*target = ve
	}
	return ok
}

// fakeSyft is a stand-in for syft: it prints the given version and writes a small document at each -o path.
func fakeSyft(version string) string {
	return strings.Replace(fakeSyftBody, "@VERSION@", version, 1)
}

const fakeSyftBody = `#!/bin/sh
[ -z "${NSELF_AGENT_SIGNING_KEY:-}" ] || { echo "the signing key leaked into syft" >&2; exit 7; }
[ -z "${FAKE_SYFT_MARK:-}" ] || echo ran >> "$FAKE_SYFT_MARK"
case "$1" in
  version) echo "Version:       @VERSION@"; exit 0 ;;
  scan) shift; src=
    while [ $# -gt 0 ]; do
      case "$1" in
        --source-name) src="$2"; shift 2 ;;
        -o) case "${2%%=*}" in
              cyclonedx-json*) echo "{\"bomFormat\":\"CycloneDX\",\"specVersion\":\"1.6\",\"metadata\":{\"component\":{\"type\":\"file\",\"name\":\"$src\"}}}" > "${2#*=}" ;;
              *) echo "{\"spdxVersion\":\"SPDX-2.3\",\"name\":\"$src\"}" > "${2#*=}" ;;
            esac; shift 2 ;;
        *) shift ;;
      esac
    done ;;
esac
`

// scriptEnv is a release directory of fake binaries, a fake syft and a versions table that pins it.
type scriptEnv struct{ dir, work, fake, table, scripts string }

func scriptSetup(t *testing.T, syftVersion string, pinnedSHA func(fake string) string) scriptEnv {
	t.Helper()
	for _, tool := range []string{"bash", "openssl", "go"} {
		if _, lerr := exec.LookPath(tool); lerr != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	if v, verr := exec.Command("openssl", "version").Output(); verr != nil || !strings.HasPrefix(string(v), "OpenSSL ") {
		t.Skip("OpenSSL 3 not installed")
	}
	goos := map[string]string{"linux": "linux", "darwin": "darwin"}[runtime.GOOS]
	if goos == "" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("unsupported host")
	}
	e := scriptEnv{work: t.TempDir(), scripts: filepath.Join("..", "..", "..", "..", "..", "scripts", "release", "agent")}
	e.dir = filepath.Join(e.work, "dist")
	if err := os.Mkdir(e.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"linux-amd64", "darwin-arm64"} {
		if err := os.WriteFile(filepath.Join(e.dir, BinaryName(target)), []byte("agent bytes "+target), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.dir, "VERSION"), []byte("1.5.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.fake = filepath.Join(e.work, "syft")
	if err := os.WriteFile(e.fake, []byte(fakeSyft(syftVersion)), 0o755); err != nil {
		t.Fatal(err)
	}
	e.table = filepath.Join(e.work, "versions.yaml")
	row := fmt.Sprintf("versions:\n  - name: syft\n    kind: tool\n    version: 1.54.1\n    sources:\n      - os: %s\n        arch: %s\n        url: https://127.0.0.1:1/syft.tgz\n        sha256: %s\n        binary_sha256: %s\n",
		goos, runtime.GOARCH, strings.Repeat("1", 64), pinnedSHA(e.fake))
	if err := os.WriteFile(e.table, []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e scriptEnv) run(script string, extraEnv []string, args ...string) ([]byte, error) {
	cmd := exec.Command("bash", append([]string{filepath.Join(e.scripts, script)}, args...)...)
	var base []string
	for _, kv := range os.Environ() { // a developer shell may export the real key or GitHub variables: the script under test must not see them
		if !strings.HasPrefix(kv, "NSELF_AGENT_SIGNING_KEY") && !strings.HasPrefix(kv, "GITHUB_") {
			base = append(base, kv)
		}
	}
	cmd.Env = append(base, append([]string{"NSELF_AGENT_SYFT=" + e.fake, "TMPDIR=" + e.work, "GOFLAGS="}, extraEnv...)...)
	return cmd.CombinedOutput()
}

// scriptRelease runs manifest.sh and provenance.sh (--rehearsal) over fake binaries with a fake syft whose sha256 the test pins, and
// returns the release directory.
func scriptRelease(t *testing.T, syftVersion string, pinnedSHA func(fake string) string) (dir string, out []byte, err error) {
	t.Helper()
	e := scriptSetup(t, syftVersion, pinnedSHA)
	if o, merr := e.run("manifest.sh", nil, "--in", e.dir, "--rehearsal"); merr != nil {
		t.Fatalf("manifest.sh: %v\n%s", merr, o)
	}
	out, err = e.run("provenance.sh", nil, "--in", e.dir, "--rehearsal", "--versions", e.table)
	return e.dir, out, err
}

func fileSHA(t *testing.T) func(string) string {
	return func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return sum(b)
	}
}

// rehearsalVerifier trusts every key in DIR/rehearsal-test.pub.
func rehearsalVerifier(t *testing.T, dir string) *Verifier {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "rehearsal-test.pub"))
	if err != nil {
		t.Fatal(err)
	}
	var keys []signing.Key
	for rest := data; ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		k, kerr := x509.ParsePKIXPublicKey(block.Bytes)
		pub, ok := k.(ed25519.PublicKey)
		if kerr != nil || !ok {
			t.Fatal("rehearsal-test.pub holds a key that is not Ed25519")
		}
		keys = append(keys, signing.Key{ID: signing.KeyID(signing.PurposeAgent, pub), Purpose: signing.PurposeAgent, Public: pub})
	}
	if len(keys) != 2 {
		t.Fatalf("rehearsal-test.pub should hold the manifest key and the provenance key, got %d", len(keys))
	}
	return mustVerifier(t, nil, keys...).AllowRehearsalKeys()
}

// TestProvenanceScriptOutputVerifies runs the release scripts (rehearsal key, fake pinned syft, no network) and feeds what they wrote
// to the same package the agent installer uses.
func TestProvenanceScriptOutputVerifies(t *testing.T) {
	dir, out, err := scriptRelease(t, "1.54.1", fileSHA(t))
	if err != nil {
		t.Fatalf("provenance.sh: %v\n%s", err, out)
	}
	v := rehearsalVerifier(t, dir)
	for _, target := range []string{"linux-amd64", "darwin-arm64"} {
		name := BinaryName(target)
		read := func(suffix string) []byte {
			b, rerr := os.ReadFile(filepath.Join(dir, name+suffix))
			if rerr != nil || len(b) == 0 {
				t.Fatalf("%s%s: %v", name, suffix, rerr)
			}
			return b
		}
		files := Files{Provenance: read(SuffixProvenance), CycloneDX: read(SuffixCycloneDX), SPDX: read(SuffixSPDX)}
		if _, err = v.Artifact(filepath.Join(dir, name), read(".manifest.json"), Expect{Version: "1.5.0", Target: target}, files); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		files.SPDX = append(files.SPDX, ' ')
		_, err = v.Artifact(filepath.Join(dir, name), read(".manifest.json"), Expect{Version: "1.5.0", Target: target}, files)
		wantReason(t, err, ReasonProvenance)
	}
}

// TestProvenanceScriptRunsOnlyThePinnedSyft: a syft whose sha256 is not the pin is never run, and nothing is written.
func TestProvenanceScriptRunsOnlyThePinnedSyft(t *testing.T) {
	for name, c := range map[string]struct {
		version string
		sha     func(string) string
	}{
		"another sha256":   {"1.54.1", func(string) string { return strings.Repeat("2", 64) }},
		"another version":  {"1.54.0", fileSHA(t)},
		"unparsable build": {"", fileSHA(t)},
	} {
		mark := filepath.Join(t.TempDir(), "syft-ran")
		t.Setenv("FAKE_SYFT_MARK", mark)
		dir, out, err := scriptRelease(t, c.version, c.sha)
		if _, serr := os.Stat(mark); c.sha != nil && name == "another sha256" && serr == nil {
			t.Fatalf("%s: a syft with another sha256 was executed", name)
		}
		if err == nil {
			t.Fatalf("%s: an unpinned syft was accepted:\n%s", name, out)
		}
		if !strings.Contains(string(out), "cannot download the pinned syft") {
			t.Fatalf("%s: expected the download refusal (the candidate must not be used), got:\n%s", name, out)
		}
		if m, _ := filepath.Glob(filepath.Join(dir, "*"+SuffixProvenance)); len(m) != 0 {
			t.Fatalf("%s: provenance written without a pinned syft: %v", name, m)
		}
	}
}

// TestProvenanceScriptReleasePathRunsInTwoPhases is the release path: --prepare runs go and syft without the key (and refuses to start
// with it), --sign takes the key from the environment and the builder from the GitHub variables and runs openssl only.
func TestProvenanceScriptReleasePathRunsInTwoPhases(t *testing.T) {
	e := scriptSetup(t, "1.54.1", fileSHA(t))
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	keyID := signing.KeyID(signing.PurposeAgent, pub)
	env := []string{"NSELF_AGENT_SIGNING_KEY=" + keyPEM, "NSELF_AGENT_SIGNING_KEY_ID=" + keyID,
		"GITHUB_WORKFLOW_REF=nself-org/plugins/.github/workflows/release-agent.yml@refs/tags/v1.5.0", "GITHUB_REPOSITORY=nself-org/plugins", "GITHUB_RUN_ID=77", "GITHUB_SHA=" + strings.Repeat("c", 40)}
	if o, merr := e.run("manifest.sh", env, "--in", e.dir); merr != nil {
		t.Fatalf("manifest.sh: %v\n%s", merr, o)
	}
	// The release path is two steps: --prepare runs go and syft and must not hold the key; --sign holds the key and runs openssl only.
	prepEnv := env[2:] // the GitHub variables only: no key
	if o, perr := e.run("provenance.sh", prepEnv, "--in", e.dir, "--prepare", "--versions", e.table); perr != nil {
		t.Fatalf("provenance.sh --prepare: %v\n%s", perr, o)
	}
	if o, perr := e.run("provenance.sh", env, "--in", e.dir, "--sign"); perr != nil {
		t.Fatalf("provenance.sh --sign: %v\n%s", perr, o)
	}
	if _, serr := os.Stat(filepath.Join(e.dir, ".provenance")); serr == nil {
		t.Fatal("--sign must remove the prepared statements")
	}
	v := mustVerifier(t, nil, signing.Key{ID: keyID, Purpose: signing.PurposeAgent, Public: pub})
	name := BinaryName("linux-amd64")
	read := func(suffix string) []byte { b, _ := os.ReadFile(filepath.Join(e.dir, name+suffix)); return b }
	files := Files{Provenance: read(SuffixProvenance), CycloneDX: read(SuffixCycloneDX), SPDX: read(SuffixSPDX)}
	if _, err = v.Artifact(filepath.Join(e.dir, name), read(".manifest.json"), pinned, files); err != nil {
		t.Fatal(err)
	}
	var env2 signing.Envelope
	if err = json.Unmarshal(files.Provenance, &env2); err != nil {
		t.Fatal(err)
	}
	payload, _ := base64Std(env2.Payload)
	st, err := provenance.Parse(payload)
	if err != nil || st.Predicate.RunDetails.Metadata.InvocationID != "https://github.com/nself-org/plugins/actions/runs/77" || st.GitCommit() != strings.Repeat("c", 40) {
		t.Fatalf("builder facts: %v %+v", err, st.Predicate.RunDetails)
	}
	if bytes.Contains(files.Provenance, []byte("PRIVATE")) {
		t.Fatal("key text in the provenance file")
	}
	// The guards around the phases. Each row names its own message: a row that failed earlier for another reason would not match it.
	ghEnv := []string{"GITHUB_WORKFLOW_REF=nself-org/plugins/.github/workflows/release-agent.yml@refs/tags/v1.5.0", "GITHUB_REPOSITORY=nself-org/plugins", "GITHUB_RUN_ID=78", "GITHUB_SHA=" + strings.Repeat("d", 40)}
	fresh := func() scriptEnv { // a release directory with manifests (manifest.sh with the real-mode key) and nothing else
		e2 := scriptSetup(t, "1.54.1", fileSHA(t))
		if o, merr := e2.run("manifest.sh", env[:2], "--in", e2.dir); merr != nil {
			t.Fatalf("manifest.sh: %v\n%s", merr, o)
		}
		return e2
	}
	refuse := func(name string, e2 scriptEnv, extra []string, want string, args ...string) {
		t.Helper()
		o, rerr := e2.run("provenance.sh", extra, args...)
		if rerr == nil || !strings.Contains(string(o), want) {
			t.Errorf("%s: want a refusal containing %q, got err=%v\n%s", name, want, rerr, o)
		}
	}
	e2 := fresh()
	refuse("prepare holding the key", e2, append(env[:1:1], ghEnv...), "without NSELF_AGENT_SIGNING_KEY", "--in", e2.dir, "--prepare", "--versions", e2.table)
	refuse("prepare holding the key, rehearsal", e2, env[:1], "without NSELF_AGENT_SIGNING_KEY", "--in", e2.dir, "--prepare", "--rehearsal", "--versions", e2.table)
	refuse("prepare without the GitHub variables", e2, nil, "GITHUB_WORKFLOW_REF, GITHUB_RUN_ID and GITHUB_SHA must be set", "--in", e2.dir, "--prepare", "--versions", e2.table)
	refuse("no phase", e2, env[:2], "name a phase", "--in", e2.dir)
	refuse("both phases with a real key", e2, env[:2], "for --rehearsal only", "--in", e2.dir, "--prepare", "--sign")
	refuse("sign before prepare", e2, append(env[:2:2], ghEnv...), "run --prepare first", "--in", e2.dir, "--sign")
	if o, perr := e2.run("provenance.sh", append([]string{}, ghEnv...), "--in", e2.dir, "--prepare", "--versions", e2.table); perr != nil {
		t.Fatalf("prepare: %v\n%s", perr, o)
	}
	refuse("prepare twice", e2, ghEnv, "already exists", "--in", e2.dir, "--prepare", "--versions", e2.table)
	refuse("sign for another run", e2, append(env[:2:2], "GITHUB_WORKFLOW_REF="+ghEnv[0][len("GITHUB_WORKFLOW_REF="):], "GITHUB_REPOSITORY=nself-org/plugins", "GITHUB_RUN_ID=79", "GITHUB_SHA="+strings.Repeat("d", 40)),
		"is not about this run", "--in", e2.dir, "--sign")
	// A file changed after --prepare no longer matches the statement.
	if err = os.WriteFile(filepath.Join(e2.dir, BinaryName("linux-amd64")+SuffixSPDX), []byte(`{"changed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse("SBOM changed after prepare", e2, append(env[:2:2], ghEnv...), "does not match", "--in", e2.dir, "--sign")
	// A PAE that is not the encoding of the statement is never signed.
	e3 := fresh()
	if o, perr := e3.run("provenance.sh", ghEnv, "--in", e3.dir, "--prepare", "--versions", e3.table); perr != nil {
		t.Fatalf("prepare: %v\n%s", perr, o)
	}
	if err = os.WriteFile(filepath.Join(e3.dir, ".provenance", "linux-amd64", "statement.pae"), []byte("DSSEv1 forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse("forged PAE", e3, append(env[:2:2], ghEnv...), "not the DSSE encoding", "--in", e3.dir, "--sign")
	// A directory prepared as a rehearsal cannot be signed by a real run.
	e4 := fresh()
	if o, perr := e4.run("provenance.sh", nil, "--in", e4.dir, "--prepare", "--rehearsal", "--versions", e4.table); perr != nil {
		t.Fatalf("rehearsal prepare: %v\n%s", perr, o)
	}
	refuse("rehearsal statements, real key", e4, append(env[:2:2], ghEnv...), "prepared as a rehearsal", "--in", e4.dir, "--sign")
}

// TestProvenanceTagsOnlyVerifierRefusesABranchRef: the production verifier (Default sets tagsOnly) accepts a release built from a tag
// only; a New verifier also accepts a rehearsal built from a branch.
func TestProvenanceTagsOnlyVerifierRefusesABranchRef(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, agent)
	r.files.Provenance = envelopeBytes(t, agent, provenance.PayloadType, r.statement(t, func(f *provenance.AgentFacts) { f.Ref = "refs/heads/main" }))
	v := mustVerifier(t, nil, agent.key())
	if _, err := v.Artifact(r.path, r.json(t), pinned, r.files); err != nil {
		t.Fatalf("a rehearsal from a branch must verify with New: %v", err)
	}
	v.tagsOnly = true
	_, err := v.Artifact(r.path, r.json(t), pinned, r.files)
	wantReason(t, err, ReasonProvenance)
	if !strings.Contains(err.Error(), "not the tag of version") {
		t.Fatalf("expected the tag rule, got %v", err)
	}
}
