package runners_test

// Purpose: the P7-HOST-07 registrar acceptance tests. Each test's doc comment cites the acceptance line it
// proves (the Ticket's acceptance list: 1 conformance, 2 sim, 3 no leak, 4 access level and fixed tags,
// 5 smoke, 6 image pin).
// Inputs:  the fixtures under testdata/ (GitLab REST v4 docs shapes) and fakeGL.
// Outputs: pass or fail.
// Constraints: no live network; every credential value is synthetic.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/conformance"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

// scoped fills the empty scope and the "runner-1" ref the conformance kit sends, so the kit reaches fixtures.
type scoped struct{ *runners.Registrar }

func (s scoped) fill(sc providers.RunnerScope) providers.RunnerScope {
	if sc.Repo == "" {
		sc.Repo = testProject
	}
	return sc
}
func (s scoped) Mint(ctx context.Context, r providers.RunnerRequest) (providers.RunnerCredential, error) {
	r.Scope = s.fill(r.Scope)
	return s.Registrar.Mint(ctx, r)
}
func (s scoped) Deregister(ctx context.Context, r providers.RunnerRef) error {
	if r.ID == "runner-1" {
		r.ID = "101"
	}
	r.Scope = s.fill(r.Scope)
	return s.Registrar.Deregister(ctx, r)
}
func (s scoped) Orphans(ctx context.Context, sc providers.RunnerScope, t time.Time) ([]providers.RunnerRef, error) {
	return s.Registrar.Orphans(ctx, s.fill(sc), t)
}
func (s scoped) Restriction(ctx context.Context, sc providers.RunnerScope, g int64) (providers.Restriction, error) {
	return s.Registrar.Restriction(ctx, s.fill(sc), g)
}

// TestConformance proves acceptance 1: the registrar passes conformance.Run on recorded-shape fixtures
// (create, delete including 404, list by description, verify absence). The kit's replay server speaks
// plain HTTP and the funnel refuses it, so a TLS front relays to it.
func TestConformance(t *testing.T) {
	conformance.Run(t, conformance.Subject{
		Name: "gitlab-runners", Fixtures: "testdata/runners",
		Secrets: [][]byte{[]byte(runnerToken), []byte("glrt-fixture-0001")},
		New: func(base string, clk conformance.Clock) providers.Provider {
			u, err := url.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			front := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(u))
			t.Cleanup(front.Close)
			f := &fakeGL{srv: front}
			reg, err := runners.New(runners.Config{Client: f.client(runnerToken), Clock: clk.Now})
			if err != nil {
				t.Fatal(err)
			}
			return scoped{reg}
		},
	})
}

// TestFixedTags proves acceptance 4 (the tag half): the tag list is the operator's tags plus one unique
// nself-glr-<ulid>, run_untagged is false, locked is true and the runner is a project_type runner at the
// numeric project id; nothing the request carries is copied; two mints never share a name.
func TestFixedTags(t *testing.T) {
	f := newFakeGL(t)
	r, err := runners.New(runners.Config{Client: f.client(runnerToken), Clock: clock, Settings: runners.Settings{Tags: []string{"nself", "fast"}}})
	if err != nil {
		t.Fatal(err)
	}
	req := runnerReq()
	req.Labels = []string{"ubuntu-latest", "arm64", "docker"} // a demand's labels are never copied
	a, err := r.MintRunner(context.Background(), req, runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.MintRunner(context.Background(), req, runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body := f.mints[0]
	tags := body["tag_list"].([]any)
	if body["runner_type"] != "project_type" || body["project_id"] != float64(testProjectID) || body["run_untagged"] != false || body["locked"] != true ||
		len(tags) != 3 || tags[0] != "nself" || tags[1] != "fast" || tags[2] != body["description"] || !strings.HasPrefix(tags[2].(string), runners.RunnerPrefix) {
		t.Fatalf("registration body %v", body)
	}
	if a.Description == b.Description || a.RunnerID == b.RunnerID || a.Credential.Value == b.Credential.Value || a.Credential.Kind != "gitlab-runner-token" {
		t.Fatalf("two mints must differ: %v %v", a, b)
	}
	if got := a.Credential.ExpiresAt.Sub(epoch); got != runners.CredentialLifetime {
		t.Fatalf("expiry %v", got)
	}
	if strings.Contains(fmt.Sprint(body), "ubuntu-latest") || strings.Contains(fmt.Sprint(body), "docker") {
		t.Fatalf("a request label reached the registration: %v", body)
	}
	for _, bad := range [][]string{{"fast"}, {"nself", "nself-glr-01"}, {"nself", "nself"}, {"nself", "has space"}, {"nself", "a", "b", "c", "d", "e", "f", "g", "h"}} {
		if _, err = runners.New(runners.Config{Client: f.client(runnerToken), Settings: runners.Settings{Tags: bad}}); err == nil {
			t.Errorf("tags %v must be refused", bad)
		}
	}
}

// TestAccessLevel proves acceptance 4 (the access half): access_level is not_protected unless Restriction
// verified a protected branch or tag by its exact name (also one with a slash); a merge request ref, an
// unknown job, a ref only a wildcard rule would cover, a 200 whose body is not that protection and a failed
// read are all not_protected.
func TestAccessLevel(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r := f.registrar(clock)
	f.branch["main"], f.branch["release/1.0"], f.branch["release-*"], f.tags["v1.0"] = true, true, true, true
	f.jobs[1] = fakeJob{Ref: "main", Status: "pending"}
	f.jobs[2] = fakeJob{Ref: "feature/x", Status: "pending"}
	f.jobs[3] = fakeJob{Ref: "v1.0", Tag: true, Status: "pending"}
	f.jobs[4] = fakeJob{Ref: "refs/merge-requests/7/head", Status: "pending"}
	f.jobs[5] = fakeJob{Ref: "v2.0", Tag: true, Status: "pending"}
	f.jobs[6] = fakeJob{Ref: "release/1.0", Status: "pending"} // a protected branch with a slash: the %2F path encoding
	f.jobs[7] = fakeJob{Ref: "release-1", Status: "pending"}   // matched only by the wildcard rule release-*: unproven, not protected
	f.jobs[8] = fakeJob{Ref: "main2", Status: "pending"}
	f.garbage["main2"] = true // the protection read answers 200 with a body that is not that protection
	for job, want := range map[int64]bool{0: false, 1: true, 2: false, 3: true, 4: false, 5: false, 6: true, 7: false, 99: false} {
		got, err := r.Restriction(ctx, repoScope, job)
		if err != nil || got.ProtectedOnly != want {
			t.Errorf("job %d: protected %v err %v, want %v", job, got.ProtectedOnly, err, want)
		}
	}
	if got, err := r.Restriction(ctx, repoScope, 8); got.ProtectedOnly || err == nil {
		t.Errorf("a 200 that is not JSON must never prove protection and surfaces as an error: %+v %v", got, err)
	}
	f.statusFor["GET /projects/acme%2Fwidgets/jobs/1"] = 500
	if _, err := r.Restriction(ctx, repoScope, 1); err == nil || providers.ClassOf(err) != providers.Transient {
		t.Errorf("a failed job read must surface as a transient error, got %v", err)
	}
	for _, protected := range []bool{false, true} {
		if _, err := r.MintRunner(ctx, runnerReq(), runners.MintOptions{ProtectedOnly: protected}); err != nil {
			t.Fatal(err)
		}
	}
	if f.mints[0]["access_level"] != "not_protected" || f.mints[1]["access_level"] != "ref_protected" {
		t.Fatalf("access levels %v %v", f.mints[0]["access_level"], f.mints[1]["access_level"])
	}
	if _, err := r.Mint(ctx, runnerReq()); err != nil || f.mints[2]["access_level"] != "not_protected" {
		t.Fatalf("the interface Mint must register not_protected: %v %v", err, f.mints[2]["access_level"])
	}
}

// TestRunnerTokenOnly proves acceptance 4 (the credential half, review F14): every request carries the
// runner-management token and none the Developer token, and a policy without runner_token_env makes no
// request at all (E700) instead of falling back to the Developer token.
func TestRunnerTokenOnly(t *testing.T) {
	f := newFakeGL(t)
	r := f.registrar(clock)
	m, err := r.MintRunner(context.Background(), runnerReq(), runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Deregister(context.Background(), m.Ref()); err != nil {
		t.Fatal(err)
	}
	if len(f.auths) < 4 {
		t.Fatalf("expected the project read, mint, read, delete and verify: %v", f.calls)
	}
	for i, a := range f.auths {
		if a != "Bearer "+runnerToken {
			t.Errorf("request %d (%s) carried %q", i, f.calls[i], a)
		}
	}
	t.Setenv("NSELF_CI_GITLAB_TOKEN", devToken) // the dispatch provider's token is set, the runner token is not
	c, err := runners.NewClient(gitlabSettings(f.srv.URL, ""))
	if err != nil {
		t.Fatal(err)
	}
	c.TLSConfig = f.client("").TLSConfig
	g, err := runners.New(runners.Config{Client: c, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	same := gitlabSettings(f.srv.URL, "NSELF_CI_GITLAB_TOKEN") // the runner token variable is the Developer token's
	if _, err = runners.NewClient(same); code(err) != "E700" || reason(err) != "runner_token_env.same_as_token_env" {
		t.Fatalf("one variable for both tokens must be refused: %v", err)
	}
	before := len(f.calls)
	_, err = g.MintRunner(context.Background(), runnerReq(), runners.MintOptions{})
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E700" || len(f.calls) != before {
		t.Fatalf("want E700 and no request, got %v after %d calls", err, len(f.calls)-before)
	}
	t.Setenv("NSELF_CI_GITLAB_RUNNER_TOKEN", runnerToken)
	if c, err = runners.NewClient(gitlabSettings(f.srv.URL, "NSELF_CI_GITLAB_RUNNER_TOKEN")); err != nil {
		t.Fatal(err)
	}
	c.TLSConfig = f.client("").TLSConfig
	g, _ = runners.New(runners.Config{Client: c, Clock: clock})
	if _, err = g.MintRunner(context.Background(), runnerReq(), runners.MintOptions{}); err != nil {
		t.Fatal(err)
	}
	if last := f.auths[len(f.auths)-1]; last != "Bearer "+runnerToken {
		t.Fatalf("policy-built client sent %q", last)
	}
}

// TestMintRefusesALegacyToken proves the glrt- rule of the Ticket objective: a reply whose token is not a
// runner authentication token is E707 and the runner it created is deleted again.
func TestMintRefusesALegacyToken(t *testing.T) {
	f := newFakeGL(t)
	f.tokenOverride = "legacy-registration-token"
	r := f.registrar(clock)
	_, err := r.MintRunner(context.Background(), runnerReq(), runners.MintOptions{})
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E707" || f.count() != 0 || f.called("DELETE /runners/") != 1 {
		t.Fatalf("err %v, runners left %d, deletes %d", err, f.count(), f.called("DELETE /runners/"))
	}
	if strings.Contains(err.Error(), "legacy-registration-token") {
		t.Fatal("the token is in the error")
	}
}

// TestMintedNeverPrintsTheToken proves acceptance 3 for the registrar: String, GoString, slog and JSON
// leave the credential out, and an error from a rejected mint carries no token either.
func TestMintedNeverPrintsTheToken(t *testing.T) {
	f := newFakeGL(t)
	r := f.registrar(clock)
	m, err := r.MintRunner(context.Background(), runnerReq(), runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("minted", "runner", m)
	for name, text := range map[string]string{"String": m.String(), "GoString": fmt.Sprintf("%#v", m), "%v": fmt.Sprintf("%v %+v", m, m), "JSON": string(raw), "slog": logged.String()} {
		if strings.Contains(text, m.Credential.Value) {
			t.Errorf("the token is in %s output: %s", name, text)
		}
	}
	f.statusFor["POST /user/runners"] = 403
	if _, err = r.MintRunner(context.Background(), runnerReq(), runners.MintOptions{}); err == nil || strings.Contains(err.Error(), runnerToken) {
		t.Fatalf("a refused mint must be an error without a token: %v", err)
	}
}

func TestRegistrarIdentity(t *testing.T) {
	f := newFakeGL(t)
	r := f.registrar(clock)
	caps, err := r.Describe(context.Background(), providers.Query{})
	if r.ID() != "gitlab-runners" || err != nil || caps == nil || len(caps) != 0 {
		t.Fatalf("id %q caps %v err %v", r.ID(), caps, err)
	}
	if _, err = r.MintRunner(context.Background(), providers.RunnerRequest{Scope: providers.RunnerScope{Repo: "bad"}}, runners.MintOptions{}); err == nil {
		t.Fatal("a scope that is not group/project must be refused")
	}
	if _, err = runners.New(runners.Config{}); err == nil {
		t.Fatal("no client must be refused")
	}
}

// TestRunnerCarriesOnlyItsJobsTags (Opus should-fix): the runner minted for a job carries the operator tags that
// job asks for plus its unique one, never the others: a runner that holds `arm64` takes any pending arm64 job of
// the project. Without a readable job it carries every configured tag except an architecture tag that
// contradicts its own.
func TestRunnerCarriesOnlyItsJobsTags(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r, err := runners.New(runners.Config{Client: f.client(runnerToken), Clock: clock, Settings: runners.Settings{Tags: []string{"nself", "arm64", "gpu"}}})
	if err != nil {
		t.Fatal(err)
	}
	f.jobs[1] = fakeJob{Ref: "main", Status: "pending", Tags: []string{"nself"}}
	f.jobs[2] = fakeJob{Ref: "main", Status: "pending", Tags: []string{"nself", "gpu", "unknown-tag"}}
	f.jobs[3] = fakeJob{Ref: "main", Status: "pending", Tags: []string{"gpu"}} // not asking for the demand tag: nothing carried
	for job, want := range map[int64][]string{1: {"nself"}, 2: {"nself", "gpu"}, 3: nil, 99: nil} {
		a, err := r.Assess(ctx, repoScope, job)
		if err != nil || strings.Join(a.Tags, ",") != strings.Join(want, ",") {
			t.Errorf("job %d: tags %v err %v, want %v", job, a.Tags, err, want)
		}
	}
	m, err := r.MintRunner(ctx, runnerReq(), runners.MintOptions{Tags: []string{"nself", "gpu"}})
	if err != nil || strings.Join(m.Tags[:2], ",") != "nself,gpu" || len(m.Tags) != 3 {
		t.Fatalf("subset: %v %v", m.Tags, err)
	}
	if _, err = r.MintRunner(ctx, runnerReq(), runners.MintOptions{Tags: []string{"nself", "other"}}); code(err) != "E702" {
		t.Fatalf("a tag the operator did not configure must be refused: %v", err)
	}
	for arch, want := range map[string]string{"amd64": "nself,gpu", "arm64": "nself,arm64,gpu", "": "nself,arm64,gpu"} {
		m, err = r.MintRunner(ctx, runnerReq(), runners.MintOptions{Arch: arch})
		if err != nil || strings.Join(m.Tags[:len(m.Tags)-1], ",") != want {
			t.Errorf("arch %q fallback: %v %v, want %s", arch, m.Tags, err, want)
		}
	}
}

// TestOwnershipNeedsTheUniqueTag (Opus note): a runner somebody named nself-glr-<valid ulid> by hand, without
// the matching tag, is not ours: Deregister and Pause refuse it.
func TestOwnershipNeedsTheUniqueTag(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r := f.registrar(clock)
	m, _ := r.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	hand := f.add(m.Description[:len(m.Description)-1]+"A", false) // a valid-looking description, no unique tag
	if err := r.Deregister(ctx, ref(itoa(hand.ID))); code(err) != "E707" || f.count() != 2 {
		t.Fatalf("a hand-made runner must be refused and kept: %v", err)
	}
	if err := r.Pause(ctx, ref(itoa(hand.ID))); code(err) != "E707" || len(f.puts) != 0 {
		t.Fatalf("a hand-made runner must not be paused: %v", err)
	}
}
