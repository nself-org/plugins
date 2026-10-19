package run

// Purpose: the policy hook reports what the resolver ignored and how fresh the
// protected-branch tip was, so a run can tell the operator (D-1125).

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestPolicyHookReportsIgnoredAndFetch: an origin that cannot be reached leaves no
// protected-branch scope; the result carries that warning and the fetch state.
func TestPolicyHookReportsIgnoredAndFetch(t *testing.T) {
	ctx := context.Background()
	root, _, db := runScopeRepo(t)
	if out, err := exec.Command("git", "-C", root, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git")).CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %s %v", out, err)
	}
	hook := PolicyHook{OpenStore: func() (*store.Store, error) { return store.Open(db, store.Options{}) }, Capture: report.Capture}
	got, err := hook.Resolve(ctx, PolicyRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got.Fetch != "none" {
		t.Errorf("fetch state = %q, want none", got.Fetch)
	}
	if len(got.Ignored) == 0 || !strings.Contains(strings.Join(got.Ignored, "\n"), "protected_branch") {
		t.Errorf("the unreachable origin went unreported: %v", got.Ignored)
	}
	notes := policyNotes(got)
	if len(notes) == 0 || !strings.Contains(strings.Join(notes, "\n"), "protected_branch") {
		t.Errorf("the run notes do not carry the policy warning: %v", notes)
	}
}

// TestPolicyNotes: a stale tip is named; fresh, none and unset states add nothing.
func TestPolicyNotes(t *testing.T) {
	if n := policyNotes(PolicyResult{Fetch: "stale"}); len(n) != 1 || !strings.Contains(n[0], "stale") {
		t.Errorf("stale tip notes = %v", n)
	}
	for _, f := range []string{"", "fresh", "none"} {
		if n := policyNotes(PolicyResult{Fetch: f}); len(n) != 0 {
			t.Errorf("fetch %q gave notes %v", f, n)
		}
	}
}
