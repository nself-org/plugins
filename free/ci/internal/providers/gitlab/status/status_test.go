package status_test

// Purpose: tests of the GitLab status adapter of P7-HOST-06: TestStatusGoldens (state mapping, name,
// description cut, URL shape, no token anywhere but the Authorization header) and TestReporterRouting
// (a GitLab remote resolves to this adapter, a GitHub remote never does).
// Inputs:  a TLS fake of POST /projects/:id/statuses/:sha, the report registry, a policy built from the
//          operator's scope.
// Outputs: pass or fail; the goldens are testdata/status_bodies.golden.json (UPDATE_GOLDEN=1 rewrites).
// Constraints: no live network; the token is synthetic.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/status"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const (
	testToken = "synthetic-gitlab-token-0001"
	sha       = "0123456789abcdef0123456789abcdef01234567"
)

type seen struct {
	Method, URI, Auth string
	Body              map[string]any
}

func fake(t *testing.T, answer int) (*rest.Client, func() []seen) {
	t.Helper()
	var mu sync.Mutex
	var got []seen
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		got = append(got, seen{r.Method, r.RequestURI, r.Header.Get("Authorization"), body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(answer)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := &rest.Client{BaseURL: srv.URL + "/api/v4", AllowPrivateBase: true, UserAgent: "status-test", Provider: "gitlab",
		TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return testToken, nil }}
	return c, func() []seen { mu.Lock(); defer mu.Unlock(); return append([]seen(nil), got...) }
}

type goldenCase struct {
	Name    string `json:"name"`
	State   string `json:"state_in"`
	Phase   string `json:"phase"`
	Desc    string `json:"description_in"`
	Target  string `json:"target_url_in"`
	Request string `json:"request"`
	Body    any    `json:"body"`
}

// TestStatusGoldens proves acceptance line 3 of P7-HOST-06: the status adapter posts name nself-ci with
// states pending|running|success|failed|canceled mapped from the evidence projection and a description
// of at most 140 characters (goldens). The five GitLab states, a 300 character description, a target
// URL and a multi-line description are posted and compared with testdata/status_bodies.golden.json.
func TestStatusGoldens(t *testing.T) {
	long := strings.Repeat("checks passed ", 30)
	cases := []goldenCase{
		{Name: "pending", State: "pending", Desc: "queued"},
		{Name: "running", State: "pending", Phase: "running", Desc: "running: 3/9 checks"},
		{Name: "success", State: "success", Desc: "pass: 9/9 checks passed"},
		{Name: "failed", State: "failure", Desc: "fail: 8/9 checks passed"},
		{Name: "error", State: "error", Desc: "the runner was lost"},
		{Name: "canceled", State: "failure", Phase: "canceled", Desc: "cancelled by the operator"},
		{Name: "long description", State: "success", Desc: long},
		{Name: "multi-line description", State: "pending", Desc: "line one\nline two"},
		{Name: "target url", State: "success", Desc: "pass", Target: "https://ci.example.test/run/1"},
	}
	for i := range cases {
		c, calls := fake(t, 201)
		cs := cases[i]
		rep := &status.Reporter{Client: func(context.Context) (*rest.Client, error) { return c, nil },
			Phase: func(report.Status) status.Phase { return status.Phase(cs.Phase) }}
		err := rep.Post(context.Background(), report.Status{Owner: "acme", Repo: "widgets", Revision: sha, State: report.State(cs.State), Description: cs.Desc, TargetURL: cs.Target})
		if err != nil {
			t.Fatalf("%s: %v", cs.Name, err)
		}
		got := calls()
		if len(got) != 1 || got[0].Method != "POST" || got[0].Auth != "Bearer "+testToken {
			t.Fatalf("%s: calls %+v", cs.Name, got)
		}
		cases[i].Request, cases[i].Body = got[0].URI, got[0].Body
		desc, _ := got[0].Body["description"].(string)
		if n := len([]rune(desc)); n > 140 || got[0].Body["name"] != "nself-ci" {
			t.Errorf("%s: description %d runes, name %v", cs.Name, n, got[0].Body["name"])
		}
		if strings.Contains(got[0].URI, testToken) || strings.Contains(string(mustJSON(got[0].Body)), testToken) {
			t.Errorf("%s: the token is in the URL or the body", cs.Name)
		}
	}
	raw, _ := json.MarshalIndent(cases, "", "  ")
	raw = append(raw, '\n')
	const path = "testdata/status_bodies.golden.json"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil || string(want) != string(raw) {
		t.Fatalf("goldens differ (UPDATE_GOLDEN=1 go test ./internal/providers/gitlab/status): %v\n%s", err, raw)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// TestMapNeverGreensAnUnknown holds the mapping table: an unknown report state is failed.
func TestMapNeverGreensAnUnknown(t *testing.T) {
	for in, want := range map[report.State]string{report.Pending: "pending", report.Success: "success", report.Failure: "failed", report.Error: "failed", "weird": "failed"} {
		if got := status.Map(in, status.PhaseNone); got != want {
			t.Errorf("Map(%q) = %q, want %q", in, got, want)
		}
	}
	if status.Map(report.Success, status.PhaseCanceled) != "success" || status.Map(report.Success, status.PhaseRunning) != "success" {
		t.Error("a phase must never turn a success into something else")
	}
}

// TestPostRefusesBadStatus proves the adapter validates before it sends and surfaces a funnel error.
func TestPostRefusesBadStatus(t *testing.T) {
	c, calls := fake(t, 403)
	rep := &status.Reporter{Client: func(context.Context) (*rest.Client, error) { return c, nil }}
	if err := rep.Post(context.Background(), report.Status{Owner: "a/b", Repo: "c", Revision: sha, State: report.Success}); err == nil {
		t.Error("a slash in the owner was accepted")
	}
	if len(calls()) != 0 {
		t.Error("a request was sent for an invalid status")
	}
	err := rep.Post(context.Background(), report.Status{Owner: "acme", Repo: "widgets", Revision: sha, State: report.Success})
	if err == nil || strings.Contains(err.Error(), testToken) {
		t.Errorf("403 must be an error without the token: %v", err)
	}
}

func operatorPolicy(t *testing.T, kv map[string]string) {
	t.Helper()
	reg, err := trust.NewRegistry(runtime.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceUser, Values: vals})
	if err != nil {
		t.Fatal(err)
	}
	runtime.UsePolicy(func(context.Context) (*policy.Effective, error) { return &eff, nil })
	t.Cleanup(func() { runtime.UsePolicy(nil) })
}

// TestReporterRouting proves acceptance line 3 of P7-HOST-06: the adapter is chosen for a GitLab remote
// and never for a GitHub remote, with the default api_url and with a self-managed one from the
// operator's policy, and a remote on another host matches neither.
func TestReporterRouting(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	if status.RegistrationError != nil {
		t.Fatal(status.RegistrationError)
	}
	kindOf := func(remote string) string {
		kind, _, ok := report.DefaultRegistry.Resolve(remote)
		if !ok {
			return ""
		}
		return kind
	}
	for remote, want := range map[string]string{
		"https://gitlab.com/acme/widgets.git":       "gitlab",
		"git@gitlab.com:acme/widgets.git":           "gitlab",
		"ssh://git@gitlab.com/acme/widgets.git":     "gitlab",
		"https://github.com/acme/widgets.git":       "github",
		"git@github.com:acme/widgets.git":           "github",
		"https://gitlab.com.evil.test/acme/w.git":   "",
		"https://git.example.test/acme/widgets.git": "",
		"":                                       "",
		"https://ci-bot@gitlab.com/acme/widgets": "gitlab",
		"https://notgitlab.com/acme/widgets":     "",
		"https://gitlab.lan/acme/widgets.git":    "",
	} {
		if got := kindOf(remote); got != want {
			t.Errorf("default policy: %q -> %q, want %q", remote, got, want)
		}
	}
	operatorPolicy(t, map[string]string{runtime.KeyGitLabAPIURL: `"https://gitlab.lan/api/v4"`})
	for remote, want := range map[string]string{
		"https://gitlab.lan/acme/widgets.git": "gitlab",
		"git@gitlab.lan:acme/widgets.git":     "gitlab",
		"https://gitlab.com/acme/widgets.git": "",
		"https://github.com/acme/widgets.git": "github",
	} {
		if got := kindOf(remote); got != want {
			t.Errorf("self-managed policy: %q -> %q, want %q", remote, got, want)
		}
	}
	if _, rep, _ := report.DefaultRegistry.Resolve("https://gitlab.lan/acme/widgets.git"); rep == nil {
		t.Error("no reporter for the self-managed remote")
	} else if _, isGitLab := rep.(*status.Reporter); !isGitLab {
		t.Errorf("a GitLab remote resolved to %T", rep)
	}
	if _, rep, _ := report.DefaultRegistry.Resolve("https://github.com/acme/widgets.git"); rep != nil {
		if _, isGitLab := rep.(*status.Reporter); isGitLab {
			t.Error("a GitHub remote resolved to the GitLab adapter")
		}
	}
	t.Setenv("NSELF_V15", "")
	if kind := kindOf("https://gitlab.lan/acme/widgets.git"); kind != "" {
		t.Errorf("v1.4 routed a GitLab remote to %q; it has no GitLab reporter", kind)
	}
}

// TestUsePhase proves the hook the coordinator installs reaches the registered reporter: with it a pending
// status posts as running and a failure as canceled, and removing it restores the plain mapping.
func TestUsePhase(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	_, rep, ok := report.DefaultRegistry.Resolve("https://gitlab.com/acme/widgets.git")
	gl, isGitLab := rep.(*status.Reporter)
	if !ok || !isGitLab {
		t.Fatalf("no registered reporter: %T", rep)
	}
	c, calls := fake(t, 201)
	gl.Client = func(context.Context) (*rest.Client, error) { return c, nil }
	t.Cleanup(func() { gl.Client = nil; status.UsePhase(nil) })
	post := func(s report.State) string {
		if err := gl.Post(context.Background(), report.Status{Owner: "acme", Repo: "widgets", Revision: sha, State: s, Description: "d"}); err != nil {
			t.Fatal(err)
		}
		got := calls()
		return got[len(got)-1].Body["state"].(string)
	}
	if post(report.Pending) != "pending" || post(report.Failure) != "failed" {
		t.Fatal("plain mapping")
	}
	status.UsePhase(func(s report.Status) status.Phase {
		if s.State == report.Pending {
			return status.PhaseRunning
		}
		return status.PhaseCanceled
	})
	if post(report.Pending) != "running" || post(report.Failure) != "canceled" || post(report.Success) != "success" {
		t.Error("the hook did not refine the states")
	}
	status.UsePhase(nil)
	if post(report.Pending) != "pending" {
		t.Error("removing the hook did not restore the mapping")
	}
}

// TestPostNestedProject proves D-1345: a project in a subgroup (Owner "group", Repo "sub/widgets") gets
// its commit status at the full, escaped project path.
func TestPostNestedProject(t *testing.T) {
	c, calls := fake(t, 201)
	rep := &status.Reporter{Client: func(context.Context) (*rest.Client, error) { return c, nil }}
	if err := rep.Post(context.Background(), report.Status{Owner: "group", Repo: "sub/widgets", Revision: sha, State: report.Success}); err != nil {
		t.Fatalf("nested project refused: %v", err)
	}
	got := calls()
	if len(got) != 1 || !strings.Contains(got[0].URI, "group%2Fsub%2Fwidgets/statuses/"+sha) {
		t.Fatalf("requests: %+v", got)
	}
}
