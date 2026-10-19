package job_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/job"
)

// fakeGit answers by the git subcommand.
type fakeGit struct {
	head, status, files string
	headExit            int
	statusErr, filesErr error
	calls               []string
}

func (g *fakeGit) capture(_ context.Context, argv ...string) (string, int, error) {
	sub := argv[5] // git -c core.fsmonitor=false -C <root> <sub> ...
	g.calls = append(g.calls, sub)
	switch sub {
	case "rev-parse":
		return g.head + "\n", g.headExit, nil
	case "status":
		return g.status, 0, g.statusErr
	case "ls-files":
		return g.files, 0, g.filesErr
	}
	return "", 1, errors.New("unexpected " + sub)
}

func TestGitWorktree(t *testing.T) {
	sha := strings.Repeat("a", 40)
	cases := map[string]struct {
		g     fakeGit
		wSHA  string
		dirty bool
	}{
		"clean":            {fakeGit{head: sha, files: "H a.go\nH b.go\n"}, sha, false},
		"empty index":      {fakeGit{head: sha}, sha, false},
		"status edit":      {fakeGit{head: sha, status: " M a.go", files: "H a.go"}, sha, true},
		"untracked":        {fakeGit{head: sha, status: "?? new.go"}, sha, true},
		"assume-unchanged": {fakeGit{head: sha, files: "H a.go\nh b.go\n"}, sha, true},
		"skip-worktree":    {fakeGit{head: sha, files: "S a.go\n"}, sha, true},
		"status failed":    {fakeGit{head: sha, statusErr: errors.New("boom")}, sha, true},
		"ls-files failed":  {fakeGit{head: sha, filesErr: errors.New("boom")}, sha, true},
		"no head":          {fakeGit{head: "", headExit: 128}, "", true},
		"short head":       {fakeGit{head: "abc123"}, "", true},
		"sha-256 head":     {fakeGit{head: strings.Repeat("b", 64)}, strings.Repeat("b", 64), false},
	}
	for name, c := range cases {
		g := c.g
		gotSHA, dirty := job.GitWorktree(g.capture)(context.Background(), "/repo")
		if gotSHA != c.wSHA || dirty != c.dirty {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", name, gotSHA, dirty, c.wSHA, c.dirty)
		}
	}
	g := fakeGit{head: "", headExit: 128}
	job.GitWorktree(g.capture)(context.Background(), "/repo")
	if len(g.calls) != 1 {
		t.Fatalf("an unreadable HEAD must stop the probe: %v", g.calls)
	}
}
