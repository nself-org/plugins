package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPAT = "fake-pat-0123456789-not-a-token"

// fakeGitHub is an httptest GitHub API. groups is served 100 per page from
// the query's page number; repoPrivate answers GET /repos/o/r.
type fakeGitHub struct {
	mu          sync.Mutex
	groups      []runnerGroup
	runners     map[string][]string // group name -> names of the runners registered in it
	repoPrivate bool
	status      int // non-zero forces this status on every request
	tokenCalls  int
	paths       []string
	auth        []string
}

func newFakeGitHub(t *testing.T, f *fakeGitHub) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.paths = append(f.paths, r.URL.Path)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/actions/runner-groups/") && strings.HasSuffix(r.URL.Path, "/runners"):
			var names []string
			for i, g := range f.groups {
				if strings.Contains(r.URL.Path, "/runner-groups/"+strconv.Itoa(i+1)+"/") {
					names = f.runners[g.Name]
				}
			}
			serveRunners(w, names)
		case strings.HasSuffix(r.URL.Path, "/actions/runners"):
			var names []string
			for _, g := range f.groups {
				names = append(names, f.runners[g.Name]...)
			}
			serveRunners(w, names)
		case strings.HasSuffix(r.URL.Path, "/actions/runner-groups"):
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			lo, hi := (page-1)*groupsPageSize, page*groupsPageSize
			if lo > len(f.groups) {
				lo = len(f.groups)
			}
			if hi > len(f.groups) {
				hi = len(f.groups)
			}
			pageGroups := append([]runnerGroup(nil), f.groups[lo:hi]...)
			for i := range pageGroups {
				pageGroups[i].ID = int64(lo + i + 1)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"total_count": len(f.groups), "runner_groups": pageGroups})
		case strings.HasSuffix(r.URL.Path, "/registration-token"):
			f.tokenCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"FAKEREGTOKEN"}`))
		case r.URL.Path == "/repos/acme/app":
			_ = json.NewEncoder(w).Encode(map[string]bool{"private": f.repoPrivate})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = old; srv.Close() })
}

func serveRunners(w http.ResponseWriter, names []string) {
	type runner struct {
		Name string `json:"name"`
	}
	out := []runner{}
	for _, n := range names {
		out = append(out, runner{n})
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"total_count": len(out), "runners": out})
}

// trustEnv sets the guard environment and a runner dir whose config.sh and
// run.sh record that they ran (marker files) and their arguments.
func trustEnv(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"" + dir + "/$(basename \"$0\").ran\"\n"
	for _, n := range []string{"config.sh", "run.sh"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GITHUB_RUNNER_DIR", dir)
	t.Setenv("GITHUB_RUNNER_LOG_PATH", filepath.Join(dir, "runner.log"))
	t.Setenv("GITHUB_RUNNER_PAT", testPAT)
	t.Setenv("GITHUB_RUNNER_ORG", "acme")
	t.Setenv("GITHUB_RUNNER_NAME", "n1")
	for _, k := range []string{"GITHUB_RUNNER_GROUP", "GITHUB_RUNNER_SCOPE", "GITHUB_RUNNER_REPO", "GITHUB_RUNNER_ALLOW_PUBLIC_REPOS", "ENV"} {
		t.Setenv(k, "")
	}
	return dir
}

func call(h http.HandlerFunc, method string) (*httptest.ResponseRecorder, map[string]interface{}) {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/", nil))
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func ran(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name+".ran"))
	return err == nil
}

func TestRegister_PublicGroupRefused(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)}}}
	newFakeGitHub(t, f)
	dir := trustEnv(t)
	logs := captureLog(t)

	rec, body := call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	if rec.Code != http.StatusConflict || body["code"] != "runner_group_public" {
		t.Fatalf("want 409 runner_group_public, got %d %v", rec.Code, body)
	}
	if ran(dir, "config.sh") {
		t.Error("config.sh ran for a public group")
	}
	if f.tokenCalls != 0 {
		t.Error("registration token requested before the guard passed")
	}
	if strings.Contains(rec.Body.String()+logs.String(), testPAT) {
		t.Error("PAT leaked into response or log")
	}
	if !strings.Contains(fmt.Sprint(body["error"]), "GITHUB_RUNNER_ALLOW_PUBLIC_REPOS") {
		t.Errorf("message must name the override: %v", body["error"])
	}
}

func TestRegister_OverrideWarnsAndProceeds(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)}}}
	newFakeGitHub(t, f)
	dir := trustEnv(t)
	t.Setenv("GITHUB_RUNNER_ALLOW_PUBLIC_REPOS", "true")
	logs := captureLog(t)

	rec, body := call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	if rec.Code != http.StatusOK || !ran(dir, "config.sh") {
		t.Fatalf("override must register: %d %v", rec.Code, body)
	}
	if !strings.Contains(logs.String(), "WARNING") || strings.Contains(logs.String(), testPAT) {
		t.Errorf("want a warning without the PAT, log: %s", logs.String())
	}
	if body["warning"] == "" {
		t.Error("response should carry the warning")
	}
}

func TestRegister_PrivateGroupProceeds(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true, AllowsPublicRepositories: pb(false)}, {Name: "ci", AllowsPublicRepositories: pb(false)}}}
	newFakeGitHub(t, f)
	dir := trustEnv(t)
	t.Setenv("GITHUB_RUNNER_GROUP", "CI") // case-insensitive match

	rec, _ := call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	args, _ := os.ReadFile(filepath.Join(dir, "config.sh.ran"))
	if rec.Code != http.StatusOK || !strings.Contains(string(args), "--runnergroup ci") {
		t.Fatalf("private group must register in group ci: %d args=%q", rec.Code, args)
	}
	if !strings.Contains(strings.Join(f.auth, ","), "Bearer "+testPAT) {
		t.Error("API calls must be authenticated")
	}
}

func TestCheckTrust_EmptyGroupResolvesDefaultAcrossPages(t *testing.T) {
	var groups []runnerGroup
	for i := 0; i < groupsPageSize; i++ {
		groups = append(groups, runnerGroup{Name: fmt.Sprintf("g%d", i), AllowsPublicRepositories: pb(false)})
	}
	groups = append(groups, runnerGroup{Name: "Everyone", Default: true, AllowsPublicRepositories: pb(true)})
	f := &fakeGitHub{groups: groups}
	newFakeGitHub(t, f)
	trustEnv(t)

	_, err := CheckTrust(t.Context(), TrustConfigFromEnv())
	te, ok := err.(*TrustError)
	if !ok || te.Code != CodeRunnerGroupPublic || !strings.Contains(te.Message, "Everyone") {
		t.Fatalf("default group on page 2 must be found and refused, got %v", err)
	}
	if len(f.paths) != 2 {
		t.Errorf("want 2 pages fetched, got %v", f.paths)
	}
}

func TestCheckTrust_RepoScope(t *testing.T) {
	f := &fakeGitHub{repoPrivate: false}
	newFakeGitHub(t, f)
	dir := trustEnv(t)
	t.Setenv("GITHUB_RUNNER_SCOPE", "repo")
	t.Setenv("GITHUB_RUNNER_REPO", "app")

	rec, body := call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	if rec.Code != http.StatusConflict || body["code"] != "runner_group_public" || ran(dir, "config.sh") {
		t.Fatalf("public repo must be refused: %d %v", rec.Code, body)
	}

	f.repoPrivate = true
	rec, _ = call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	args, _ := os.ReadFile(filepath.Join(dir, "config.sh.ran"))
	if rec.Code != http.StatusOK || !strings.Contains(string(args), "https://github.com/acme/app") || strings.Contains(string(args), "--runnergroup") {
		t.Fatalf("private repo registers against the repo URL without a group: %d %q", rec.Code, args)
	}
	if f.tokenCalls != 1 || f.paths[len(f.paths)-1] != "/repos/acme/app/actions/runners/registration-token" {
		t.Errorf("repo scope must use the repo token endpoint: %v", f.paths)
	}
}

func TestGuard_FailsClosedOnAPIError(t *testing.T) {
	f := &fakeGitHub{status: http.StatusForbidden}
	newFakeGitHub(t, f)
	dir := trustEnv(t)

	rec, body := call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	if rec.Code != http.StatusBadGateway || body["code"] != "runner_trust_unverifiable" || ran(dir, "config.sh") {
		t.Fatalf("API failure must refuse: %d %v", rec.Code, body)
	}
	rec, body = call(NewHandler(NewRunnerState()).Start, http.MethodPost)
	if rec.Code != http.StatusBadGateway || ran(dir, "run.sh") {
		t.Fatalf("Start must refuse when the check fails: %d %v", rec.Code, body)
	}
}

func TestStart_RefusesPublicGroupAndHonoursOverride(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)}}}
	newFakeGitHub(t, f)
	dir := trustEnv(t)
	h := NewHandler(NewRunnerState())

	rec, body := call(h.Start, http.MethodPost)
	if rec.Code != http.StatusConflict || body["code"] != "runner_group_public" || ran(dir, "run.sh") {
		t.Fatalf("Start must refuse a public group: %d %v", rec.Code, body)
	}

	t.Setenv("GITHUB_RUNNER_ALLOW_PUBLIC_REPOS", "true")
	rec, _ = call(h.Start, http.MethodPost)
	deadline := time.Now().Add(5 * time.Second)
	for !ran(dir, "run.sh") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if rec.Code != http.StatusOK || !ran(dir, "run.sh") {
		t.Fatalf("override must start the runner: %d", rec.Code)
	}
}

func TestStatus_StopsRunnerWhenGroupTurnsPublic(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true, AllowsPublicRepositories: pb(false)}}}
	newFakeGitHub(t, f)
	trustEnv(t)

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	state := NewRunnerState()
	state.Running, state.PID = true, cmd.Process.Pid
	h := NewHandler(state)

	rec, body := call(h.Status, http.MethodGet)
	trust := body["trust"].(map[string]interface{})
	if rec.Code != http.StatusOK || trust["ok"] != true || trust["stopped_runner"] == true {
		t.Fatalf("private group leaves the runner alone: %v", body)
	}

	f.mu.Lock()
	f.groups[0].AllowsPublicRepositories = pb(true)
	f.mu.Unlock()
	t.Setenv("GITHUB_RUNNER_ALLOW_PUBLIC_REPOS", "true")
	_, body = call(h.Status, http.MethodGet)
	if body["trust"].(map[string]interface{})["stopped_runner"] == true {
		t.Fatal("override must not stop the runner")
	}

	t.Setenv("GITHUB_RUNNER_ALLOW_PUBLIC_REPOS", "")
	_, body = call(h.Status, http.MethodGet)
	trust = body["trust"].(map[string]interface{})
	if trust["code"] != "runner_group_public" || trust["stopped_runner"] != true {
		t.Fatalf("flipped group must stop the runner: %v", trust)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner process was not signalled")
	}
}

func TestDeployHostWarning(t *testing.T) {
	for env, want := range map[string]bool{"prod": true, "production": true, "STAGING": true, " Prod ": true, "dev": false, "development": false, "": false} {
		t.Setenv("ENV", env)
		h := NewHandler(NewRunnerState())
		_, health := call(h.Health, http.MethodGet)
		if _, got := health["deploy_host_warning"]; got != want {
			t.Errorf("ENV=%q health warning present=%v, want %v", env, got, want)
		}
		if (deployHostWarning() != "") != want {
			t.Errorf("ENV=%q deployHostWarning mismatch", env)
		}
	}
}

func pb(b bool) *bool { return &b }

func TestCheckTrust_MissingFlagIsUnverifiable(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true}}}
	newFakeGitHub(t, f)
	trustEnv(t)

	_, err := CheckTrust(t.Context(), TrustConfigFromEnv())
	te, ok := err.(*TrustError)
	if !ok || te.Code != CodeTrustUnverifiable {
		t.Fatalf("a group without allows_public_repositories must fail closed, got %v", err)
	}
}

func TestCheckTrust_UnknownScopeFailsClosed(t *testing.T) {
	cfg := TrustConfig{PAT: testPAT, Org: "acme", Scope: "repository", Repo: "app"}
	_, err := CheckTrust(context.Background(), cfg)
	var te *TrustError
	if !errors.As(err, &te) || te.Code != CodeTrustUnverifiable {
		t.Fatalf("unknown scope must be unverifiable: %v", err)
	}
}

func TestStart_StopsRunningRunnerWhenGroupTurnsPublic(t *testing.T) {
	f := &fakeGitHub{groups: []runnerGroup{{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)}}}
	newFakeGitHub(t, f)
	trustEnv(t)
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	state := NewRunnerState()
	state.Running, state.PID = true, cmd.Process.Pid
	h := NewHandler(state)

	rec, body := call(h.Start, http.MethodPost)
	if rec.Code != http.StatusConflict || body["code"] != "runner_group_public" {
		t.Fatalf("Start on a running runner must run the guard first: %d %v", rec.Code, body)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("running runner was not signalled")
	}
}

// The guard checks the configured group; a runner registered in another group
// (config changed without re-registering) must not pass on that check.
func TestCheckTrust_RunnerRegisteredInAnotherGroupIsRefused(t *testing.T) {
	f := &fakeGitHub{
		groups: []runnerGroup{
			{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)},
			{Name: "CI", AllowsPublicRepositories: pb(false)},
		},
		runners: map[string][]string{"Default": {"n1"}},
	}
	newFakeGitHub(t, f)
	trustEnv(t)
	t.Setenv("GITHUB_RUNNER_GROUP", "CI")

	_, err := CheckTrust(t.Context(), TrustConfigFromEnv())
	var te *TrustError
	if !errors.As(err, &te) || te.Code != CodeRunnerGroupMismatch || te.Status != http.StatusConflict {
		t.Fatalf("a runner in the public Default group must be refused when CI is configured, got %v", err)
	}

	// registered in the configured group: passes
	f.runners = map[string][]string{"CI": {"other", "n1"}}
	if res, err := CheckTrust(t.Context(), TrustConfigFromEnv()); err != nil || res.Group != "CI" {
		t.Fatalf("a runner in the configured group must pass: %v", err)
	}
	// not registered anywhere yet: passes (it is about to be)
	f.runners = map[string][]string{"CI": {"other"}}
	if _, err := CheckTrust(t.Context(), TrustConfigFromEnv()); err != nil {
		t.Fatalf("an unregistered runner must pass: %v", err)
	}
	// registering replaces the runner into the configured group: no mismatch check
	f.runners = map[string][]string{"Default": {"n1"}}
	cfg := TrustConfigFromEnv()
	cfg.Registering = true
	if _, err := CheckTrust(t.Context(), cfg); err != nil {
		t.Fatalf("re-registering must be able to move the runner: %v", err)
	}
}

func TestRegister_MovesRunnerRegisteredInAnotherGroup(t *testing.T) {
	f := &fakeGitHub{
		groups: []runnerGroup{
			{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)},
			{Name: "CI", AllowsPublicRepositories: pb(false)},
		},
		runners: map[string][]string{"Default": {"n1"}},
	}
	newFakeGitHub(t, f)
	dir := trustEnv(t)
	t.Setenv("GITHUB_RUNNER_GROUP", "CI")

	rec, _ := call(NewHandler(NewRunnerState()).Register, http.MethodPost)
	args, _ := os.ReadFile(filepath.Join(dir, "config.sh.ran"))
	if rec.Code != http.StatusOK || !strings.Contains(string(args), "--runnergroup CI") {
		t.Fatalf("register must move the runner into CI: %d args=%q", rec.Code, args)
	}
}

func TestStart_StopsRunnerRegisteredInAnotherGroup(t *testing.T) {
	f := &fakeGitHub{
		groups: []runnerGroup{
			{Name: "Default", Default: true, AllowsPublicRepositories: pb(true)},
			{Name: "CI", AllowsPublicRepositories: pb(false)},
		},
		runners: map[string][]string{"Default": {"n1"}},
	}
	newFakeGitHub(t, f)
	trustEnv(t)
	t.Setenv("GITHUB_RUNNER_GROUP", "CI")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	state := NewRunnerState()
	state.Running, state.PID = true, cmd.Process.Pid

	rec, body := call(NewHandler(state).Start, http.MethodPost)
	if rec.Code != http.StatusConflict || body["code"] != CodeRunnerGroupMismatch {
		t.Fatalf("Start must refuse a runner in another group: %d %v", rec.Code, body)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("running runner was not signalled")
	}
}
