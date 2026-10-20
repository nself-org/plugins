package pipeline_test

// Purpose: shared fixtures of the P7-HOST-06 tests: a stateful fake of the GitLab REST v4 routes the
// provider uses (served over TLS so the funnel's rules apply), a provider wired to it with a pinned
// clock and no sleeping, and the documents a finished pipeline leaves behind.
// Inputs:  per-test fakeGL knobs.
// Outputs: recorded requests and the state of the fake.
// Constraints: no live network and no GitLab call anywhere (DEF-003); every credential value is
// synthetic; the nonce source is deterministic so request bodies can be compared. The routes and
// fields are authored from the GitLab REST v4 documentation: pipelines, jobs, repository files and
// the version route (https://docs.gitlab.com/api/pipelines/, read 2026-10-19).

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

const (
	testToken    = "synthetic-gitlab-token-0001"
	testLogin    = "ci-operator"
	testRevision = "0123456789abcdef0123456789abcdef01234567"
	testHeadSHA  = "fedcba9876543210fedcba9876543210fedcba98"
	testCommit   = "839d28593b2409647f172553274791852aacd8b3"
	apiPrefix    = "/api/v4"
	projectBase  = apiPrefix + "/projects/acme%2Fwidgets"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func v15(t *testing.T) { t.Helper(); t.Setenv("NSELF_V15", "1") }

var epoch = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

type recorded struct {
	Method, Path, Query string
	Body                []byte
}

type fakeJob struct {
	ID          int64
	Name        string
	Status      string
	ArchiveSize int64 // reported size; 0 means len(Zip)
	Zip         []byte
	Expired     bool
}

type fakePipe struct {
	ID                             int64
	Status, Source, Ref, SHA, Name string
	User                           string
	Vars                           []map[string]string
	VarsStatus                     int
	Jobs                           []*fakeJob
	Cancelled                      int
	Nonce                          string
	Created                        time.Time
}

// fakeGL serves the routes of the pipeline provider.
type fakeGL struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex

	Reqs          []recorded
	Version       string // "18.2.1-ee": inputs; "16.11.2": variables; "" answers 404
	Login         string
	DefaultBranch string
	Visibility    string
	ProjectStatus int
	// CreateMode: "201" (the pipeline in the answer), "502-exists" (created, answer 502), "502-none"
	// (nothing created, 502), "400" (refused), "403".
	CreateMode string
	Pipes      map[int64]*fakePipe
	NextID     int64
	Include    []byte // the include file raw at any sha
	Root       []byte // the main .gitlab-ci.yml raw at any ref; nil answers 404
	IncludeSt  int
	BranchSt   int             // a status for the branch route (404 = the ref does not exist)
	Merged     string          // the merged_yaml the lint route answers; empty derives it from Include
	LintSt     int             // a status other than 200 for the lint route
	LintBad    bool            // valid: false
	Stale      bool            // the pipeline list shows nothing yet
	Extra      func(*fakePipe) // runs on every created pipeline (plants a second one)
	NameInVars bool            // variables-mode pipelines get the workflow:name too
}

func newFakeGL(t *testing.T) *fakeGL {
	t.Helper()
	f := &fakeGL{t: t, Version: "18.2.1-ee", Login: testLogin, DefaultBranch: "main", Visibility: "public", CreateMode: "201",
		Pipes: map[int64]*fakePipe{}, NextID: 9000, Include: pinnedInclude(t), Root: rootFile()}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGL) client() *rest.Client {
	return &rest.Client{BaseURL: f.srv.URL + apiPrefix, AllowPrivateBase: true, UserAgent: "pipeline-test", Provider: "gitlab",
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return testToken, nil }}
}

// download reads a link through the fake's own TLS client (the funnel refuses a private base for a
// download, which a real instance is not).
func (f *fakeGL) download(ctx context.Context, _ *rest.Client, link string, max int64, w io.Writer) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", link, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return providers.NewError(providers.Config, "E702", "request", resp.StatusCode, time.Time{})
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if int64(len(data)) > max {
		return providers.NewError(providers.Config, "E702", "download.too_large", 0, time.Time{})
	}
	_, err = w.Write(data)
	return err
}

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
	Journal      pipeline.Journal
	ID           string
	Window       time.Duration
	Nonce        func() (string, error)
	UntrustedRef string
	Rendering    func(context.Context, int, []byte) ([]byte, error)
}

func (f *fakeGL) provider(o options) *pipeline.Provider {
	window := o.Window
	if window == 0 {
		window = 15 * time.Second
	}
	nonce := o.Nonce
	if nonce == nil {
		nonce = nonces()
	}
	return pipeline.New(pipeline.Config{ID: o.ID, Client: func(context.Context) (*rest.Client, error) { return f.client(), nil },
		Clock: func() time.Time { return epoch }, Wait: func(context.Context, time.Duration) error { return nil }, Nonce: nonce,
		Journal: o.Journal, Download: f.download, FindWindow: window, UntrustedRef: o.UntrustedRef, Rendering: o.Rendering})
}

func (f *fakeGL) requests(method, contains string) []recorded {
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

func (f *fakeGL) pipeJSON(p *fakePipe) map[string]any {
	return map[string]any{"id": p.ID, "status": p.Status, "source": p.Source, "ref": p.Ref, "sha": p.SHA, "name": p.Name,
		"user": map[string]any{"username": p.User}, "created_at": p.Created.Format(time.RFC3339)}
}

var (
	pipeRE = regexp.MustCompile(`^/pipelines/([0-9]+)(/variables|/jobs|/cancel)?$`)
	jobRE  = regexp.MustCompile(`^/jobs/([0-9]+)/artifacts$`)
)

func (f *fakeGL) serve(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Reqs = append(f.Reqs, recorded{req.Method, req.URL.EscapedPath(), req.URL.RawQuery, body})
	p := req.URL.EscapedPath()
	switch {
	case p == apiPrefix+"/version":
		if f.Version == "" {
			w.WriteHeader(404)
			return
		}
		writeJSON(w, 200, map[string]any{"version": f.Version, "revision": "abcdef0"})
	case p == apiPrefix+"/user":
		writeJSON(w, 200, map[string]any{"username": f.Login})
	case p == projectBase && req.Method == "GET":
		if f.ProjectStatus != 0 {
			w.WriteHeader(f.ProjectStatus)
			return
		}
		writeJSON(w, 200, map[string]any{"id": 77, "default_branch": f.DefaultBranch, "visibility": f.Visibility})
	case p == projectBase+"/pipeline" && req.Method == "POST":
		f.create(w, body)
	case p == projectBase+"/pipelines" && req.Method == "GET":
		f.list(w, req)
	case p == projectBase+"/ci/lint" && req.Method == "GET":
		if f.LintSt != 0 {
			w.WriteHeader(f.LintSt)
			return
		}
		merged := f.Merged
		if merged == "" {
			merged = mergedFrom(f.t, f.Include, nil)
		}
		writeJSON(w, 200, map[string]any{"valid": !f.LintBad, "merged_yaml": merged})
	case strings.HasPrefix(p, projectBase+"/pipelines/"):
		f.pipeline(w, req, strings.TrimPrefix(p, projectBase))
	case strings.HasPrefix(p, projectBase+"/repository/branches/"):
		if f.BranchSt != 0 {
			w.WriteHeader(f.BranchSt)
			return
		}
		writeJSON(w, 200, map[string]any{"name": "x"})
	case p == projectBase+"/repository/files/.gitlab-ci.yml/raw":
		if f.Root == nil {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(f.Root)
	case strings.HasPrefix(p, projectBase+"/repository/files/") && strings.HasSuffix(p, "/raw"):
		if f.IncludeSt != 0 {
			w.WriteHeader(f.IncludeSt)
			return
		}
		_, _ = w.Write(f.Include)
	case strings.HasPrefix(p, projectBase+"/jobs/"):
		m := jobRE.FindStringSubmatch(strings.TrimPrefix(p, projectBase))
		id := int64(0)
		if m != nil {
			id, _ = strconv.ParseInt(m[1], 10, 64)
		}
		for _, pl := range f.Pipes {
			for _, j := range pl.Jobs {
				if j.ID == id && j.Zip != nil {
					_, _ = w.Write(j.Zip)
					return
				}
			}
		}
		w.WriteHeader(404)
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeGL) list(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	var all []map[string]any
	for id := f.NextID; id > 9000 && !f.Stale; id-- {
		pl := f.Pipes[id]
		if pl != nil && pl.Ref == q.Get("ref") && pl.Source == q.Get("source") {
			item := f.pipeJSON(pl)
			delete(item, "name")
			delete(item, "user")
			all = append(all, item)
		}
	}
	per, _ := strconv.Atoi(q.Get("per_page"))
	page, _ := strconv.Atoi(q.Get("page"))
	if per <= 0 || page <= 0 {
		per, page = len(all)+1, 1
	}
	out := []map[string]any{}
	for i := (page - 1) * per; i < len(all) && i < page*per; i++ {
		out = append(out, all[i])
	}
	writeJSON(w, 200, out)
}

func (f *fakeGL) pipeline(w http.ResponseWriter, req *http.Request, p string) {
	m := pipeRE.FindStringSubmatch(strings.TrimPrefix(p, "/pipelines"))
	if m == nil {
		m = pipeRE.FindStringSubmatch(p)
	}
	if m == nil {
		w.WriteHeader(404)
		return
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	pl := f.Pipes[id]
	if pl == nil {
		w.WriteHeader(404)
		return
	}
	switch m[2] {
	case "":
		writeJSON(w, 200, f.pipeJSON(pl))
	case "/variables":
		if pl.VarsStatus != 0 {
			w.WriteHeader(pl.VarsStatus)
			return
		}
		writeJSON(w, 200, pl.Vars)
	case "/jobs":
		var out []map[string]any
		for _, j := range pl.Jobs {
			item := map[string]any{"id": j.ID, "name": j.Name, "status": j.Status}
			if j.Zip != nil {
				size := j.ArchiveSize
				if size == 0 {
					size = int64(len(j.Zip))
				}
				item["artifacts"] = []map[string]any{{"file_type": "archive", "size": size}, {"file_type": "metadata", "size": 50}}
				if j.Expired {
					item["artifacts_expire_at"] = epoch.Add(-time.Hour).Format(time.RFC3339)
				}
			}
			out = append(out, item)
		}
		writeJSON(w, 200, out)
	case "/cancel":
		if req.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		pl.Cancelled++
		if pl.Status != "success" && pl.Status != "failed" && pl.Status != "canceled" {
			pl.Status = "canceled"
		}
		writeJSON(w, 200, f.pipeJSON(pl))
	}
}

// create makes the pipeline the request asks for, answering by CreateMode.
func (f *fakeGL) create(w http.ResponseWriter, body []byte) {
	var in struct {
		Ref       string              `json:"ref"`
		Inputs    map[string]string   `json:"inputs"`
		Variables []map[string]string `json:"variables"`
	}
	if json.Unmarshal(body, &in) != nil {
		w.WriteHeader(422)
		return
	}
	switch f.CreateMode {
	case "400":
		writeJSON(w, 400, map[string]any{"message": map[string]any{"base": []string{"Insufficient permissions to set pipeline variables"}}})
		return
	case "403":
		w.WriteHeader(403)
		return
	case "502-none":
		w.WriteHeader(502)
		return
	}
	nonce := in.Inputs["nonce"]
	name := "nself-ci " + nonce
	for _, v := range in.Variables {
		if v["key"] == "NSELF_CI_NONCE" {
			nonce = v["value"]
		}
	}
	if len(in.Variables) > 0 {
		name = ""
		if f.NameInVars {
			name = "nself-ci " + nonce
		}
	}
	f.NextID++
	pl := &fakePipe{ID: f.NextID, Status: "created", Source: "api", Ref: in.Ref, SHA: testHeadSHA, Name: name, User: f.Login, Vars: in.Variables,
		Nonce: nonce, Created: epoch, Jobs: []*fakeJob{{ID: 5000 + f.NextID, Name: gitlabtmpl.Job, Status: "created"}}}
	f.Pipes[pl.ID] = pl
	if f.Extra != nil {
		f.Extra(pl)
	}
	if f.CreateMode == "201" {
		writeJSON(w, 201, f.pipeJSON(pl))
		return
	}
	w.WriteHeader(502)
}

// finish completes a pipeline with an evidence artifact for the attempt it was dispatched for.
func (f *fakeGL) finish(id int64, attempt, revision string, mut func(*model.Evidence)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pl := f.Pipes[id]
	pl.Status = "success"
	pl.Jobs[0].Status = "success"
	pl.Jobs[0].Zip = evidenceZip(f.t, attempt, revision, mut)
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

// pinnedInclude is the include file the coordinator expects on a pipeline for the pinned agent commit.
func pinnedInclude(t *testing.T) []byte {
	t.Helper()
	out, err := gitlabtmpl.Render(gitlabtmpl.Options{AgentCommit: testCommit})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// attemptFor is a dispatchable attempt on acme/widgets.
func attemptFor(id string, n int) providers.Attempt {
	spec := []byte(`{"kind":"test","run":["go","test","./..."]}`)
	return providers.Attempt{ID: id, N: n, Project: "widgets", Repo: "acme/widgets", Revision: testRevision, Trust: "owner", Label: "saas-linux-small-amd64",
		CompatMode: "v1.5", Spec: spec, TimeoutMs: 20 * 60 * 1000}
}

func wantCode(t *testing.T, err error, code, reason string) {
	t.Helper()
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != code || (reason != "" && pe.Reason != reason) {
		t.Fatalf("want %s/%s, got %v", code, reason, err)
	}
}

// mergedFrom is what GitLab's lint answers as merged_yaml for a project whose only CI file is the
// include: the body document of the include file, optionally changed by edit.
func mergedFrom(t *testing.T, include []byte, edit func(map[string]any)) string {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(include))
	var body map[string]any
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if _, ok := doc[gitlabtmpl.Job]; ok {
			body = doc
		}
	}
	if body == nil {
		t.Fatal("no body document in the include file")
	}
	if edit != nil {
		edit(body)
	}
	out, err := yaml.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// rootFile is a main .gitlab-ci.yml that declares every pipeline input and maps it to a variable (the
// generated inputs root), so the dispatch may use the inputs transport; it is also the exact file the
// untrusted ref must carry for inputs.
func rootFile() []byte { return gitlabtmpl.RenderRoot(gitlabtmpl.RootInputs) }
