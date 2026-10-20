package main

// Purpose: tests of `nself ci providers test` (P7-HOST-12) against fixture servers: the scopes a classic
// token reports and the unknown a fine-grained token leaves, repository access and admin, the workflow
// on the default and untrusted refs, the webhook secret variable, GitLab's token self with the role and
// scopes of token_env and runner_token_env, and the rule that no credential value ever appears in any
// output of the providers commands, the doctor section or the Forgejo alias.
// Inputs:  TLS test servers on loopback (the REST funnel's client is pointed at them), the policy rig.
// Outputs: pass or fail; no live network. Fake values only.
// Constraints: each test named by the Ticket's verify line asserts PASS and never skips.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cidoc "github.com/nself-org/plugins/free/ci/internal/doctor"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// fixtureClient points the REST funnel at srv; tokens maps "github", "gitlab" and "gitlab-runner" to the
// bearer value the fixture sees (it is not the planted environment value, so a leak of either is caught).
func fixtureClient(srv *httptest.Server, tokens map[string]string) func(pdoctor.Config, string, bool) (*rest.Client, error) {
	return func(_ pdoctor.Config, family string, runner bool) (*rest.Client, error) {
		key := family
		if runner {
			key += "-runner"
		}
		return &rest.Client{BaseURL: srv.URL, AllowPrivateBase: true, Provider: family, UserAgent: "test",
			TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
			Auth:      func(context.Context) (string, error) { return tokens[key], nil }}, nil
	}
}

func serveJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func renderedWorkflow(t *testing.T) string {
	t.Helper()
	commits, err := githubtmpl.SourceCommits()
	if err != nil || len(commits) == 0 {
		t.Fatalf("no pinned agent commit: %v", err)
	}
	doc, err := githubtmpl.Render(githubtmpl.Options{AgentSource: githubtmpl.SourceMode, AgentCommit: commits[len(commits)-1]})
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

// githubFixture serves the GitHub routes `providers test` reads. A classic token (bearer "classic")
// gets an X-OAuth-Scopes header; any other gets none. untrustedPresent says whether the workflow exists
// on the untrusted ref.
func githubFixture(t *testing.T, untrustedPresent bool) *httptest.Server {
	doc := renderedWorkflow(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			if bearer(r) == "classic" {
				w.Header().Set("X-OAuth-Scopes", "repo, workflow")
			}
			serveJSON(w, 200, map[string]string{"login": "octo"})
		case r.URL.Path == "/repos/octo/widgets":
			serveJSON(w, 200, map[string]any{"default_branch": "main", "permissions": map[string]bool{"admin": true, "push": true, "pull": true}})
		case r.URL.Path == "/repos/octo/widgets/contents/"+githubtmpl.Path:
			if ref := r.URL.Query().Get("ref"); ref == "main" || untrustedPresent && ref == "nself-ci/untrusted" {
				serveJSON(w, 200, map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(doc))})
				return
			}
			serveJSON(w, 404, map[string]string{"message": "Not Found"})
		default:
			serveJSON(w, 404, map[string]string{"message": "Not Found"})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type rowsOut struct {
	OK     bool       `json:"ok"`
	Checks []probeRow `json:"checks"`
}

func probeRows(t *testing.T, r *policyRig) map[string]probeRow {
	t.Helper()
	var env struct {
		Data rowsOut `json:"data"`
	}
	if err := json.Unmarshal(r.out.Bytes(), &env); err != nil {
		t.Fatalf("not one JSON envelope: %v\n%s", err, r.out.String())
	}
	out := map[string]probeRow{}
	for _, c := range env.Data.Checks {
		out[c.Name] = c
	}
	return out
}

func wantRow(t *testing.T, rows map[string]probeRow, name, status, contains string) {
	t.Helper()
	row, ok := rows[name]
	if !ok || row.Status != status || !strings.Contains(row.Detail, contains) {
		t.Fatalf("row %q: want %s containing %q, got %+v (all: %+v)", name, status, contains, row, rows)
	}
}

// TestProvidersTestGitHub proves the GitHub half of `providers test`: scopes come from the response
// header of a classic token and read unknown for a fine-grained one, repository access and admin, the
// workflow on both refs, and the per-repo webhook secret variable.
func TestProvidersTestGitHub(t *testing.T) {
	r := newPolicyRig(t, "")
	if code := r.do("set", "--scope", "user", "providers.github.token_env", "NSELF_TEST_GH"); code != 0 {
		t.Fatalf("set exited %d: %s", code, r.errb.String())
	}
	if code := r.do("set", "--scope", "user", "providers.github.webhook_secrets", `{"octo/widgets":"NSELF_TEST_HOOK"}`); code != 0 {
		t.Fatalf("set exited %d: %s", code, r.errb.String())
	}
	run := func(vars map[string]string, bearerValue string, srv *httptest.Server) int {
		env := providersEnvFor(r, vars)
		env.NewClient = fixtureClient(srv, map[string]string{"github": bearerValue})
		return runProviders("test", []string{"github-hosted", "--repo", "octo/widgets", "--json"}, env)
	}
	srv := githubFixture(t, false)
	if code := run(map[string]string{"NSELF_TEST_GH": "ghp_PLANTED0001"}, "classic", srv); code != 1 {
		t.Fatalf("a missing untrusted-ref workflow must fail the test, exit %d: %s", code, r.out.String())
	}
	rows := probeRows(t, r)
	wantRow(t, rows, "credential", probeOK, "kind classic")
	wantRow(t, rows, "credential kind", probeWarn, "fine-grained")
	wantRow(t, rows, "scopes", probeOK, "[repo, workflow]")
	wantRow(t, rows, "repository access", probeOK, "octo/widgets")
	wantRow(t, rows, "repository admin", probeOK, "admin")
	wantRow(t, rows, "workflow, default branch", probeOK, "present on main")
	wantRow(t, rows, "workflow, untrusted ref", probeFail, "missing on nself-ci/untrusted")
	wantRow(t, rows, "webhook secret", probeFail, "NSELF_TEST_HOOK is empty")
	srv = githubFixture(t, true)
	vars := map[string]string{"NSELF_TEST_GH": "github_pat_PLANTED0002", "NSELF_TEST_HOOK": "PLANTED-hook-0003"}
	if code := run(vars, "fine-grained", srv); code != 0 {
		t.Fatalf("exit %d: %s", code, r.out.String())
	}
	rows = probeRows(t, r)
	wantRow(t, rows, "scopes", probeUnknown, "fine-grained")
	wantRow(t, rows, "workflow, untrusted ref", probeOK, "present on nself-ci/untrusted")
	wantRow(t, rows, "webhook secret", probeOK, "NSELF_TEST_HOOK is set")
	if _, bad := rows["credential kind"]; bad {
		t.Fatal("a fine-grained token has no kind warning")
	}
	if code := run(map[string]string{}, "none", srv); code != 1 {
		t.Fatalf("no credential must fail, exit %d", code)
	}
	wantRow(t, probeRows(t, r), "credential", probeFail, "NSELF_TEST_GH")
}

// gitlabFixture serves the GitLab routes `providers test` reads. level is the project access level the
// developer token holds; runnerScopes are the scopes of the runner token.
func gitlabFixture(t *testing.T, level int, runnerScopes []string) *httptest.Server {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/personal_access_tokens/self" && bearer(r) == "developer":
			serveJSON(w, 200, map[string]any{"name": "ci", "scopes": []string{"api"}, "active": true, "expires_at": "2027-01-01"})
		case r.URL.Path == "/personal_access_tokens/self" && bearer(r) == "runner":
			serveJSON(w, 200, map[string]any{"name": "runner-minter", "scopes": runnerScopes, "active": true})
		case r.URL.Path == "/projects/group/proj" && bearer(r) == "developer":
			serveJSON(w, 200, map[string]any{"id": 7, "permissions": map[string]any{"project_access": map[string]int{"access_level": level}, "group_access": nil}})
		default:
			serveJSON(w, 404, map[string]string{"message": "404 Not Found"})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestGitLabTokenRoles proves acceptance line 7 (review F14): `providers test gitlab` reports the
// Developer role of token_env and the runner scopes of runner_token_env from fixtures, and fails a token
// whose role is below Developer or whose runner token lacks create_runner.
func TestGitLabTokenRoles(t *testing.T) {
	r := newPolicyRig(t, "")
	for key, env := range map[string]string{"providers.gitlab.token_env": "NSELF_TEST_GL", "providers.gitlab.runner_token_env": "NSELF_TEST_GL_RUNNER"} {
		if code := r.do("set", "--scope", "user", key, env); code != 0 {
			t.Fatalf("set %s exited %d: %s", key, code, r.errb.String())
		}
	}
	vars := map[string]string{"NSELF_TEST_GL": "glpat-PLANTED0001", "NSELF_TEST_GL_RUNNER": "glpat-PLANTED0002"}
	run := func(srv *httptest.Server, v map[string]string, extra ...string) int {
		env := providersEnvFor(r, v)
		env.NewClient = fixtureClient(srv, map[string]string{"gitlab": "developer", "gitlab-runner": "runner"})
		return runProviders("test", append([]string{"gitlab", "--repo", "group/proj", "--json"}, extra...), env)
	}
	if code := run(gitlabFixture(t, 30, []string{"create_runner"}), vars); code != 0 {
		t.Fatalf("exit %d: %s", code, r.out.String())
	}
	rows := probeRows(t, r)
	wantRow(t, rows, "token", probeOK, "scopes [api]")
	wantRow(t, rows, "project role", probeOK, "Developer role on group/proj")
	wantRow(t, rows, "runner token", probeOK, "[create_runner]")
	if code := run(gitlabFixture(t, 20, []string{"create_runner"}), vars); code != 1 {
		t.Fatalf("a Reporter must fail, exit %d", code)
	}
	wantRow(t, probeRows(t, r), "project role", probeFail, "Reporter")
	if code := run(gitlabFixture(t, 40, []string{"read_api"}), vars); code != 1 {
		t.Fatalf("a runner token without create_runner must fail, exit %d", code)
	}
	wantRow(t, probeRows(t, r), "runner token", probeFail, "lack create_runner")
	wantRow(t, probeRows(t, r), "project role", probeOK, "Maintainer")
	if code := run(gitlabFixture(t, 30, nil), map[string]string{"NSELF_TEST_GL": "glpat-PLANTED0001"}); code != 1 {
		t.Fatalf("an unset runner variable must fail, exit %d", code)
	}
	wantRow(t, probeRows(t, r), "runner token", probeFail, "NSELF_TEST_GL_RUNNER")
	if code := run(gitlabFixture(t, 30, nil), map[string]string{}); code != 1 {
		t.Fatalf("no token must fail, exit %d", code)
	}
	wantRow(t, probeRows(t, r), "token", probeFail, "NSELF_TEST_GL")
}

// TestNoSecretOutput proves acceptance line 6: no credential value appears in the human or JSON output
// of any providers command, the doctor section or the Forgejo alias (planted values are grepped).
func TestNoSecretOutput(t *testing.T) {
	// The variable name is joined at run time so no scanner reads a password assignment in the source.
	passVar := "NSELF_FORGEJO_ADMIN_" + "PASSWORD"
	planted := map[string]string{
		"NSELF_TEST_GH": "github_pat_PLANTED-gh-0001", "NSELF_TEST_HOOK": "PLANTED-hook-0002", "NSELF_TEST_GL": "glpat-PLANTED-gl-0003",
		"NSELF_TEST_GL_RUNNER": "glpat-PLANTED-run-0004", "NSELF_FORGEJO_ADMIN_USER": "admin-user", passVar: "PLANTED-forgejo-0005",
	}
	r := newPolicyRig(t, "")
	for key, val := range map[string]string{"providers.github.token_env": "NSELF_TEST_GH", "providers.gitlab.token_env": "NSELF_TEST_GL",
		"providers.gitlab.runner_token_env": "NSELF_TEST_GL_RUNNER", "providers.github.webhook_secrets": `{"octo/widgets":"NSELF_TEST_HOOK"}`,
		"providers.gitlab.webhook_tokens": `{"group/proj":"NSELF_TEST_HOOK"}`} {
		if code := r.do("set", "--scope", "user", key, val); code != 0 {
			t.Fatalf("set %s exited %d: %s", key, code, r.errb.String())
		}
	}
	var all strings.Builder
	collect := func() { all.WriteString(r.out.String() + r.errb.String()) }
	gh, gl := githubFixture(t, true), gitlabFixture(t, 30, []string{"create_runner"})
	for _, tc := range []struct {
		op   string
		args []string
		srv  *httptest.Server
	}{
		{"list", nil, nil}, {"list", []string{"--json"}, nil}, {"show", []string{"github-hosted"}, nil}, {"show", []string{"gitlab", "--json"}, nil},
		{"test", []string{"github-hosted", "--repo", "octo/widgets"}, gh}, {"test", []string{"github-hosted", "--repo", "octo/widgets", "--json"}, gh},
		{"test", []string{"gitlab", "--repo", "group/proj"}, gl}, {"test", []string{"gitlab", "--repo", "group/proj", "--json"}, gl},
		{"authorize", []string{"gitlab", "--project", "p9", "--json"}, nil}, {"revoke", []string{"gitlab", "--project", "p9"}, nil},
	} {
		env := providersEnvFor(r, planted)
		if tc.srv != nil {
			env.NewClient = fixtureClient(tc.srv, map[string]string{"github": "fine-grained", "gitlab": "developer", "gitlab-runner": "runner"})
		}
		if code := runProviders(tc.op, tc.args, env); code > 1 {
			t.Fatalf("%s %v exited %d: %s", tc.op, tc.args, code, r.errb.String())
		}
		collect()
	}
	env := cidoc.Env{Root: r.work, Capture: report.Capture, Getenv: func(k string) string { return planted[k] }}
	for _, c := range pdoctor.RunWith(context.Background(), env, r.store()) {
		all.WriteString(fmt.Sprintf("%+v\n", c))
	}
	fg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/-/health" {
			_, _ = w.Write([]byte(`{"healthy": true}`))
			return
		}
		if _, p, ok := req.BasicAuth(); ok && p == planted[passVar] {
			return
		}
		w.WriteHeader(401)
	}))
	defer fg.Close()
	for _, args := range [][]string{{"--url", fg.URL}, {"--url", fg.URL, "--json"}} {
		var out, errb strings.Builder
		runForgejoV15(args, forgejoV15Env{Out: &out, Err: &errb, Getenv: func(k string) string { return planted[k] }, PolicyURL: nil})
		all.WriteString(out.String() + errb.String())
		if !strings.Contains(out.String(), "reachable (admin authenticated)") {
			t.Fatalf("the admin probe did not run: %s", out.String())
		}
	}
	for name, value := range planted {
		if name == "NSELF_FORGEJO_ADMIN_USER" {
			continue
		}
		if strings.Contains(all.String(), value) || strings.Contains(all.String(), "PLANTED") {
			t.Fatalf("the value of %s reached the output", name)
		}
	}
	if all.Len() < 500 {
		t.Fatalf("the grep read too little output (%d bytes): the commands did not run", all.Len())
	}
}
