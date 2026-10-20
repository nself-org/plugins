package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func TestForeignRewriteTrust(t *testing.T) {
	for _, kind := range []string{"amend", "cherry-pick", "rebase"} {
		for _, ref := range []string{"refs/remotes/origin/feature", "refs/pull/7/head", "FETCH_HEAD"} {
			t.Run(kind+"/"+ref, func(t *testing.T) {
				root := t.TempDir()
				git := func(args ...string) string {
					t.Helper()
					cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
					cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("git %v: %s %v", args, out, err)
					}
					return strings.TrimSpace(string(out))
				}
				write := func(name string) {
					t.Helper()
					if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
						t.Fatal(err)
					}
					git("add", name)
					git("commit", "-qm", name)
				}
				git("init", "-q", "-b", "main")
				write("base")
				base := git("rev-parse", "HEAD")
				git("remote", "add", "origin", "https://example.invalid/o/r")
				git("update-ref", "refs/remotes/origin/main", base)
				git("checkout", "-qb", "foreign")
				write("foreign")
				foreign := git("rev-parse", "HEAD")
				if ref == "FETCH_HEAD" {
					if err := os.WriteFile(filepath.Join(root, ".git", "FETCH_HEAD"), []byte(base+"\t\tbase\n"+foreign+"\tnot-for-merge\tforeign\n"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					git("update-ref", ref, foreign)
				}
				switch kind {
				case "amend":
					git("commit", "--amend", "-qm", "rewritten")
				case "cherry-pick":
					git("checkout", "-q", "main")
					write("local")
					git("cherry-pick", foreign)
				case "rebase":
					git("checkout", "-q", "main")
					write("local")
					git("rebase", "main", "foreign")
				}
				head := git("rev-parse", "HEAD")
				if head == foreign {
					t.Fatal("fixture did not rewrite")
				}
				g := (SourceTrust{}).gather(context.Background(), root, head)
				if got := source.ClassifyLocal(g); got.Class == "owner" {
					t.Fatalf("foreign %s became owner: %+v", kind, g)
				}
			})
		}
	}
}

func TestFetchHeadsEveryLine(t *testing.T) {
	for _, n := range []int{2, 65, 100} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			root := t.TempDir()
			var body strings.Builder
			for i := 0; i < n; i++ {
				fmt.Fprintf(&body, "%040x\tnot-for-merge\tfixture\n", i+1)
			}
			if err := os.WriteFile(filepath.Join(root, "FETCH_HEAD"), []byte(body.String()), 0600); err != nil {
				t.Fatal(err)
			}
			got, ok := fetchHeads(root, "FETCH_HEAD")
			if !ok || len(got) != n {
				t.Fatalf("FETCH_HEAD lines = %d, ok = %v; want %d", len(got), ok, n)
			}
		})
	}
}
