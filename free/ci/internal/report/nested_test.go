package report

// Purpose: a status can carry a nested GitLab project path, and a description loses every line break (D-1345).
// Constraints: pure, no store; the GitHub adapter must still refuse a nested owner.

import (
	"context"
	"strings"
	"testing"
)

func TestStatusValidateAcceptsANestedGroup(t *testing.T) {
	deep := strings.TrimSuffix(strings.Repeat("g/", 19), "/") // 19 group segments + the repo = 20
	for _, owner := range []string{"group/sub", "a/b/c", deep} {
		if err := (Status{Owner: owner, Repo: "project", Revision: sha, State: Success}).Validate(); err != nil {
			t.Errorf("owner %q: %v", owner, err)
		}
	}
}

func TestRepostKeepsANestedOwner(t *testing.T) {
	k := KeyFor("group/sub", "project", sha)
	owner, repo, ok := k.ownerRepo()
	if !ok || owner != "group/sub" || repo != "project" {
		t.Fatalf("ownerRepo = %q %q %v", owner, repo, ok)
	}
}

func TestGitHubRefusesANestedOwner(t *testing.T) {
	ran := false
	gh := GH{Capture: func(context.Context, ...string) (string, int, error) { ran = true; return "", 0, nil }}
	err := gh.Post(context.Background(), Status{Owner: "group/sub", Repo: "project", Revision: sha, State: Success})
	if err == nil || ran {
		t.Fatalf("err=%v ran=%v: gh must not be asked for a nested path", err, ran)
	}
}

func TestTruncateRemovesEveryLineBreak(t *testing.T) {
	for in, want := range map[string]string{"a\nb": "a b", "a\rb": "a b", "a\r\nb": "a b", "a\n\rb": "a  b"} {
		if got := Truncate(in); got != want {
			t.Errorf("Truncate(%q) = %q, want %q", in, got, want)
		}
	}
}
