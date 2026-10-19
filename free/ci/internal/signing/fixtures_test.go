package signing_test

// Purpose: shared fixtures for the signer tests: an in-memory run store and
// blob store holding one finished release run, a fake secret resolver and
// store, and builders for a manifest and a statement that match it.
// Inputs:  none. Outputs: *fixture and helpers.
// Constraints: no process, database or network; every digest is the real
// sha256 of the fixture bytes.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
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
	runID    = "run-1"
	project  = "proj"
	revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// fakeSrc is the run store.
type fakeSrc struct {
	pipe     store.PipelineRow
	known    bool
	jobs     []store.RunJob
	attempts []store.AttemptView
	evidence []store.EvidenceView
	recs     []records.Record
	err      error
	extra    int // fake rows returned beyond the real ones, to reach the store's row cap
}

func (f *fakeSrc) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	if f.err != nil {
		return store.PipelineRow{}, false, f.err
	}
	return f.pipe, f.known && id == f.pipe.ID, nil
}
func (f *fakeSrc) PipelineJobs(context.Context, string) ([]store.RunJob, error) { return f.jobs, f.err }
func (f *fakeSrc) PipelineAttempts(context.Context, string) ([]store.AttemptView, error) {
	return f.attempts, f.err
}
func (f *fakeSrc) EvidenceByRevision(_ context.Context, rev, in string) ([]store.EvidenceView, error) {
	if rev != f.pipe.Revision || in != f.pipe.InputDigest {
		return nil, nil
	}
	return f.evidence, f.err
}
func (f *fakeSrc) ArtifactList(_ context.Context, flt records.Filter) ([]records.Record, error) {
	var out []records.Record
	for _, r := range f.recs {
		if r.Project == flt.Project && r.Run == flt.Run && (flt.Name == "" || r.Name == flt.Name) && (flt.Kind == "" || r.Kind == flt.Kind) {
			out = append(out, r)
		}
	}
	for i := 0; i < f.extra; i++ {
		out = append(out, records.Record{ID: "pad", Project: flt.Project, Run: "other"})
	}
	return out, f.err
}

// fakeBlobs is the blob store: digest -> bytes.
type fakeBlobs struct{ data map[string][]byte }

type memInfo struct {
	os.FileInfo
	size int64
}

func (m memInfo) Size() int64 { return m.size }

func (b *fakeBlobs) Stat(_ cas.Realm, digest string) (os.FileInfo, error) {
	d, ok := b.data[digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return memInfo{size: int64(len(d))}, nil
}

func (b *fakeBlobs) Open(_ cas.Realm, digest string) (io.ReadCloser, error) {
	d, ok := b.data[digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(string(d))), nil
}

// fakeRes is the secret resolver (and, with Set, the secret store).
type fakeRes struct {
	vals      map[string][]byte // ref -> value
	probeErr  error
	resolveEr error
	calls     int
	attempts  []string
	onResolve func() // runs inside Resolve, before it answers
}

func (r *fakeRes) Probe(context.Context) error { return r.probeErr }

func (r *fakeRes) Resolve(_ context.Context, attempt string, refs []secrets.Ref) ([]secrets.Value, error) {
	r.calls++
	r.attempts = append(r.attempts, attempt)
	if r.onResolve != nil {
		r.onResolve()
	}
	if r.resolveEr != nil {
		return nil, r.resolveEr
	}
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

type fakeStore struct {
	res      *fakeRes
	probeErr error
	setErr   error
	setErrOn string // fail only this name
	lockErr  error
	sets     []string
	mu       sync.Mutex // the key-management lock
	locks    int
}

func (s *fakeStore) Lock(context.Context) (func(), error) {
	if s.lockErr != nil {
		return nil, s.lockErr
	}
	s.mu.Lock()
	s.locks++
	return s.mu.Unlock, nil
}

func (s *fakeStore) Probe(context.Context) error { return s.probeErr }

func (s *fakeStore) Set(_ context.Context, name string, value []byte) error {
	if s.setErr != nil || (s.setErrOn != "" && s.setErrOn == name) {
		return errors.New("set failed")
	}
	s.sets = append(s.sets, name)
	s.res.vals["release/"+name] = append([]byte(nil), value...)
	return nil
}

type fixture struct {
	t     *testing.T
	src   *fakeSrc
	blobs *fakeBlobs
	res   *fakeRes
	key   *signing.Key
	files map[string][]byte // name -> bytes
	evRaw []byte
}

// newFixture builds a finished release run with two artifacts, their SBOMs,
// stored evidence and a stored release key.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	key, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, key: key, blobs: &fakeBlobs{data: map[string][]byte{}},
		res:   &fakeRes{vals: map[string][]byte{signing.KeyRef: value}},
		files: map[string][]byte{"app": []byte("app-bytes"), "image": []byte("image-bytes!"), "app.cdx": []byte("cdx-1"), "app.spdx": []byte("spdx-1"), "image.cdx": []byte("cdx-2"), "image.spdx": []byte("spdx-2")}}
	ev, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: revision, RepoID: "repo-1", RunID: runID, Result: "pass"})
	f.evRaw = ev
	f.src = &fakeSrc{known: true,
		pipe:     store.PipelineRow{ID: runID, Project: project, Revision: revision, InputDigest: "in1"},
		jobs:     []store.RunJob{{ID: "job-test", Name: "test", Kind: "check"}, {ID: "job-rel", Name: "publish", Kind: "release"}},
		attempts: []store.AttemptView{{ID: "att-test", JobID: "job-test", N: 1, State: "passed"}, {ID: "att-rel-1", JobID: "job-rel", N: 1, State: "failed"}, {ID: "att-rel-2", JobID: "job-rel", N: 2, State: "passed"}},
		evidence: []store.EvidenceView{{ID: "e-job", PipelineID: runID, AttemptID: "att-test", Body: []byte("{}")}, {ID: "e-run", PipelineID: runID, Body: ev}}}
	for name, b := range f.files {
		f.blobs.data[sum(b)] = b
		kind := records.KindBinary
		switch {
		case strings.Contains(name, "."):
			kind = records.KindSBOM
		case name == "image":
			kind = records.KindArchive
		}
		f.src.recs = append(f.src.recs, records.Record{ID: "r-" + name, Project: project, Run: runID, Attempt: "att-rel-2", Revision: revision, Name: name, Kind: kind,
			SHA256: sum(b), Size: int64(len(b)), State: records.StateCommitted, CreatedAt: time.Unix(1, 0)})
	}
	return f
}

func (f *fixture) handler() signing.Handler {
	return signing.Handler{Src: f.src, Blobs: f.blobs, Project: project, Resolver: f.res}
}

// manifestPayload is a manifest every check accepts; tweak edits it first.
func (f *fixture) manifestPayload(tweak func(*manifest.Manifest)) []byte {
	f.t.Helper()
	art := func(name, cdx, spdx string, kind string) manifest.Artifact {
		b := f.files[name]
		return manifest.Artifact{Name: name, Path: "artifacts/" + name, Kind: kind, SHA256: sum(b), Size: int64(len(b)),
			SBOM: manifest.SBOM{CycloneDX: manifest.FileRef{Path: "sbom/" + name + ".cdx.json", SHA256: sum(f.files[cdx])}, SPDX: manifest.FileRef{Path: "sbom/" + name + ".spdx.json", SHA256: sum(f.files[spdx])}}}
	}
	m := manifest.Manifest{Schema: manifest.Schema, RepoID: "repo-1", Revision: revision,
		Release:    manifest.Release{RunID: runID, Pipeline: "release", JobID: "job-rel", AttemptID: "att-rel-2", IdempotencyKey: strings.Repeat("9", 64), FinishedAt: "2026-10-19T12:00:00Z"},
		Evidence:   manifest.EvidenceRef{Path: manifest.EvidencePath, SHA256: sum(f.evRaw), Result: manifest.EvidenceResult, Class: manifest.EvidenceClass},
		Artifacts:  []manifest.Artifact{art("app", "app.cdx", "app.spdx", manifest.KindBinary), art("image", "image.cdx", "image.spdx", manifest.KindArchive)},
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, SHA256: strings.Repeat("a", 64), PredicateType: manifest.ProvenancePredicate, SLSABuildLevel: manifest.SLSALevel},
		Checksums:  manifest.FileRef{Path: manifest.ChecksumsPath, SHA256: strings.Repeat("b", 64)},
		Signing:    manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: f.key.ID},
		Producer:   manifest.Producer{NselfCIVersion: "1.5.0", Coordinator: "coordinator-1"}}
	if tweak != nil {
		tweak(&m)
	}
	b, err := manifest.Build(m)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

// statementPayload is an in-toto statement over the two artifacts.
func (f *fixture) statementPayload(tweak func(map[string]any)) []byte {
	f.t.Helper()
	st := map[string]any{"_type": "https://in-toto.io/Statement/v1", "predicateType": manifest.ProvenancePredicate, "predicate": f.predicate(nil),
		"subject": []map[string]any{{"name": "app", "digest": map[string]string{"sha256": sum(f.files["app"])}}, {"name": "image", "digest": map[string]string{"sha256": sum(f.files["image"])}}}}
	if tweak != nil {
		tweak(st)
	}
	b, _ := json.Marshal(st)
	return b
}

func request(items ...signing.Item) signing.Request {
	return signing.Request{RunID: runID, Items: items}
}

func item(typ string, payload []byte) signing.Item {
	return signing.Item{PayloadType: typ, PayloadB64: b64(payload)}
}

func decodeAll(t *testing.T, req signing.Request) [][]byte {
	t.Helper()
	raw, _ := json.Marshal(req)
	_, p, err := signing.DecodeRequest(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	var e *signing.Error
	if !errors.As(err, &e) {
		t.Fatalf("not an E717 error: %v", err)
	}
	return e.Reason
}

// predicate is the SLSA provenance v1 predicate of the Epic's contract for the fixture run.
func (f *fixture) predicate(tweak func(map[string]any)) map[string]any {
	p := map[string]any{
		"buildDefinition": map[string]any{
			"buildType":            signing.ReleaseBuildType,
			"externalParameters":   map[string]any{"repository": "repo-1", "revision": revision, "pipeline": "release", "job": "publish"},
			"resolvedDependencies": []map[string]any{{"uri": "git+repo-1@" + revision, "digest": map[string]string{"gitCommit": revision}}},
		},
		"runDetails": map[string]any{"metadata": map[string]any{"invocationId": runID + "/att-rel-2"}},
	}
	if tweak != nil {
		tweak(p)
	}
	return p
}
