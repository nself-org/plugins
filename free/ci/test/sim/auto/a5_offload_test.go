package auto

// Purpose: A5 dirty-tree offload: a dirty tree becomes a snapshot commit under a private ref, a
// secret-shaped untracked file is named and left out, and the ref is released.
// Inputs: offload.Take over a scripted git (no process runs). Outputs: pass or fail.
// Constraints: the scripted capture answers by git subcommand; nothing touches a real repository.

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/offload"
)

const base = "1111111111111111111111111111111111111111"
const snap = "2222222222222222222222222222222222222222"

type gitLog struct{ calls []string }

func (g *gitLog) capture(_ context.Context, argv ...string) (string, int, error) {
	i := slices.Index(argv, "git")
	sub := argv[i:]
	for _, a := range sub {
		switch a {
		case "rev-parse":
			g.calls = append(g.calls, a)
			return base + "\n", 0, nil
		case "status":
			g.calls = append(g.calls, a)
			return " M a.txt\n?? notes.txt\n?? .env\n", 0, nil
		case "ls-files":
			g.calls = append(g.calls, a)
			if slices.Contains(sub, "--others") {
				return "notes.txt\x00.env\x00", 0, nil
			}
			return "a.txt\x00", 0, nil
		case "write-tree":
			g.calls = append(g.calls, a)
			return "tree\n", 0, nil
		case "commit-tree":
			g.calls = append(g.calls, a)
			return snap + "\n", 0, nil
		case "update-ref", "read-tree", "add":
			g.calls = append(g.calls, a)
			return "", 0, nil
		}
	}
	return "", 1, nil
}

func TestConformanceA5Offload(t *testing.T) {
	g := &gitLog{}
	s, err := offload.Take(context.Background(), g.capture, t.TempDir(), t.TempDir(), "run1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Clean || s.SHA != snap || s.Ref != offload.RefPrefix+"run1" {
		t.Fatalf("snapshot = %+v", s)
	}
	if !slices.Equal(s.Excluded, []string{".env"}) {
		t.Fatalf("excluded = %v, want the env file by name", s.Excluded)
	}
	if !slices.Contains(g.calls, "update-ref") || !slices.Contains(g.calls, "commit-tree") {
		t.Fatalf("calls = %v", g.calls)
	}
	n := len(g.calls)
	if err = offload.Release(context.Background(), g.capture, t.TempDir(), s); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) <= n || !strings.Contains(strings.Join(g.calls[n:], " "), "update-ref") {
		t.Fatalf("release did not delete the ref: %v", g.calls[n:])
	}
	if _, err = offload.Take(context.Background(), g.capture, t.TempDir(), t.TempDir(), "bad id"); err == nil {
		t.Fatal("a bad snapshot id was accepted")
	}
}
