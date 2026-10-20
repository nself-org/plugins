package source_test

// Purpose: P7-TRUST-13 tests: Classify over normalized trigger facts, ClassifyLocal over git facts, the
// real-git provenance fixtures through run.SourceTrust, the audited `--trust owner`, the machine cap,
// the forge re-fetch and FuzzClassify. External package: internal/run imports internal/trust/source.
// Every test name starts with TestClassify so the mutation gate (testdata/mutants) selects it.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func fact[T any](v T, src string) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: time.Unix(1, 0).UTC()}
}

// facts is a same-repo trigger by a write-access author with every fact given by the API.
func facts(kind model.TriggerKind, edit ...func(*model.TriggerFacts)) model.TriggerFacts {
	f := model.TriggerFacts{
		Schema: "ci.trigger-facts/v1", Source: "github", Kind: kind, Auth: "per-repo", Repo: "acme/widgets",
		Actor: fact("alice", "api"), ActorPermission: fact("write", "api"), HeadRepo: fact("acme/widgets", "api"),
		Fork: fact(false, "api"), FirstTime: fact(false, "api"), Bot: fact(false, "api"), ProtectedRef: fact(false, "api"),
		SHAReachable: fact(false, "api"), HeadCurrent: fact(true, "api"), Visibility: fact("public", "api"),
		ChangedFiles: fact([]string{"main.go"}, "api"),
	}
	for _, e := range edit {
		e(&f)
	}
	return f
}

func admin(f *model.TriggerFacts)     { f.ActorPermission = fact("admin", "api") }
func protected(f *model.TriggerFacts) { f.ProtectedRef = fact(true, "api") }
func reachable(f *model.TriggerFacts) { f.SHAReachable = fact(true, "api") }

func TestClassifyFactsTable(t *testing.T) {
	cases := []struct {
		name string
		f    model.TriggerFacts
		want model.TrustClass
	}{
		{"fork PR", facts("pull_request", func(f *model.TriggerFacts) { f.Fork = fact(true, "api") }), "untrusted"},
		{"head repo differs", facts("pull_request", func(f *model.TriggerFacts) { f.HeadRepo = fact("evil/widgets", "api") }), "untrusted"},
		{"same-repo PR, write access", facts("pull_request"), "collaborator"},
		{"same-repo PR, admin is still a branch", facts("pull_request", admin), "collaborator"},
		{"PR with no proof of the head repo", facts("pull_request", func(f *model.TriggerFacts) { f.HeadRepo, f.Fork = model.Fact[string]{}, model.Fact[bool]{} }), "untrusted"},
		{"push to protected main by owner", facts("push", admin, protected), "owner"},
		{"push to protected main by write access", facts("push", protected), "collaborator"},
		{"push to a branch by admin", facts("push", admin), "collaborator"},
		{"shared auth", facts("push", admin, protected, func(f *model.TriggerFacts) { f.Auth = "shared" }), "untrusted"},
		{"bearer auth", facts("push", admin, protected, func(f *model.TriggerFacts) { f.Auth = "bearer" }), "untrusted"},
		{"no auth", facts("push", admin, protected, func(f *model.TriggerFacts) { f.Auth = "none" }), "untrusted"},
		{"all facts unknown", model.TriggerFacts{Schema: "ci.trigger-facts/v1", Kind: "push"}, "untrusted"},
		{"tag on an unreachable sha", facts("tag", admin), "untrusted"},
		{"tag on a fork sha", facts("tag", admin, func(f *model.TriggerFacts) { f.Fork = fact(true, "api") }, reachable), "untrusted"},
		{"tag on a reachable sha", facts("tag", admin, reachable), "owner"},
		{"api trigger on an unreachable sha", facts("api", admin), "untrusted"},
		{"api trigger on a reachable sha", facts("api", admin, reachable), "owner"},
		{"manual run, write access, reachable", facts("manual", reachable), "collaborator"},
		{"schedule on the protected default branch", facts("schedule", func(f *model.TriggerFacts) { f.ActorPermission = model.Fact[string]{} }, protected), "owner"},
		{"schedule with no provenance fact", facts("schedule", func(f *model.TriggerFacts) { f.ActorPermission = model.Fact[string]{} }), "untrusted"},
		{"schedule claimed by a payload-only delivery", facts("schedule", func(f *model.TriggerFacts) {
			f.ActorPermission, f.ProtectedRef = model.Fact[string]{}, fact(true, "payload")
		}), "untrusted"},
		{"unknown kind", facts("weird", admin, protected), "untrusted"},
		{"dependabot login", facts("push", admin, protected, func(f *model.TriggerFacts) { f.Actor = fact("dependabot[bot]", "api") }), "untrusted"},
		{"renovate login", facts("push", admin, protected, func(f *model.TriggerFacts) { f.Actor = fact("renovate", "api") }), "untrusted"},
		{"bot fact", facts("push", admin, protected, func(f *model.TriggerFacts) { f.Bot = fact(true, "api") }), "untrusted"},
		{"lockfile-only diff", facts("push", admin, protected, func(f *model.TriggerFacts) { f.ChangedFiles = fact([]string{"pnpm-lock.yaml", "sub/go.sum"}, "api") }), "untrusted"},
		{"lockfile plus code", facts("push", admin, protected, func(f *model.TriggerFacts) { f.ChangedFiles = fact([]string{"pnpm-lock.yaml", "main.go"}, "api") }), "owner"},
		{"head moved since the delivery", facts("push", admin, protected, func(f *model.TriggerFacts) { f.HeadCurrent = fact(false, "api") }), "untrusted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := source.Classify(c.f)
			if got.Class != c.want {
				t.Fatalf("class %s, want %s (reasons %v)", got.Class, c.want, got.Reasons)
			}
			if len(got.Reasons) == 0 || !got.Class.Valid() {
				t.Fatalf("result without reasons or with a bad class: %+v", got)
			}
		})
	}
}

func TestClassifyPayloadNeverRaises(t *testing.T) {
	claims := func(f *model.TriggerFacts) {
		f.ProtectedRef, f.SHAReachable = fact(true, "payload"), fact(true, "payload")
		f.GroupProtectedOnly = fact(true, "payload")
	}
	// API write access, payload claims protected_ref and sha_reachable: what the API gives decides.
	if got := source.Classify(facts("push", claims)); got.Class != "collaborator" {
		t.Fatalf("payload protected_ref/sha_reachable raised the class: %s %v", got.Class, got.Reasons)
	}
	if got := source.Classify(facts("tag", admin, claims)); got.Class != "untrusted" {
		t.Fatalf("a payload sha_reachable opened a tag: %s", got.Class)
	}
	// A payload permission claim (admin) over API write access is ignored; with no API fact it is missing.
	payloadAdmin := facts("push", protected, func(f *model.TriggerFacts) { f.ActorPermission = fact("admin", "payload") })
	if got := source.Classify(payloadAdmin); got.Class != "untrusted" {
		t.Fatalf("a payload-sourced owner claim raised the class: %s", got.Class)
	}
	callerAdmin := facts("push", protected, func(f *model.TriggerFacts) { f.ActorPermission = fact("admin", "caller") })
	if got := source.Classify(callerAdmin); got.Class != "untrusted" {
		t.Fatalf("a caller-supplied permission raised trust: %s", got.Class)
	}
	pr := facts("pull_request", func(f *model.TriggerFacts) {
		f.Fork, f.HeadRepo = fact(false, "payload"), fact("acme/widgets", "payload")
	})
	if got := source.Classify(pr); got.Class != "untrusted" {
		t.Fatalf("payload-only proof of a same-repo PR: %s", got.Class)
	}
	// Payload may LOWER trust: a payload fork claim wins over an API same-repo claim.
	if got := source.Classify(facts("pull_request", func(f *model.TriggerFacts) { f.Fork = fact(true, "payload") })); got.Class != "untrusted" {
		t.Fatalf("a payload fork claim did not lower the class: %s", got.Class)
	}
}

func TestClassifyRunnerDemand(t *testing.T) {
	demand := func(only *bool, event, head string, edit ...func(*model.TriggerFacts)) model.TriggerFacts {
		f := facts("runner_demand", admin, protected, func(f *model.TriggerFacts) {
			if only != nil {
				f.GroupProtectedOnly = fact(*only, "api")
			}
			if event != "" {
				f.RunEvent = fact(event, "api")
			}
			f.HeadRepo = fact(head, "api")
		})
		for _, e := range edit {
			e(&f)
		}
		return f
	}
	yes, no := true, false
	cases := []struct {
		name string
		f    model.TriggerFacts
		want model.TrustClass
	}{
		{"group not protected-only", demand(&no, "push", "acme/widgets"), "untrusted"},
		{"protected-only unknown", demand(nil, "push", "acme/widgets"), "untrusted"},
		{"protected-only, push, same repo: other facts decide", demand(&yes, "push", "acme/widgets"), "owner"},
		{"protected-only, push, same repo, unprotected ref", demand(&yes, "push", "acme/widgets", func(f *model.TriggerFacts) { f.ProtectedRef = fact(false, "api") }), "collaborator"},
		{"pull_request_target", demand(&yes, "pull_request_target", "acme/widgets"), "untrusted"},
		{"workflow_run", demand(&yes, "workflow_run", "acme/widgets"), "untrusted"},
		{"fork head repo", demand(&yes, "push", "evil/widgets"), "untrusted"},
		{"event unknown", demand(&yes, "", "acme/widgets"), "untrusted"},
		{"issue_comment runs the default branch on a stranger's input", demand(&yes, "issue_comment", "acme/widgets"), "untrusted"},
		{"workflow_dispatch", demand(&yes, "workflow_dispatch", "acme/widgets"), "owner"},
	}
	for _, c := range cases {
		if got := source.Classify(c.f); got.Class != c.want {
			t.Errorf("%s: %s, want %s (%v)", c.name, got.Class, c.want, got.Reasons)
		}
	}
}

func TestClassifyZoneApprovalAndOrder(t *testing.T) {
	zones := map[string]model.PrivacyZone{"public": "hosted-allowed", "private": "private-infrastructure", "": "private-infrastructure", "changed": "private-infrastructure"}
	for vis, want := range zones {
		f := facts("push", admin, protected)
		if vis == "" {
			f.Visibility = model.Fact[string]{}
		} else {
			f.Visibility = fact(vis, "api")
		}
		if got := source.Classify(f).VisibilityZone; got != want {
			t.Errorf("visibility %q: zone %s, want %s", vis, got, want)
		}
	}
	if got := source.Classify(facts("push", admin, protected, func(f *model.TriggerFacts) { f.Visibility = fact("public", "payload") })).VisibilityZone; got != "private-infrastructure" {
		t.Errorf("a payload visibility claim opened the zone: %s", got)
	}
	first := facts("pull_request", func(f *model.TriggerFacts) { f.FirstTime = fact(true, "api") })
	if !source.Classify(first).RequiresApproval {
		t.Error("a first-time contributor needs approval")
	}
	if source.Classify(facts("pull_request")).RequiresApproval {
		t.Error("a known contributor, API says so, needs no approval")
	}
	unknown := facts("pull_request", func(f *model.TriggerFacts) { f.FirstTime = model.Fact[bool]{} })
	if !source.Classify(unknown).RequiresApproval {
		t.Error("an unknown first_time fact needs approval below owner")
	}
	if source.Classify(facts("push", admin, protected, func(f *model.TriggerFacts) { f.FirstTime = model.Fact[bool]{} })).RequiresApproval {
		t.Error("an owner push needs no approval")
	}
	// Reasons come in rule order: auth, actor, repo, provenance, bot.
	r := source.Classify(facts("pull_request", func(f *model.TriggerFacts) {
		f.Auth, f.Fork, f.Bot = "shared", fact(true, "api"), fact(true, "api")
		f.ActorPermission = fact("read", "api")
	}))
	want := []string{source.ReasonAuthNotPerRepo, source.ReasonActor, source.ReasonFork, source.ReasonProvenance, source.ReasonBot}
	if !reflect.DeepEqual(r.Reasons, want) {
		t.Errorf("reasons %v, want %v", r.Reasons, want)
	}
}

func TestClassifyApplyCap(t *testing.T) {
	owner := source.Result{Class: "owner", Reasons: []string{source.ReasonVerified}}
	got := source.ApplyCap(owner, "collaborator")
	if got.Class != "collaborator" || got.Reasons[len(got.Reasons)-1] != source.ReasonMachineCap {
		t.Fatalf("cap did not lower: %+v", got)
	}
	if len(owner.Reasons) != 1 {
		t.Fatal("ApplyCap changed its input")
	}
	if got = source.ApplyCap(owner, "owner"); got.Class != "owner" || len(got.Reasons) != 1 {
		t.Fatalf("the default cap changed the result: %+v", got)
	}
	if got = source.ApplyCap(source.Result{Class: "collaborator"}, "internal"); got.Class != "collaborator" || len(got.Reasons) != 0 {
		t.Fatalf("a cap above the class raised it or added a reason: %+v", got)
	}
	if got = source.ApplyCap(owner, "bogus"); got.Class != "untrusted" {
		t.Fatalf("an unknown cap must decode to untrusted: %+v", got)
	}
}

func TestClassifyMadeHere(t *testing.T) {
	for subject, want := range map[string]bool{
		"commit: w": true, "commit (initial): w": true, "commit (amend): w": true, "commit (merge): w": true,
		"rebase (pick): w": true, "rebase -i (pick): w": true, "rebase (reword): w": true, "rebase (squash): w": true, "rebase (continue): w": true,
		"pull -q --rebase origin main (pick): w": true, "pull origin main: Merge made by the 'ort' strategy.": true, "revert: Revert \"w\"": true,
		"pull origin main: checkout": false,
		"cherry-pick: w":             true, "cherry-pick (continue): w": true, "merge feat: Merge made by the 'ort' strategy.": true,
		"rebase (finish): refs/heads/x onto 0123": false, "rebase (start): checkout main": false, "rebase (abort): returning to refs/heads/x": false,
		"cherry-pick: fast-forward": false, "merge feat: Fast-forward": false, "rebase (pick): fast-forward": false, "pull: Fast-forward": false,
		"checkout: moving from a to b": false, "branch: Created from FETCH_HEAD": false, "fetch origin: fast-forward": false,
		"reset: moving to HEAD~1": false, "clone: from https://x": false, "": false,
	} {
		if got := source.MadeHere(subject); got != want {
			t.Errorf("MadeHere(%q) = %v", subject, got)
		}
	}
}

func TestClassifyOldestEntries(t *testing.T) {
	got := source.OldestEntries([]source.ReflogEntry{
		{SHA: "foreign", Unix: 9, Subject: "cherry-pick: w"}, {SHA: "foreign", Unix: 1, Subject: "branch: Created from FETCH_HEAD"},
		{SHA: "own", Unix: 9, Subject: "checkout: moving from main to feat"}, {SHA: "own", Unix: 1, Subject: "commit: w"},
		{SHA: "tie", Unix: 5, Subject: "commit: w"}, {SHA: "tie", Unix: 5, Subject: "checkout: moving from a to b"},
		{SHA: "tie2", Unix: 5, Subject: "checkout: moving from a to b"}, {SHA: "tie2", Unix: 5, Subject: "commit: w"},
	})
	want := map[string]string{"foreign": "branch: Created from FETCH_HEAD", "own": "commit: w", "tie": "commit: w", "tie2": "commit: w"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OldestEntries = %v", got)
	}
}

func TestClassifyUnverified(t *testing.T) {
	r := source.Unverified(source.Result{Class: "owner", Reasons: []string{source.ReasonVerified}})
	if r.Class != "untrusted" || !reflect.DeepEqual(r.Reasons, []string{source.ReasonRefetchFailed}) || !r.RequiresApproval {
		t.Fatalf("%+v", r)
	}
}

func TestClassifyBotsAndLockfiles(t *testing.T) {
	for login, want := range map[string]bool{"dependabot[bot]": true, "renovate[bot]": true, "Dependabot": true, "renovate": true, "ci-bot": true, "alice": false, "robot-fan": false, "dependabotlike": false} {
		if got := source.IsBot(login); got != want {
			t.Errorf("IsBot(%q) = %v", login, got)
		}
	}
	if source.LockfileOnly(nil) || !source.LockfileOnly([]string{"a/Cargo.lock"}) || source.LockfileOnly([]string{"Cargo.lock", "src/lib.rs"}) || source.LockfileOnly([]string{"go.sum.txt"}) {
		t.Error("LockfileOnly misjudged")
	}
}

func c(shas ...string) (cs []source.Commit) {
	for _, s := range shas {
		cs = append(cs, source.Commit{SHA: s})
	}
	return cs
}

func TestClassifyLocalFacts(t *testing.T) {
	const def = "refs/remotes/origin/main"
	cases := []struct {
		name string
		g    source.GitFacts
		want model.TrustClass
	}{
		{"no head", source.GitFacts{}, "untrusted"},
		{"head on the default branch", source.GitFacts{Head: "a", DefaultRemoteRef: def, ReachableFromDefault: true}, "owner"},
		{"own unpushed commit made here", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: work"}}, "owner"},
		{"amend, rebase, cherry-pick and merge count", source.GitFacts{Head: "d", DefaultRemoteRef: def, OffDefault: c("a", "b", "c", "d"),
			ReflogCreated: map[string]string{"a": "commit (amend): w", "b": "rebase (pick): w", "c": "cherry-pick: w", "d": "merge feat: Merge made by the 'ort' strategy."}, RewriteVerified: map[string]bool{"a": true, "b": true, "c": true}}, "owner"},
		{"stranger commit, reflog says checkout", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "checkout: moving from main to pr"}}, "untrusted"},
		{"stranger commit, no reflog entry", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a")}, "untrusted"},
		{"fast-forward merge is not creation", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "merge FETCH_HEAD: Fast-forward"}}, "untrusted"},
		{"pull is not creation", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "pull: Fast-forward"}}, "untrusted"},
		{"rebase start only checks out the upstream", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "rebase (start): checkout main"}}, "untrusted"},
		{"fetch is not creation", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "fetch origin: fast-forward"}}, "untrusted"},
		{"one made here, one foreign", source.GitFacts{Head: "b", DefaultRemoteRef: def, OffDefault: c("a", "b"), ReflogCreated: map[string]string{"a": "checkout: x", "b": "commit: mine"}}, "untrusted"},
		{"a commit beyond the default arrived, one was made here", source.GitFacts{Head: "b", DefaultRemoteRef: def, OffDefault: c("a", "b"), ReflogCreated: map[string]string{"a": "branch: Created from FETCH_HEAD", "b": "commit: mine"}}, "untrusted"},
		{"made here and pushed to origin", source.GitFacts{Head: "a", DefaultRemoteRef: def, OnOriginRef: true, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: mine"}}, "owner"},
		{"only on another origin branch", source.GitFacts{Head: "a", DefaultRemoteRef: def, OnOriginRef: true, OffDefault: c("a")}, "collaborator"},
		{"only on a pull ref", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a")}, "untrusted"},
		{"truncated commit list closes the owner path", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: w"}, Missing: []string{source.MissingCommits}}, "untrusted"},
		{"unpushed unknown closes the owner path", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: w"}, Missing: []string{source.MissingCommits}}, "untrusted"},
		{"reflog unknown closes the owner path", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: w"}, Missing: []string{source.MissingReflog}}, "untrusted"},
		{"remote refs unknown closes the owner path", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: w"}, Missing: []string{source.MissingRemoteRefs}}, "untrusted"},
		{"default ref unknown closes the owner path", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: w"}, Missing: []string{source.MissingDefaultRef}}, "untrusted"},
		{"remote refs unknown closes collaborator", source.GitFacts{Head: "a", DefaultRemoteRef: def, OnOriginRef: true, Missing: []string{source.MissingRemoteRefs}}, "untrusted"},
		{"no default ref", source.GitFacts{Head: "a", OffDefault: c("a"), ReflogCreated: map[string]string{"a": "commit: w"}, Missing: []string{source.MissingDefaultRef}}, "untrusted"},
		{"no remote at all", source.GitFacts{Head: "a", NoRemotes: true}, "owner"},
		{"no remote but a fact is missing", source.GitFacts{Head: "a", NoRemotes: true, Missing: []string{source.MissingReflog}}, "untrusted"},
		{"not a repository", source.GitFacts{Head: "a", NotARepo: true}, "owner"},
		{"a merge, squash or rebase is unfinished", source.GitFacts{Head: "a", DefaultRemoteRef: def, ReachableFromDefault: true, InProgress: true}, "untrusted"},
		{"unfinished operation in a repository with no remote", source.GitFacts{Head: "a", NoRemotes: true, InProgress: true}, "untrusted"},
		{"remote configured, never fetched, all commits made here", source.GitFacts{Head: "b", NeverFetched: true, OffDefault: c("a", "b"), ReflogCreated: map[string]string{"a": "commit (initial): w", "b": "commit: w"}}, "owner"},
		{"remote configured, never fetched, no reflog (an extracted archive)", source.GitFacts{Head: "b", NeverFetched: true, OffDefault: c("a", "b")}, "untrusted"},
	}
	for _, tc := range cases {
		got := source.ClassifyLocal(tc.g)
		if got.Class != tc.want || len(got.Reasons) == 0 {
			t.Errorf("%s: %s %v, want %s", tc.name, got.Class, got.Reasons, tc.want)
		}
	}
}

// ---- real git: the provenance fixtures ----

type fx struct {
	t                         *testing.T
	op, origin, stranger, tmp string
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (x *fx) commit(dir, file string) string {
	x.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o644); err != nil {
		x.t.Fatal(err)
	}
	git(x.t, dir, "add", file)
	git(x.t, dir, "commit", "-q", "-m", "add "+file)
	return git(x.t, dir, "rev-parse", "HEAD")
}

func newFx(t *testing.T) *fx {
	t.Helper()
	x := &fx{t: t, tmp: t.TempDir()}
	x.origin, x.op, x.stranger = filepath.Join(x.tmp, "origin.git"), filepath.Join(x.tmp, "op"), filepath.Join(x.tmp, "stranger")
	git(t, x.tmp, "init", "-q", "--bare", "-b", "main", x.origin)
	git(t, x.tmp, "clone", "-q", x.origin, x.op)
	cfg := func(dir, name string) {
		git(t, dir, "config", "user.name", name)
		git(t, dir, "config", "user.email", "operator@example.test") // the stranger impersonates the operator
		git(t, dir, "config", "commit.gpgsign", "false")
	}
	cfg(x.op, "Operator")
	x.commit(x.op, "base.txt")
	git(t, x.op, "push", "-q", "-u", "origin", "main")
	git(t, x.op, "remote", "set-head", "origin", "main")
	git(t, x.tmp, "clone", "-q", x.origin, x.stranger)
	cfg(x.stranger, "Mallory")
	git(t, x.stranger, "checkout", "-q", "-b", "pr")
	x.commit(x.stranger, "evil.txt")
	return x
}

// checkoutStranger is `gh pr checkout` of a fork PR: fetch into FETCH_HEAD only, then a local branch.
func (x *fx) checkoutStranger() {
	git(x.t, x.op, "fetch", "-q", x.stranger, "pr")
	git(x.t, x.op, "checkout", "-q", "-b", "pr", "FETCH_HEAD")
}

func (x *fx) decide(requested string, h run.SourceTrust) source.Result {
	return x.decideIn(x.op, requested, h)
}

func (x *fx) decideIn(root, requested string, h run.SourceTrust) source.Result {
	x.t.Helper()
	res, err := h.Decide(context.Background(), run.TrustRequest{Root: root, Revision: git(x.t, root, "rev-parse", "HEAD"), Requested: requested})
	if err != nil {
		x.t.Fatalf("Decide: %v", err)
	}
	return res
}

func TestClassifyGitFixtures(t *testing.T) {
	check := func(name string, setup func(*fx), want model.TrustClass, reason string) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			x := newFx(t)
			setup(x)
			got := x.decide("", run.SourceTrust{})
			if got.Class != want || !contains(got.Reasons, reason) {
				t.Fatalf("%s %v, want %s with %s", got.Class, got.Reasons, want, reason)
			}
		})
	}
	check("head on origin default", func(*fx) {}, "owner", source.ReasonLocalReachable)
	check("own unpushed commits, reflog says made here", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
		x.commit(x.op, "two.txt")
	}, "owner", source.ReasonLocalCreated)
	check("own commit amended", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
		git(x.t, x.op, "commit", "-q", "--amend", "-m", "amended")
	}, "owner", source.ReasonLocalCreated)
	check("stranger branch (gh pr checkout), author email is the operator's", func(x *fx) { x.checkoutStranger() }, "untrusted", source.ReasonLocalForeign)
	check("stranger branch once FETCH_HEAD moved on", func(x *fx) {
		x.checkoutStranger()
		git(x.t, x.op, "fetch", "-q", "origin")
	}, "untrusted", source.ReasonLocalForeign)
	check("stranger branch fast-forwarded into main", func(x *fx) {
		git(x.t, x.op, "fetch", "-q", x.stranger, "pr")
		git(x.t, x.op, "merge", "-q", "--ff-only", "FETCH_HEAD")
		git(x.t, x.op, "fetch", "-q", "origin")
	}, "untrusted", source.ReasonLocalForeign)
	check("stranger branch with an own commit on top", func(x *fx) {
		x.checkoutStranger()
		git(x.t, x.op, "fetch", "-q", "origin")
		x.commit(x.op, "mine.txt")
	}, "untrusted", source.ReasonLocalForeign)
	check("commit only on another origin branch", func(x *fx) {
		git(x.t, x.stranger, "push", "-q", "origin", "pr")
		git(x.t, x.op, "fetch", "-q", "origin")
		git(x.t, x.op, "checkout", "-q", "-b", "pr", "origin/pr")
	}, "collaborator", source.ReasonLocalOnOrigin)
	check("fast-forward rebase onto the stranger's commit", func(x *fx) {
		git(x.t, x.op, "fetch", "-q", x.stranger, "pr")
		sha := git(x.t, x.op, "rev-parse", "FETCH_HEAD")
		git(x.t, x.op, "checkout", "-q", "-b", "x", "origin/main")
		git(x.t, x.op, "rebase", "-q", sha)
		git(x.t, x.op, "fetch", "-q", "origin")
	}, "untrusted", source.ReasonLocalForeign)
	check("fast-forward cherry-pick of the stranger's series", func(x *fx) {
		x.commit(x.stranger, "evil2.txt")
		git(x.t, x.op, "fetch", "-q", x.stranger, "pr")
		git(x.t, x.op, "checkout", "-q", "-b", "y", "origin/main")
		git(x.t, x.op, "cherry-pick", "--ff", "origin/main..FETCH_HEAD")
		git(x.t, x.op, "fetch", "-q", "origin")
	}, "untrusted", source.ReasonLocalForeign)
	check("own pushed feature branch", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
		git(x.t, x.op, "push", "-q", "-u", "origin", "feat")
	}, "owner", source.ReasonLocalCreated)
	check("own commit fetched back from a peer stays the operator's", func(x *fx) {
		git(x.t, x.op, "branch", "basebr", "origin/main")
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
		peer := filepath.Join(x.tmp, "peer")
		git(x.t, x.tmp, "clone", "-q", x.op, peer)
		git(x.t, x.op, "fetch", "-q", peer, "refs/remotes/origin/main", "feat")
	}, "owner", source.ReasonLocalCreated)
	check("own commit rebased by git pull --rebase", func(x *fx) {
		x.commit(x.op, "mine.txt")
		other := filepath.Join(x.tmp, "other")
		git(x.t, x.tmp, "clone", "-q", x.origin, other)
		git(x.t, other, "config", "user.name", "Other")
		git(x.t, other, "config", "user.email", "other@example.test")
		x.commit(other, "theirs.txt")
		git(x.t, other, "push", "-q", "origin", "main")
		git(x.t, x.op, "pull", "-q", "--rebase", "origin", "main")
	}, "owner", source.ReasonLocalCreated)
	check("own commit merged by git pull", func(x *fx) {
		x.commit(x.op, "mine.txt")
		other := filepath.Join(x.tmp, "other")
		git(x.t, x.tmp, "clone", "-q", x.origin, other)
		git(x.t, other, "config", "user.name", "Other")
		git(x.t, other, "config", "user.email", "other@example.test")
		x.commit(other, "theirs.txt")
		git(x.t, other, "push", "-q", "origin", "main")
		git(x.t, x.op, "pull", "-q", "--no-rebase", "--no-edit", "origin", "main")
	}, "owner", source.ReasonLocalCreated)
	check("a collaborator's commit pulled from origin", func(x *fx) {
		git(x.t, x.stranger, "push", "-q", "origin", "pr")
		git(x.t, x.op, "checkout", "-q", "-b", "pr")
		git(x.t, x.op, "pull", "-q", "origin", "pr")
	}, "collaborator", source.ReasonLocalOnOrigin)
	check("merge --squash of the stranger's branch left uncommitted", func(x *fx) {
		git(x.t, x.op, "fetch", "-q", x.stranger, "pr")
		git(x.t, x.op, "merge", "-q", "--squash", "FETCH_HEAD")
	}, "untrusted", source.ReasonLocalInProgress)
	check("fork PR mapped into refs/remotes/origin/pr/*", func(x *fx) {
		git(x.t, x.stranger, "push", "-q", "origin", "pr:refs/pull/1/head")
		git(x.t, x.op, "config", "--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/pr/*")
		git(x.t, x.op, "fetch", "-q", "origin")
		git(x.t, x.op, "checkout", "-q", "-b", "prx", "origin/pr/1")
	}, "untrusted", source.ReasonLocalForeign)
	check("fork PR mapped into a name no list knows", func(x *fx) {
		git(x.t, x.stranger, "push", "-q", "origin", "pr:refs/pull/1/head")
		git(x.t, x.op, "config", "--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/fork-prs/*")
		git(x.t, x.op, "fetch", "-q", "origin")
		git(x.t, x.op, "checkout", "-q", "-b", "prx", "origin/fork-prs/1")
	}, "untrusted", source.ReasonLocalForeign)
	check("a git replace graft puts the stranger's commit under origin/main", func(x *fx) {
		x.commit(x.op, "n.txt")
		git(x.t, x.op, "push", "-q", "origin", "main")
		tip := git(x.t, x.op, "rev-parse", "origin/main")
		git(x.t, x.op, "checkout", "-q", "main")
		x.checkoutStranger()
		git(x.t, x.op, "replace", "--graft", tip, git(x.t, x.op, "rev-parse", "HEAD"))
	}, "untrusted", source.ReasonLocalForeign)
	check("origin/HEAD pointed at another remote's branch", func(x *fx) {
		x.checkoutStranger()
		git(x.t, x.op, "update-ref", "refs/remotes/attacker/main", "HEAD")
		git(x.t, x.op, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/attacker/main")
	}, "untrusted", source.ReasonLocalForeign)
	check("remote configured but never fetched, own commits", func(x *fx) {
		git(x.t, x.op, "remote", "remove", "origin")
		git(x.t, x.op, "remote", "add", "origin", "https://example.test/x.git")
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
	}, "owner", source.ReasonLocalCreated)
	check("remote configured but never fetched, history from an archive", func(x *fx) {
		git(x.t, x.op, "remote", "remove", "origin")
		git(x.t, x.op, "remote", "add", "origin", "https://example.test/x.git")
		x.commit(x.op, "one.txt")
		if err := os.RemoveAll(filepath.Join(x.op, ".git", "logs")); err != nil {
			x.t.Fatal(err)
		}
	}, "untrusted", source.ReasonLocalForeign)
	check("repository with no remote", func(x *fx) {
		git(x.t, x.op, "remote", "remove", "origin")
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
	}, "owner", source.ReasonLocalNoRemote)
}

// `evidence verify` classifies inside a temporary worktree: its HEAD log has no entry for the commit,
// the branch logs shared with the main checkout do.
func TestClassifyGitLinkedWorktree(t *testing.T) {
	t.Parallel()
	x := newFx(t)
	git(t, x.op, "checkout", "-q", "-b", "feat")
	sha := x.commit(x.op, "one.txt")
	wt := filepath.Join(x.tmp, "verify-wt")
	git(t, x.op, "worktree", "add", "-q", "--detach", wt, sha)
	if res := x.decideIn(wt, "", run.SourceTrust{}); res.Class != "owner" || !contains(res.Reasons, source.ReasonLocalCreated) {
		t.Fatalf("own commit seen from a linked worktree: %+v", res)
	}
}

// A git that cannot run its refspec question must not pass the pull-request namespaces as branches.
func TestClassifyGitConfigErrorClosesCollaborator(t *testing.T) {
	t.Parallel()
	x := newFx(t)
	git(t, x.stranger, "push", "-q", "origin", "pr:refs/pull/1/head")
	git(t, x.op, "config", "--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/fork-prs/*")
	git(t, x.op, "fetch", "-q", "origin")
	git(t, x.op, "checkout", "-q", "-b", "prx", "origin/fork-prs/1")
	flaky := run.SourceTrust{Capture: func(ctx context.Context, argv ...string) (string, int, error) {
		for _, a := range argv {
			if a == "config" {
				return "", 0, errors.New("runner failure")
			}
		}
		return report.Capture(ctx, argv...)
	}}
	if res := x.decide("", flaky); res.Class != "untrusted" {
		t.Fatalf("a failed refspec question left the pull-request namespace as a branch: %+v", res)
	}
}

func TestClassifyGitNoRepoNoRevision(t *testing.T) {
	h := run.SourceTrust{}
	dir := t.TempDir()
	res, err := h.Decide(context.Background(), run.TrustRequest{Root: dir, Revision: strings.Repeat("a", 40)})
	if err != nil || res.Class != "owner" || !contains(res.Reasons, source.ReasonLocalNoGit) {
		t.Fatalf("not a repository: %+v %v", res, err)
	}
	res, err = h.Decide(context.Background(), run.TrustRequest{Root: filepath.Join(dir, "gone"), Revision: strings.Repeat("a", 40)})
	if err != nil || res.Class != "untrusted" {
		t.Fatalf("a root git cannot enter says nothing about the history, so it is not the operator's: %+v %v", res, err)
	}
	res, err = h.Decide(context.Background(), run.TrustRequest{Root: dir})
	if err != nil || res.Class != "owner" || !contains(res.Reasons, source.ReasonLocalNoGit) {
		t.Fatalf("no revision, no repository: %+v %v", res, err)
	}
	git(t, dir, "init", "-q")
	res, err = h.Decide(context.Background(), run.TrustRequest{Root: dir})
	if err != nil || res.Class != "owner" || !contains(res.Reasons, source.ReasonLocalNoRevision) {
		t.Fatalf("no revision, a repository without a commit: %+v %v", res, err)
	}
	// git that cannot run in the directory (another user's repository: "dubious ownership") says nothing.
	broken := run.SourceTrust{Capture: func(context.Context, ...string) (string, int, error) {
		return "fatal: detected dubious ownership in repository", 128, nil
	}}
	res, err = broken.Decide(context.Background(), run.TrustRequest{Root: dir})
	if err != nil || res.Class != "untrusted" || !contains(res.Reasons, source.ReasonLocalGitBroken) {
		t.Fatalf("git unavailable: %+v %v", res, err)
	}
	res, err = broken.Decide(context.Background(), run.TrustRequest{Root: dir, Revision: strings.Repeat("a", 40)})
	if err != nil || res.Class != "untrusted" {
		t.Fatalf("every git question failed: %+v %v", res, err)
	}
	for _, rev := range []string{"--all", "HEAD", "abc", strings.Repeat("g", 40)} {
		res, err = h.Decide(context.Background(), run.TrustRequest{Root: dir, Revision: rev})
		if err != nil || res.Class != "untrusted" || !contains(res.Reasons, source.ReasonLocalBadRevision) {
			t.Fatalf("revision %q: %+v %v", rev, res, err)
		}
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ---- --trust owner, the machine cap, the forge re-fetch ----

type db struct {
	t    *testing.T
	path string
}

func newDB(t *testing.T) *db              { return &db{t: t, path: filepath.Join(t.TempDir(), "state.db")} }
func (d *db) open() (*store.Store, error) { return store.Open(d.path, store.Options{}) }
func (d *db) rows() []store.AuditEntry {
	st, err := d.open()
	if err != nil {
		d.t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	es, err := st.AuditEntries(context.Background(), 0, 100)
	if err != nil {
		d.t.Fatal(err)
	}
	return es
}
func (d *db) setCap(v string) {
	st, err := d.open()
	if err != nil {
		d.t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	w := store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: "trust.machine_invoked_cap", Value: []byte(`"` + v + `"`), Actor: "test"}
	if err := st.ApplyWrite(context.Background(), w, nil); err != nil {
		d.t.Fatal(err)
	}
}

func TestClassifyTrustOwnerOverrideIsAudited(t *testing.T) {
	t.Parallel()
	x, d := newFx(t), newDB(t)
	x.checkoutStranger()
	h := run.SourceTrust{OpenStore: d.open}
	res := x.decide("owner", h)
	if res.Class != "owner" || !contains(res.Reasons, source.ReasonLocalOverride) {
		t.Fatalf("--trust owner on the stranger fixture: %+v", res)
	}
	rows := d.rows()
	if len(rows) != 1 || rows[0].Action != "trust.override" || !strings.Contains(rows[0].Detail, "classified=untrusted") || !strings.Contains(rows[0].Detail, source.ReasonLocalForeign) {
		t.Fatalf("want exactly one override row naming the verdict, got %+v", rows)
	}
	// Lowering needs no row, and neither does --trust owner on a source that is already owner.
	git(t, x.op, "checkout", "-q", "main")
	if res = x.decide("owner", h); res.Class != "owner" || len(d.rows()) != 1 {
		t.Fatalf("an owner source written to the audit log: %+v rows=%d", res, len(d.rows()))
	}
	if res = x.decide("collaborator", h); res.Class != "collaborator" || len(d.rows()) != 1 {
		t.Fatalf("--trust collaborator: %+v", res)
	}
}

func TestClassifyTrustOwnerNeedsTheAuditRow(t *testing.T) {
	t.Parallel()
	x := newFx(t)
	x.checkoutStranger()
	h := run.SourceTrust{OpenStore: func() (*store.Store, error) { return nil, errors.New("no store") }}
	_, err := h.Decide(context.Background(), run.TrustRequest{Root: x.op, Revision: git(t, x.op, "rev-parse", "HEAD"), Requested: "owner"})
	if run.CodeOf(err) != run.CodeTrust {
		t.Fatalf("an override that cannot be audited must be refused with E625: %v", err)
	}
}

func TestClassifyRequestAboveTheVerdictIsNotGranted(t *testing.T) {
	t.Parallel()
	x := newFx(t)
	x.checkoutStranger()
	if res := x.decide("collaborator", run.SourceTrust{}); res.Class != "untrusted" {
		t.Fatalf("--trust collaborator raised an untrusted source: %s", res.Class)
	}
}

func TestClassifyMachineInvokedCap(t *testing.T) {
	t.Parallel()
	x, d := newFx(t), newDB(t)
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "NSELF_INVOKED_BY" {
				return v
			}
			return ""
		}
	}
	d.setCap("collaborator")
	capped := run.SourceTrust{OpenStore: d.open, Getenv: env("mcp")}
	res := x.decide("", capped)
	if res.Class != "collaborator" || !contains(res.Reasons, source.ReasonMachineCap) {
		t.Fatalf("owner fixture under cap collaborator: %+v", res)
	}
	_, err := capped.Decide(context.Background(), run.TrustRequest{Root: x.op, Revision: git(t, x.op, "rev-parse", "HEAD"), Requested: "owner"})
	var re *roles.Error
	if !errors.As(err, &re) || re.Reason != "machine_cap" || len(d.rows()) != 0 {
		t.Fatalf("--trust owner under the cap: %v (audit rows %d)", err, len(d.rows()))
	}
	if res = x.decide("collaborator", capped); res.Class != "collaborator" {
		t.Fatalf("--trust collaborator under the cap: %+v", res)
	}
	if res = x.decide("", run.SourceTrust{OpenStore: d.open, Getenv: env("")}); res.Class != "owner" || contains(res.Reasons, source.ReasonMachineCap) {
		t.Fatalf("NSELF_INVOKED_BY unset: the class must be unchanged: %+v", res)
	}
	d2 := newDB(t) // cap at its default
	if res = x.decide("", run.SourceTrust{OpenStore: d2.open, Getenv: env("mcp")}); res.Class != "owner" || contains(res.Reasons, source.ReasonMachineCap) {
		t.Fatalf("default cap: the class must be unchanged: %+v", res)
	}
	if res = x.decide("owner", run.SourceTrust{OpenStore: d2.open, Getenv: env("mcp")}); res.Class != "owner" {
		t.Fatalf("--trust owner under the default cap: %+v", res)
	}
}

func TestClassifyForgeRefetchesBeforeClassifying(t *testing.T) {
	calls := 0
	forged := facts("push", func(f *model.TriggerFacts) {
		f.ActorPermission, f.ProtectedRef = fact("admin", "payload"), fact(true, "payload")
	})
	h := run.SourceTrust{Refetch: func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
		calls++
		f.ActorPermission, f.ProtectedRef = fact("write", "api"), fact(false, "api")
		return f, nil
	}}
	res, err := h.ClassifyForge(context.Background(), t.TempDir(), forged)
	if err != nil || calls != 1 || res.Class != "collaborator" {
		t.Fatalf("calls=%d res=%+v err=%v: the API facts must decide, after exactly one re-fetch", calls, res, err)
	}
	h.Refetch = func(context.Context, model.TriggerFacts) (model.TriggerFacts, error) {
		return model.TriggerFacts{}, errors.New("token scope")
	}
	res, err = h.ClassifyForge(context.Background(), t.TempDir(), forged)
	if err != nil || res.Class != "untrusted" || !contains(res.Reasons, source.ReasonRefetchFailed) {
		t.Fatalf("a failed re-fetch leaves payload facts that cannot raise: %+v %v", res, err)
	}
	// No registered provider can re-fetch, so API-labelled facts that arrived with the delivery cannot be checked.
	res, err = (run.SourceTrust{}).ClassifyForge(context.Background(), t.TempDir(), facts("push", admin, protected))
	if err != nil || res.Class != "untrusted" || !contains(res.Reasons, source.ReasonRefetchFailed) || contains(res.Reasons, source.ReasonVerified) {
		t.Fatalf("an unknown provider must leave the run untrusted: %+v %v", res, err)
	}
	h.Refetch = func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) { return f, nil }
	if res, err = h.ClassifyForge(context.Background(), t.TempDir(), facts("push", admin, protected)); err != nil || res.Class != "owner" {
		t.Fatalf("verified API facts: %+v %v", res, err)
	}
	d := newDB(t)
	d.setCap("untrusted")
	m := run.SourceTrust{OpenStore: d.open, Getenv: func(string) string { return "mcp" }, Refetch: h.Refetch}
	if res, err = m.ClassifyForge(context.Background(), t.TempDir(), facts("push", admin, protected)); err != nil || res.Class != "untrusted" || !contains(res.Reasons, source.ReasonMachineCap) {
		t.Fatalf("the cap applies to forge triggers too: %+v %v", res, err)
	}
}

func TestClassifyHookIsRegistered(t *testing.T) {
	if _, ok := run.Registered.Trust.(run.SourceTrust); !ok {
		t.Fatalf("the registered TrustClassifier is %T", run.Registered.Trust)
	}
}

func FuzzClassify(f *testing.F) {
	seeds, _ := filepath.Glob(filepath.Join("testdata", "facts", "*.json"))
	for _, s := range seeds {
		if b, err := os.ReadFile(s); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var tf model.TriggerFacts
		if json.Unmarshal(data, &tf) != nil {
			return
		}
		r := source.Classify(tf)
		if !r.Class.Valid() || len(r.Reasons) == 0 || !r.VisibilityZone.Valid() {
			t.Fatalf("invalid result %+v for %s", r, data)
		}
		if tf.Auth != "per-repo" && r.Class != "untrusted" {
			t.Fatalf("auth %q classed %s", tf.Auth, r.Class)
		}
		if r.Class == "owner" && !(tf.Kind == "schedule" || trustedFact(tf.ActorPermission.Source)) {
			t.Fatalf("owner without an authoritative actor fact: %s", data)
		}
	})
}

func trustedFact(src string) bool { return src == "api" || src == "local" }
