package reproduce

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The default probe must run in the repository root, as the gate runner does,
// so tools that resolve their version by cwd answer the same question.
func TestDefaultToolProbeRunsInRepoRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script probe")
	}
	root := t.TempDir()
	script := "#!/bin/sh\necho \"v-from-$(cat .tool-versions)\"\n"
	if err := os.WriteFile(filepath.Join(root, "tool"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tool-versions"), []byte("root"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := Source{Doc: modelEvidenceWithTool("./tool", "v-from-root")}
	if got := toolDrift(context.Background(), Env{Root: root}, src); len(got) != 0 {
		t.Fatalf("drift = %+v; the probe did not run in the repository root", got)
	}
}
