package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitInit(t *testing.T, gitignore string) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if gitignore != "" {
		if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(gitignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestIgnoredWarning runs the real `git check-ignore` through the exec funnel
// in fixture repositories, plus the runner contract with fakes.
func TestIgnoredWarning(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, gitignore string
		warn            bool
	}{
		{"nself dir ignored", ".nself/\n", true},
		{"nself glob ignored", ".nself/*\n", true},
		{"two-line fix", ".nself/*\n!.nself/ci.yaml\n", false},
		{"no rule", "node_modules/\n", false},
		{"no gitignore", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := gitInit(t, c.gitignore)
			w, err := CheckIgnored(ctx, root, ExecRunner())
			if err != nil {
				t.Fatal(err)
			}
			if (w != nil) != c.warn {
				t.Fatalf("warning = %v, want %v", w, c.warn)
			}
			if w != nil && (w.Code != CodeIgnored || !strings.Contains(w.Message, ".nself/*\n  !.nself/ci.yaml")) {
				t.Errorf("warning lacks the two-line fix: %+v", w)
			}
			if _, err := os.Stat(filepath.Join(root, ".gitignore")); c.gitignore == "" && err == nil {
				t.Error("the check created a .gitignore")
			}
		})
	}
	t.Run("not a repository is no warning", func(t *testing.T) {
		if w, err := CheckIgnored(ctx, t.TempDir(), ExecRunner()); err != nil || w != nil {
			t.Fatalf("got %v %v", w, err)
		}
	})
	t.Run("runner contract", func(t *testing.T) {
		var got []string
		w, err := CheckIgnored(ctx, "/repo", func(_ context.Context, argv []string) (int, error) { got = argv; return 0, nil })
		if err != nil || w == nil {
			t.Fatalf("exit 0 must warn: %v %v", w, err)
		}
		if strings.Join(got, " ") != "git -c core.fsmonitor=false -C /repo check-ignore -q -- .nself/ci.yaml" {
			t.Errorf("argv: %v", got)
		}
		if _, err := CheckIgnored(ctx, "/repo", func(context.Context, []string) (int, error) { return -1, errors.New("no git") }); err == nil {
			t.Error("a runner error must surface")
		}
		if w, err := CheckIgnored(ctx, "/repo", nil); w != nil || err != nil {
			t.Error("a nil runner must be a no-op")
		}
	})
}
