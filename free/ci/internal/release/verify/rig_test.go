package verify_test

// Purpose: the release fixture of the verify tests: a real git repository, a
// signed release directory built from it, and hooks to bend every part before
// it is written. Every key and digest is synthetic.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
)

const finishedAt = "2026-10-19T12:00:00Z"

// trustMode says where the ci-release public key is trusted from.
type trustMode int

const (
	operatorKeys trustMode = iota // an operator-scope keys file
	pinnedKeys                    // the keys file on the pinned default-branch commit
	releasedOnly                  // the keys file exists only in the released revision
)

type rig struct {
	t                     *testing.T
	dir, repo, rev, base  string // release dir, repository, released revision, pinned commit
	signer, other         signing.Signer
	pub, otherPub         ed25519.PublicKey
	keysFile, revokedFile string
	ev                    model.Evidence
	inputs                model.InputDigests
	arts                  map[string][]byte
	opt                   verify.Options
	tweakEvidence         func(*model.Evidence)
	tweakFacts            func(*provenance.ReleaseFacts)
	tweakManifest         func(*manifest.Manifest)
	tweakManifestPayload  func([]byte) []byte
	tweakProvPayload      func([]byte) []byte
	manSigner, provSigner signing.Signer
	manType, provType     string
	manSigs               int // signatures on the manifest envelope (default 1)
	skipSBOM              string
}

func seedKey(n byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed([]byte(strings.Repeat(string(rune('a'+n)), 32)))
}

func git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t testing.TB, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t testing.TB, repo string, files map[string]string) string {
	t.Helper()
	for n, b := range files {
		writeFile(t, repo, n, b)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "c")
	return git(t, repo, "rev-parse", "HEAD")
}

func hexOf(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func keysLine(id string, pub ed25519.PublicKey) string {
	return id + " " + base64.StdEncoding.EncodeToString(pub) + "\n"
}

// newRig makes the repository, the trust roots and a published good release.
func newRig(t *testing.T, mode trustMode) *rig {
	t.Helper()
	fx := fixture(t, mode)
	r := &rig{t: t, repo: fx.dir, rev: fx.rev, base: fx.base, inputs: fx.inputs, dir: filepath.Join(t.TempDir(), "release"), arts: map[string][]byte{
		"nself-linux-amd64": []byte("linux binary"), "nself-darwin-arm64": []byte("darwin binary")}}
	priv, other := seedKey(1), seedKey(2)
	r.pub, r.otherPub = priv.Public().(ed25519.PublicKey), other.Public().(ed25519.PublicKey)
	r.signer, r.other = signing.NewEd25519Signer(signing.PurposeCIRelease, priv), signing.NewEd25519Signer(signing.PurposeCIRelease, other)
	line := keysLine(signing.KeyID(signing.PurposeCIRelease, r.pub), r.pub)
	r.opt = verify.Options{Root: r.repo}
	if mode == operatorKeys {
		r.keysFile = filepath.Join(t.TempDir(), "ci-release.keys")
		writeFile(t, filepath.Dir(r.keysFile), "ci-release.keys", line)
		r.opt.OperatorKeys = r.keysFile
	} else {
		r.opt.Pin = r.base
	}
	r.revokedFile = filepath.Join(t.TempDir(), "ci-release.revoked")
	r.opt.OperatorRevoked = r.revokedFile // absent until a test writes it
	r.manSigner, r.provSigner = r.signer, r.signer
	r.manType, r.provType, r.manSigs = manifest.PayloadType, provenance.PayloadType, 1
	r.publish()
	return r
}

func (r *rig) keyID() string { return signing.KeyID(signing.PurposeCIRelease, r.pub) }

func (r *rig) names() []string {
	out := make([]string, 0, len(r.arts))
	for n := range r.arts {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (r *rig) write(rel string, body []byte) {
	r.t.Helper()
	writeFile(r.t, r.dir, rel, string(body))
}

func (r *rig) read(rel string) []byte {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(rel)))
	if err != nil {
		r.t.Fatal(err)
	}
	return b
}

func (r *rig) envelope(s signing.Signer, payloadType string, payload []byte) []byte {
	r.t.Helper()
	env, err := signing.SignEnvelope(s, payloadType, payload)
	if err != nil {
		r.t.Fatal(err)
	}
	b, err := json.Marshal(env)
	if err != nil {
		r.t.Fatal(err)
	}
	return b
}

// publish writes the whole release directory from the rig's current state.
func (r *rig) publish() {
	t := r.t
	t.Helper()
	if err := os.RemoveAll(r.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inputs := r.inputs
	inputs.Lockfiles = append([]model.DigestRef(nil), r.inputs.Lockfiles...)
	inputs.Generated = append([]model.DigestRef(nil), r.inputs.Generated...)
	var err error
	fin := finishedAt
	repoID := "local:" + strings.Repeat("e", 64)
	checks := []model.Check{
		{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "publish", JobID: "release", Kind: "release", Tool: "nself", Required: true, Substantive: true, Result: "pass"},
	}
	ev := model.Evidence{
		Schema: "ci.evidence/v1", Revision: r.rev, RepoID: repoID, RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "release", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"}, InputDigests: inputs,
		Runner:   model.RunnerRef{ID: "runner-7", Provider: "local", Platform: "linux/amd64", Isolation: "container"},
		Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"}, Timings: model.Timings{FinishedAt: &fin},
		ToolVersions: map[string]string{"go": "1.27.2"}, Checks: checks, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Result:    "pass", Counts: model.Counts{Pass: 2, Total: 2},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
	ev.Artifacts = []model.Artifact{}
	for _, n := range r.names() {
		ev.Artifacts = append(ev.Artifacts, model.Artifact{Path: n, SHA256: hexOf(r.arts[n]), Size: int64(len(r.arts[n])), Committed: true})
	}
	if r.tweakEvidence != nil {
		r.tweakEvidence(&ev)
	}
	r.ev = ev
	evDoc, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	r.write("evidence.json", evDoc)

	m := manifest.Manifest{
		Schema: manifest.Schema, RepoID: repoID, Revision: r.rev,
		Release: manifest.Release{RunID: "run-1", Pipeline: "release", JobID: "publish", AttemptID: "run-1.publish-1a2b3c4d.a1",
			IdempotencyKey: strings.Repeat("9", 64), FinishedAt: finishedAt},
		Evidence:   manifest.EvidenceRef{Path: "evidence.json", SHA256: hexOf(evDoc), Result: "pass", Class: "release"},
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, PredicateType: manifest.ProvenancePredicate, SLSABuildLevel: manifest.SLSALevel},
		Checksums:  manifest.FileRef{Path: manifest.ChecksumsPath},
		Signing:    manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: r.keyID()},
		Producer:   manifest.Producer{NselfCIVersion: "1.5.0", Coordinator: "coordinator-1"},
	}
	facts := provenance.ReleaseFacts{
		Visibility: attest.Public, RepoID: m.RepoID, Revision: r.rev, Pipeline: "release", Job: "publish", ConfigDigest: inputs.Config,
		Isolation: "container", NselfCIVersion: "1.5.0", Coordinator: "coordinator-1", Runner: "runner-7",
		RunID: "run-1", AttemptID: m.Release.AttemptID, Evidence: hexOf(evDoc),
		StartedOn: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC), FinishedOn: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
	}
	for _, l := range inputs.Lockfiles {
		facts.Lockfiles = append(facts.Lockfiles, provenance.FileDigest{Name: l.Path, SHA256: l.SHA256})
	}
	for _, n := range r.names() {
		body := r.arts[n]
		r.write("artifacts/"+n, body)
		cdx, spdx := []byte("cdx of "+n), []byte("spdx of "+n)
		if r.skipSBOM != n {
			r.write("sbom/"+n+".cdx.json", cdx)
			r.write("sbom/"+n+".spdx.json", spdx)
		}
		a := manifest.Artifact{Name: n, Path: "artifacts/" + n, Kind: manifest.KindBinary, SHA256: hexOf(body), Size: int64(len(body)),
			SBOM: manifest.SBOM{
				CycloneDX: manifest.FileRef{Path: "sbom/" + n + ".cdx.json", SHA256: hexOf(cdx)},
				SPDX:      manifest.FileRef{Path: "sbom/" + n + ".spdx.json", SHA256: hexOf(spdx)}}}
		m.Artifacts = append(m.Artifacts, a)
		facts.Subjects = append(facts.Subjects, provenance.FileDigest{Name: n, SHA256: a.SHA256})
		facts.SBOMs = append(facts.SBOMs, provenance.FileDigest{Name: a.SBOM.CycloneDX.Path, SHA256: a.SBOM.CycloneDX.SHA256},
			provenance.FileDigest{Name: a.SBOM.SPDX.Path, SHA256: a.SBOM.SPDX.SHA256})
	}
	if r.tweakFacts != nil {
		r.tweakFacts(&facts)
	}
	prov, err := provenance.Build(facts)
	if err != nil {
		t.Fatal(err)
	}
	if r.tweakProvPayload != nil {
		prov = r.tweakProvPayload(prov)
	}
	provDoc := r.envelope(r.provSigner, r.provType, prov)
	r.write(manifest.ProvenancePath, provDoc)
	m.Provenance.SHA256 = hexOf(provDoc)

	sums := map[string]string{"evidence.json": hexOf(evDoc), manifest.ProvenancePath: hexOf(provDoc)}
	for _, a := range m.Artifacts {
		sums[a.Path], sums[a.SBOM.CycloneDX.Path], sums[a.SBOM.SPDX.Path] = a.SHA256, a.SBOM.CycloneDX.SHA256, a.SBOM.SPDX.SHA256
	}
	paths := make([]string, 0, len(sums))
	for p := range sums {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var sb strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&sb, "%s  %s\n", sums[p], p)
	}
	r.write(manifest.ChecksumsPath, []byte(sb.String()))
	m.Checksums.SHA256 = hexOf([]byte(sb.String()))
	if r.tweakManifest != nil {
		r.tweakManifest(&m)
	}
	payload, err := manifest.Build(m)
	if err != nil {
		t.Fatal(err)
	}
	if r.tweakManifestPayload != nil {
		payload = r.tweakManifestPayload(payload)
	}
	env, err := signing.SignEnvelope(r.manSigner, r.manType, payload)
	if err != nil {
		t.Fatal(err)
	}
	if r.manSigs == 2 {
		second, serr := signing.SignEnvelope(r.other, r.manType, payload)
		if serr != nil {
			t.Fatal(serr)
		}
		env.Signatures = append(env.Signatures, second.Signatures...)
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	r.write(verify.ManifestFile, b)
}

func (r *rig) verify() verify.Result {
	r.t.Helper()
	return verify.Release(context.Background(), r.dir, r.opt)
}

// wantFail checks the result failed with reason and exit.
func wantFail(t *testing.T, res verify.Result, reason string, exit int) {
	t.Helper()
	if res.Verified || res.Reason != reason || res.Exit != exit || res.Code != "E719" || res.SLSABuildLevel != "L1" {
		t.Fatalf("want %s exit %d, got verified=%v reason=%q exit=%d code=%q: %s", reason, exit, res.Verified, res.Reason, res.Exit, res.Code, res.Message)
	}
}

func base64decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

// repoFixture is a git repository built once per trust mode and shared
// read-only: git is slow on a loaded machine. A test that changes the
// repository calls ownRepo first.
type repoFixture struct {
	dir, base, rev string
	inputs         model.InputDigests
}

var (
	fixMu    sync.Mutex
	fixtures = map[trustMode]*repoFixture{}
	fixRoot  string
)

func TestMain(m *testing.M) {
	var err error
	if fixRoot, err = os.MkdirTemp("", "verify-fixtures"); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(fixRoot)
	os.Exit(code)
}

func fixture(t *testing.T, mode trustMode) *repoFixture {
	t.Helper()
	fixMu.Lock()
	defer fixMu.Unlock()
	if fx := fixtures[mode]; fx != nil {
		return fx
	}
	dir := filepath.Join(fixRoot, fmt.Sprintf("repo-%d", mode))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pub := seedKey(1).Public().(ed25519.PublicKey)
	line := keysLine(signing.KeyID(signing.PurposeCIRelease, pub), pub)
	git(t, dir, "init", "-q", "-b", "main", "--template=")
	base := map[string]string{"go.mod": "module x\n\ngo 1.22\n", "go.sum": "h1:one\n", "a.txt": "a\n"}
	if mode == pinnedKeys {
		base[".nself/trust/ci-release.keys"] = line
	}
	fx := &repoFixture{dir: dir, base: commit(t, dir, base)}
	rel := map[string]string{"a.txt": "released\n"}
	if mode == releasedOnly {
		rel[".nself/trust/ci-release.keys"] = line
	}
	fx.rev = commit(t, dir, rel)
	inputs, err := evidence.InputsAt(context.Background(), nil, dir, fx.rev)
	if err != nil {
		t.Fatal(err)
	}
	fx.inputs = inputs
	fixtures[mode] = fx
	return fx
}

// ownRepo gives the rig a private copy of the repository it may change.
func (r *rig) ownRepo() {
	r.t.Helper()
	dst := r.t.TempDir()
	err := filepath.WalkDir(r.repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(r.repo, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		r.t.Fatal(err)
	}
	r.repo, r.opt.Root = dst, dst
}
