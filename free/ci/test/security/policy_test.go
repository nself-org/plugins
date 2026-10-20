//go:build security

package security

// Purpose: a pull request cannot widen the policy it is judged by (I16, TM-31). A real git
// repository with an origin carries the protected branch's policy; the pull request's
// worktree `.nself/ci.yaml` plants widening values for every guarded key. The resolver
// ignores each with E671, the digest and every effective value stay as they were, and the
// dispatcher fed the planted resolution still keeps private work off the hosted provider
// and secrets away from fork code. A tightening revision is honoured (control).
// Inputs:  temporary git repositories; the production policy keys.
// Outputs: pass or fail.
// Constraints: git runs through a test Runner with a private environment; a missing git is a
//          t.Fatal, never a skip.

import (
	"bytes"
	"context"
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
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func gitRunner(ctx context.Context, argv ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.ExitCode(), nil
	}
	return out.String(), 0, err
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, exit, err := gitRunner(context.Background(), append([]string{"git", "-C", dir}, args...)...)
	if err != nil || exit != 0 {
		t.Fatalf("git %v: exit %d err %v\n%s", args, exit, err, out)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// policyRepo returns a clone whose origin main carries the protected policy.
func policyRepo(t *testing.T, protected string) string {
	t.Helper()
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, root, "init", "-q", "-b", "main", work)
	git(t, work, "remote", "add", "origin", origin)
	writeFile(t, filepath.Join(work, ".nself/ci.yaml"), "version: 1\n"+protected)
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "base")
	git(t, work, "push", "-q", "origin", "main")
	return work
}

func resolveAt(t *testing.T, work, visibility string, class model.TrustClass) policy.Resolved {
	t.Helper()
	r := policy.Resolver{Run: gitRunner}
	got, err := r.Resolve(context.Background(), policy.Repo{Root: work, Project: "project", Visibility: visibility, Class: class}, policy.Revision{Dir: work})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return got
}

// forkWidening widens every key a fork PR's defaults fix: network, secrets, isolation floor and the
// first-time-contributor hold. privateWidening widens what a private repository's defaults fix: the
// privacy zone and the provider list.
const (
	forkWidening    = "policy:\n  trust:\n    network: lan\n    secrets: [project, deploy]\n    isolation_min: process\n    approval:\n      first_time: false\n"
	privateWidening = "policy:\n  trust:\n    privacy_zone: hosted-allowed\n    providers: [evil]\n"
)

// assertIgnoredNotApplied is the I16 check: the planted resolution equals the clean one, every key of
// want is ignored with E671 from the revision scope, and no effective value comes from the revision.
func assertIgnoredNotApplied(t *testing.T, base, got policy.Resolved, want ...string) {
	t.Helper()
	if got.Digest != base.Digest || !reflect.DeepEqual(got.Values, base.Values) || !reflect.DeepEqual(got.Sources, base.Sources) {
		t.Fatalf("the planted PR changed the policy:\nbase %v\n got %v", base.Values, got.Values)
	}
	ignored := map[string]string{}
	for _, ig := range got.Ignored {
		if ig.Scope != policy.SourceRevision {
			t.Errorf("ignored entry %s from scope %s, want revision", ig.Key, ig.Scope)
		}
		ignored[ig.Key] = ig.Reason
	}
	for _, key := range want {
		if !strings.HasPrefix(ignored[key], "E671") {
			t.Errorf("%s was not ignored with E671: %q", key, ignored[key])
		}
	}
	for key, src := range got.Sources {
		if src == policy.SourceRevision {
			t.Errorf("effective key %s comes from the revision", key)
		}
	}
}

// TestPlantedPRCannotWidenPolicy is the I16 case for a fork PR: the planted worktree policy is
// ignored key by key, and the policy that reaches the scheduler is the one without the PR.
func TestPlantedPRCannotWidenPolicy(t *testing.T) {
	work := policyRepo(t, "")
	base := resolveAt(t, work, "public", "untrusted")
	writeFile(t, filepath.Join(work, ".nself/ci.yaml"), "version: 1\n"+forkWidening)
	got := resolveAt(t, work, "public", "untrusted")
	assertIgnoredNotApplied(t, base, got, "trust.network", "trust.secrets", "trust.isolation_min", "trust.approval.first_time")
	if got.Values["trust.approval.first_time"] != true {
		t.Fatalf("the first-time hold was switched off by the PR: %v", got.Values["trust.approval.first_time"])
	}
	// The planted resolution reaches the dispatcher: a fork job asking for secrets or LAN is placed nowhere.
	r := newRig(t, got.Effective, gvisorRunner("gv1", 2), lanRunner("lan1", 2))
	id, a, err := r.submit(newJob("forksec", "untrusted", "restricted", "project"), "untrusted", settle)
	waitsBecause(t, r, id, a, err, "secrets.untrusted")
	id, a, err = r.submit(newJob("forknet", "untrusted", "lan"), "untrusted", settle)
	waitsBecause(t, r, id, a, err, "network.lan_for_untrusted")
}

// TestPlantedPRCannotAuthorizeHosted: a PR on a private repository that plants a hosted-allowed
// privacy zone and a provider list gets neither; private work stays off the hosted runner even with
// the LAN node taken.
func TestPlantedPRCannotAuthorizeHosted(t *testing.T) {
	work := policyRepo(t, "")
	base := resolveAt(t, work, "private", "owner")
	writeFile(t, filepath.Join(work, ".nself/ci.yaml"), "version: 1\n"+privateWidening)
	got := resolveAt(t, work, "private", "owner")
	assertIgnoredNotApplied(t, base, got, "trust.privacy_zone", "trust.providers")
	r := newRig(t, got.Effective, lanRunner("lan1", 1), hostedRunner("hosted1"))
	if _, a, err := r.submit(newJob("held", "owner", "lan"), "owner", 2*time.Second); runnerOf(t, a, err) != "lan1" {
		t.Fatal("setup: the first private job must take the LAN slot")
	}
	id, a, err := r.submit(newJob("private", "owner", "internet"), "owner", settle)
	waitsBecause(t, r, id, a, err, "privacy.hosted_not_authorized")
	if hostedSawRequest(r.hosted) {
		t.Fatal("the planted policy sent private work to the hosted provider")
	}
}

// TestRevisionMayTighten (control): a revision that only tightens is honoured, so the E671 above
// is the widening rule at work and not a resolver that ignores the worktree altogether.
func TestRevisionMayTighten(t *testing.T) {
	work := policyRepo(t, "")
	base := resolveAt(t, work, "private", "owner")
	writeFile(t, filepath.Join(work, ".nself/ci.yaml"), "version: 1\npolicy:\n  trust:\n    network: restricted\n")
	got := resolveAt(t, work, "private", "owner")
	if got.Values["trust.network"] != "restricted" || got.Sources["trust.network"] != policy.SourceRevision {
		t.Fatalf("a tightening revision was not applied: %v %v", got.Values["trust.network"], got.Sources["trust.network"])
	}
	if got.Digest == base.Digest {
		t.Fatal("the digest did not change with an applied tightening")
	}
	r := newRig(t, got.Effective, lanRunner("lan1", 2))
	id, a, err := r.submit(newJob("tight", "owner", "lan"), "owner", settle)
	waitsBecause(t, r, id, a, err, "network.scope_not_offered")
}

// put writes one operator policy row (an operator-scope key of the store).
func put(t *testing.T, st *store.Store, scope, subject, key, value string) {
	t.Helper()
	err := st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: scope, Subject: subject, Key: key, Value: []byte(value), Actor: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
