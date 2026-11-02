package webhook

// Purpose: exercise pause-on-running through the coordinator's real HTTP ingress.
// Inputs: recorded running hooks, authenticated deliveries and a fake runner-management API.
// Outputs: exact API effects and HTTP acknowledgments. Constraints: no live forge or queued work.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const runningDescription = "nself-glr-01ARZ3NDEKTSV4RRFFQ69G5FAV"

type runningAPI struct {
	mu                           sync.Mutex
	paths                        []string
	puts                         int
	paused                       bool
	jobStatus                    string
	jobRunner                    int
	jobHTTP, runnerHTTP, putHTTP int
	foreign                      bool
	missingUniqueTag             bool
}

func (a *runningAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.paths = append(a.paths, r.Method+" "+r.URL.EscapedPath())
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/jobs/9001"):
		w.WriteHeader(a.jobHTTP)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": a.jobStatus, "runner": map[string]int{"id": a.jobRunner}})
	case r.URL.Path == "/runners/42" && r.Method == "GET":
		w.WriteHeader(a.runnerHTTP)
		description := runningDescription
		if a.foreign {
			description = "persistent-runner"
		}
		tags := []string{description}
		if a.missingUniqueTag {
			tags = []string{"nself"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "description": description, "tag_list": tags, "paused": a.paused})
	case r.URL.Path == "/runners/42" && r.Method == "PUT":
		var body map[string]bool
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || !body["paused"] {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a.puts++
		w.WriteHeader(a.putHTTP)
		if a.putHTTP == http.StatusOK {
			a.paused = true
		}
		_, _ = w.Write([]byte(`{}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestRunningHookIngress(t *testing.T) {
	for _, tc := range []struct {
		name                                                       string
		edit                                                       func([]byte) []byte
		setup                                                      func(*runningAPI)
		bearer, unbound, stale, unsigned, wrongHeader, wrongSecret bool
		wantCode, wantCalls, wantPuts                              int
		project                                                    string
		noConfig                                                   bool
	}{
		{name: "legacy signed identity", wantCode: 200, wantCalls: 3, wantPuts: 1, project: "1001"},
		{name: "modern bound id beats path", edit: func(b []byte) []byte {
			b = replace(b, `"project_id":1001`, `"project_id":9999`)
			return append([]byte(`{"project":{"id":1001,"path_with_namespace":"acme/app"},`), b[1:]...)
		}, wantCode: 200, wantCalls: 3, wantPuts: 1, project: "1001"},
		{name: "unbound bearer uses verified path", bearer: true, unbound: true, wantCode: 200, wantCalls: 3, wantPuts: 1, project: "acme%2Fapp"},
		{name: "bound bearer uses id", bearer: true, wantCode: 200, wantCalls: 3, wantPuts: 1, project: "1001"},
		{name: "no credentials", unsigned: true, wantCode: 403},
		{name: "wrong signing secret", wrongSecret: true, wantCode: 403},
		{name: "wrong bearer token", bearer: true, wrongSecret: true, wantCode: 403},
		{name: "stale signature", stale: true, wantCode: 403},
		{name: "wrong event header", wrongHeader: true, wantCode: 200},
		{name: "wrong object kind", edit: func(b []byte) []byte { return replace(b, `"object_kind":"build"`, `"object_kind":"push"`) }, wantCode: 200},
		{name: "bound identity mismatch", edit: func(b []byte) []byte { return replace(b, `"project_id":1001`, `"project_id":9999`) }, wantCode: 403},
		{name: "missing runner", edit: func(b []byte) []byte { return replace(b, `"runner":{"id":42}`, `"runner":null`) }, wantCode: 200},
		{name: "invalid runner", edit: func(b []byte) []byte { return replace(b, `"id":42`, `"id":-1`) }, wantCode: 200},
		{name: "invalid job", edit: func(b []byte) []byte { return replace(b, `"build_id":9001`, `"build_id":0`) }, wantCode: 200},
		{name: "no nself job tag", edit: func(b []byte) []byte { return replace(b, `"tag_list":["nself","linux"]`, `"tag_list":["linux"]`) }, wantCode: 200, wantCalls: 3, wantPuts: 1, project: "1001"},
		{name: "no job tags", edit: func(b []byte) []byte { return replace(b, `"tag_list":["nself","linux"]`, `"tag_list":[]`) }, wantCode: 200, wantCalls: 3, wantPuts: 1, project: "1001"},
		{name: "runner tags do not count", edit: func(b []byte) []byte {
			b = replace(b, `"tag_list":["nself","linux"]`, `"tag_list":[]`)
			return replace(b, `"runner":{"id":42}`, `"runner":{"id":42,"tag_list":["nself"]}`)
		}, wantCode: 200, wantCalls: 3, wantPuts: 1, project: "1001"},
		{name: "runners not configured", noConfig: true, wantCode: 200},
		{name: "API job pending", setup: func(a *runningAPI) { a.jobStatus = "pending" }, wantCode: 200, wantCalls: 1, project: "1001"},
		{name: "API runner mismatch", setup: func(a *runningAPI) { a.jobRunner = 43 }, wantCode: 200, wantCalls: 1, project: "1001"},
		{name: "API job gone", setup: func(a *runningAPI) { a.jobHTTP = 404 }, wantCode: 200, wantCalls: 1, project: "1001"},
		{name: "API runner gone", setup: func(a *runningAPI) { a.runnerHTTP = 404 }, wantCode: 200, wantCalls: 2, project: "1001"},
		{name: "foreign runner", setup: func(a *runningAPI) { a.foreign = true }, wantCode: 200, wantCalls: 2, project: "1001"},
		{name: "operator runner with our description", setup: func(a *runningAPI) { a.missingUniqueTag = true }, wantCode: 200, wantCalls: 2, project: "1001"},
		{name: "API job failure", setup: func(a *runningAPI) { a.jobHTTP = 403 }, wantCode: 403, wantCalls: 1, project: "1001"},
		{name: "API runner failure", setup: func(a *runningAPI) { a.runnerHTTP = 403 }, wantCode: 403, wantCalls: 2, project: "1001"},
		{name: "API pause failure", setup: func(a *runningAPI) { a.putHTTP = 403 }, wantCode: 403, wantCalls: 3, wantPuts: 1, project: "1001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pol := Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv, ID: 1001}}}
			if tc.bearer {
				id := int64(1001)
				if tc.unbound {
					id = 0
				}
				pol = Secrets{Tokens: map[string]RepoSecret{"acme/app": {Env: tokenEnv, ID: id}}}
			}
			rg := newRig(t, pol)
			wire(t, rg)
			a := &runningAPI{jobStatus: "running", jobRunner: 42, jobHTTP: 200, runnerHTTP: 200, putHTTP: 200}
			if tc.setup != nil {
				tc.setup(a)
			}
			srv := httptest.NewTLSServer(http.HandlerFunc(a.serve))
			t.Cleanup(srv.Close)
			reg, err := runners.New(runners.Config{Client: &rest.Client{BaseURL: srv.URL, Provider: Name, AllowPrivateBase: true, TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig}})
			if err != nil {
				t.Fatal(err)
			}
			runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return reg, nil })
			if tc.noConfig {
				runners.UseRegistrar(nil)
			}
			t.Cleanup(func() { runners.UseRegistrar(nil) })
			in := newIntake(t, "acme/app")
			body := fixture(t, "job_running")
			if tc.edit != nil {
				body = tc.edit(body)
			}
			d := delivery{event: "Job Hook", id: "running-1", ts: rg.now, signSecret: signingSecret}
			if tc.bearer {
				d = delivery{event: "Job Hook", token: bareToken}
			}
			if tc.unsigned {
				d.signSecret = ""
			}
			if tc.wrongSecret {
				d.signSecret, d.token = otherSecret, "synthetic-wrong-token"
			}
			if tc.stale {
				d.ts = d.ts.Add(-6 * time.Minute)
			}
			if tc.wrongHeader {
				d.event = "Push Hook"
			}
			w := in.post(d, body)
			if w.Code != tc.wantCode || in.queued() != 0 {
				t.Fatalf("HTTP=%d body=%s queued=%d", w.Code, w.Body, in.queued())
			}
			if tc.wantCode == 200 && !strings.Contains(w.Body.String(), `"accepted":false`) {
				t.Fatalf("not declined: %s", w.Body)
			}
			a.mu.Lock()
			if len(a.paths) != tc.wantCalls || a.puts != tc.wantPuts {
				t.Fatalf("calls=%v puts=%d", a.paths, a.puts)
			}
			if tc.project != "" && a.paths[0] != "GET /projects/"+tc.project+"/jobs/9001" {
				t.Fatalf("unverified project address: %v", a.paths)
			}
			a.mu.Unlock()
			if tc.wantPuts == 1 && tc.wantCode == 200 {
				w = in.post(d, body)
				a.mu.Lock()
				defer a.mu.Unlock()
				if w.Code != 200 || a.puts != 1 || in.queued() != 0 {
					t.Fatalf("replay HTTP=%d puts=%d", w.Code, a.puts)
				}
			}
		})
	}
}
