//go:build cachesec

package cachesec

// Purpose: a published release directory for group G4 (P7-CACHE-11), rebuilt from the shape of
// internal/release/verify/rig_test.go: a real git repository, a trust file holding the ci-release public key,
// evidence.json, SBOMs, a signed provenance statement, SHA256SUMS and a DSSE-signed manifest, all through the
// production builders (manifest.Build, provenance.Build, sdk SignEnvelope) and judged by verify.Release.
// Inputs:  a seed byte (the honest ci-release key), optional manifest signer override, the run id of the evidence.
// Outputs: g4Release (dir, options), Verify() returning verify.Result.
// Constraints: every key and digest is synthetic; git runs only inside t.TempDir().

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
)

const g4Finished = "2026-10-19T12:00:00Z"

// g4Release is one release directory and the options that verify it.
type g4Release struct {
	t        *testing.T
	Dir      string
	Opt      verify.Options
	good     sdk.Signer
	pub      ed25519.PublicKey
	manSign  sdk.Signer // signs the manifest envelope (the honest key unless a case swaps it)
	keyLine  string
	repo     string
	rev      string
	repoID   string
	inputs   model.InputDigests
	arts     map[string][]byte
	runID    string
	rewriteK bool // rewrite the key id inside the manifest payload to the signer's (wrong-purpose cases)
}

func g4Hex(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func g4Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func g4Write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newG4Release publishes a good release of runID signed by the ci-release key of the seed.
func newG4Release(t *testing.T, runID string, swap func(r *g4Release)) *g4Release {
	t.Helper()
	r := &g4Release{t: t, runID: runID, arts: map[string][]byte{"nself-linux-amd64": []byte("linux binary"), "nself-darwin-arm64": []byte("darwin binary")}}
	r.repo = t.TempDir()
	g4Git(t, r.repo, "init", "-q", "-b", "main", "--template=")
	g4Write(t, r.repo, "go.mod", "module x\n\ngo 1.22\n")
	g4Write(t, r.repo, "go.sum", "h1:one\n")
	g4Git(t, r.repo, "add", "-A")
	g4Git(t, r.repo, "commit", "-q", "-m", "c")
	r.rev = g4Git(t, r.repo, "rev-parse", "HEAD")
	var err error
	if r.inputs, err = evidence.InputsAt(context.Background(), nil, r.repo, r.rev); err != nil {
		t.Fatal(err)
	}
	id, ok := evidence.RepoID(context.Background(), nil, r.repo)
	if !ok {
		t.Fatal("no repository id")
	}
	r.repoID = id
	priv := ed25519.NewKeyFromSeed([]byte(strings.Repeat("b", 32)))
	pub := priv.Public().(ed25519.PublicKey)
	r.pub = pub
	r.good = sdk.NewEd25519Signer(sdk.PurposeCIRelease, priv)
	r.manSign = r.good
	r.keyLine = sdk.KeyID(sdk.PurposeCIRelease, pub) + " " + base64.StdEncoding.EncodeToString(pub) + "\n"
	keys := filepath.Join(t.TempDir(), "ci-release.keys")
	g4Write(t, filepath.Dir(keys), "ci-release.keys", r.keyLine)
	r.Opt = verify.Options{Root: r.repo, OperatorKeys: keys, OperatorRevoked: filepath.Join(t.TempDir(), "ci-release.revoked"),
		PolicyDigest: "policy-1", RequiredJobs: []string{"build"}}
	r.Dir = filepath.Join(t.TempDir(), "release")
	if swap != nil {
		swap(r)
	}
	r.publish()
	return r
}

func (r *g4Release) envelope(s sdk.Signer, payloadType string, payload []byte) []byte {
	r.t.Helper()
	env, err := sdk.SignEnvelope(s, payloadType, payload)
	if err != nil {
		r.t.Fatal(err)
	}
	b, err := json.Marshal(env)
	if err != nil {
		r.t.Fatal(err)
	}
	return b
}

func (r *g4Release) names() []string {
	out := make([]string, 0, len(r.arts))
	for n := range r.arts {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// publish writes the whole directory from the rig's state.
func (r *g4Release) publish() {
	t := r.t
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fin := g4Finished
	ev := model.Evidence{Schema: "ci.evidence/v1", Revision: r.rev, RepoID: r.repoID, RunID: r.runID,
		Pipeline: model.PipelineRef{ID: "p1", Name: "release", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"}, InputDigests: r.inputs,
		Runner:   model.RunnerRef{ID: "runner-7", Provider: "local", Platform: "linux/amd64", Isolation: "container"},
		Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"}, Timings: model.Timings{FinishedAt: &fin},
		ToolVersions: map[string]string{"go": "1.27.2"}, Retries: []model.Retry{},
		Checks: []model.Check{{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
			{ID: "publish", JobID: "publish", Kind: "release", Tool: "nself", Required: true, Substantive: true, Result: "pass"}},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Result:    "pass", Counts: model.Counts{Pass: 2, Total: 2},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"}, Artifacts: []model.Artifact{}}
	for _, n := range r.names() {
		ev.Artifacts = append(ev.Artifacts, model.Artifact{Path: n, SHA256: g4Hex(r.arts[n]), Size: int64(len(r.arts[n])), Committed: true})
	}
	evDoc, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	g4Write(t, r.Dir, "evidence.json", string(evDoc))
	keyID := sdk.KeyID(sdk.PurposeCIRelease, r.pub)
	m := manifest.Manifest{Schema: manifest.Schema, RepoID: r.repoID, Revision: r.rev,
		Release: manifest.Release{RunID: r.runID, Pipeline: "release", JobID: "publish", AttemptID: r.runID + ".publish-1a2b3c4d.a1",
			IdempotencyKey: strings.Repeat("9", 64), FinishedAt: g4Finished},
		Evidence:   manifest.EvidenceRef{Path: "evidence.json", SHA256: g4Hex(evDoc), Result: "pass", Class: "release"},
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, PredicateType: manifest.ProvenancePredicate, SLSABuildLevel: manifest.SLSALevel},
		Checksums:  manifest.FileRef{Path: manifest.ChecksumsPath},
		Signing:    manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: keyID},
		Producer:   manifest.Producer{NselfCIVersion: "1.5.0", Coordinator: "coordinator-1"}}
	facts := provenance.ReleaseFacts{Visibility: attest.Public, RepoID: m.RepoID, Revision: r.rev, Pipeline: "release", Job: "publish", ConfigDigest: r.inputs.Config,
		Isolation: "container", NselfCIVersion: "1.5.0", Coordinator: "coordinator-1", Runner: "runner-7", RunID: r.runID, AttemptID: m.Release.AttemptID, Evidence: g4Hex(evDoc),
		StartedOn: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC), FinishedOn: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	for _, l := range r.inputs.Lockfiles {
		facts.Lockfiles = append(facts.Lockfiles, provenance.FileDigest{Name: l.Path, SHA256: l.SHA256})
	}
	sums := map[string]string{"evidence.json": g4Hex(evDoc)}
	for _, n := range r.names() {
		body := r.arts[n]
		cdx, spdx := []byte("cdx of "+n), []byte("spdx of "+n)
		g4Write(t, r.Dir, "artifacts/"+n, string(body))
		g4Write(t, r.Dir, "sbom/"+n+".cdx.json", string(cdx))
		g4Write(t, r.Dir, "sbom/"+n+".spdx.json", string(spdx))
		a := manifest.Artifact{Name: n, Path: "artifacts/" + n, Kind: manifest.KindBinary, SHA256: g4Hex(body), Size: int64(len(body)),
			SBOM: manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/" + n + ".cdx.json", SHA256: g4Hex(cdx)}, SPDX: manifest.FileRef{Path: "sbom/" + n + ".spdx.json", SHA256: g4Hex(spdx)}}}
		m.Artifacts = append(m.Artifacts, a)
		facts.Subjects = append(facts.Subjects, provenance.FileDigest{Name: n, SHA256: a.SHA256})
		facts.SBOMs = append(facts.SBOMs, provenance.FileDigest{Name: a.SBOM.CycloneDX.Path, SHA256: a.SBOM.CycloneDX.SHA256}, provenance.FileDigest{Name: a.SBOM.SPDX.Path, SHA256: a.SBOM.SPDX.SHA256})
		sums[a.Path], sums[a.SBOM.CycloneDX.Path], sums[a.SBOM.SPDX.Path] = a.SHA256, a.SBOM.CycloneDX.SHA256, a.SBOM.SPDX.SHA256
	}
	prov, err := provenance.Build(facts)
	if err != nil {
		t.Fatal(err)
	}
	provDoc := r.envelope(r.good, provenance.PayloadType, prov)
	g4Write(t, r.Dir, manifest.ProvenancePath, string(provDoc))
	m.Provenance.SHA256, sums[manifest.ProvenancePath] = g4Hex(provDoc), g4Hex(provDoc)
	paths := make([]string, 0, len(sums))
	for p := range sums {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var sb strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&sb, "%s  %s\n", sums[p], p)
	}
	g4Write(t, r.Dir, manifest.ChecksumsPath, sb.String())
	m.Checksums.SHA256 = g4Hex([]byte(sb.String()))
	payload, err := manifest.Build(m)
	if err != nil {
		t.Fatal(err)
	}
	if r.rewriteK { // a re-signed manifest: the key id inside names the foreign key too
		payload = []byte(strings.Replace(string(payload), keyID, r.manSign.KeyID(), 1))
	}
	g4Write(t, r.Dir, verify.ManifestFile, string(r.envelope(r.manSign, manifest.PayloadType, payload)))
}

// Verify judges the directory with the rig's options.
func (r *g4Release) Verify() verify.Result {
	r.t.Helper()
	return verify.Release(context.Background(), r.Dir, r.Opt)
}
