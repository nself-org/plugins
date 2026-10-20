//go:build security

package redteam

// Purpose: red team group C, planted PR policy in a real git fixture (TM-31), second half of the policy group.
// Inputs:  see the sibling file of the same group.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: local fixtures only; see c_evidence_test.go header.

import (
	"context"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func cGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.invalid", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.invalid", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestC31PlantedPRPolicy (TM-31): a PR checkout whose .nself/ci.yaml widens the policy never changes
// the effective policy, whether the PR branched from an older default-branch policy or from the tip.
func TestC31PlantedPRPolicy(t *testing.T) {
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	cGit(t, root, "init", "-q", "--bare", "-b", "main", origin)
	cGit(t, root, "clone", "-q", origin, work)
	write := func(body string) {
		if err := os.MkdirAll(filepath.Join(work, ".nself"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, ".nself", "ci.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("version: 1\npolicy:\n  trust:\n    isolation_min: process\n    network: internet\n")
	cGit(t, work, "checkout", "-q", "-b", "main")
	cGit(t, work, "add", "-A")
	cGit(t, work, "commit", "-q", "-m", "weak policy")
	cGit(t, work, "push", "-q", "origin", "main")
	cGit(t, work, "checkout", "-q", "-b", "pr")
	write("version: 1\npolicy:\n  trust:\n    isolation_min: process\n    network: internet\n") // the PR keeps the old, weaker policy
	cGit(t, work, "checkout", "-q", "main")
	write("version: 1\npolicy:\n  trust:\n    isolation_min: vm\n    network: restricted\n    secrets: []\n    approval:\n      first_time: true\n") // the tip tightens after the branch point
	cGit(t, work, "commit", "-q", "-a", "-m", "tighten")
	cGit(t, work, "push", "-q", "origin", "main")
	cGit(t, work, "checkout", "-q", "pr")
	run := func(ctx context.Context, argv ...string) (string, int, error) {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), ee.ExitCode(), nil
		}
		return string(out), 0, err
	}
	res := &policy.Resolver{Run: run}
	repo := policy.Repo{Root: work, Project: "p", Visibility: "private", Class: "untrusted"}
	tip, err := res.Resolve(context.Background(), repo, policy.Revision{})
	if err != nil {
		t.Fatalf("control: %v", err)
	}
	if got := tip.Values["trust.isolation_min"]; got != "vm" {
		t.Fatalf("control: the protected branch policy was not read (isolation_min %v)", got)
	}
	for name, body := range map[string]string{
		"older weaker policy kept (merge-base)": "version: 1\npolicy:\n  trust:\n    isolation_min: process\n    network: internet\n",
		"explicit widening":                     "version: 1\npolicy:\n  trust:\n    isolation_min: process\n    network: privileged\n    secrets: [release, deploy]\n    accepts: [untrusted, collaborator, internal, owner]\n    machine_invoked_cap: owner\n    privacy_zone: hosted-allowed\n    approval:\n      first_time: false\n",
		"spend keys":                            "version: 1\npolicy:\n  economics:\n    paid_enabled: true\n    budgets:\n      day: 999999999\n",
		"dotted duplicate of a strict key":      "version: 1\npolicy:\n  trust:\n    isolation_min: process\n  trust.isolation_min: process\n",
	} {
		write(body)
		got, err := res.Resolve(context.Background(), repo, policy.Revision{Dir: work})
		if err != nil {
			continue // refusing the PR policy is a safe outcome
		}
		for _, key := range []string{"trust.isolation_min", "trust.network", "trust.secrets", "trust.accepts", "trust.machine_invoked_cap", "trust.privacy_zone", "trust.approval.first_time", "economics.paid_enabled", "economics.budgets.day"} {
			if !jsonEqual(got.Values[key], tip.Values[key]) {
				t.Errorf("%s: %s is %v, the protected tip says %v", name, key, got.Values[key], tip.Values[key])
			}
		}
		if got.Digest == "" || got.PinnedCommit != tip.PinnedCommit {
			t.Errorf("%s: digest %q pinned %q (tip pinned %q)", name, got.Digest, got.PinnedCommit, tip.PinnedCommit)
		}
	}
}
