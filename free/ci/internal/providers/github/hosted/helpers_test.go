package hosted_test

// Purpose: shared fixtures of the P7-HOST-03 tests: a stateful fake of the GitHub REST routes the
// provider uses (served over TLS so the funnel's rules apply), a provider wired to it with a pinned
// clock and no sleeping, and the documents a finished run leaves behind.
// Inputs:  per-test fakeGH knobs.
// Outputs: recorded requests and the state of the fake.
// Constraints: no live network; every credential value is synthetic; the nonce source is
// deterministic so request bodies can be compared.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

const (
	testToken    = "synthetic-test-token-0001"
	testLogin    = "ci-operator"
	testRevision = "0123456789abcdef0123456789abcdef01234567"
	testHeadSHA  = "fedcba9876543210fedcba9876543210fedcba98"
	testCommit   = "839d28593b2409647f172553274791852aacd8b3"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

var epoch = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

type recorded struct {
	Method, Path, Query string
	Body                []byte
}

type fakeRun struct {
	ID                 int64
	Status, Event      string
	Path, Branch, SHA  string
	Title, Login       string
	RunAttempt         int
	Cancelled          bool
	Nonce              string
	ArtifactID         int64
	ArtifactName       string
	ArtifactSize       int64
	EvidenceZip        []byte
	WorkflowOverride   []byte
	ArtifactSizeReport int64
}

// fakeGH serves the routes of the hosted provider.
type fakeGH struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex

	Reqs          []recorded
	Private       bool
	Visibility    string
	DefaultBranch string
	RepoStatus    int
	Login         string
	// DispatchMode: "200" (run id in the answer), "204" (no content), "create-then-502" (the run
	// exists and the answer is a 502), "nothing-then-502" (no run, 502).
	DispatchMode string
	Runs         map[int64]*fakeRun
	NextID       int64
	Workflow     []byte
	Stale        bool           // runs list shows no run yet
	Extra        func(*fakeRun) // adds a second run for the same nonce when set
}

func newFakeGH(t *testing.T) *fakeGH {
	t.Helper()
	f := &fakeGH{t: t, Visibility: "public", DefaultBranch: "main", Login: testLogin, DispatchMode: "200", Runs: map[int64]*fakeRun{}, NextID: 9000}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGH) client() *rest.Client {
	return &rest.Client{BaseURL: f.srv.URL, AllowPrivateBase: true, APIVersion: "2022-11-28", UserAgent: "hosted-test",
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return testToken, nil }}
}

func (f *fakeGH) download(ctx context.Context, c *rest.Client, link string, max int64, w io.Writer) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", link, nil)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if int64(len(data)) > max {
		return providers.NewError(providers.Config, "E702", "download.too_large", 0, time.Time{})
	}
	_, err = w.Write(data)
	return err
}

// nonces returns a deterministic nonce source.
func nonces() func() (string, error) {
	var n int
	var mu sync.Mutex
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("%032x", n), nil
	}
}

type options struct {
	Settings hosted.Settings
	Journal  hosted.Journal
	ID       string
	Window   time.Duration
	Nonce    func() (string, error)
	Wait     func(context.Context, time.Duration) error
}

func (f *fakeGH) provider(o options) *hosted.Provider {
	s := o.Settings
	if s.UntrustedRef == "" {
		s = hosted.DefaultSettings()
	}
	window := o.Window
	if window == 0 {
		window = 15 * time.Second
	}
	nonce := o.Nonce
	if nonce == nil {
		nonce = nonces()
	}
	wait := o.Wait
	if wait == nil {
		wait = func(context.Context, time.Duration) error { return nil }
	}
	return hosted.New(hosted.Config{ID: o.ID, Client: func(context.Context) (*rest.Client, error) { return f.client(), nil },
		Settings: func(context.Context) (hosted.Settings, error) { return s, nil }, Clock: func() time.Time { return epoch },
		Wait: wait, Nonce: nonce, Journal: o.Journal, Download: f.download,
		FindWindow: window})
}

func (f *fakeGH) requests(method, contains string) []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recorded
	for _, r := range f.Reqs {
		if (method == "" || r.Method == method) && strings.Contains(r.Path, contains) {
			out = append(out, r)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeGH) runJSON(r *fakeRun) map[string]any {
	return map[string]any{"id": r.ID, "status": r.Status, "event": r.Event, "path": r.Path, "head_branch": r.Branch, "head_sha": r.SHA,
		"display_title": r.Title, "run_attempt": r.RunAttempt, "actor": map[string]any{"login": r.Login}}
}

func (f *fakeGH) serve(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Reqs = append(f.Reqs, recorded{req.Method, req.URL.Path, req.URL.RawQuery, body})
	p := strings.TrimPrefix(req.URL.Path, "/repos/acme/widgets")
	switch {
	case req.URL.Path == "/user":
		writeJSON(w, 200, map[string]any{"login": f.Login})
	case req.URL.Path == "/repos/acme/widgets" && req.Method == "GET":
		if f.RepoStatus != 0 {
			w.WriteHeader(f.RepoStatus)
			return
		}
		writeJSON(w, 200, map[string]any{"private": f.Private, "visibility": f.Visibility, "default_branch": f.DefaultBranch})
	case strings.HasSuffix(p, "/dispatches") && req.Method == "POST":
		f.dispatch(w, body)
	case strings.HasSuffix(p, "/runs") && req.Method == "GET":
		var ids []int64
		for id := range f.Runs {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		per, _ := strconv.Atoi(req.URL.Query().Get("per_page"))
		page, _ := strconv.Atoi(req.URL.Query().Get("page"))
		if per <= 0 {
			per = 30
		}
		if page <= 0 {
			page = 1
		}
		var list []map[string]any
		for i := (page - 1) * per; !f.Stale && i < len(ids) && i < page*per; i++ {
			list = append(list, f.runJSON(f.Runs[ids[i]]))
		}
		writeJSON(w, 200, map[string]any{"workflow_runs": list})
	case strings.HasSuffix(p, "/cancel"):
		r := f.runByPath(p)
		switch {
		case r == nil:
			w.WriteHeader(404)
		case r.Status == "completed":
			w.WriteHeader(409)
		default:
			r.Cancelled, r.Status = true, "completed"
			w.WriteHeader(202)
		}
	case strings.Contains(p, "/actions/runs/") && strings.HasSuffix(p, "/artifacts"):
		r := f.runByPath(p)
		if r == nil || r.ArtifactID == 0 {
			writeJSON(w, 200, map[string]any{"artifacts": []any{}})
			return
		}
		size := r.ArtifactSizeReport
		if size == 0 {
			size = int64(len(r.EvidenceZip))
		}
		writeJSON(w, 200, map[string]any{"artifacts": []map[string]any{{"id": r.ArtifactID, "name": r.ArtifactName, "size_in_bytes": size, "expired": false}}})
	case strings.Contains(p, "/actions/artifacts/") && strings.HasSuffix(p, "/zip"):
		for _, r := range f.Runs {
			if strings.Contains(p, "/artifacts/"+strconv.FormatInt(r.ArtifactID, 10)+"/") {
				_, _ = w.Write(r.EvidenceZip)
				return
			}
		}
		w.WriteHeader(404)
	case strings.Contains(p, "/actions/runs/"):
		if r := f.runByPath(p); r != nil {
			writeJSON(w, 200, f.runJSON(r))
			return
		}
		w.WriteHeader(404)
	case strings.HasPrefix(p, "/contents/"):
		wf := f.Workflow
		for _, r := range f.Runs {
			if r.WorkflowOverride != nil && r.SHA == req.URL.Query().Get("ref") {
				wf = r.WorkflowOverride
			}
		}
		writeJSON(w, 200, map[string]any{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(wf)})
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeGH) runByPath(p string) *fakeRun {
	i := strings.Index(p, "/actions/runs/")
	rest := strings.TrimPrefix(p[i:], "/actions/runs/")
	id, _ := strconv.ParseInt(strings.SplitN(rest, "/", 2)[0], 10, 64)
	return f.Runs[id]
}

// dispatch creates the run the request asks for, answering by mode.
func (f *fakeGH) dispatch(w http.ResponseWriter, body []byte) {
	var in struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	if json.Unmarshal(body, &in) != nil {
		w.WriteHeader(422)
		return
	}
	if f.DispatchMode == "nothing-then-502" {
		w.WriteHeader(502)
		return
	}
	f.NextID++
	r := &fakeRun{ID: f.NextID, Status: "queued", Event: "workflow_dispatch", Path: hosted.WorkflowPath, Branch: in.Ref, SHA: testHeadSHA,
		Title: "nself-ci " + in.Inputs["nonce"], Login: f.Login, RunAttempt: 1, Nonce: in.Inputs["nonce"]}
	f.Runs[r.ID] = r
	if f.Extra != nil {
		f.Extra(r)
	}
	switch f.DispatchMode {
	case "200":
		writeJSON(w, 200, map[string]any{"workflow_run_id": r.ID, "run_url": "x", "html_url": "y"})
	case "create-then-502":
		w.WriteHeader(502)
	default:
		w.WriteHeader(204)
	}
}

// finish completes a run with an evidence artifact for the attempt the run was dispatched for.
func (f *fakeGH) finish(id int64, attempt, revision string, mut func(*model.Evidence)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.Runs[id]
	r.Status, r.ArtifactID, r.ArtifactName = "completed", 4000+id, hosted.EvidenceArtifact(r.Nonce)
	r.EvidenceZip = evidenceZip(f.t, attempt, revision, mut)
}

func sampleEvidence(attempt, revision string) model.Evidence {
	return model.Evidence{
		Schema: "ci.evidence/v1", Revision: revision, RepoID: "local:rrrrrrrr", RunID: "r1", Attempt: &model.AttemptRef{ID: attempt, N: 1},
		Pipeline: model.PipelineRef{ID: "default", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner", PolicyDigest: "p",
		InputDigests:  model.InputDigests{Config: strings.Repeat("c", 64), Lockfiles: []model.DigestRef{}, Generated: []model.DigestRef{}},
		WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		Runner:   model.RunnerRef{ID: "agent", Provider: "agent", Platform: "linux/amd64", Isolation: "process"},
		Producer: model.Producer{Kind: "self-reported", Version: "1"}, ToolVersions: map[string]string{},
		Checks:    []model.Check{{ID: "unit", JobID: "unit", Kind: "test", Tool: "t", Required: true, Substantive: true, Result: "pass"}},
		Artifacts: []model.Artifact{}, Retries: []model.Retry{}, Selection: model.Selection{Mode: "full", Reason: "selection.no_base", Digest: "d"},
		Result: "pass", Counts: model.Counts{Pass: 1, Total: 1}, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
}

func evidenceZip(t *testing.T, attempt, revision string, mut func(*model.Evidence)) []byte {
	t.Helper()
	ev := sampleEvidence(attempt, revision)
	if mut != nil {
		mut(&ev)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("evidence.json")
	_, _ = w.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

// pinnedWorkflow is the workflow the coordinator expects on a run for the pinned agent commit.
func pinnedWorkflow(t *testing.T) []byte {
	t.Helper()
	out, err := githubtmpl.Render(githubtmpl.Options{AgentSource: githubtmpl.SourceMode, AgentCommit: testCommit})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// attemptFor is a dispatchable attempt on acme/widgets.
func attemptFor(id string, n int) providers.Attempt {
	spec := []byte(`{"kind":"test","run":["go","test","./..."]}`)
	return providers.Attempt{ID: id, N: n, Project: "widgets", Repo: "acme/widgets", Revision: testRevision, Trust: "owner", Label: "ubuntu-24.04",
		CompatMode: "v1.5", Spec: spec, TimeoutMs: 20 * 60 * 1000}
}

func inputsOf(t *testing.T, r recorded) (string, map[string]any, map[string]any) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	in, _ := body["inputs"].(map[string]any)
	ref, _ := body["ref"].(string)
	return ref, in, body
}
