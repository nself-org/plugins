//go:build !windows

package exec

// Purpose: D-1062 regression. Setsid keeps a detached run out of the caller's
// process group, but a cgroup or pidfd based killer (systemd KillMode=control-group,
// `docker stop`) still stops it. The operator guide must say so, with the way out.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetachGuideNamesCgroupLimit(t *testing.T) {
	guide := filepath.Join("..", "..", "..", "..", ".github", "wiki", "plugins", "CI.md")
	b, err := os.ReadFile(guide)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, want := range []string{
		"## Detached runs",
		"--detach",
		"KillMode=control-group",
		"docker stop",
		"systemd-run",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("CI.md Detached runs section is missing %q", want)
		}
	}
}
