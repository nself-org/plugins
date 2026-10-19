package jit_test

// Purpose: the P7-HOST-04 acceptance tests. Each test's doc comment cites the acceptance line it
// proves (numbers follow the Ticket's acceptance list, 1 = conformance ... 8 = fixed labels).
// Inputs:  the fixtures under testdata/ (GitHub REST docs shapes, API 2022-11-28) and fakeGH.
// Outputs: pass or fail.
// Constraints: no live network; every credential value is synthetic.

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/conformance"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
)

// scoped fills the empty scope and group the conformance kit sends, so the kit reaches fixtures.
type scoped struct {
	*jit.Registrar
	group int64
}

func (s scoped) fill(sc providers.RunnerScope) providers.RunnerScope {
	if sc.Repo == "" {
		sc.Repo = testScope
	}
	return sc
}
func (s scoped) Mint(ctx context.Context, r providers.RunnerRequest) (providers.RunnerCredential, error) {
	r.Scope = s.fill(r.Scope)
	if s.group != 0 {
		r.Attempt.Trust = "untrusted"
	}
	return s.Registrar.Mint(ctx, r)
}
func (s scoped) Deregister(ctx context.Context, r providers.RunnerRef) error {
	r.Scope = s.fill(r.Scope)
	return s.Registrar.Deregister(ctx, r)
}
func (s scoped) Orphans(ctx context.Context, sc providers.RunnerScope, t time.Time) ([]providers.RunnerRef, error) {
	return s.Registrar.Orphans(ctx, s.fill(sc), t)
}
func (s scoped) Restriction(ctx context.Context, sc providers.RunnerScope, g int64) (providers.Restriction, error) {
	if g == 0 {
		g = s.group
	}
	return s.Registrar.Restriction(ctx, s.fill(sc), g)
}

// TestConformance proves acceptance 1: the registrar passes conformance.Run on recorded-shape
// fixtures for repo and org scope (mint, list with pagination, delete including 404, group read).
// The kit's replay server speaks plain HTTP and the funnel refuses it, so a TLS front relays to it.
func TestConformance(t *testing.T) {
	for _, tc := range []struct {
		dir, scope, jitConfig string
		groups                map[string]int64
		group                 int64
	}{
		{"repo", jit.ScopeRepo, "synthetic-jit-config-repo-value", nil, 0},
		{"org", jit.ScopeOrg, "synthetic-jit-config-org-value", map[string]int64{"untrusted": 11}, 11},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			conformance.Run(t, conformance.Subject{
				Name: "github-jit-" + tc.dir, Fixtures: "testdata/" + tc.dir,
				Secrets: [][]byte{[]byte(testToken), []byte(tc.jitConfig)},
				New: func(base string, clk conformance.Clock) providers.Provider {
					u, err := url.Parse(base)
					if err != nil {
						t.Fatal(err)
					}
					front := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(u))
					t.Cleanup(front.Close)
					reg, err := jit.New(jit.Config{Client: testClient(front.URL, front), Clock: clk.Now, PageSize: 2,
						Settings: jit.Settings{Scope: tc.scope, Groups: tc.groups}})
					if err != nil {
						t.Fatal(err)
					}
					return scoped{reg, tc.group}
				},
			})
		})
	}
}

// TestUniqueNames proves acceptance 2: two Mint calls (same attempt, same instant) produce distinct
// runner names and distinct nself-jit-<ulid> labels, and the label is the name.
func TestUniqueNames(t *testing.T) {
	f := newFakeGH(t)
	r := f.registrar(jit.Settings{}, nil)
	req := providers.RunnerRequest{Attempt: providers.Attempt{ID: "attempt-1"}, Scope: repoOnly}
	a, err := r.MintRunner(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.MintRunner(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	last := func(m jit.Minted) string { return m.Labels[len(m.Labels)-1] }
	if a.Name == b.Name || last(a) == last(b) || last(a) != a.Name || !strings.HasPrefix(a.Name, jit.RunnerPrefix) {
		t.Fatalf("names %q %q labels %q %q", a.Name, b.Name, last(a), last(b))
	}
	if a.RunnerID == b.RunnerID || a.Credential.Value == b.Credential.Value {
		t.Fatal("runner ids or credentials repeat")
	}
	if got := a.Credential.ExpiresAt; !got.Equal(epoch.Add(60 * time.Minute)) {
		t.Fatalf("expiry %v", got)
	}
}

func group(public, restricted bool, workflows ...string) map[string]any {
	return map[string]any{"id": 11, "name": "g", "allows_public_repositories": public, "restricted_to_workflows": restricted, "selected_workflows": workflows}
}

// TestRestriction proves acceptance 3: Verified+ProtectedOnly only for the fixture group with public
// repositories off, restricted_to_workflows on and every workflow pinned to a protected branch; each
// negation (public allowed, unrestricted, unprotected branch, SHA or tag pin) is ProtectedOnly false
// with a reason.
func TestRestriction(t *testing.T) {
	const ok = "acme/widgets/.github/workflows/ci.yml@refs/heads/main"
	sha := "acme/widgets/.github/workflows/ci.yml@" + strings.Repeat("a", 40)
	cases := []struct {
		name   string
		g      map[string]any
		reason string
	}{
		{"all-conditions", group(false, true, ok), ""},
		{"public-allowed", group(true, true, ok), "public_repositories_allowed"},
		{"unrestricted", group(false, false, ok), "not_restricted_to_workflows"},
		{"unprotected-branch", group(false, true, "acme/widgets/.github/workflows/ci.yml@refs/heads/dev"), "workflow_branch_unprotected"},
		{"sha-pin", group(false, true, sha), "workflow_not_branch_pinned"},
		{"tag-pin", group(false, true, "acme/widgets/.github/workflows/ci.yml@refs/tags/v1"), "workflow_not_branch_pinned"},
		{"bare-ref", group(false, true, "acme/widgets/.github/workflows/ci.yml@main"), "workflow_not_branch_pinned"},
		{"wildcard", group(false, true, "acme/widgets/.github/workflows/ci.yml@refs/heads/*"), "workflow_not_branch_pinned"},
		{"one-bad-of-two", group(false, true, ok, sha), "workflow_not_branch_pinned"},
		{"tag-pin-after-branch-text", group(false, true, "acme/widgets/.github/workflows/a@refs/heads/main.yml@refs/tags/v1"), "workflow_not_branch_pinned"},
		{"empty-workflow-path", group(false, true, "acme/widgets/@refs/heads/main"), "workflow_not_branch_pinned"},
		{"not-under-github-workflows", group(false, true, "acme/widgets/ci.yml@refs/heads/main"), "workflow_not_branch_pinned"},
		{"dotdot-branch", group(false, true, "acme/widgets/.github/workflows/ci.yml@refs/heads/dev/../../main"), "workflow_not_branch_pinned"},
		{"encoded-branch", group(false, true, "acme/widgets/.github/workflows/ci.yml@refs/heads/ma%69n"), "workflow_not_branch_pinned"},
		{"no-workflows", group(false, true), "no_selected_workflows"},
		{"foreign-owner-pin", group(false, true, "evil/widgets/.github/workflows/ci.yml@refs/heads/main"), "workflow_foreign_owner"},
		{"two-workflows-one-branch", group(false, true, ok, "acme/widgets/.github/workflows/other.yml@refs/heads/main"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGH(t)
			f.group, f.protect["acme/widgets@main"] = tc.g, true
			r := f.registrar(jit.Settings{Scope: jit.ScopeOrg}, nil)
			v, err := r.RestrictionVerdict(context.Background(), repoOnly, 11)
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.reason == ""; v.ProtectedOnly != want || v.Verified != true || v.Reason != tc.reason {
				t.Fatalf("got %+v want reason %q", v, tc.reason)
			}
			got, err := r.Restriction(context.Background(), repoOnly, 11)
			if err != nil || got.ProtectedOnly != (tc.reason == "") {
				t.Fatalf("Restriction %+v %v", got, err)
			}
			if tc.name == "two-workflows-one-branch" && f.protectCalls != 2 {
				t.Fatalf("one branch must be read once per verdict: %d protection reads for 2 verdicts", f.protectCalls)
			}
		})
	}
	t.Run("api-errors-are-never-protected", func(t *testing.T) {
		for _, tc := range []struct {
			name           string
			group, protect int
		}{{"group-500", 500, 0}, {"group-403", 403, 0}, {"protection-500", 0, 500}, {"protection-403", 0, 403}} {
			f := newFakeGH(t)
			f.group, f.protect["acme/widgets@main"] = group(false, true, ok), true
			f.groupStatus, f.protectStatus = tc.group, tc.protect
			r := f.registrar(jit.Settings{Scope: jit.ScopeOrg}, nil)
			v, err := r.RestrictionVerdict(context.Background(), repoOnly, 11)
			got, rerr := r.Restriction(context.Background(), repoOnly, 11)
			if err == nil || rerr == nil || v.ProtectedOnly || got.ProtectedOnly {
				t.Fatalf("%s: verdict %+v err %v", tc.name, v, err)
			}
		}
	})
	t.Run("repo-scope-is-never-proven", func(t *testing.T) {
		f := newFakeGH(t)
		f.group, f.protect["acme/widgets@main"] = group(false, true, ok), true
		for _, gid := range []int64{1, 11} {
			v, err := f.registrar(jit.Settings{}, nil).RestrictionVerdict(context.Background(), repoOnly, gid)
			if err != nil || v.Verified || v.ProtectedOnly || v.Reason != "repo_scope_unrestricted" || f.called("GET") != 0 {
				t.Fatalf("group %d: %+v %v calls=%v", gid, v, err, f.calls)
			}
		}
	})
	t.Run("missing-fields-are-not-proof", func(t *testing.T) {
		for _, g := range []map[string]any{
			{"id": 11, "restricted_to_workflows": true, "selected_workflows": []string{ok}},     // no allows_public_repositories
			{"id": 11, "allows_public_repositories": false, "selected_workflows": []string{ok}}, // no restricted_to_workflows
			{"id": 11},
		} {
			f := newFakeGH(t)
			f.group, f.protect["acme/widgets@main"] = g, true
			v, err := f.registrar(jit.Settings{Scope: jit.ScopeOrg}, nil).RestrictionVerdict(context.Background(), repoOnly, 11)
			if err != nil || v.Verified || v.ProtectedOnly || v.Reason != "group_fields_missing" {
				t.Fatalf("%v: %+v %v", g, v, err)
			}
		}
	})
	t.Run("unreadable-group", func(t *testing.T) {
		f := newFakeGH(t)
		v, err := f.registrar(jit.Settings{Scope: jit.ScopeOrg}, nil).RestrictionVerdict(context.Background(), repoOnly, 11)
		if err != nil || v.Verified || v.ProtectedOnly || v.Reason != "group_unreadable" {
			t.Fatalf("%+v %v", v, err)
		}
		v, _ = f.registrar(jit.Settings{}, nil).RestrictionVerdict(context.Background(), repoOnly, 0)
		if v.ProtectedOnly || v.Reason != "no_group" || f.called("GET") != 1 {
			t.Fatalf("%+v calls=%v", v, f.calls)
		}
	})
}

// TestOrgScopeNeedsAdminOrg proves acceptance 4: an org-scope mint answered 403 with
// X-Accepted-OAuth-Scopes: admin:org is E701 naming admin:org.
func TestOrgScopeNeedsAdminOrg(t *testing.T) {
	f := newFakeGH(t)
	f.deny403 = true
	r := f.registrar(jit.Settings{Scope: jit.ScopeOrg, Groups: map[string]int64{"untrusted": 11}}, nil)
	_, err := r.Mint(context.Background(), providers.RunnerRequest{Scope: repoOnly, Attempt: providers.Attempt{Trust: "untrusted"}})
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E701" || !strings.Contains(pe.Reason, "admin:org") {
		t.Fatalf("want E701 admin:org, got %v", err)
	}
}

// TestDeregisterVerified proves acceptance 5 (first half): a runner still answering after DELETE is
// E707 deregister_unverified; one already gone (or gone during the delete: 404) is done; a clean delete
// is read back as absent; a runner that is not an nself-jit one is never deleted; a ref from the other
// scope or with a malformed id is refused before any request.
func TestDeregisterVerified(t *testing.T) {
	ctx := context.Background()
	f := newFakeGH(t)
	r := f.registrar(jit.Settings{}, nil)
	mint := func() jit.Minted {
		m, err := r.MintRunner(ctx, providers.RunnerRequest{Scope: repoOnly})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := mint()
	if !strings.HasPrefix(m.Ref().ID, "repo:") {
		t.Fatalf("ref %q", m.Ref().ID)
	}
	if err := r.Deregister(ctx, m.Ref()); err != nil || f.called("GET")+f.called("DELETE") != 3 {
		t.Fatalf("clean delete is read, delete, read: %v calls=%v", err, f.calls)
	}
	n := len(f.calls)
	if err := r.Deregister(ctx, m.Ref()); err != nil || len(f.calls) != n+1 {
		t.Fatalf("already gone must be done after one read: %v calls=%v", err, f.calls[n:])
	}
	f.raceDelete = true
	if err := r.Deregister(ctx, mint().Ref()); err != nil {
		t.Fatalf("a 404 on DELETE is done: %v", err)
	}
	f.raceDelete, f.keepDel = false, true
	err := r.Deregister(ctx, mint().Ref())
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E707" || pe.Reason != "deregister_unverified" {
		t.Fatalf("want E707 deregister_unverified, got %v", err)
	}
	f.keepDel = false
	f.runners = append(f.runners, runnerRow{ID: 900, Name: "persistent-box"})
	n = f.called("DELETE")
	err = r.Deregister(ctx, providers.RunnerRef{ID: "repo:900", Scope: repoOnly})
	if !errors.As(err, &pe) || pe.Reason != "deregister.not_ours" || f.called("DELETE") != n {
		t.Fatalf("a persistent runner must never be deleted: %v", err)
	}
	n = len(f.calls)
	for id, reason := range map[string]string{"org:900": "deregister.scope_changed", "1/../../other": "runner.id", "repo:": "runner.id", "x:5": "runner.id", "5?x=1": "runner.id"} {
		err = r.Deregister(ctx, providers.RunnerRef{ID: id, Scope: repoOnly})
		if !errors.As(err, &pe) || pe.Reason != reason {
			t.Errorf("%q: want %s, got %v", id, reason, err)
		}
	}
	if len(f.calls) != n {
		t.Fatalf("a refused ref reached the API: %v", f.calls[n:])
	}
}

// TestListMustBeProof proves a runner list that lacks its fields (null, {}, no total) is E707
// list.malformed and never an empty list that could pass for a sweep result.
func TestListMustBeProof(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"total_count":0}`, `{"runners":[]}`} {
		f := newFakeGH(t)
		f.listBody = body
		_, err := f.registrar(jit.Settings{}, nil).Orphans(context.Background(), repoOnly, time.Time{})
		var pe *providers.Error
		if !errors.As(err, &pe) || pe.Reason != "list.malformed" {
			t.Errorf("%s: want list.malformed, got %v", body, err)
		}
	}
	f := newFakeGH(t)
	f.listBody = `{"total_count":0,"runners":[]}`
	if got, err := f.registrar(jit.Settings{}, nil).Orphans(context.Background(), repoOnly, time.Time{}); err != nil || len(got) != 0 {
		t.Fatalf("an empty but well-formed list is fine: %v %v", got, err)
	}
}

// TestOrphans proves acceptance 5 (second half): only nself-jit-* runners older than 70 minutes and
// not busy come back, across pages and at both scopes; a persistent runner, a young one, a busy one,
// one 69 minutes old and one with a foreign prefix never do, and one 71 minutes old does.
func TestOrphans(t *testing.T) {
	old, young, older := "nself-jit-01KDV6SJR0ABCDEFGHJKMNPQRS", "nself-jit-01KDVBYC600000000000000000", "nself-jit-01KDTZXVG00000000000000000"
	min71, min69 := "nself-jit-01KDV9K9V00000000000000000", "nself-jit-01KDV9PZ100000000000000000"
	for _, set := range []jit.Settings{{Scope: jit.ScopeRepo}, {Scope: jit.ScopeOrg}} {
		t.Run("scope-"+set.Scope, func(t *testing.T) {
			f := newFakeGH(t)
			f.runners = []runnerRow{{1, "persistent-box", false}, {2, old, false}, {3, young, false}, {4, "nself-jit-01KDV3BQ400000000000000000", true},
				{5, older, false}, {6, "nself-jit-not-a-ulid-0000000000000", false}, {7, "other-" + old, false}, {8, min71, false}, {9, min69, false}}
			r := f.registrar(set, nil)
			got, err := r.Orphans(context.Background(), repoOnly, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, x := range got {
				ids = append(ids, x.ID)
				if x.Scope != repoOnly {
					t.Fatalf("scope lost: %+v", x)
				}
			}
			if want := set.Scope + ":2," + set.Scope + ":5," + set.Scope + ":8"; strings.Join(ids, ",") != want {
				t.Fatalf("orphans %v, want %s", ids, want)
			}
			if n := f.called("GET /"); n != 5 {
				t.Fatalf("paging: %d list calls for 9 runners at 2 per page", n)
			}
			if set.Scope == jit.ScopeOrg && f.called("GET /orgs/acme/actions/runners") != 5 {
				t.Fatalf("org scope must list the org: %v", f.calls)
			}
			// An explicit cutoff replaces the 70-minute default.
			early, _ := r.Orphans(context.Background(), repoOnly, epoch.Add(-24*time.Hour))
			if len(early) != 0 {
				t.Fatalf("cutoff ignored: %v", early)
			}
		})
	}
}

// TestPageSizeClamp proves a page size above GitHub's maximum of 100 is clamped, so a full page of 100
// is not mistaken for the last page (which would hide runners from the absence check and the sweep).
func TestPageSizeClamp(t *testing.T) {
	f := newFakeGH(t)
	for i := int64(1); i <= 150; i++ {
		f.runners = append(f.runners, runnerRow{ID: i, Name: "nself-jit-01KDTZXVG00000000000000000"})
	}
	r, err := jit.New(jit.Config{Client: testClient(f.srv.URL, f.srv), Clock: func() time.Time { return epoch }, PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Orphans(context.Background(), repoOnly, time.Time{})
	if err != nil || len(got) != 150 {
		t.Fatalf("saw %d of 150 orphans (%v)", len(got), err)
	}
}

// TestFixedLabels proves acceptance 6: a request carrying ubuntu-latest (and other labels) registers
// only the fixed set; only an arch label of the caller's survives; the body names group 1 at repo
// scope and the operator's label is used.
func TestFixedLabels(t *testing.T) {
	f := newFakeGH(t)
	r := f.registrar(jit.Settings{Label: "buildbox"}, nil)
	m, err := r.MintRunner(context.Background(), providers.RunnerRequest{Scope: repoOnly, Labels: []string{"ubuntu-latest", "windows-2022", "nself", "ARM64", "self-hosted"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"self-hosted", "buildbox", "linux", "ARM64", m.Name}
	body := f.minted[0]
	got, _ := body["labels"].([]any)
	if len(got) != len(want) {
		t.Fatalf("labels %v", got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("labels %v, want %v", got, want)
		}
	}
	if body["runner_group_id"] != float64(1) || body["name"] != m.Name {
		t.Fatalf("body %v", body)
	}
	d, _ := r.MintRunner(context.Background(), providers.RunnerRequest{Scope: repoOnly, Labels: []string{"ubuntu-latest"}})
	if d.Labels[3] != "X64" || len(d.Labels) != 5 {
		t.Fatalf("default labels %v", d.Labels)
	}
	o := f.registrar(jit.Settings{Scope: jit.ScopeOrg, Groups: map[string]int64{"untrusted": 11, "trusted": 12}}, nil)
	for _, req := range []providers.RunnerRequest{
		{Scope: repoOnly, Attempt: providers.Attempt{Trust: "untrusted"}, GroupID: 12}, // asks for a more trusted tier's group
		{Scope: repoOnly, Attempt: providers.Attempt{Trust: "untrusted"}, GroupID: 99},
		{Scope: repoOnly, Attempt: providers.Attempt{Trust: "unknown-tier"}},
		{Scope: repoOnly, GroupID: 12}, // no tier at all
	} {
		_, err = o.Mint(context.Background(), req)
		var pe *providers.Error
		if !errors.As(err, &pe) || pe.Code != "E700" {
			t.Fatalf("%+v: a group that is not the tier's must be E700, got %v", req, err)
		}
	}
	if len(f.minted) != 2 {
		t.Fatalf("a refused request reached the API: %d mints", len(f.minted))
	}
	if _, err := o.MintRunner(context.Background(), providers.RunnerRequest{Scope: repoOnly, Attempt: providers.Attempt{Trust: "untrusted"}, GroupID: 11}); err != nil {
		t.Fatal(err)
	}
	if f.minted[len(f.minted)-1]["runner_group_id"] != float64(11) {
		t.Fatalf("org group not sent: %v", f.minted[len(f.minted)-1])
	}
}
