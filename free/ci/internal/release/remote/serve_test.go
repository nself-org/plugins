package remote_test

// Purpose: the two handlers through the real user-API wrapper in local (operator) mode, with an
// in-memory run view and a real blob store: what is listed is served, everything else is the one
// E713 body, a resized blob is streamed as stored (the operator's re-hash rejects it), and a serve
// that has not started answers "not found". Role and project refusals need a token and TLS: they
// are in the integration tests (TestTeamSignMemberRefused, TestTeamSignHidesWhatTheCallerMaySee).
// Constraints: no network beyond 127.0.0.1; temp dirs only; not parallel (shared route registry).

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	uRun  = "run-1"
	uProj = "/srv/checkout"
	uRev  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func digestOf(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// fakeStore is a finished release run in memory.
type fakeStore struct{ recs []records.Record }

func (f *fakeStore) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	return store.PipelineRow{ID: uRun, Project: uProj, Revision: uRev, InputDigest: "in"}, id == uRun, nil
}
func (f *fakeStore) PipelineJobs(context.Context, string) ([]store.RunJob, error) {
	return []store.RunJob{{ID: "r", Name: "publish", Kind: "release"}}, nil
}
func (f *fakeStore) PipelineAttempts(context.Context, string) ([]store.AttemptView, error) {
	return []store.AttemptView{{ID: "r.a1", JobID: "r", N: 1, State: "passed"}}, nil
}
func (f *fakeStore) EvidenceByRevision(context.Context, string, string) ([]store.EvidenceView, error) {
	return []store.EvidenceView{{ID: "e", PipelineID: uRun, Body: []byte(`{"run_id":"run-1"}`)}}, nil
}
func (f *fakeStore) ArtifactList(context.Context, records.Filter) ([]records.Record, error) {
	return f.recs, nil
}
func (f *fakeStore) AttemptCoordinator(context.Context, string) (string, error) { return "c1", nil }

type fakeRuns struct{}

func (fakeRuns) Get(_ context.Context, ref string) (clientapi.RunItem, bool, error) {
	it := clientapi.RunItem{Project: "acme/app"}
	it.ID = uRun
	return it, ref == uRun, nil
}

type localAPI struct {
	srv   *httptest.Server
	blobs *cas.Store
	app   []byte
}

// newLocalAPI serves the routes to the local operator over a plain-HTTP test server.
func newLocalAPI(t *testing.T) *localAPI {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := cas.New(filepath.Join(home, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	l := &localAPI{blobs: blobs, app: []byte("app-bytes")}
	put := func(b []byte) string {
		d, _, perr := blobs.Put(context.Background(), cas.ArtifactRealm(uProj), bytes.NewReader(b))
		if perr != nil {
			t.Fatal(perr)
		}
		return d
	}
	rec := func(name string, kind records.Kind, b []byte) records.Record {
		return records.Record{ID: "r-" + name, Project: uProj, Run: uRun, Attempt: "r.a1", Revision: uRev, Name: name, Kind: kind,
			SHA256: put(b), Size: int64(len(b)), State: records.StateCommitted}
	}
	fs := &fakeStore{recs: []records.Record{rec("app", records.KindBinary, l.app), rec("app.cdx.json", records.KindSBOM, []byte("{}")), rec("build.log", records.KindLog, []byte("log"))}}
	useDeps(t, &remote.Deps{Runs: fakeRuns{}, Store: fs, Blobs: blobs})
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "alice"
	api := clientapi.New(clientapi.Config{Roles: svc, Local: true, Operator: "cli:alice", Poll: time.Millisecond})
	l.srv = httptest.NewServer(api.Handler())
	t.Cleanup(l.srv.Close)
	return l
}

func (l *localAPI) get(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(l.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestHandlersServeOnlyTheListedReleaseInputs(t *testing.T) {
	l := newLocalAPI(t)
	resp, body := l.get(t, remote.InputsPath(uRun))
	var in remote.Inputs
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &in) != nil || in.Schema != remote.Schema || len(in.Records) != 2 || in.Pipeline.Project != uProj {
		t.Fatalf("inputs: %d %s", resp.StatusCode, body)
	}
	resp, body = l.get(t, remote.BlobPath(uRun, digestOf(l.app)))
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, l.app) || resp.Header.Get("Content-Length") != "9" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("blob: %d %q %v", resp.StatusCode, body, resp.Header)
	}

	_, want := l.get(t, remote.BlobPath(uRun, strings.Repeat("0", 64)))
	if !strings.Contains(string(want), `"E713"`) {
		t.Fatalf("not-found body %s", want)
	}
	for name, path := range map[string]string{
		"log digest (not a release input)": remote.BlobPath(uRun, digestOf([]byte("log"))),
		"unknown run blob":                 remote.BlobPath("run-nope", digestOf(l.app)),
		"unknown run inputs":               remote.InputsPath("run-nope"),
		"upper-case digest":                remote.BlobPath(uRun, strings.ToUpper(digestOf(l.app))),
		"short digest":                     remote.BlobPath(uRun, "abc"),
	} {
		if resp, body := l.get(t, path); resp.StatusCode != http.StatusNotFound || !bytes.Equal(body, want) {
			t.Errorf("%s: %d %s, want 404 %s", name, resp.StatusCode, body, want)
		}
	}
}

func TestHandlersStreamAResizedBlobAsStored(t *testing.T) {
	l := newLocalAPI(t)
	// the coordinator's bytes grew after the run: same digest path, other bytes under it
	d := digestOf(l.app)
	bigger := append(append([]byte{}, l.app...), "-and-more"...)
	useDeps(t, &remote.Deps{Runs: fakeRuns{}, Store: &fakeStore{recs: []records.Record{{ID: "r", Project: uProj, Run: uRun, Attempt: "r.a1", Revision: uRev, Name: "app",
		Kind: records.KindBinary, SHA256: d, Size: int64(len(l.app)), State: records.StateCommitted}}}, Blobs: sizedBlobs{inner: l.blobs, digest: d, data: bigger}})
	resp, body := l.get(t, remote.BlobPath(uRun, d))
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, bigger) {
		t.Fatalf("resized blob: %d %q", resp.StatusCode, body)
	}
}

// sizedBlobs answers one digest with other bytes (and their size); the handler only reads Size.
type sizedBlobs struct {
	inner  *cas.Store
	digest string
	data   []byte
}

type sizedInfo struct {
	os.FileInfo
	n int64
}

func (s sizedInfo) Size() int64 { return s.n }

func (s sizedBlobs) Stat(r cas.Realm, d string) (os.FileInfo, error) {
	if d == s.digest {
		return sizedInfo{n: int64(len(s.data))}, nil
	}
	return s.inner.Stat(r, d)
}

func (s sizedBlobs) Open(r cas.Realm, d string) (io.ReadCloser, error) {
	if d == s.digest {
		return io.NopCloser(bytes.NewReader(s.data)), nil
	}
	return s.inner.Open(r, d)
}

func TestHandlersAnswerNotFoundUntilTheServeStarts(t *testing.T) {
	l := newLocalAPI(t)
	useDeps(t, nil)
	for _, path := range []string{remote.InputsPath(uRun), remote.BlobPath(uRun, digestOf(l.app))} {
		if resp, body := l.get(t, path); resp.StatusCode != http.StatusNotFound || !strings.Contains(string(body), `"E713"`) {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, body)
		}
	}
}
