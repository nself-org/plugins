//go:build cachesec

package cachesec

// Purpose: the signer fixture of group G4 (P7-CACHE-11), rebuilt from internal/signing/fixtures_test.go: an
// in-memory run store and blob store holding one finished release run, a counting secret resolver that holds a
// freshly generated release key, and a manifest payload builder that matches the stored facts.
// Inputs: none. Outputs: g4SignerFx (handler, manifest builder, request helpers).
// Constraints: no process, database or network; every digest is the real sha256 of the fixture bytes; the key is
//          generated per test by signing.Generate (a throwaway).

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	g4SigRun  = "run-sig-1"
	g4SigProj = "proj"
	g4SigRev  = "cccccccccccccccccccccccccccccccccccccccc"
)

type g4SigSrc struct {
	pipe     store.PipelineRow
	jobs     []store.RunJob
	attempts []store.AttemptView
	evidence []store.EvidenceView
	recs     []records.Record
}

func (f *g4SigSrc) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	return f.pipe, id == f.pipe.ID, nil
}
func (f *g4SigSrc) PipelineJobs(context.Context, string) ([]store.RunJob, error) { return f.jobs, nil }
func (f *g4SigSrc) PipelineAttempts(context.Context, string) ([]store.AttemptView, error) {
	return f.attempts, nil
}
func (f *g4SigSrc) EvidenceByRevision(context.Context, string, string) ([]store.EvidenceView, error) {
	return f.evidence, nil
}
func (f *g4SigSrc) ArtifactList(_ context.Context, flt records.Filter) ([]records.Record, error) {
	var out []records.Record
	for _, r := range f.recs {
		if r.Project == flt.Project && r.Run == flt.Run && (flt.Name == "" || r.Name == flt.Name) && (flt.Kind == "" || r.Kind == flt.Kind) {
			out = append(out, r)
		}
	}
	return out, nil
}

type g4SigBlobs struct{ data map[string][]byte }

type g4SigInfo struct {
	os.FileInfo
	n int64
}

func (i g4SigInfo) Size() int64 { return i.n }

func (b *g4SigBlobs) Stat(_ cas.Realm, d string) (os.FileInfo, error) {
	v, ok := b.data[d]
	if !ok {
		return nil, os.ErrNotExist
	}
	return g4SigInfo{n: int64(len(v))}, nil
}
func (b *g4SigBlobs) Open(_ cas.Realm, d string) (io.ReadCloser, error) {
	v, ok := b.data[d]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(string(v))), nil
}

// g4SigRes is the secret resolver: it holds the release key and counts every resolution.
type g4SigRes struct {
	vals  map[string][]byte
	calls int
}

func (r *g4SigRes) Probe(context.Context) error { return nil }
func (r *g4SigRes) Resolve(_ context.Context, _ string, refs []secrets.Ref) ([]secrets.Value, error) {
	r.calls++
	var out []secrets.Value
	for _, ref := range refs {
		v, ok := r.vals[ref.Raw]
		if !ok {
			return nil, &secrets.Error{Code: "E676", Message: "not found", Cause: "E541"}
		}
		out = append(out, secrets.Value{Ref: ref, Bytes: append([]byte(nil), v...)})
	}
	return out, nil
}

type g4SignerFx struct {
	t     *testing.T
	src   *g4SigSrc
	blobs *g4SigBlobs
	res   *g4SigRes
	key   *signing.Key
	files map[string][]byte
	evRaw []byte
}

func newG4SignerFx(t *testing.T) *g4SignerFx {
	t.Helper()
	key, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	f := &g4SignerFx{t: t, key: key, blobs: &g4SigBlobs{data: map[string][]byte{}}, res: &g4SigRes{vals: map[string][]byte{signing.KeyRef: value}},
		files: map[string][]byte{"app": []byte("app-bytes"), "app.cdx.json": []byte("cdx-1"), "app.spdx.json": []byte("spdx-1")}}
	finished := "2026-10-19T12:00:00Z"
	f.evRaw, _ = json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: g4SigRev, RepoID: "repo-1", RunID: g4SigRun, Result: "pass",
		Pipeline: model.PipelineRef{ID: g4SigRun, Name: "release"}, Timings: model.Timings{FinishedAt: &finished}})
	f.src = &g4SigSrc{pipe: store.PipelineRow{ID: g4SigRun, Project: g4SigProj, Revision: g4SigRev, InputDigest: "in1"},
		jobs:     []store.RunJob{{ID: "job-test", Name: "test", Kind: "check"}, {ID: "job-rel", Name: "publish", Kind: "release"}},
		attempts: []store.AttemptView{{ID: "att-test", JobID: "job-test", N: 1, State: "passed"}, {ID: "att-rel", JobID: "job-rel", N: 1, State: "passed"}},
		evidence: []store.EvidenceView{{ID: "e-run", PipelineID: g4SigRun, Body: f.evRaw}}}
	for name, b := range f.files {
		d := g4Hex(b)
		f.blobs.data[d] = b
		kind := records.KindBinary
		if strings.Contains(name, ".") {
			kind = records.KindSBOM
		}
		f.src.recs = append(f.src.recs, records.Record{ID: "r-" + name, Project: g4SigProj, Run: g4SigRun, Attempt: "att-rel", Revision: g4SigRev, Name: name, Kind: kind,
			SHA256: d, Size: int64(len(b)), State: records.StateCommitted, CreatedAt: time.Unix(1, 0)})
	}
	return f
}

func (f *g4SignerFx) handler() signing.Handler {
	return signing.Handler{Src: f.src, Blobs: f.blobs, Project: g4SigProj, Resolver: f.res}
}

// payload is a manifest every check accepts; tweak edits it first.
func (f *g4SignerFx) payload(tweak func(*manifest.Manifest)) []byte {
	f.t.Helper()
	b := f.files["app"]
	m := manifest.Manifest{Schema: manifest.Schema, RepoID: "repo-1", Revision: g4SigRev,
		Release:  manifest.Release{RunID: g4SigRun, Pipeline: "release", JobID: "job-rel", AttemptID: "att-rel", IdempotencyKey: strings.Repeat("9", 64), FinishedAt: "2026-10-19T12:00:00Z"},
		Evidence: manifest.EvidenceRef{Path: manifest.EvidencePath, SHA256: g4Hex(f.evRaw), Result: manifest.EvidenceResult, Class: manifest.EvidenceClass},
		Artifacts: []manifest.Artifact{{Name: "app", Path: "artifacts/app", Kind: manifest.KindBinary, SHA256: g4Hex(b), Size: int64(len(b)),
			SBOM: manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/app.cdx.json", SHA256: g4Hex(f.files["app.cdx.json"])}, SPDX: manifest.FileRef{Path: "sbom/app.spdx.json", SHA256: g4Hex(f.files["app.spdx.json"])}}}},
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, SHA256: strings.Repeat("a", 64), PredicateType: manifest.ProvenancePredicate, SLSABuildLevel: manifest.SLSALevel},
		Checksums:  manifest.FileRef{Path: manifest.ChecksumsPath, SHA256: strings.Repeat("b", 64)},
		Signing:    manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: f.key.ID},
		Producer:   manifest.Producer{NselfCIVersion: "1.5.0", Coordinator: "coordinator-1"}}
	if tweak != nil {
		tweak(&m)
	}
	out, err := manifest.Build(m)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

// request wraps one manifest payload as the signer's wire request and decodes it back like the signer process does.
func (f *g4SignerFx) request(payload []byte) (signing.Request, [][]byte) {
	f.t.Helper()
	req := signing.Request{RunID: g4SigRun, Items: []signing.Item{{PayloadType: manifest.PayloadType, PayloadB64: base64.StdEncoding.EncodeToString(payload)}}}
	raw, _ := json.Marshal(req)
	got, payloads, err := signing.DecodeRequest(strings.NewReader(string(raw)))
	if err != nil {
		f.t.Fatal(err)
	}
	return got, payloads
}
