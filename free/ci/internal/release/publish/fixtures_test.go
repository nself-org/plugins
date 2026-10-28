package publish_test

// Purpose: shared fixtures: one finished release run (two artifacts, their
// SBOMs, stored evidence) in an in-memory run store and blob store, a real
// release key behind the real signing.Handler used as an in-process signer
// that counts its calls, and a real store as the release ledger.
// Constraints: no process or network; every digest is the real sha256.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	runID    = "run-1"
	project  = "proj"
	revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	relJob   = "run-1.publish-1a2b3c4d"
	relAtt   = relJob + ".a1"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type fakeSrc struct {
	pipe     store.PipelineRow
	jobs     []store.RunJob
	attempts []store.AttemptView
	evidence []store.EvidenceView
	recs     []records.Record
}

func (f *fakeSrc) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	return f.pipe, id == f.pipe.ID, nil
}
func (f *fakeSrc) AttemptCoordinator(context.Context, string) (string, error)   { return "", nil }
func (f *fakeSrc) PipelineJobs(context.Context, string) ([]store.RunJob, error) { return f.jobs, nil }
func (f *fakeSrc) PipelineAttempts(context.Context, string) ([]store.AttemptView, error) {
	return f.attempts, nil
}
func (f *fakeSrc) EvidenceByRevision(_ context.Context, rev, in string) ([]store.EvidenceView, error) {
	if rev != f.pipe.Revision || in != f.pipe.InputDigest {
		return nil, nil
	}
	return f.evidence, nil
}
func (f *fakeSrc) ArtifactList(_ context.Context, flt records.Filter) ([]records.Record, error) {
	var out []records.Record
	for _, r := range f.recs {
		if r.Project == flt.Project && r.Run == flt.Run && (flt.Name == "" || r.Name == flt.Name) && (flt.Kind == "" || r.Kind == flt.Kind) {
			out = append(out, r)
		}
	}
	return out, nil
}

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
	return io.NopCloser(bytes.NewReader(d)), nil
}

// fakeRes is the secret resolver behind the signer; calls counts resolutions.
type fakeRes struct {
	vals  map[string][]byte
	calls int
}

func (r *fakeRes) Probe(context.Context) error { return nil }
func (r *fakeRes) Resolve(_ context.Context, _ string, refs []secrets.Ref) ([]secrets.Value, error) {
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

// inProcSigner is the signer client over the real handler.
type inProcSigner struct {
	h     signing.Handler
	res   *fakeRes
	signs int
	pubs  int
}

func (s *inProcSigner) Sign(ctx context.Context, run string, items []signing.Item) ([]sdk.Envelope, error) {
	s.signs++
	req := signing.Request{RunID: run, Items: items}
	raw, _ := json.Marshal(req)
	_, payloads, err := signing.DecodeRequest(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.h.Handle(ctx, req, payloads)
}

func (s *inProcSigner) PublicKey(ctx context.Context) (signing.KeyInfo, error) {
	s.pubs++
	return signing.Public(ctx, s.res, signing.KeyAttempt)
}

type fixture struct {
	t      *testing.T
	src    *fakeSrc
	blobs  *fakeBlobs
	res    *fakeRes
	signer *inProcSigner
	ledger *store.Store
	home   string
	files  map[string][]byte
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	key, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	_ = key
	f := &fixture{t: t, blobs: &fakeBlobs{data: map[string][]byte{}}, home: t.TempDir(),
		res: &fakeRes{vals: map[string][]byte{signing.KeyRef: value}},
		files: map[string][]byte{"app": []byte("app-bytes"), "image.tar": []byte("image-bytes!"), "app.cdx.json": []byte("cdx-1"),
			"app.spdx.json": []byte("spdx-1"), "image.tar.cdx.json": []byte("cdx-2"), "image.tar.spdx.json": []byte("spdx-2")}}
	start, finish := "2026-10-19T11:59:00Z", "2026-10-19T12:00:00Z"
	ev := model.Evidence{Schema: "ci.evidence/v1", Revision: revision, RepoID: "repo-1", RunID: runID, Result: "pass",
		Pipeline: model.PipelineRef{ID: runID, Name: "release", Kind: "default"}, Trigger: "local", SourceTrust: "owner", PolicyDigest: "policy-1",
		WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		InputDigests: model.InputDigests{Config: strings.Repeat("c", 64), Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("d", 64)}}, Generated: []model.DigestRef{}},
		Runner:       model.RunnerRef{ID: "local", Provider: "local", Platform: "linux/amd64", Isolation: "process"}, Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"},
		Timings: model.Timings{StartedAt: &start, FinishedAt: &finish}, ToolVersions: map[string]string{}, Retries: []model.Retry{}, Artifacts: []model.Artifact{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"}, Counts: model.Counts{Pass: 2, Total: 2},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
		Checks: []model.Check{{ID: "test", JobID: "test", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
			{ID: "publish", JobID: "publish", Kind: "release", Tool: "nself", Required: true, Substantive: true, Result: "pass"}}}
	body, _ := json.Marshal(ev)
	f.src = &fakeSrc{pipe: store.PipelineRow{ID: runID, Project: project, Revision: revision, InputDigest: "in1"},
		jobs:     []store.RunJob{{ID: "run-1.test-0", Name: "test", Kind: "test"}, {ID: relJob, Name: "publish", Kind: "release"}},
		attempts: []store.AttemptView{{ID: "run-1.test-0.a1", JobID: "run-1.test-0", N: 1, State: "passed"}, {ID: relAtt, JobID: relJob, N: 1, State: "passed"}},
		evidence: []store.EvidenceView{{ID: "e-run", PipelineID: runID, Body: body}}}
	for name, b := range f.files {
		f.blobs.data[sum(b)] = b
		kind := records.KindBinary
		switch {
		case strings.HasSuffix(name, ".json"):
			kind = records.KindSBOM
		case name == "image.tar":
			kind = records.KindArchive
		}
		r := records.Record{ID: "r-" + name, Project: project, Run: runID, Attempt: relAtt, Job: "publish", Revision: revision, Name: name, Kind: kind, SHA256: sum(b),
			Size: int64(len(b)), State: records.StateCommitted, CreatedAt: time.Unix(1, 0)}
		if kind == records.KindBinary {
			r.Platform = "linux/amd64"
		}
		f.src.recs = append(f.src.recs, r)
	}
	f.signer = &inProcSigner{h: signing.Handler{Src: f.src, Blobs: f.blobs, Project: project, Resolver: f.res}, res: f.res}
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f.ledger = st
	return f
}

// deps builds Deps; verify is real verify.Release unless the test replaces it.
func (f *fixture) deps() publish.Deps {
	return publish.Deps{Src: f.src, Blobs: f.blobs, Ledger: f.ledger, Signer: f.signer, Home: f.home, Root: "", Verify: acceptEvidenceGap}
}

// readDir returns every file of dir by relative slash path.
func readDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// setChecks rewrites the checks of the stored evidence.
func (f *fixture) setChecks(fn func([]model.Check) []model.Check) {
	f.rewriteEvidence(func(ev *model.Evidence) { ev.Checks = fn(ev.Checks) })
}

func (f *fixture) setResult(r model.Result) {
	f.rewriteEvidence(func(ev *model.Evidence) { ev.Result = r })
}

func (f *fixture) rewriteEvidence(fn func(*model.Evidence)) {
	var ev model.Evidence
	if err := json.Unmarshal(f.src.evidence[0].Body, &ev); err != nil {
		f.t.Fatal(err)
	}
	fn(&ev)
	b, _ := json.Marshal(ev)
	f.src.evidence[0].Body = b
}

type flagSet = flag.FlagSet

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func bytesEqualMaps(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !bytes.Equal(v, b[k]) {
			return false
		}
	}
	return true
}
