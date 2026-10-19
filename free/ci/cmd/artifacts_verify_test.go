package main

// Purpose: tests of `nself ci artifacts verify`: the exit-code mapping and the
// tamper matrix, run through the real command on a signed release directory
// built here (a small copy of the verify package's fixture, because that one
// lives in another test package). Every key and digest is synthetic.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

type verifyRig struct {
	t            *testing.T
	dir, repo    string
	rev, base    string
	home         string
	pub          ed25519.PublicKey
	signer       signing.Signer
	artifact     []byte
	skipSBOM     bool
	schema       string // overrides the manifest schema in the signed payload
	badSubject   bool
	evidenceRev  string
	selection    string
	lockfileBent bool
	inputs       model.InputDigests
	repoID       string
}

// verifyTemplate is a two-commit repository built once per top-level test and
// copied into each rig: git is slow on a loaded machine.
type verifyTemplate struct {
	dir, base, rev string
	repoID         string
	inputs         model.InputDigests
}

func buildVerifyTemplate(t *testing.T) *verifyTemplate {
	t.Helper()
	dir := t.TempDir()
	vgit(t, dir, "init", "-q", "-b", "main", "--template=")
	for n, b := range map[string]string{"go.mod": "module x\n\ngo 1.22\n", "go.sum": "h1:one\n", "a.txt": "a\n"} {
		vput(t, dir, n, b)
	}
	vgit(t, dir, "add", "-A")
	vgit(t, dir, "commit", "-q", "-m", "base")
	tp := &verifyTemplate{dir: dir, base: vgit(t, dir, "rev-parse", "HEAD")}
	vput(t, dir, "a.txt", "released\n")
	vgit(t, dir, "add", "-A")
	vgit(t, dir, "commit", "-q", "-m", "release")
	tp.rev = vgit(t, dir, "rev-parse", "HEAD")
	inputs, err := evidence.InputsAt(context.Background(), nil, dir, tp.rev)
	if err != nil {
		t.Fatal(err)
	}
	tp.inputs = inputs
	id, ok := evidence.RepoID(context.Background(), nil, dir)
	if !ok {
		t.Fatal("no repository id")
	}
	tp.repoID = id
	return tp
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
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
		t.Fatal(err)
	}
}

func vgit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func vput(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func vsum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func newVerifyRig(t *testing.T, tp *verifyTemplate) *verifyRig {
	t.Helper()
	r := &verifyRig{t: t, repo: t.TempDir(), dir: filepath.Join(t.TempDir(), "release"), home: t.TempDir(), artifact: []byte("binary"),
		base: tp.base, rev: tp.rev, inputs: tp.inputs, repoID: tp.repoID}
	copyTree(t, tp.dir, r.repo)
	priv := ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))
	r.pub = priv.Public().(ed25519.PublicKey)
	r.signer = signing.NewEd25519Signer(signing.PurposeCIRelease, priv)
	vput(t, r.home, "trust/ci-release.keys", r.signer.KeyID()+" "+base64.StdEncoding.EncodeToString(r.pub)+"\n")
	return r
}

func (r *verifyRig) write(rel, body string) { vput(r.t, r.dir, rel, body) }

func (r *verifyRig) read(rel string) string {
	b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(rel)))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *verifyRig) seal(s signing.Signer, payloadType string, payload []byte) string {
	env, err := signing.SignEnvelope(s, payloadType, payload)
	if err != nil {
		r.t.Fatal(err)
	}
	b, _ := json.Marshal(env)
	return string(b)
}

// publish writes a release directory; the fields of r bend it.
func (r *verifyRig) publish() {
	t := r.t
	_ = os.RemoveAll(r.dir)
	inputs := r.inputs
	inputs.Lockfiles = append([]model.DigestRef(nil), r.inputs.Lockfiles...)
	var err error
	fin := "2026-10-19T12:00:00Z"
	rev := r.rev
	if r.evidenceRev != "" {
		rev = r.evidenceRev
	}
	mode := model.SelectionMode("full")
	if r.selection != "" {
		mode = model.SelectionMode(r.selection)
	}
	ev := model.Evidence{
		Schema: "ci.evidence/v1", Revision: rev, RepoID: r.repoID, RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "release", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"}, InputDigests: inputs,
		Runner:   model.RunnerRef{ID: "runner-7", Provider: "local", Platform: "linux/amd64", Isolation: "container"},
		Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"}, Timings: model.Timings{FinishedAt: &fin},
		ToolVersions: map[string]string{"go": "1.27.2"}, Retries: []model.Retry{},
		Checks:    []model.Check{{ID: "publish", JobID: "publish", Kind: "release", Tool: "nself", Required: true, Substantive: true, Result: "pass"}},
		Selection: model.Selection{Mode: mode, Reason: "selection.full_required", Digest: "d"}, Result: "pass",
		Counts: model.Counts{Pass: 1, Total: 1}, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
		Artifacts: []model.Artifact{{Path: "nself-linux-amd64", SHA256: vsum(r.artifact), Size: int64(len(r.artifact)), Committed: true}},
	}
	if r.lockfileBent {
		ev.InputDigests.Lockfiles[0].SHA256 = strings.Repeat("1", 64)
	}
	evDoc, _ := json.Marshal(ev)
	r.write("evidence.json", string(evDoc))
	r.write("artifacts/nself-linux-amd64", string(r.artifact))
	cdx, spdx := "cdx", "spdx"
	if !r.skipSBOM {
		r.write("sbom/nself-linux-amd64.cdx.json", cdx)
		r.write("sbom/nself-linux-amd64.spdx.json", spdx)
	}
	subject := vsum(r.artifact)
	if r.badSubject {
		subject = strings.Repeat("1", 64)
	}
	facts := provenance.ReleaseFacts{
		Visibility: attest.Public, RepoID: ev.RepoID, Revision: r.rev, Pipeline: "release", Job: "publish", ConfigDigest: inputs.Config,
		Isolation: "container", NselfCIVersion: "1.5.0", Coordinator: "coordinator-1", Runner: "runner-7", RunID: "run-1",
		AttemptID: "run-1.publish-1a2b3c4d.a1", Evidence: vsum(evDoc),
		StartedOn: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC), FinishedOn: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		Subjects: []provenance.FileDigest{{Name: "nself-linux-amd64", SHA256: subject}},
		SBOMs:    []provenance.FileDigest{{Name: "sbom/nself-linux-amd64.cdx.json", SHA256: vsum([]byte(cdx))}, {Name: "sbom/nself-linux-amd64.spdx.json", SHA256: vsum([]byte(spdx))}},
	}
	for _, l := range inputs.Lockfiles {
		facts.Lockfiles = append(facts.Lockfiles, provenance.FileDigest{Name: l.Path, SHA256: l.SHA256})
	}
	stmt, err := provenance.Build(facts)
	if err != nil {
		t.Fatal(err)
	}
	provDoc := r.seal(r.signer, provenance.PayloadType, stmt)
	r.write(manifest.ProvenancePath, provDoc)
	sums := fmt.Sprintf("%s  artifacts/nself-linux-amd64\n%s  evidence.json\n%s  %s\n%s  sbom/nself-linux-amd64.cdx.json\n%s  sbom/nself-linux-amd64.spdx.json\n",
		vsum(r.artifact), vsum(evDoc), vsum([]byte(provDoc)), manifest.ProvenancePath, vsum([]byte(cdx)), vsum([]byte(spdx)))
	r.write("SHA256SUMS", sums)
	m := manifest.Manifest{Schema: manifest.Schema, RepoID: ev.RepoID, Revision: r.rev,
		Release:  manifest.Release{RunID: "run-1", Pipeline: "release", JobID: "publish", AttemptID: facts.AttemptID, IdempotencyKey: strings.Repeat("9", 64), FinishedAt: fin},
		Evidence: manifest.EvidenceRef{Path: "evidence.json", SHA256: vsum(evDoc), Result: "pass", Class: "release"},
		Artifacts: []manifest.Artifact{{Name: "nself-linux-amd64", Path: "artifacts/nself-linux-amd64", Kind: manifest.KindBinary, SHA256: vsum(r.artifact), Size: int64(len(r.artifact)),
			SBOM: manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/nself-linux-amd64.cdx.json", SHA256: vsum([]byte(cdx))}, SPDX: manifest.FileRef{Path: "sbom/nself-linux-amd64.spdx.json", SHA256: vsum([]byte(spdx))}}}},
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, SHA256: vsum([]byte(provDoc)), PredicateType: manifest.ProvenancePredicate, SLSABuildLevel: manifest.SLSALevel},
		Checksums:  manifest.FileRef{Path: "SHA256SUMS", SHA256: vsum([]byte(sums))},
		Signing:    manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: r.signer.KeyID()},
		Producer:   manifest.Producer{NselfCIVersion: "1.5.0", Coordinator: "coordinator-1"}}
	payload, err := manifest.Build(m)
	if err != nil {
		t.Fatal(err)
	}
	if r.schema != "" {
		payload = []byte(strings.Replace(string(payload), manifest.Schema, r.schema, 1))
	}
	r.write(verify.ManifestFile, r.seal(r.signer, manifest.PayloadType, payload))
}

// run executes the command with the rig's environment and returns exit, stdout, stderr.
func (r *verifyRig) run(args ...string) (int, string, string) {
	r.t.Helper()
	r.t.Setenv("NSELF_V15", "1")
	r.t.Setenv("NSELF_CI_HOME", r.home)
	r.t.Setenv("NSELF_CI_REPO", r.repo)
	old := releasePin
	releasePin = func(context.Context, string) (string, error) { return "", nil }
	r.t.Cleanup(func() { releasePin = old })
	return runArt(r.t, append([]string{"verify"}, args...)...)
}

func verifyDataOf(t *testing.T, out string) map[string]any {
	t.Helper()
	data, _ := checkEnvelope(t, out)["data"].(map[string]any)
	if data == nil {
		t.Fatalf("no data in %s", out)
	}
	return data
}

func TestArtifactsVerifyExitCodes(t *testing.T) {
	r := newVerifyRig(t, buildVerifyTemplate(t))
	r.publish()
	code, out, errs := r.run(r.dir)
	if code != 0 || strings.TrimSpace(errs) != "note: quarantine state was not checked: no quarantine source is configured" || !strings.Contains(out, "release verified (SLSA Build L1)") || !strings.Contains(out, r.rev[:12]) {
		t.Fatalf("good release: %d\n%s\n%s", code, out, errs)
	}
	code, out, _ = r.run(r.dir, "--json")
	data := verifyDataOf(t, out)
	if code != 0 || data["verified"] != true || data["slsa_build_level"] != "L1" || data["exit_code"] != float64(0) || data["artifacts"] != float64(1) {
		t.Fatalf("good --json: %d %v", code, data)
	}
	if code, _, _ = r.run(filepath.Join(r.dir, verify.ManifestFile)); code != 0 {
		t.Fatalf("manifest path input: %d", code)
	}
	oldDir := releaseDirOf
	releaseDirOf = func(_ context.Context, run string) (string, bool, error) { return r.dir, run == "run-1", nil }
	t.Cleanup(func() { releaseDirOf = oldDir })
	if code, _, _ = r.run("run-1"); code != 0 {
		t.Fatalf("run input: %d", code)
	}
	code, out, _ = r.run("run-2", "--json")
	if data = verifyDataOf(t, out); code != 11 || data["reason"] != verify.ReasonReleaseMissing || data["slsa_build_level"] != "L1" {
		t.Fatalf("unknown run: %d %v", code, data)
	}
	releaseDirOf = func(context.Context, string) (string, bool, error) { return "", false, errors.New("store offline") }
	if code, _, errs = r.run("run-1"); code != 2 || !strings.Contains(errs, "store offline") {
		t.Fatalf("lookup failure: %d %s", code, errs)
	}
	releaseDirOf = nil
	if code, _, _ = r.run(filepath.Join(r.dir, "evidence.json")); code != 2 {
		t.Fatalf("a file that is not the manifest: %d", code)
	}
	if code, _, errs = r.run(); code != 1 || !strings.Contains(errs, "usage: nself ci artifacts verify") {
		t.Fatalf("no input: %d %s", code, errs)
	}
	if code, _, _ = r.run(r.dir, "extra"); code != 1 {
		t.Fatalf("two inputs: %d", code)
	}
	if code, _, _ = r.run(r.dir, "--nope"); code != 1 {
		t.Fatalf("bad flag: %d", code)
	}
	// 11: a swapped artifact; the text form says why on stderr.
	r.write("artifacts/nself-linux-amd64", "evil")
	code, out, errs = r.run(r.dir)
	if code != 11 || out != "" || !strings.Contains(errs, "release NOT verified: E719 checksum") {
		t.Fatalf("swapped artifact: %d\n%s\n%s", code, out, errs)
	}
	// 1: evidence that does not prove a full pass, and quarantined evidence.
	r.selection = "affected"
	r.publish()
	if code, out, _ = r.run(r.dir, "--json"); code != 1 || verifyDataOf(t, out)["reason"] != verify.ReasonEvidenceFailing {
		t.Fatalf("failing evidence: %d %s", code, out)
	}
	r.selection = ""
	r.publish()
	oldQ := releaseQuarantined
	releaseQuarantined = func(context.Context, model.Evidence) (bool, error) { return true, nil }
	t.Cleanup(func() { releaseQuarantined = oldQ })
	if code, out, _ = r.run(r.dir, "--json"); code != 1 || verifyDataOf(t, out)["reason"] != verify.ReasonQuarantined {
		t.Fatalf("quarantined evidence: %d %s", code, out)
	}
}

func TestArtifactsVerifyKeysFlagAndPin(t *testing.T) {
	r := newVerifyRig(t, buildVerifyTemplate(t))
	r.publish()
	keys := filepath.Join(t.TempDir(), "mine.keys")
	vput(t, filepath.Dir(keys), "mine.keys", r.signer.KeyID()+" "+base64.StdEncoding.EncodeToString(r.pub)+"\n")
	if err := os.Remove(filepath.Join(r.home, "trust", "ci-release.keys")); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := r.run(r.dir, "--json"); code != 11 || verifyDataOf(t, out)["reason"] != verify.ReasonSignature {
		t.Fatalf("no trust roots: %d %s", code, out)
	}
	if code, _, errs := r.run(r.dir, "--keys", keys); code != 0 {
		t.Fatalf("--keys: %d %s", code, errs)
	}
	other := ed25519.NewKeyFromSeed([]byte("fedcba9876543210fedcba9876543210"))
	vput(t, filepath.Dir(keys), "mine.keys", signing.KeyID(signing.PurposeCIRelease, other.Public().(ed25519.PublicKey))+" "+base64.StdEncoding.EncodeToString(other.Public().(ed25519.PublicKey))+"\n")
	if code, _, _ := r.run(r.dir, "--keys", keys); code != 11 {
		t.Fatalf("--keys with another key: %d", code)
	}
	// A pin that cannot be resolved fails the verification: the branch may hold a revocation.
	vput(t, r.home, "trust/ci-release.keys", r.signer.KeyID()+" "+base64.StdEncoding.EncodeToString(r.pub)+"\n")
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_HOME", r.home)
	t.Setenv("NSELF_CI_REPO", r.repo)
	old := releasePin
	releasePin = func(context.Context, string) (string, error) { return "", errors.New("origin unreachable") }
	t.Cleanup(func() { releasePin = old })
	code, out, _ := runArt(t, "verify", r.dir, "--json")
	if data := verifyDataOf(t, out); code != 11 || data["reason"] != verify.ReasonSignature || !strings.Contains(fmt.Sprint(data["message"]), "origin unreachable") {
		t.Fatalf("unresolved pin must fail closed: %d %s", code, out)
	}
	// A key only the pinned default branch lists is trusted.
	vput(t, r.repo, ".nself/trust/ci-release.keys", r.signer.KeyID()+" "+base64.StdEncoding.EncodeToString(r.pub)+"\n")
	vgit(t, r.repo, "add", "-A")
	vgit(t, r.repo, "commit", "-q", "-m", "trust")
	tip := vgit(t, r.repo, "rev-parse", "HEAD")
	releasePin = func(context.Context, string) (string, error) { return tip, nil }
	if code, _, errs := runArt(t, "verify", r.dir); code != 0 {
		t.Fatalf("pinned key: %d %s", code, errs)
	}
}

func TestArtifactsVerifyTamperMatrix(t *testing.T) {
	tp := buildVerifyTemplate(t)
	cases := []struct {
		name   string
		bend   func(r *verifyRig)
		after  func(r *verifyRig)
		reason string
		exit   int
	}{
		{name: "swapped artifact", after: func(r *verifyRig) { r.write("artifacts/nself-linux-amd64", "evil") }, reason: verify.ReasonChecksum},
		{name: "edited SHA256SUMS", after: func(r *verifyRig) {
			r.write("SHA256SUMS", strings.Replace(r.read("SHA256SUMS"), "  evidence.json", "  evidence.jsoN", 1))
		}, reason: verify.ReasonChecksum},
		{name: "unlisted file", after: func(r *verifyRig) { r.write("artifacts/extra", "x") }, reason: verify.ReasonChecksum},
		{name: "plugins-purpose key", after: func(r *verifyRig) {
			s := signing.NewEd25519Signer(signing.PurposePlugins, ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef")))
			var env signing.Envelope
			if err := json.Unmarshal([]byte(r.read(verify.ManifestFile)), &env); err != nil {
				t.Fatal(err)
			}
			payload, _ := base64.StdEncoding.DecodeString(env.Payload)
			r.write(verify.ManifestFile, r.seal(s, manifest.PayloadType, payload))
		}, reason: verify.ReasonKeyPurpose},
		{name: "revoked key", after: func(r *verifyRig) { vput(t, r.home, "trust/ci-release.revoked", r.signer.KeyID()+"\n") }, reason: verify.ReasonRevoked},
		{name: "missing sbom", bend: func(r *verifyRig) { r.skipSBOM = true }, reason: verify.ReasonSBOMLink},
		{name: "provenance for another digest", bend: func(r *verifyRig) { r.badSubject = true }, reason: verify.ReasonProvenanceSubject},
		{name: "evidence of another revision", bend: func(r *verifyRig) { r.evidenceRev = r.base }, reason: verify.ReasonRevision},
		{name: "unknown major", bend: func(r *verifyRig) { r.schema = "ci.release-manifest/v2" }, reason: verify.ReasonManifestSchema},
		{name: "stale evidence", bend: func(r *verifyRig) { r.lockfileBent = true }, reason: verify.ReasonEvidenceStale},
		{name: "failing evidence", bend: func(r *verifyRig) { r.selection = "affected" }, reason: verify.ReasonEvidenceFailing, exit: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newVerifyRig(t, tp)
			if c.bend != nil {
				c.bend(r)
			}
			r.publish()
			if c.after != nil {
				c.after(r)
			}
			want := c.exit
			if want == 0 {
				want = 11
			}
			code, out, _ := r.run(r.dir, "--json")
			data := verifyDataOf(t, out)
			if code != want || data["reason"] != c.reason || data["verified"] != false || data["code"] != "E719" || data["slsa_build_level"] != "L1" {
				t.Fatalf("want %s/%d, got %d %v", c.reason, want, code, data)
			}
		})
	}
}

func TestArtifactsVerifyKeysFileMustExist(t *testing.T) {
	r := newVerifyRig(t, buildVerifyTemplate(t))
	r.publish()
	code, _, errs := r.run(r.dir, "--keys", filepath.Join(t.TempDir(), "missing.keys"))
	if code != 2 || !strings.Contains(errs, "missing.keys") {
		t.Fatalf("a missing --keys file: %d %s", code, errs)
	}
}

// An origin whose default branch cannot be read must not read as "no branch":
// the branch may hold a revocation.
func TestResolveReleasePinFailsClosedWhenOriginIsUnreadable(t *testing.T) {
	tp := buildVerifyTemplate(t)
	repo := t.TempDir()
	copyTree(t, tp.dir, repo)
	if pin, err := resolveReleasePin(context.Background(), repo); err != nil || pin != "" {
		t.Fatalf("no origin: want no pin and no error, got %q %v", pin, err)
	}
	vgit(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "no-such-remote"))
	pin, err := resolveReleasePin(context.Background(), repo)
	if err == nil || pin != "" || !strings.Contains(err.Error(), "could not be fetched or read") {
		t.Fatalf("unreadable origin: %q %v", pin, err)
	}
}
