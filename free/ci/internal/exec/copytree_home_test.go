package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A state home that lives inside the source repository (NSELF_CI_HOME=<repo>/.h)
// must not be copied into the job workspace: the workspace is itself under
// <home>/work, so copying it recursed until "file name too long". Regression
// test for D-1043.
func TestRunSkipsStateHomeInsideSource(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(src, ".h")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "state.db"), []byte("db"), 0600); err != nil {
		t.Fatal(err)
	}
	r := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "home-in-src", CoordinatorID: "coord", Home: home, SourceDir: src,
		Command: []string{"sh", "-c", "test -f keep.txt && test ! -e .h"}})
	if r.Err != nil || r.ExitCode != 0 {
		t.Fatalf("run with home inside source: %+v", r)
	}
}
