package runners_test

// Purpose: shared fixtures of the P7-HOST-07 tests: a stateful fake of the GitLab REST v4 routes the
// registrar uses (served over TLS so the funnel's rules apply) and a Registrar wired to it with a pinned
// clock.
// Inputs:  per-test fakeGL knobs.
// Outputs: recorded requests, request bodies and the state of the fake.
// Constraints: no live network and no GitLab call anywhere (DEF-003); every credential value is synthetic.
//          The routes and fields are authored from the GitLab REST v4 documentation: runners (POST
//          /user/runners, GET/PUT/DELETE /runners/:id, GET /runners/:id/jobs, GET /projects/:id/runners),
//          jobs and protected branches and tags (https://docs.gitlab.com/api/runners/, read 2026-10-20).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const (
	testProject   = "acme/widgets"
	testProjectID = 4242
	runnerToken   = "synthetic-runner-mgmt-token-0001"
	devToken      = "synthetic-developer-token-0002"
)

var epoch = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func clock() time.Time { return epoch }

type fakeRunner struct {
	ID      int64
	Desc    string
	Tags    []string
	Paused  bool
	Running bool // has a running job
}

type fakeJob struct {
	Tags     []string // the job's tag_list
	Ref      string
	Tag      bool
	Status   string
	RunnerID int64
}

// fakeGL is the slice of the GitLab REST API the registrar uses.
type fakeGL struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex

	runners []*fakeRunner
	nextID  int64
	jobs    map[int64]fakeJob
	branch  map[string]bool // protected branch names
	tags    map[string]bool // protected tag names

	mints []map[string]any // decoded POST /user/runners bodies
	puts  []map[string]any // decoded PUT /runners/:id bodies
	calls []string         // "METHOD path" (escaped)
	auths []string         // Authorization header of every request

	garbage       map[string]bool // protection reads of these names answer 200 with a body that is not the protection
	tokenOverride string          // when set, the token POST /user/runners replies
	keepDel       bool            // DELETE answers 204 but the runner stays
	raceDelete    bool            // DELETE removes the runner but answers 404
	jobsStatus    int             // when non-zero, GET /runners/:id/jobs answers this status
	statusFor     map[string]int
}

func newFakeGL(t *testing.T) *fakeGL {
	f := &fakeGL{t: t, nextID: 100, jobs: map[int64]fakeJob{}, branch: map[string]bool{}, tags: map[string]bool{}, garbage: map[string]bool{}, statusFor: map[string]int{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGL) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (f *fakeGL) find(id int64) (int, *fakeRunner) {
	for i, x := range f.runners {
		if x.ID == id {
			return i, x
		}
	}
	return -1, nil
}

func (f *fakeGL) view(x *fakeRunner) map[string]any {
	return map[string]any{"id": x.ID, "description": x.Desc, "paused": x.Paused, "runner_type": "project_type", "tag_list": x.Tags}
}

func (f *fakeGL) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeGL) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.runners) }

func (f *fakeGL) descriptions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, x := range f.runners {
		out = append(out, x.Desc)
	}
	return out
}

func projectKey(seg string) bool {
	return seg == "acme%2Fwidgets" || seg == strconv.Itoa(testProjectID)
}

func (f *fakeGL) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.EscapedPath()
	f.calls = append(f.calls, r.Method+" "+p)
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	if st, ok := f.statusFor[r.Method+" "+p]; ok {
		f.reply(w, st, map[string]string{"message": "injected"})
		return
	}
	seg := strings.Split(strings.TrimPrefix(p, "/"), "/")
	switch {
	case r.Method == "POST" && p == "/user/runners":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mints = append(f.mints, body)
		f.nextID++
		x := &fakeRunner{ID: f.nextID, Desc: body["description"].(string)}
		for _, t := range body["tag_list"].([]any) {
			x.Tags = append(x.Tags, t.(string))
		}
		f.runners = append(f.runners, x)
		token := "glrt-synthetic-" + strconv.FormatInt(x.ID, 10)
		if f.tokenOverride != "" {
			token = f.tokenOverride
		}
		f.reply(w, 201, map[string]any{"id": x.ID, "token": token, "token_expires_at": nil})
	case len(seg) == 2 && seg[0] == "projects" && r.Method == "GET":
		if !projectKey(seg[1]) {
			f.reply(w, 404, map[string]string{"message": "404 Project Not Found"})
			return
		}
		f.reply(w, 200, map[string]any{"id": testProjectID, "default_branch": "main", "visibility": "private"})
	case len(seg) == 3 && seg[0] == "projects" && seg[2] == "runners" && r.Method == "GET":
		size, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		lo, hi := min((page-1)*size, len(f.runners)), min(page*size, len(f.runners))
		rows := []map[string]any{}
		for _, x := range f.runners[lo:hi] {
			rows = append(rows, f.view(x))
		}
		f.reply(w, 200, rows)
	case len(seg) == 4 && seg[0] == "projects" && seg[2] == "jobs" && r.Method == "GET":
		id, _ := strconv.ParseInt(seg[3], 10, 64)
		j, ok := f.jobs[id]
		if !ok {
			f.reply(w, 404, map[string]string{"message": "404 Not found"})
			return
		}
		out := map[string]any{"id": id, "ref": j.Ref, "tag": j.Tag, "status": j.Status, "tag_list": j.Tags}
		if j.RunnerID != 0 {
			out["runner"] = map[string]any{"id": j.RunnerID}
		}
		f.reply(w, 200, out)
	case len(seg) == 4 && seg[0] == "projects" && seg[2] == "protected_branches" && r.Method == "GET":
		name, _ := url.PathUnescape(seg[3])
		f.protection(w, f.branch, name)
	case len(seg) == 4 && seg[0] == "projects" && seg[2] == "protected_tags" && r.Method == "GET":
		name, _ := url.PathUnescape(seg[3])
		f.protection(w, f.tags, name)
	case len(seg) == 2 && seg[0] == "runners":
		id, _ := strconv.ParseInt(seg[1], 10, 64)
		i, x := f.find(id)
		switch {
		case x == nil:
			f.reply(w, 404, map[string]string{"message": "404 Runner Not Found"})
		case r.Method == "GET":
			f.reply(w, 200, f.view(x))
		case r.Method == "PUT":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.puts = append(f.puts, body)
			if v, ok := body["paused"].(bool); ok {
				x.Paused = v
			}
			f.reply(w, 200, f.view(x))
		case r.Method == "DELETE":
			if !f.keepDel {
				f.runners = append(f.runners[:i], f.runners[i+1:]...)
			}
			if f.raceDelete {
				f.reply(w, 404, map[string]string{"message": "404 Runner Not Found"})
				return
			}
			w.WriteHeader(204)
		}
	case len(seg) == 3 && seg[0] == "runners" && seg[2] == "jobs" && r.Method == "GET":
		if f.jobsStatus != 0 {
			f.reply(w, f.jobsStatus, map[string]string{"message": "injected"})
			return
		}
		id, _ := strconv.ParseInt(seg[1], 10, 64)
		_, x := f.find(id)
		rows := []map[string]any{}
		if x != nil && x.Running {
			rows = append(rows, map[string]any{"id": 9001, "status": "running"})
		}
		f.reply(w, 200, rows)
	default:
		f.reply(w, 404, map[string]string{"message": "unexpected " + r.Method + " " + p})
	}
}

func (f *fakeGL) protection(w http.ResponseWriter, set map[string]bool, name string) {
	if f.garbage[name] {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html>sign in</html>"))
		return
	}
	if set[name] {
		f.reply(w, 200, map[string]any{"name": name})
		return
	}
	f.reply(w, 404, map[string]string{"message": "404 Not found"})
}

// add registers a runner the way an earlier mint would have, born at the given time.
func (f *fakeGL) add(desc string, running bool) *fakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	x := &fakeRunner{ID: f.nextID, Desc: desc, Running: running}
	f.runners = append(f.runners, x)
	return x
}

func (f *fakeGL) client(token string) *rest.Client {
	return &rest.Client{BaseURL: f.srv.URL, AllowPrivateBase: true, UserAgent: "runners-test", Provider: "gitlab",
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return token, nil }}
}

// registrar builds a Registrar over f with the given clock.
func (f *fakeGL) registrar(clk func() time.Time) *runners.Registrar {
	f.t.Helper()
	r, err := runners.New(runners.Config{Client: f.client(runnerToken), Clock: clk})
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

var repoScope = providers.RunnerScope{Repo: testProject}

func runnerReq() providers.RunnerRequest {
	return providers.RunnerRequest{Attempt: providers.Attempt{ID: "attempt-1", Trust: "untrusted"}, Scope: repoScope}
}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

func itoa64(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

func readFile(p string) (string, error) { b, err := os.ReadFile(p); return string(b), err }
